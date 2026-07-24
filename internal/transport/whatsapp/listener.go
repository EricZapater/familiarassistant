package whatsapp

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ericzapater/familiarassistant/internal/config"
	"github.com/ericzapater/familiarassistant/internal/domain"
	"github.com/ericzapater/familiarassistant/internal/orchestrator"
	"go.mau.fi/whatsmeow/types/events"
)

// Listener escolta esdeveniments de WhatsApp, valida la privacitat i canalitza les peticions cap a l'orquestrador.
type Listener struct {
	allowedGroupID string
	allowedMyID    string
	orchestrator   *orchestrator.Service
	sender         domain.MessageSender
	tpUsersMap     map[string]config.UserTPConfig
}

// NewListener crea un nou Listener de WhatsApp.
func NewListener(allowedGroupID, allowedMyID string, orchestrator *orchestrator.Service, sender domain.MessageSender, tpUsersMap map[string]config.UserTPConfig) *Listener {
	return &Listener{
		allowedGroupID: strings.TrimSpace(allowedGroupID),
		allowedMyID:    strings.TrimSpace(allowedMyID),
		orchestrator:   orchestrator,
		sender:         sender,
		tpUsersMap:     tpUsersMap,
	}
}

// HandleEvent és la funció de callback registrada a whatsmeow via AddEventHandler.
func (l *Listener) HandleEvent(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		l.handleMessage(v)
	}
}

func (l *Listener) handleMessage(msg *events.Message) {
	chatID := msg.Info.Chat.String()
	log.Printf("[WhatsApp Listener] Rebut missatge del xat ID: %s (de: %s)", chatID, msg.Info.Sender.User)

	// Guardarraïl de Privacitat:
	// - Si és un grup (g.us), només permetem el grup familiar configurat.
	// - Si és un xat privat, només permetem missatges del propi usuari autoritzat (allowedMyID).
	isGroup := msg.Info.Chat.Server == "g.us"
	if isGroup && chatID != l.allowedGroupID {
		// Silent drop per a grups no autoritzats
		return
	}
	if !isGroup && msg.Info.Sender.User != l.allowedMyID {
		// Silent drop per a missatges privats de tercers 
		return
	}

	// Extreure el text net del missatge
	rawText := extractTextMessage(msg)
	rawText = strings.TrimSpace(rawText)
	if rawText == "" {
		return
	}

	// Identificar comandaments `/nutri`, `/calendar`, `/training` o `/flushcache`
	var cmd domain.CommandType
	var cleanQuestion string

	switch {
	case strings.HasPrefix(strings.ToLower(rawText), "/nutri"):
		cmd = domain.CmdNutri
		cleanQuestion = strings.TrimSpace(rawText[len("/nutri"):])
	case strings.HasPrefix(strings.ToLower(rawText), "/calendar"):
		cmd = domain.CmdCalendar
		cleanQuestion = strings.TrimSpace(rawText[len("/calendar"):])
	case strings.HasPrefix(strings.ToLower(rawText), "/mister"):
		cmd = domain.CmdMister
		cleanQuestion = strings.TrimSpace(rawText[len("/mister"):])
	case strings.HasPrefix(strings.ToLower(rawText), "/bondia"):
		cmd = domain.CmdBonDia
		cleanQuestion = strings.TrimSpace(rawText[len("/bondia"):])
	case strings.HasPrefix(strings.ToLower(rawText), "/flushcache"):
		cmd = domain.CmdFlushCache
		cleanQuestion = ""
	default:
		// No és un comandament reconegut per al bot, ignorar-ho silenciosament
		return
	}

	senderPhone := strings.TrimPrefix(strings.TrimSpace(msg.Info.Sender.User), "+")
	var tpUserCfg config.UserTPConfig

	// Flux de Seguretat per al comandament /mister:
	// Si el telèfon NO està registrat al JSON de TP_USERS_CONFIG, realitza un 'return' silenciós.
	if cmd == domain.CmdMister {
		cfg, registered := l.tpUsersMap[senderPhone]
		if !registered {
			log.Printf("[WhatsApp Listener Security Filter] Telèfon %s no registrat a TP_USERS_CONFIG. Return silenciós.", senderPhone)
			return
		}
		tpUserCfg = cfg
	}

	log.Printf("[WhatsApp Listener] Rebut comandament '/%s' de %s (%s) al grup %s. Pregunta: '%s'", cmd, senderPhone, tpUserCfg.Name, chatID, cleanQuestion)

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	query := domain.Query{
		Command:     cmd,
		RawText:     cleanQuestion,
		Timestamp:   msg.Info.Timestamp,
		SenderPhone: senderPhone,
		UserName:    tpUserCfg.Name,
		TPUsername:  tpUserCfg.Username,
		TPPassword:  tpUserCfg.Password,
	}

	// Invocació de l'orquestrador de domini
	respText, err := l.orchestrator.ProcessQuery(ctx, query)
	if err != nil {
		log.Printf("[WhatsApp Listener] Error al processar la petició de WhatsApp: %v", err)
		// Gestió d'errors amigable per a l'usuari (sense panics)
		userErrorMsg := fmt.Sprintf("⚠️ Ho sento @%s, hi ha hagut un error al processar la teva petició. Torna-ho a provar d'aquí a uns moments.", msg.Info.Sender.User)
		_ = l.sender.SendText(context.Background(), chatID, userErrorMsg)
		return
	}

	// Enviar resposta al grup de WhatsApp
	err = l.sender.SendText(context.Background(), chatID, respText)
	if err != nil {
		log.Printf("[WhatsApp Listener] Error enviant la resposta a WhatsApp: %v", err)
	}
}

func extractTextMessage(msg *events.Message) string {
	if msg.Message == nil {
		return ""
	}
	if conversation := msg.Message.GetConversation(); conversation != "" {
		return conversation
	}
	if extended := msg.Message.GetExtendedTextMessage(); extended != nil {
		return extended.GetText()
	}
	return ""
}
