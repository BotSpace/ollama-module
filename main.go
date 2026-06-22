// ollama-module-go — Botmother tashqi modul: Ollama LLM.
//
// Node turi:
//   - ollama.Chat — action: Ollama orqali LLM'ga so'rov yuboradi,
//     javobni llm_output state'iga yozadi.
//
// Ollama = lokal/self-hosted LLM server (https://ollama.com).
// Credential: global "ollama" (mode: bearer). base_url + ixtiyoriy token.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	botmodule "github.com/BotSpace/botmodule-go"
)

const (
	moduleID       = "ollama"
	defaultBaseURL = "http://localhost:11434"
)

var httpClient = &http.Client{Timeout: 120 * time.Second}

func main() {
	m := botmodule.New(moduleID, "Ollama")
	m.Version = "0.1.0"
	m.Docs = docs

	// Credential turini modul e'lon QILMAYDI — platformada global "ollama"
	// credential bor (type_key: ollama). Foydalanuvchi shuni tanlaydi.
	m.AddNode(botmodule.Node{
		Type:        "ollama.Chat",
		Title:       "Ollama Chat",
		Description: "Ollama orqali LLM'ga so'rov yuboradi va javobni qaytaradi",
		Category:    "ai",
		Icon:        "brain-circuit",
		Color:       "ai-violet",
		Width:       200,
		Content: []botmodule.Field{
			{
				Type:           "credential",
				Key:            "api_credential",
				Label:          "Ollama credential",
				CredentialType: "ollama",
				HelpText:       "Global Ollama credential (base_url + token)",
			},
			{
				Type:        "text",
				Key:         "model",
				Label:       "Model",
				Placeholder: "llama3.2",
				HelpText:    "Ollama'da yuklangan model nomi, masalan llama3.2, qwen2.5, gemma3",
			},
			{
				Type:        "textarea",
				Key:         "system",
				Label:       "System prompt",
				Placeholder: "Sen foydali yordamchisan.",
				HelpText:    "Ixtiyoriy — modelning rolini belgilaydi",
				Optional:    true,
			},
			{
				Type:        "textarea",
				Key:         "prompt",
				Label:       "Foydalanuvchi xabari",
				Placeholder: "{{message.text}}",
				HelpText:    "Modelga yuboriladigan matn",
			},
			{
				Type:        "number",
				Key:         "temperature",
				Label:       "Temperature",
				Placeholder: "0.7",
				HelpText:    "0–2 oralig'ida; bo'sh = model defaulti",
				Optional:    true,
			},
		},
		Defaults: map[string]any{
			"model":  "llama3.2",
			"prompt": "{{message.text}}",
		},
		ProducesState: []string{"llm_output", "llm_model", "llm_tokens", "llm_error"},
		Execute:       executeChat,
	})

	m.Serve(":8100")
}

func executeChat(c *botmodule.ExecuteCtx) botmodule.Result {
	baseURL := defaultBaseURL
	var token string
	if cred, ok := c.Credential("api_credential"); ok {
		if v := strings.TrimSpace(cred.Data["base_url"]); v != "" {
			baseURL = v
		}
		token = cred.Data["token"]
		if token == "" {
			token = cred.Data["api_key"]
		}
	}
	baseURL = strings.TrimRight(baseURL, "/")

	model := c.String("model")
	if model == "" {
		model = "llama3.2"
	}
	prompt := c.String("prompt")
	if prompt == "" {
		return errResult("prompt bo'sh")
	}

	messages := []map[string]string{}
	if sys := c.String("system"); sys != "" {
		messages = append(messages, map[string]string{"role": "system", "content": sys})
	}
	messages = append(messages, map[string]string{"role": "user", "content": prompt})

	payload := map[string]any{"model": model, "messages": messages, "stream": false}
	if temp, hasTemp := c.Data["temperature"]; hasTemp && temp != "" && temp != nil {
		payload["options"] = map[string]any{"temperature": c.Int("temperature")} // ponytail: number field beradi int; float kerak bo'lsa SDK'ga float helper qo'shilsin
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return errResult("so'rov qurilmadi: " + err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return errResult("Ollama so'rovi muvaffaqiyatsiz: " + err.Error())
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return errResult(fmt.Sprintf("Ollama %d: %s", resp.StatusCode, truncate(string(raw), 300)))
	}

	var out struct {
		Model           string `json:"model"`
		Message         struct {
			Content string `json:"content"`
		} `json:"message"`
		PromptEvalCount int    `json:"prompt_eval_count"`
		EvalCount       int    `json:"eval_count"`
		Error           string `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return errResult("javob parse bo'lmadi: " + err.Error())
	}
	if out.Error != "" {
		return errResult("Ollama: " + out.Error)
	}
	if out.Message.Content == "" {
		return errResult("javob bo'sh")
	}

	return botmodule.Result{
		ContextUpdates: map[string]any{
			"llm_output": out.Message.Content,
			"llm_model":  out.Model,
			"llm_tokens": out.PromptEvalCount + out.EvalCount,
			"llm_error":  "",
		},
		ExitOutput: "success",
	}
}

func errResult(msg string) botmodule.Result {
	return botmodule.Result{
		ContextUpdates: map[string]any{
			"llm_output": "",
			"llm_error":  msg,
		},
		ExitOutput: "error",
		Error:      msg, // UI error list + alert'da ko'rsatiladi
	}
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

const docs = `# Ollama

[Ollama](https://ollama.com) orqali lokal/self-hosted LLM modellariga murojaat
qiladi (llama3.2, qwen2.5, gemma3, mistral va boshqalar).

## Node turi

### ` + "`ollama.Chat`" + ` (action, AI)

Tanlangan modelga chat so'rovi yuboradi va javobni state'ga yozadi.

| Field | Tavsif |
|---|---|
| **api_credential** | Global Ollama credential (base_url + ixtiyoriy bearer token) |
| **model** | Ollama'da yuklangan model, masalan ` + "`llama3.2`" + `, ` + "`qwen2.5`" + ` |
| **system** | System prompt (ixtiyoriy) |
| **prompt** | Foydalanuvchi xabari, masalan ` + "`{{message.text}}`" + ` |
| **temperature** | 0–2 (ixtiyoriy) |

**Chiqish state'lari:**

- ` + "`llm_output`" + ` — model javobi
- ` + "`llm_model`" + ` — ishlatilgan model
- ` + "`llm_tokens`" + ` — sarflangan token soni (prompt + eval)
- ` + "`llm_error`" + ` — xato matni (muvaffaqiyatda bo'sh)

**Chiqish edge'lari:** ` + "`success`" + ` / ` + "`error`" + `

## Misol flow

` + "```" + `
Xabar kelganda (trigger)
  → Ollama Chat (prompt: {{message.text}})
  → Matn yuborish ({{llm_output}})
` + "```" + `
`
