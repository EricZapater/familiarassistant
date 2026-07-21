package domain

import "time"

// CommandType representa el tipus de comandament sol·licitat per l'usuari.
type CommandType string

const (
	CmdNutri      CommandType = "nutri"
	CmdCalendar   CommandType = "calendar"
	CmdTraining   CommandType = "training"
	CmdFlushCache CommandType = "flushcache"
	CmdBonDia     CommandType = "bondia"
)

// Query és la petició de l'usuari parsejada i col·locada en domini.
type Query struct {
	Command     CommandType
	RawText     string
	Timestamp   time.Time
	SenderPhone string
	UserName    string
	TPUsername  string
	TPPassword  string
	TPCookie    string
	TPToken     string
}

// CacheEntry representa un registre emmagatzemat a la memòria cau de PostgreSQL.
type CacheEntry struct {
	Key       string
	Response  string
	ExpiresAt time.Time
}

// MealPlan representa una àpat de la pauta nutricional.
type MealPlan struct {
	ID        int    `json:"id"`
	DayOfWeek string `json:"dia_setmana"` // dilluns, dimarts, dimecres...
	Meal      string `json:"apat"`        // esmorzar, dinar, berenar, sopar
	Menu      string `json:"menu"`        // descripció del plat/àpat
}

// CalendarEvent representa un esdeveniment obtingut de Google Calendar.
type CalendarEvent struct {
	ID          string    `json:"id"`
	Summary     string    `json:"summary"`
	Description string    `json:"description,omitempty"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	Location    string    `json:"location,omitempty"`
}

// PMCData representa les mètriques de rendiment Performance Management Chart (CTL, ATL, TSB).
type PMCData struct {
	UserName string  `json:"user_name"`
	CTL      float64 `json:"ctl"` // Fitness (Chronic Training Load)
	ATL      float64 `json:"atl"` // Fatigue (Acute Training Load)
	TSB      float64 `json:"tsb"` // Form (Training Stress Balance)
}

// WorkoutData representa la descripció i detalls d'una sessió d'entrenament planificada a TrainingPeaks.
type WorkoutData struct {
	Date        string  `json:"date"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	PlannedTSS  float64 `json:"planned_tss"`
}

