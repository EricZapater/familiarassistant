package google

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ericzapater/familiarassistant/internal/domain"
	calendar "google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

// CalendarService implementa domain.CalendarService utilitzant l'SDK oficial de Google Calendar.
type CalendarService struct {
	srv        *calendar.Service
	calendarID string
	loc        *time.Location
}

// NewCalendarService crea un nou servei de Google Calendar autenticat via Service Account.
func NewCalendarService(ctx context.Context, credentialsFile, calendarID, timezone string) (*CalendarService, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.Local
	}

	srv, err := calendar.NewService(ctx, option.WithCredentialsFile(credentialsFile))
	if err != nil {
		return nil, fmt.Errorf("error inicialitzant client de Google Calendar: %w", err)
	}

	return &CalendarService{
		srv:        srv,
		calendarID: calendarID,
		loc:        loc,
	}, nil
}

// GetTodayEvents recupera tots els esdeveniments d'avui (de 00:00:00 a 23:59:59).
func (s *CalendarService) GetTodayEvents(ctx context.Context) ([]domain.CalendarEvent, error) {
	now := time.Now().In(s.loc)
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.loc)
	endOfDay := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 999999999, s.loc)

	return s.fetchEvents(ctx, startOfDay, endOfDay)
}

// GetWeekEvents recupera els esdeveniments des d'avui fins a 7 dies vista.
func (s *CalendarService) GetWeekEvents(ctx context.Context) ([]domain.CalendarEvent, error) {
	now := time.Now().In(s.loc)
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.loc)
	endOfWeek := startOfDay.AddDate(0, 0, 7)

	return s.fetchEvents(ctx, startOfDay, endOfWeek)
}

func (s *CalendarService) fetchEvents(ctx context.Context, timeMin, timeMax time.Time) ([]domain.CalendarEvent, error) {
	eventsList, err := s.srv.Events.List(s.calendarID).
		Context(ctx).
		TimeMin(timeMin.Format(time.RFC3339)).
		TimeMax(timeMax.Format(time.RFC3339)).
		SingleEvents(true).
		OrderBy("startTime").
		Do()

	if err != nil {
		return nil, fmt.Errorf("error consultant l'API de Google Calendar (calendarID=%s): %w", s.calendarID, err)
	}

	var results []domain.CalendarEvent
	for _, item := range eventsList.Items {
		start := parseCalendarTime(item.Start, s.loc)
		end := parseCalendarTime(item.End, s.loc)

		results = append(results, domain.CalendarEvent{
			ID:          item.Id,
			Summary:     item.Summary,
			Description: item.Description,
			StartTime:   start,
			EndTime:     end,
			Location:    item.Location,
		})
	}

	log.Printf("[Google Calendar API Result] S'han obtingut %d esdeveniments (període: %s fins %s):", len(results), timeMin.Format("02/01/2006 15:04"), timeMax.Format("02/01/2006 15:04"))
	for i, evt := range results {
		log.Printf("  📅 Esdeveniment [%d/%d]: %s | Inici: %s | Fi: %s | Lloc: %s",
			i+1, len(results), evt.Summary, evt.StartTime.Format("02/01/2006 15:04"), evt.EndTime.Format("02/01/2006 15:04"), evt.Location)
	}

	return results, nil
}

func parseCalendarTime(evtTime *calendar.EventDateTime, loc *time.Location) time.Time {
	if evtTime == nil {
		return time.Time{}
	}
	if evtTime.DateTime != "" {
		t, err := time.Parse(time.RFC3339, evtTime.DateTime)
		if err == nil {
			return t.In(loc)
		}
	}
	if evtTime.Date != "" {
		t, err := time.Parse("2006-01-02", evtTime.Date)
		if err == nil {
			return t.In(loc)
		}
	}
	return time.Time{}
}

// CreateEvent crea un nou esdeveniment a Google Calendar.
func (s *CalendarService) CreateEvent(ctx context.Context, event domain.CalendarEvent) (*domain.CalendarEvent, error) {
	gEvent := &calendar.Event{
		Summary:     event.Summary,
		Description: event.Description,
		Location:    event.Location,
		Start: &calendar.EventDateTime{
			DateTime: event.StartTime.Format(time.RFC3339),
			TimeZone: s.loc.String(),
		},
		End: &calendar.EventDateTime{
			DateTime: event.EndTime.Format(time.RFC3339),
			TimeZone: s.loc.String(),
		},
	}

	log.Printf("[Google Calendar API] Creant nou esdeveniment: %q | Inici: %s | Fi: %s",
		event.Summary, event.StartTime.Format("02/01/2006 15:04"), event.EndTime.Format("02/01/2006 15:04"))

	created, err := s.srv.Events.Insert(s.calendarID, gEvent).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("error creant esdeveniment a Google Calendar: %w", err)
	}

	log.Printf("[Google Calendar API Result] Esdeveniment creat amb èxit | ID: %s", created.Id)

	event.ID = created.Id
	return &event, nil
}
