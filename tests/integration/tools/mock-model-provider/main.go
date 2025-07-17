package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
)

// Fake model list response
type Model struct {
	ID       string            `json:"id"`
	Object   string            `json:"object"`
	Created  int               `json:"created"`
	OwnedBy  string            `json:"owned_by"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type ModelsResponse struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

// Fake chat completion request/response
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
}

type ChatChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type ChatResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []ChatChoice `json:"choices"`
}

// Handler for /v1/models
func handleModels(w http.ResponseWriter, _ *http.Request) {
	models := ModelsResponse{
		Object: "list",
		Data: []Model{
			{
				ID:      "gpt-4o",
				Object:  "model",
				Created: 1687610602,
				OwnedBy: "openai",
				Metadata: map[string]string{
					"usage": "llm",
				},
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(models); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// Handler for /v1/chat/completions
func handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	resp := ChatResponse{
		ID:      "chatcmpl-fakeid123",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   req.Model,
		Choices: []ChatChoice{
			{
				Index: 0,
				Message: ChatMessage{
					Role:    "assistant",
					Content: "This is a fake response for testing.",
				},
				FinishReason: "stop",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "validate" {
		return
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/v1/models", handleModels)
	http.HandleFunc("/v1/chat/completions", handleChatCompletions)
	http.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("http://127.0.0.1:" + port)); err != nil {
			http.Error(w, "Failed to write response", http.StatusInternalServerError)
			return
		}
	})
	log.Println("Fake OpenAI proxy server listening on :" + port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
