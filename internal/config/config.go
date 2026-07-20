package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Config manté la configuració global de l'aplicació llegida de l'entorn.
type Config struct {
	WhatsAppGroupID       string
	WhatsAppMyID          string
	GeminiAPIKey          string
	GeminiModel           string
	PostgresDSN           string
	GoogleCalendarID      string
	GoogleCredentialsFile string
	Timezone              string
}

// Load carregar les variables d'entorn (des de .env si existeix) i les valida.
func Load() (*Config, error) {
	// Intentem carregar .env, però no ho fem fallar si no existeix (per entorns VPS amb env vars reals)
	_ = godotenv.Load()

	cfg := &Config{
		WhatsAppGroupID:       getEnv("WHATSAPP_GROUP_ID", ""),
		WhatsAppMyID:          getEnv("WHATSAPP_MY_ID", ""),
		GeminiAPIKey:          getEnv("GEMINI_API_KEY", ""),
		GeminiModel:           getEnv("GEMINI_MODEL", "gemini-2.5-flash"),
		PostgresDSN:           getEnv("POSTGRES_DSN", ""),
		GoogleCalendarID:      getEnv("GOOGLE_CALENDAR_ID", ""),
		GoogleCredentialsFile: getEnv("GOOGLE_CREDENTIALS_FILE", ""),
		Timezone:              getEnv("TIMEZONE", "Europe/Madrid"),
	}

	var missing []string
	if cfg.WhatsAppGroupID == "" {
		missing = append(missing, "WHATSAPP_GROUP_ID")
	}
	if cfg.WhatsAppMyID == "" {
		missing = append(missing, "WHATSAPP_MY_ID")
	}
	if cfg.GeminiAPIKey == "" {
		missing = append(missing, "GEMINI_API_KEY")
	}
	if cfg.PostgresDSN == "" {
		missing = append(missing, "POSTGRES_DSN")
	}
	if cfg.GoogleCalendarID == "" {
		missing = append(missing, "GOOGLE_CALENDAR_ID")
	}
	if cfg.GoogleCredentialsFile == "" {
		missing = append(missing, "GOOGLE_CREDENTIALS_FILE")
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("falten les següents variables d'entorn obligatòries: %s", strings.Join(missing, ", "))
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return fallback
}
