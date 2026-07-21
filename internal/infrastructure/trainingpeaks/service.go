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

type scriptBridgeResponse struct {
	Status      string  `json:"status"`
	Message     string  `json:"message,omitempty"`
	CTL         float64 `json:"ctl,omitempty"`
	ATL         float64 `json:"atl,omitempty"`
	TSB         float64 `json:"tsb,omitempty"`
	Date        string  `json:"date,omitempty"`
	Title       string  `json:"title,omitempty"`
	Description string  `json:"description,omitempty"`
	PlannedTSS  float64 `json:"planned_tss,omitempty"`
}

func (s *Service) runBridgeScript(ctx context.Context, cmdName, username, password, cookie, token, date string) (*scriptBridgeResponse, error) {
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

	var resp scriptBridgeResponse
	if err := json.Unmarshal(outBuf.Bytes(), &resp); err != nil {
		log.Printf("[TrainingPeaks MCP Subprocess] Error parsejant JSON de sortida: %v. Raw output: %s", err, outBuf.String())
		return nil, fmt.Errorf("resposta invàlida del subprocés de TrainingPeaks: %w", err)
	}

	if resp.Status == "error" {
		return nil, fmt.Errorf("%s", resp.Message)
	}

	return &resp, nil
}

// GetPMCData recupera les mètriques de Fitness (CTL), Fatigue (ATL) i Form (TSB).
func (s *Service) GetPMCData(ctx context.Context, username, password, cookie, token string) (*domain.PMCData, error) {
	resp, err := s.runBridgeScript(ctx, "get_pmc", username, password, cookie, token, "")
	if err != nil {
		return nil, fmt.Errorf("no s'han pogut obtenir les mètriques PMC: %w", err)
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
	resp, err := s.runBridgeScript(ctx, "get_workout", username, password, cookie, token, date)
	if err != nil {
		return nil, fmt.Errorf("no s'han pogut obtenir els entrenaments planificats: %w", err)
	}

	workout := domain.WorkoutData{
		Date:        resp.Date,
		Title:       resp.Title,
		Description: resp.Description,
		PlannedTSS:  resp.PlannedTSS,
	}

	return []domain.WorkoutData{workout}, nil
}
