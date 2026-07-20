package domain

import "context"

// CacheRepository és la interfície per interactuar amb la cache de PostgreSQL.
type CacheRepository interface {
	Get(ctx context.Context, key string) (*CacheEntry, error)
	Set(ctx context.Context, entry CacheEntry) error
	Flush(ctx context.Context) error
}

// MealPlanRepository és la interfície per consultar la pauta nutricional a PostgreSQL.
type MealPlanRepository interface {
	GetByDayOfWeek(ctx context.Context, day string) ([]MealPlan, error)
	GetAll(ctx context.Context) ([]MealPlan, error)
}

// CalendarService és la interfície per interactuar amb Google Calendar API.
type CalendarService interface {
	GetTodayEvents(ctx context.Context) ([]CalendarEvent, error)
	GetWeekEvents(ctx context.Context) ([]CalendarEvent, error)
	CreateEvent(ctx context.Context, event CalendarEvent) (*CalendarEvent, error)
}

// ToolProvider defineix el contracte per a la resolució de crides a eines (Function Calling) per part de Gemini.
type ToolProvider interface {
	ExecuteFunction(ctx context.Context, name string, args map[string]any) (any, error)
}

// AIService defineix el contracte amb l'Engine de IA (Gemini).
type AIService interface {
	Chat(ctx context.Context, query Query, tools ToolProvider) (string, error)
}

// MessageSender és el port d'eixida per enviar missatges de text a un chat/grup de WhatsApp.
type MessageSender interface {
	SendText(ctx context.Context, chatID string, text string) error
}
