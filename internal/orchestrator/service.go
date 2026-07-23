package orchestrator

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ericzapater/familiarassistant/internal/config"
	"github.com/ericzapater/familiarassistant/internal/domain"
	"github.com/ericzapater/familiarassistant/internal/infrastructure/bondia"
)

// Service és l'orquestrador central de l'aplicació (cervell del negoci).
type Service struct {
	cacheRepo   domain.CacheRepository
	mealRepo    domain.MealPlanRepository
	calendarSvc domain.CalendarService
	aiSvc       domain.AIService
	tpService   domain.TrainingPeaksService
	bondiaSvc   *bondia.Service
	tpUsersMap  map[string]config.UserTPConfig
	loc         *time.Location
}

// NewService crea un nou servei d'orquestració amb les seves dependències injectades.
func NewService(
	cacheRepo domain.CacheRepository,
	mealRepo domain.MealPlanRepository,
	calendarSvc domain.CalendarService,
	aiSvc domain.AIService,
	tpService domain.TrainingPeaksService,
	bondiaSvc *bondia.Service,
	tpUsersMap map[string]config.UserTPConfig,
	timezone string,
) *Service {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.Local
	}

	return &Service{
		cacheRepo:   cacheRepo,
		mealRepo:    mealRepo,
		calendarSvc: calendarSvc,
		aiSvc:       aiSvc,
		tpService:   tpService,
		bondiaSvc:   bondiaSvc,
		tpUsersMap:  tpUsersMap,
		loc:         loc,
	}
}

// ProcessQuery rep la petició, comprova la cache, crida l'IA si cal i desa el resultat a la cache.
func (s *Service) ProcessQuery(ctx context.Context, query domain.Query) (string, error) {
	if query.Command == domain.CmdFlushCache {
		log.Println("[Orchestrator] Executant buidat de cache (/flushcache)...")
		if err := s.cacheRepo.Flush(ctx); err != nil {
			return "", fmt.Errorf("error buidant la cache: %w", err)
		}
		return "🗑️ S'ha buidat la memòria cau de PostgreSQL correctament.", nil
	}

	now := time.Now().In(s.loc)
	cacheKey, expiresAt := s.generateCacheParams(query, now)

	// 1. Consulta la memòria cau a PostgreSQL (Estalvi de tokens: 100% en cache hit)
	cachedEntry, err := s.cacheRepo.Get(ctx, cacheKey)
	if err != nil {
		log.Printf("[Orchestrator] Error llegint cache (key=%s): %v", cacheKey, err)
		// En cas d'error de cache, continuem sense cache
	} else if cachedEntry != nil {
		log.Printf("[Orchestrator] Cache HIT per a la clau '%s'", cacheKey)
		return cachedEntry.Response, nil
	}

	log.Printf("[Orchestrator] Cache MISS per a la clau '%s'...", cacheKey)

	// 2. Si no hi ha cache, processem directament si és /mister o cridem Gemini altrament
	var response string
	if query.Command == domain.CmdMister {
		var err error
		response, err = s.handleDirectTrainingQuery(ctx, query)
		if err != nil {
			return "", err
		}
	} else {
		var err error
		response, err = s.aiSvc.Chat(ctx, query, s)
		if err != nil {
			return "", fmt.Errorf("error al processar la consulta amb l'assistent d'IA: %w", err)
		}
	}

	// 3. Desa el resultat a la cache si s'ha obtingut una resposta vàlida
	if strings.TrimSpace(response) != "" {
		err := s.cacheRepo.Set(ctx, domain.CacheEntry{
			Key:       cacheKey,
			Response:  response,
			ExpiresAt: expiresAt,
		})
		if err != nil {
			log.Printf("[Orchestrator] Error guardant a la cache (key=%s): %v", cacheKey, err)
		} else {
			log.Printf("[Orchestrator] Resposta desada a la cache fins a %s", expiresAt.Format("15:04:05 02-01-2006"))
		}
	}

	return response, nil
}

