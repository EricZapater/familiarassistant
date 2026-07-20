package google

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/ericzapater/familiarassistant/internal/domain"
	"google.golang.org/genai"
)

// GeminiService implementa domain.AIService utilitzant el nou SDK oficial de Google Gen AI (`google.golang.org/genai`).
type GeminiService struct {
	client *genai.Client
	model  string
}

// NewGeminiService crea un nou client d'IA d'acord amb les especificacions oficials.
func NewGeminiService(ctx context.Context, apiKey string, model string) (*GeminiService, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("error inicialitzant client de Gemini: %w", err)
	}

	if strings.TrimSpace(model) == "" {
		model = "gemini-2.5-flash"
	}
	model = strings.TrimPrefix(strings.TrimSpace(model), "models/")

	return &GeminiService{
		client: client,
		model:  model,
	}, nil
}

// Chat executa la petició a Gemini 2.0 Flash gestionant el cicle de vida de Function Calling.
func (g *GeminiService) Chat(ctx context.Context, query domain.Query, tools domain.ToolProvider) (string, error) {
	nowStr := query.Timestamp.Format("2006-01-02 15:04 (Monday)")
	systemPrompt := fmt.Sprintf(`Ets un assistent familiar atent, clar i amable que respon sempre en català.
La data i hora actual de la petició és: %s.

Tens accés a tres eines (tools):
1. ConsultarPauta: permet veure el menú/pauta nutricional per a un dia determinat (ex: 'dilluns', 'dimarts', ..., 'diumenge' o 'tots').
2. ConsultarCalendari: permet consultar els esdeveniments del calendari familiar ('today' o 'week').
3. CrearEsdeveniment: permet afegir o agendar un nou esdeveniment al calendari familiar de Google.

Quan l'usuari demani afegir, crear, anotar o agendar un esdeveniment, calcula la data i hora d'inici en format ISO 8601 basant-te en la data actual (%s) i crida 'CrearEsdeveniment'.
Quan l'usuari pregunti sobre l'agenda, compromisos o quins esdeveniments hi ha, crida 'ConsultarCalendari'.

Respon de manera natural, utilitzant emojis apropiats, i confirma clarament quan un esdeveniment s'hagi creat correctament.`, nowStr, nowStr)

	// Definició de les eines (Tools) per a Function Calling
	toolsConfig := []*genai.Tool{
		{
			FunctionDeclarations: []*genai.FunctionDeclaration{
				{
					Name:        "ConsultarPauta",
					Description: "Consulta el menú o la pauta nutricional diària de la família per a un dia de la setmana específic o per a tota la setmana.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"dia_setmana": {
								Type:        genai.TypeString,
								Description: "El dia de la setmana a consultar en català (ex: 'dilluns', 'dimarts', 'dimecres', 'dijous', 'divendres', 'dissabte', 'diumenge', 'tots').",
							},
						},
						Required: []string{"dia_setmana"},
					},
				},
				{
					Name:        "ConsultarCalendari",
					Description: "Consulta l'agenda i els esdeveniments del calendari familiar de Google.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"periode": {
								Type:        genai.TypeString,
								Description: "El període a consultar: 'today' per als esdeveniments d'avui, o 'week' per als esdeveniments de tota la setmana.",
							},
						},
						Required: []string{"periode"},
					},
				},
				{
					Name:        "CrearEsdeveniment",
					Description: "Afegeix o crea un nou esdeveniment al calendari familiar de Google.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"titol": {
								Type:        genai.TypeString,
								Description: "El títol o nom de l'esdeveniment (ex: 'Cita metge', 'Sopar de feina').",
							},
							"data_inici": {
								Type:        genai.TypeString,
								Description: "La data i hora d'inici en format ISO 8601 amb la zona horària (ex: '2026-07-25T10:00:00+02:00').",
							},
							"durada_minuts": {
								Type:        genai.TypeInteger,
								Description: "La durada de l'esdeveniment en minuts (per defecte 60).",
							},
							"lloc": {
								Type:        genai.TypeString,
								Description: "El lloc o ubicació de l'esdeveniment (opcional).",
							},
							"descripcio": {
								Type:        genai.TypeString,
								Description: "Descripció o detalls addicionals (opcional).",
							},
						},
						Required: []string{"titol", "data_inici"},
					},
				},
			},
		},
	}

	config := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{{Text: systemPrompt}},
		},
		Tools:       toolsConfig,
		Temperature: genai.Ptr(float32(0.3)),
	}

	// Historial de conversa per gestionar les interaccions del Function Calling
	contents := []*genai.Content{
		{
			Role: "user",
			Parts: []*genai.Part{
				{Text: fmt.Sprintf("Comandament: %s. Pregunta: %s", query.Command, query.RawText)},
			},
		},
	}

	// Cicle de Function Calling (màxim 5 iteracions per seguretat)
	for i := 0; i < 5; i++ {
		resp, err := g.client.Models.GenerateContent(ctx, g.model, contents, config)
		if err != nil {
			// Si el model configurat ha expirat o no està disponible, intentem un fallback automàtic
			if strings.Contains(err.Error(), "no longer available") || strings.Contains(err.Error(), "NOT_FOUND") || strings.Contains(err.Error(), "404") {
				fallbackModel := "gemini-2.5-flash"
				if g.model == "gemini-2.5-flash" {
					fallbackModel = "gemini-1.5-flash"
				}
				log.Printf("[GeminiService] El model '%s' no està disponible. Provant fallback a '%s'...", g.model, fallbackModel)
				g.model = fallbackModel
				resp, err = g.client.Models.GenerateContent(ctx, g.model, contents, config)
			}
			if err != nil {
				return "", fmt.Errorf("error cridant Gemini (%s): %w", g.model, err)
			}
		}

		if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
			return "", fmt.Errorf("Gemini ha retornat una resposta buida")
		}

		candidateContent := resp.Candidates[0].Content
		contents = append(contents, candidateContent)

		// Comprovem si el model requereix cridar una funció (FunctionCall)
		funcCalls := extractFunctionCalls(candidateContent)
		if len(funcCalls) == 0 {
			// El model ha generat la resposta de text final
			text := extractTextResponse(candidateContent)
			if text != "" {
				return text, nil
			}
			return "No s'ha pogut obtenir una resposta de text del model.", nil
		}

		// Processar totes les crides a funcions sol·licitades pel model
		var responseParts []*genai.Part
		for _, fc := range funcCalls {
			fnResult, err := tools.ExecuteFunction(ctx, fc.Name, fc.Args)
			if err != nil {
				fnResult = map[string]string{"error": err.Error()}
			}

			// Embolcallem el resultat en una clau "result" per garantir que arrays/slices es serialitzen correctament a JSON per a Gemini
			respMap := map[string]any{
				"result": fnResult,
			}

			log.Printf("[GeminiService] Resposta enviada a Gemini per a FunctionCall '%s': %+v", fc.Name, respMap)

			responseParts = append(responseParts, &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     fc.Name,
					Response: respMap,
				},
			})
		}

		// Afegir les respostes de les funcions a l'historial per a la següent iteració amb el model
		contents = append(contents, &genai.Content{
			Role:  "user",
			Parts: responseParts,
		})
	}

	return "", fmt.Errorf("s'ha superat el nombre màxim d'iteracions de Function Calling")
}

type parsedFunctionCall struct {
	Name string
	Args map[string]any
}

func extractFunctionCalls(content *genai.Content) []parsedFunctionCall {
	var calls []parsedFunctionCall
	for _, part := range content.Parts {
		if part.FunctionCall != nil {
			calls = append(calls, parsedFunctionCall{
				Name: part.FunctionCall.Name,
				Args: part.FunctionCall.Args,
			})
		}
	}
	return calls
}

func extractTextResponse(content *genai.Content) string {
	var sb strings.Builder
	for _, part := range content.Parts {
		if part.Text != "" {
			sb.WriteString(part.Text)
		}
	}
	return strings.TrimSpace(sb.String())
}
