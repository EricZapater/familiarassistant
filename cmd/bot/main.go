package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ericzapater/familiarassistant/internal/config"
	"github.com/ericzapater/familiarassistant/internal/infrastructure/database"
	"github.com/ericzapater/familiarassistant/internal/infrastructure/bondia"
	googleinfra "github.com/ericzapater/familiarassistant/internal/infrastructure/google"
	tpinfra "github.com/ericzapater/familiarassistant/internal/infrastructure/trainingpeaks"
	wainfra "github.com/ericzapater/familiarassistant/internal/infrastructure/whatsapp"
	"github.com/ericzapater/familiarassistant/internal/orchestrator"
	walistener "github.com/ericzapater/familiarassistant/internal/transport/whatsapp"
)

func main() {
	log.Println("--- Arrancant l'Assistent Familiar de WhatsApp ---")

	// 1. Carregar la configuració des de les variables d'entorn (.env)
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Fatal: Error de configuració: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 2. Inicialitzar l'adaptador de PostgreSQL (Cache + Pauta Nutricional)
	postgresRepo, err := database.NewPostgresRepository(cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("Fatal: No s'ha pogut connectar a PostgreSQL: %v", err)
	}
	defer postgresRepo.Close()
	log.Println("✓ Connexió a PostgreSQL establerta correctament")

	// 3. Inicialitzar l'adaptador de Google Calendar API
	calendarSvc, err := googleinfra.NewCalendarService(ctx, cfg.GoogleCredentialsFile, cfg.GoogleCalendarID, cfg.Timezone)
	if err != nil {
		log.Fatalf("Fatal: No s'ha pogut inicialitzar Google Calendar: %v", err)
	}
	log.Println("✓ Client de Google Calendar v3 inicialitzat")

	// 4. Inicialitzar l'adaptador de Gemini (SDK oficial google.golang.org/genai)
	geminiSvc, err := googleinfra.NewGeminiService(ctx, cfg.GeminiAPIKey, cfg.GeminiModel)
	if err != nil {
		log.Fatalf("Fatal: No s'ha pogut inicialitzar Gemini AI Service: %v", err)
	}
	log.Printf("✓ Client de Google Gemini inicialitzat (Model: %s)", cfg.GeminiModel)

	// 5. Inicialitzar el connector d'infraestructura de TrainingPeaks MCP
	tpSvc := tpinfra.NewService()
	log.Printf("✓ Connector de TrainingPeaks MCP carregat amb %d usuaris registrats", len(cfg.TPUsersMap))

	// 5b. Inicialitzar el connector de notícies /bondia
	bondiaSvc := bondia.NewService()
	log.Println("✓ Connector de Notícies BonDia inicialitzat")

	// 6. Inicialitzar el servei d'Orquestració (Lògica pura de Domini)
	orchSvc := orchestrator.NewService(
		postgresRepo,
		postgresRepo,
		calendarSvc,
		geminiSvc,
		tpSvc,
		bondiaSvc,
		cfg.TPUsersMap,
		cfg.Timezone,
	)
	log.Println("✓ Servei d'Orquestració de domini creat")

	// 7. Inicialitzar el client i adaptador de WhatsApp (whatsmeow)
	waClient, msgSender, err := wainfra.NewWhatsAppClient(cfg.WhatsAppDBPath)
	if err != nil {
		log.Fatalf("Fatal: Error inicialitzant WhatsApp: %v", err)
	}
	defer waClient.Disconnect()
	log.Println("✓ Client de WhatsApp connectat")

	// 8. Configurar el Listener de transport de WhatsApp amb el Guardarraïl de Privacitat
	listener := walistener.NewListener(cfg.WhatsAppGroupID, cfg.WhatsAppMyID, orchSvc, msgSender, cfg.TPUsersMap)
	waClient.GetClient().AddEventHandler(listener.HandleEvent)
	log.Printf("✓ Listener de WhatsApp activat per al grup: %s", cfg.WhatsAppGroupID)

	log.Println("🟢 Assistent Familiar actiu i escoltant peticions /nutri, /calendar i /training...")


	// 8. Capturar senyals del sistema per a un tancament ordenat (Graceful Shutdown)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("\n--- Aturant l'Assistent Familiar de manera ordenada... ---")
}
