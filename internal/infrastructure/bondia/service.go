package bondia

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
)

// NewsItem representa un element de notícies o efemèrides recuperat pel bridge de Python.
type NewsItem struct {
	Type        string `json:"type"`        // efemeride, catalunya, bones_noticies
	Year        string `json:"year"`        // utilitzat per a efemèrides
	Title       string `json:"title"`
	Description string `json:"description"`
	Link        string `json:"link"`
}

// Service implementa la recuperació de notícies i efemèrides executant l'script de Python.
type Service struct {
	scriptPath string
}

// NewService crea un nou servei de BonDia i cerca l'script bondia_bridge.py.
func NewService() *Service {
	candidates := []string{
		"internal/infrastructure/bondia/bondia_bridge.py",
		"bondia_bridge.py",
		"/app/internal/infrastructure/bondia/bondia_bridge.py",
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

	log.Printf("[BonDia Infrastructure] Inicialitzat servei amb script bridge: %s", selectedPath)
	return &Service{
		scriptPath: selectedPath,
	}
}

// GetNewsAndCuriosities executa l'script de Python i retorna els elements llistats.
func (s *Service) GetNewsAndCuriosities(ctx context.Context) ([]NewsItem, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctxTimeout, "python3", s.scriptPath)
	
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	log.Printf("[BonDia Subprocess] Executant bondia_bridge.py")
	runErr := cmd.Run()

	if errStr := strings.TrimSpace(errBuf.String()); errStr != "" {
		log.Printf("[BonDia Subprocess Trace]:\n%s", errStr)
	}

	if runErr != nil {
		return nil, fmt.Errorf("error executant el subprocés de BonDia (%w): %s", runErr, errBuf.String())
	}

	var items []NewsItem
	if err := json.Unmarshal(outBuf.Bytes(), &items); err != nil {
		return nil, fmt.Errorf("error descodificant la sortida del bridge: %w", err)
	}

	return items, nil
}