// ExecuteFunction implementa la interfície domain.ToolProvider per respondre a les crides de Function Calling de Gemini.
func (s *Service) ExecuteFunction(ctx context.Context, name string, args map[string]any) (any, error) {
	log.Printf("[Orchestrator] Executant Function Calling tool: %s amb arguments: %v", name, args)

	switch name {
	case "ConsultarPauta":
		dia, _ := args["dia_setmana"].(string)
		if dia == "" || strings.ToLower(dia) == "tots" {
			plans, err := s.mealRepo.GetAll(ctx)
			if err != nil {
				return nil, fmt.Errorf("error consultant tota la pauta: %w", err)
			}
			return plans, nil
		}

		plans, err := s.mealRepo.GetByDayOfWeek(ctx, dia)
		if err != nil {
			return nil, fmt.Errorf("error consultant pauta per al dia %s: %w", dia, err)
		}
		if len(plans) == 0 {
			return map[string]string{"missatge": fmt.Sprintf("No s'ha trobat cap menú registrat per al dia '%s'.", dia)}, nil
		}
		return plans, nil

	case "ConsultarCalendari":
		periode, _ := args["periode"].(string)
		if strings.ToLower(periode) == "week" || strings.ToLower(periode) == "setmana" {
			events, err := s.calendarSvc.GetWeekEvents(ctx)
			if err != nil {
				return nil, fmt.Errorf("error obtenint esdeveniments de la setmana: %w", err)
			}
			if len(events) == 0 {
				return map[string]string{"missatge": "No hi ha esdeveniments al calendari per als pròxims 7 dies."}, nil
			}
			return events, nil
		}

		// Per defecte consulta 'today' (avui)
		events, err := s.calendarSvc.GetTodayEvents(ctx)
		if err != nil {
			return nil, fmt.Errorf("error obtenint esdeveniments d'avui: %w", err)
		}
		if len(events) == 0 {
			return map[string]string{"missatge": "No hi ha cap esdeveniment al calendari programat per avui."}, nil
		}
		return events, nil

	case "CrearEsdeveniment":
		titol, _ := args["titol"].(string)
		dataIniciStr, _ := args["data_inici"].(string)
		lloc, _ := args["lloc"].(string)
		descripcio, _ := args["descripcio"].(string)

		if titol == "" || dataIniciStr == "" {
			return nil, fmt.Errorf("falten camps obligatoris ('titol' o 'data_inici') per crear l'esdeveniment")
		}

		startTime, err := time.Parse(time.RFC3339, dataIniciStr)
		if err != nil {
			startTime, err = time.Parse("2006-01-02T15:04:05", dataIniciStr)
			if err != nil {
				return nil, fmt.Errorf("format de data_inici invàlid (%s): %w", dataIniciStr, err)
			}
		}
		startTime = startTime.In(s.loc)

		duradaMin := 60
		if val, ok := args["durada_minuts"].(float64); ok && val > 0 {
			duradaMin = int(val)
		}
		endTime := startTime.Add(time.Duration(duradaMin) * time.Minute)

		createdEvt, err := s.calendarSvc.CreateEvent(ctx, domain.CalendarEvent{
			Summary:     titol,
			Description: descripcio,
			StartTime:   startTime,
			EndTime:     endTime,
			Location:    lloc,
		})
		if err != nil {
			return nil, fmt.Errorf("error creant esdeveniment: %w", err)
		}

		// Netejem la cache perquè la pròxima consulta reflecteixi el nou esdeveniment
		_ = s.cacheRepo.Flush(ctx)

		return map[string]any{
			"status":       "success",
			"missatge":     fmt.Sprintf("Esdeveniment '%s' creat correctament per al %s", createdEvt.Summary, createdEvt.StartTime.Format("02/01/2006 a les 15:04")),
			"esdeveniment": createdEvt,
		}, nil

	case "ObtenirMetriquesRendiment":
		nomUsuari, _ := args["nom_usuari"].(string)
		userCfg, found := s.findTPUserConfig(nomUsuari)
		if !found {
			return map[string]string{
				"error": "⚠️ No m'he pogut connectar al teu compte de TrainingPeaks actualment.",
			}, nil
		}

		data, err := s.tpService.GetPMCData(ctx, userCfg.Username, userCfg.Password, userCfg.Cookie, userCfg.Token)
		if err != nil {
			log.Printf("[Orchestrator] Error en connector TrainingPeaks (GetPMCData): %v", err)
			return map[string]string{
				"error": "⚠️ No m'he pogut connectar al teu compte de TrainingPeaks actualment.",
			}, nil
		}
		data.UserName = userCfg.Name
		return data, nil

	case "ObtenirEntrenamentPlanificat":
		nomUsuari, _ := args["nom_usuari"].(string)
		dateStr, _ := args["data"].(string)
		if dateStr == "" {
			dateStr = time.Now().In(s.loc).Format("2006-01-02")
		}

		userCfg, found := s.findTPUserConfig(nomUsuari)
		if !found {
			return map[string]string{
				"error": "⚠️ No m'he pogut connectar al teu compte de TrainingPeaks actualment.",
			}, nil
		}

		workouts, err := s.tpService.GetDailyWorkouts(ctx, userCfg.Username, userCfg.Password, userCfg.Cookie, userCfg.Token, dateStr)
		if err != nil {
			log.Printf("[Orchestrator] Error en connector TrainingPeaks (GetDailyWorkouts): %v", err)
			return map[string]string{
				"error": "⚠️ No m'he pogut connectar al teu compte de TrainingPeaks actualment.",
			}, nil
		}
		return workouts, nil

	case "ObtenirNoticiesICuriositats":
		items, err := s.bondiaSvc.GetNewsAndCuriosities(ctx)
		if err != nil {
			log.Printf("[Orchestrator] Error en connector BonDia (GetNewsAndCuriosities): %v", err)
			return nil, fmt.Errorf("error obtenint notícies i curiositats: %w", err)
		}
		return items, nil

	default:
		return nil, fmt.Errorf("funció desconeguda: %s", name)
	}
}

