package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ericzapater/familiarassistant/internal/config"
	"github.com/ericzapater/familiarassistant/internal/infrastructure/database"
	googleinfra "github.com/ericzapater/familiarassistant/internal/infrastructure/google"
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

	// 5. Inicialitzar el servei d'Orquestració (Lògica pura de Domini)
	orchSvc := orchestrator.NewService(
		postgresRepo,
		postgresRepo,
		calendarSvc,
		geminiSvc,
		cfg.Timezone,
	)
	log.Println("✓ Servei d'Orquestració de domini creat")

	// 6. Inicialitzar el client i adaptador de WhatsApp (whatsmeow)
	waClient, msgSender, err := wainfra.NewWhatsAppClient("whatsapp_session.db")
	if err != nil {
		log.Fatalf("Fatal: Error inicialitzant WhatsApp: %v", err)
	}
	defer waClient.Disconnect()
	log.Println("✓ Client de WhatsApp connectat")

	// 7. Configurar el Listener de transport de WhatsApp amb el Guardarraïl de Privacitat
	listener := walistener.NewListener(cfg.WhatsAppGroupID, cfg.WhatsAppMyID, orchSvc, msgSender)
	waClient.GetClient().AddEventHandler(listener.HandleEvent)
	log.Printf("✓ Listener de WhatsApp activat per al grup: %s", cfg.WhatsAppGroupID)

	log.Println("🟢 Assistent Familiar actiu i escoltant peticions /nutri i /calendar...")

	// 8. Capturar senyals del sistema per a un tancament ordenat (Graceful Shutdown)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("\n--- Aturant l'Assistent Familiar de manera ordenada... ---")
}
