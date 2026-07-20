package domain

import "time"

// CommandType representa el tipus de comandament sol·licitat per l'usuari.
type CommandType string

const (
	CmdNutri      CommandType = "nutri"
	CmdCalendar   CommandType = "calendar"
	CmdFlushCache CommandType = "flushcache"
)

// Query és la petició de l'usuari parsejada i col·locada en domini.
type Query struct {
	Command   CommandType
	RawText   string
	Timestamp time.Time
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
