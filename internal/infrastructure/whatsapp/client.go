package whatsapp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ericzapater/familiarassistant/internal/domain"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

// Client embolcalla el client de whatsmeow i implementa domain.MessageSender.
type Client struct {
	client *whatsmeow.Client
}

// NewWhatsAppClient inicialitza el client de whatsmeow, gestiona el login/QR i connecta a WhatsApp.
func NewWhatsAppClient(dbPath string) (*Client, domain.MessageSender, error) {
	if dir := filepath.Dir(dbPath); dir != "." && dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}

	dbLog := waLog.Stdout("Database", "WARN", true)
	ctx := context.Background()
	container, err := sqlstore.New(ctx, "sqlite3", fmt.Sprintf("file:%s?_foreign_keys=on", dbPath), dbLog)
	if err != nil {
		return nil, nil, fmt.Errorf("error inicialitzant sqlstore per a WhatsApp: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("error obtenint dispositiu de WhatsApp store: %w", err)
	}

	clientLog := waLog.Stdout("Client", "INFO", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)

	if client.Store.ID == nil {
		// Nova sessió: Generar QR al terminal
		qrChan, _ := client.GetQRChannel(context.Background())
		err = client.Connect()
		if err != nil {
			return nil, nil, fmt.Errorf("error connectant client de WhatsApp: %w", err)
		}

		for evt := range qrChan {
			if evt.Event == "code" {
				fmt.Println("\n=== ESCANEJA AQUEST CODI QR AMB WHATSAPP PER INICIAR SESSIÓ ===")
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
			} else {
				fmt.Println("Esdeveniment de login de WhatsApp:", evt.Event)
			}
		}
	} else {
		// Sessió existent: connectar directament
		err = client.Connect()
		if err != nil {
			return nil, nil, fmt.Errorf("error connectant client de WhatsApp amb sessió desada: %w", err)
		}
	}

	c := &Client{client: client}
	return c, c, nil
}

// GetClient retorna el client subjacent de whatsmeow per registrar event handlers.
func (c *Client) GetClient() *whatsmeow.Client {
	return c.client
}

// Disconnect desconnecta el client de WhatsApp.
func (c *Client) Disconnect() {
	c.client.Disconnect()
}

// SendText implementa domain.MessageSender enviant un missatge de text al ChatID indicat.
func (c *Client) SendText(ctx context.Context, chatID string, text string) error {
	jid, err := types.ParseJID(chatID)
	if err != nil {
		return fmt.Errorf("JID de WhatsApp no vàlid (%s): %w", chatID, err)
	}

	msg := &waE2E.Message{
		Conversation: proto.String(text),
	}

	_, err = c.client.SendMessage(ctx, jid, msg)
	if err != nil {
		return fmt.Errorf("error enviant missatge a WhatsApp (JID=%s): %w", chatID, err)
	}

	return nil
}