func (s *Service) findTPUserConfig(name string) (config.UserTPConfig, bool) {
	nameClean := strings.ToLower(strings.TrimSpace(name))
	for _, u := range s.tpUsersMap {
		if strings.ToLower(u.Name) == nameClean || strings.ToLower(u.Username) == nameClean {
			return u, true
		}
	}
	// Fallback si només tenim un usuari registrat
	if len(s.tpUsersMap) == 1 {
		for _, u := range s.tpUsersMap {
			return u, true
		}
	}
	return config.UserTPConfig{}, false
}

// generateCacheParams genera la clau i la data d'expiració de la cache segons el tipus de comandament.
func (s *Service) generateCacheParams(query domain.Query, now time.Time) (string, time.Time) {
	switch query.Command {
	case domain.CmdNutri:
		normalizedText := strings.ToLower(strings.TrimSpace(query.RawText))
		hash := sha256.Sum256([]byte(normalizedText))
		questionHash := fmt.Sprintf("%x", hash[:8])

		dateStr := now.Format("2006-01-02")
		key := fmt.Sprintf("menu:%s:%s", questionHash, dateStr)
		endOfDay := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, s.loc)
		return key, endOfDay

	case domain.CmdCalendar:
		// La cache de calendari només coincideix si la pregunta és exactament la mateixa (normalitzada)
		normalizedText := strings.ToLower(strings.TrimSpace(query.RawText))
		hash := sha256.Sum256([]byte(normalizedText))
		questionHash := fmt.Sprintf("%x", hash[:8]) // 16 caràcters hex per identificar la pregunta única

		roundedMinute := (now.Minute() / 15) * 15
		keyTime := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), roundedMinute, 0, 0, s.loc)
		key := fmt.Sprintf("calendar:%s:%s", questionHash, keyTime.Format("2006-01-02T15:04"))
		expiresAt := now.Add(15 * time.Minute)
		return key, expiresAt

	case domain.CmdMister:
		normalizedText := strings.ToLower(strings.TrimSpace(query.RawText))
		hash := sha256.Sum256([]byte(normalizedText))
		questionHash := fmt.Sprintf("%x", hash[:8])
		user := strings.ToLower(strings.TrimSpace(query.UserName))
		if user == "" {
			user = "default"
		}
		dateStr := now.Format("2006-01-02")
		key := fmt.Sprintf("mister:%s:%s:%s", user, questionHash, dateStr)
		expiresAt := now.Add(15 * time.Minute)
		return key, expiresAt

	case domain.CmdBonDia:
		dateStr := now.Format("2006-01-02")
		key := fmt.Sprintf("bondia:%s", dateStr)
		endOfDay := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, s.loc)
		return key, endOfDay

	default:
		key := fmt.Sprintf("generic:%d", now.Unix())
		return key, now.Add(5 * time.Minute)
	}
}

