package trainingpeaks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ericzapater/familiarassistant/internal/domain"
)

// Service implementa domain.TrainingPeaksService executant el subprocés del bridge MCP de TrainingPeaks.
type Service struct {
	scriptPath string
}

// NewService crea una nova instància del servei de TrainingPeaks i cerca la ruta de l'script tp_bridge.py.
func NewService() *Service {
	// Busquem tp_bridge.py en diverses ubicacions probables (directori actual, internal/infrastructure/trainingpeaks, etc.)
	candidates := []string{
		"internal/infrastructure/trainingpeaks/tp_bridge.py",
		"tp_bridge.py",
		"/app/internal/infrastructure/trainingpeaks/tp_bridge.py",
	}

	selectedPath := candidates[0]
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			selectedPath = path
			break
		}
	}

	absPath, err := filepath.Abs(selectedPath)
	if err == nil {
		selectedPath = absPath
	}

	log.Printf("[TrainingPeaks Infrastructure] Inicialitzat servei amb script bridge: %s", selectedPath)
	return &Service{
		scriptPath: selectedPath,
	}
}

func (s *Service) runBridgeScriptRaw(ctx context.Context, cmdName, username, password, cookie, token, date, startDate, endDate string) ([]byte, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctxTimeout, "python3", s.scriptPath)
	
	// Injectem credencials i paràmetres com a variables d'entorn temporals
	env := os.Environ()
	env = append(env, fmt.Sprintf("TP_COMMAND=%s", cmdName))
	env = append(env, fmt.Sprintf("TP_USERNAME=%s", username))
	env = append(env, fmt.Sprintf("TP_PASSWORD=%s", password))
	if cookie != "" {
		env = append(env, fmt.Sprintf("TP_COOKIE=%s", cookie))
	}
	if token != "" {
		env = append(env, fmt.Sprintf("TP_TOKEN=%s", token))
	}
	if date != "" {
		env = append(env, fmt.Sprintf("TP_DATE=%s", date))
	}
	if startDate != "" {
		env = append(env, fmt.Sprintf("TP_START_DATE=%s", startDate))
	}
	if endDate != "" {
		env = append(env, fmt.Sprintf("TP_END_DATE=%s", endDate))
	}
	cmd.Env = env

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	log.Printf("[TrainingPeaks MCP Subprocess] Executant tp_bridge.py (cmd=%s, user=%s)", cmdName, username)
	runErr := cmd.Run()

	if errStr := strings.TrimSpace(errBuf.String()); errStr != "" {
		log.Printf("[TrainingPeaks MCP Subprocess Trace]:\n%s", errStr)
	}

	if runErr != nil {
		log.Printf("[TrainingPeaks MCP Subprocess] Error executant script: %v", runErr)
		return nil, fmt.Errorf("error executant el subprocés de TrainingPeaks (%w): %s", runErr, errBuf.String())
	}

	log.Printf("[TrainingPeaks MCP Subprocess JSON Output]: %s", strings.TrimSpace(outBuf.String()))
	return outBuf.Bytes(), nil
}

// GetPMCData recupera les mètriques de Fitness (CTL), Fatigue (ATL) i Form (TSB).
func (s *Service) GetPMCData(ctx context.Context, username, password, cookie, token string) (*domain.PMCData, error) {
	raw, err := s.runBridgeScriptRaw(ctx, "get_pmc", username, password, cookie, token, "", "", "")
	if err != nil {
		return nil, fmt.Errorf("no s'han pogut obtenir les mètriques PMC: %w", err)
	}

	var resp struct {
		Status  string  `json:"status"`
		Message string  `json:"message,omitempty"`
		CTL     float64 `json:"ctl,omitempty"`
		ATL     float64 `json:"atl,omitempty"`
		TSB     float64 `json:"tsb,omitempty"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("resposta invàlida de PMC: %w", err)
	}
	if resp.Status == "error" {
		return nil, fmt.Errorf("%s", resp.Message)
	}

	return &domain.PMCData{
		UserName: username,
		CTL:      resp.CTL,
		ATL:      resp.ATL,
		TSB:      resp.TSB,
	}, nil
}

// GetDailyWorkouts recupera la informació de les sessions planificades per a la data sol·licitada.
func (s *Service) GetDailyWorkouts(ctx context.Context, username, password, cookie, token, date string) ([]domain.WorkoutData, error) {
	raw, err := s.runBridgeScriptRaw(ctx, "get_workout", username, password, cookie, token, date, "", "")
	if err != nil {
		return nil, fmt.Errorf("no s'han pogut obtenir els entrenaments planificats: %w", err)
	}

	var resp struct {
		Status      string  `json:"status"`
		Message     string  `json:"message,omitempty"`
		Date        string  `json:"date,omitempty"`
		Title       string  `json:"title,omitempty"`
		Description string  `json:"description,omitempty"`
		PlannedTSS  float64 `json:"planned_tss,omitempty"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("resposta invàlida de workout: %w", err)
	}
	if resp.Status == "error" {
		return nil, fmt.Errorf("%s", resp.Message)
	}

	return []domain.WorkoutData{
		{
			Date:        resp.Date,
			Title:       resp.Title,
			Description: resp.Description,
			PlannedTSS:  resp.PlannedTSS,
		},
	}, nil
}

// GetWorkoutsRange recupera la llista d'entrenaments de l'atleta per a un rang de dates específic.
func (s *Service) GetWorkoutsRange(ctx context.Context, username, password, cookie, token, startDate, endDate string) ([]domain.WorkoutData, error) {
	raw, err := s.runBridgeScriptRaw(ctx, "get_workouts_range", username, password, cookie, token, "", startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("no s'han pogut obtenir els entrenaments en el rang %s-%s: %w", startDate, endDate, err)
	}

	var resp struct {
		Status   string `json:"status"`
		Message  string `json:"message,omitempty"`
		Workouts []struct {
			Date        string  `json:"date"`
			Title       string  `json:"title"`
			Description string  `json:"description"`
			PlannedTSS  float64 `json:"planned_tss"`
			Sport       string  `json:"sport"`
			Completed   bool    `json:"completed"`
		} `json:"workouts"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("resposta de rang invàlida: %w", err)
	}
	if resp.Status == "error" {
		return nil, fmt.Errorf("%s", resp.Message)
	}

	var result []domain.WorkoutData
	for _, w := range resp.Workouts {
		displayTitle := w.Title
		if w.Sport != "" {
			displayTitle = fmt.Sprintf("%s: %s", w.Sport, w.Title)
		}
		result = append(result, domain.WorkoutData{
			Date:        w.Date,
			Title:       displayTitle,
			Description: w.Description,
			PlannedTSS:  w.PlannedTSS,
		})
	}
	return result, nil
}