// handleDirectTrainingQuery gestiona directament la consulta d'entrenaments sense passar per la IA.
func (s *Service) handleDirectTrainingQuery(ctx context.Context, query domain.Query) (string, error) {
	log.Printf("[Orchestrator] Processant petició de training directa per a %s (telèfon: %s)...", query.UserName, query.SenderPhone)

	userCfg, found := s.findTPUserConfig(query.UserName)
	if !found {
		return "⚠️ No s'ha trobat cap configuració de TrainingPeaks associada al teu usuari.", nil
	}

	baseTime := time.Now().In(s.loc)
	if !query.Timestamp.IsZero() {
		baseTime = query.Timestamp.In(s.loc)
	}

	rawText := strings.TrimSpace(query.RawText)
	if rawText != "" {
		// Consulta d'un sol dia
		targetDate, parsed := parseDateQuery(rawText, baseTime)
		if !parsed {
			return fmt.Sprintf("⚠️ No he pogut entendre la data '%s'. Si us plau, utilitza un format com YYYY-MM-DD o un dia de la setmana (ex: 'el proper dimarts').", rawText), nil
		}
		targetDateStr := targetDate.Format("2006-01-02")

		workouts, err := s.tpService.GetWorkoutsRange(ctx, userCfg.Username, userCfg.Password, userCfg.Cookie, userCfg.Token, targetDateStr, targetDateStr)
		if err != nil {
			log.Printf("[Orchestrator] Error obtenint entrenaments de TrainingPeaks: %v", err)
			return "", fmt.Errorf("error obtenint entrenaments de TrainingPeaks: %w", err)
		}

		dayOfWeekName := getWeekdayCatalan(targetDate.Weekday())
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("🏋️‍♂️ *Entrenament de %s per a %s (%s)*:\n", capitalize(userCfg.Name), formatDateStr(targetDateStr), dayOfWeekName))

		if len(workouts) == 0 {
			sb.WriteString("\nNo hi ha cap entrenament planificat a TrainingPeaks per a aquest dia.")
			return sb.String(), nil
		}

		for _, w := range workouts {
			sb.WriteString(fmt.Sprintf("\n• *%s*", w.Title))
			if w.PlannedTSS > 0 {
				sb.WriteString(fmt.Sprintf(" (%.1f TSS)", w.PlannedTSS))
			}
			sb.WriteString("\n")
			if w.Description != "" {
				descLines := strings.Split(w.Description, "\n")
				for _, line := range descLines {
					lineClean := strings.TrimSpace(line)
					if lineClean != "" {
						sb.WriteString(fmt.Sprintf("  _ %s _\n", lineClean))
					}
				}
			}
		}
		return sb.String(), nil
	}

	// Calculem el rang de dates per defecte: des d'avui fins a d'aquí 7 dies (8 dies en total)
	startDate := baseTime.Format("2006-01-02")
	endDate := baseTime.AddDate(0, 0, 7).Format("2006-01-02")

	workouts, err := s.tpService.GetWorkoutsRange(ctx, userCfg.Username, userCfg.Password, userCfg.Cookie, userCfg.Token, startDate, endDate)
	if err != nil {
		log.Printf("[Orchestrator] Error obtenint entrenaments de TrainingPeaks: %v", err)
		return "", fmt.Errorf("error obtenint entrenaments de TrainingPeaks: %w", err)
	}

	if len(workouts) == 0 {
		return fmt.Sprintf("📅 *Entrenaments de la propera setmana (%s a %s)*:\n\nNo hi ha cap entrenament planificat a TrainingPeaks per a aquest període.", formatDateStr(startDate), formatDateStr(endDate)), nil
	}

	// Agrupem els entrenaments per data
	workoutsByDate := make(map[string][]domain.WorkoutData)
	var datesOrdered []string
	
	// Preparem els propers 8 dies (des d'avui fins a d'aquí 7 dies) per mostrar-los en ordre
	for i := 0; i <= 7; i++ {
		dStr := baseTime.AddDate(0, 0, i).Format("2006-01-02")
		datesOrdered = append(datesOrdered, dStr)
	}

	for _, w := range workouts {
		workoutsByDate[w.Date] = append(workoutsByDate[w.Date], w)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🏋️‍♂️ *Entrenaments de la propera setmana per a %s* (%s - %s):\n", capitalize(userCfg.Name), formatDateStr(startDate), formatDateStr(endDate)))

	hasWorkouts := false
	for _, dateStr := range datesOrdered {
		wList, found := workoutsByDate[dateStr]
		if !found || len(wList) == 0 {
			continue
		}
		
		hasWorkouts = true
		parsedDate, err := time.Parse("2006-01-02", dateStr)
		var dateHeader string
		if err == nil {
			dateHeader = parsedDate.Format("02/01")
			dateHeader = fmt.Sprintf("%s (%s)", dateHeader, getWeekdayCatalan(parsedDate.Weekday()))
		} else {
			dateHeader = dateStr
		}

		sb.WriteString(fmt.Sprintf("\n📅 *%s*:\n", dateHeader))
		for _, w := range wList {
			sb.WriteString(fmt.Sprintf("• *%s*", w.Title))
			if w.PlannedTSS > 0 {
				sb.WriteString(fmt.Sprintf(" (%.1f TSS)", w.PlannedTSS))
			}
			sb.WriteString("\n")
		}
	}

	if !hasWorkouts {
		return fmt.Sprintf("📅 *Entrenaments de la propera setmana (%s a %s)*:\n\nNo s'ha trobat cap entrenament planificat en aquests dies.", formatDateStr(startDate), formatDateStr(endDate)), nil
	}

	return sb.String(), nil
}

func parseDateQuery(text string, baseTime time.Time) (time.Time, bool) {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return time.Time{}, false
	}

	// 1. Provar formats de data habituals
	formats := []string{
		"2006-01-02",
		"02-01-2006",
		"02/01/2006",
		"2006/01/02",
	}
	for _, fmtStr := range formats {
		if t, err := time.Parse(fmtStr, text); err == nil {
			return t, true
		}
	}

	// 2. Paraules relatives
	if text == "avui" {
		return baseTime, true
	}
	if text == "demà" || text == "dema" {
		return baseTime.AddDate(0, 0, 1), true
	}
	if text == "ahir" {
		return baseTime.AddDate(0, 0, -1), true
	}
	if strings.Contains(text, "demà passat") || strings.Contains(text, "dema passat") {
		return baseTime.AddDate(0, 0, 2), true
	}

	// 3. Dies de la setmana en català
	weekdays := map[string]time.Weekday{
		"dilluns":   time.Monday,
		"dimarts":   time.Tuesday,
		"dimecres":  time.Wednesday,
		"dijous":    time.Thursday,
		"divendres": time.Friday,
		"dissabte":  time.Saturday,
		"diumenge":  time.Sunday,
	}

	for word, wd := range weekdays {
		if strings.Contains(text, word) {
			currentWd := baseTime.Weekday()
			diff := int(wd) - int(currentWd)
			if diff <= 0 {
				diff += 7
			}
			return baseTime.AddDate(0, 0, diff), true
		}
	}

	return time.Time{}, false
}

func formatDateStr(dateStr string) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}
	return t.Format("02/01/2006")
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func getWeekdayCatalan(wd time.Weekday) string {
	days := map[time.Weekday]string{
		time.Monday:    "Dilluns",
		time.Tuesday:   "Dimarts",
		time.Wednesday: "Dimecres",
		time.Thursday:  "Dijous",
		time.Friday:    "Divendres",
		time.Saturday:  "Dissabte",
		time.Sunday:    "Diumenge",
	}
	return days[wd]
}

