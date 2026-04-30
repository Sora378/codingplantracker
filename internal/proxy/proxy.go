package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Sora378/codingplantracker/internal/config"
	"github.com/Sora378/codingplantracker/internal/db"
	"github.com/Sora378/codingplantracker/internal/models"
)

const (
	maxProxyRequestBodyBytes  = 8 << 20
	maxProxyResponseBodyBytes = 16 << 20
)

type Proxy struct {
	port      int
	configDir string
	listener  net.Listener
	server    *http.Server
	isRunning bool
}

type ChatCompletionResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Usage   struct {
		TotalTokens      int `json:"total_tokens"`
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func NewProxy(configDir string, port int) *Proxy {
	return &Proxy{
		port:      port,
		configDir: configDir,
	}
}

func (p *Proxy) Start() error {
	if p.isRunning {
		return fmt.Errorf("proxy already running")
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p.port))
	if err != nil {
		return fmt.Errorf("failed to start proxy: %w", err)
	}
	p.listener = listener
	p.isRunning = true

	mux := http.NewServeMux()
	mux.HandleFunc("/", p.handleRequest)

	p.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}

	go p.server.Serve(listener)
	return nil
}

func (p *Proxy) Stop() {
	if p.server != nil {
		p.server.Close()
	}
	p.isRunning = false
}

func (p *Proxy) handleRequest(w http.ResponseWriter, r *http.Request) {
	// Only handle MiniMax API calls
	targetPath := r.URL.Path
	if !strings.HasPrefix(targetPath, "/v1/") {
		http.Error(w, "Proxy only forwards /v1/* requests", http.StatusBadRequest)
		return
	}

	// Read request body
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxProxyRequestBodyBytes))
	if err != nil {
		http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
		return
	}

	// Get API key from config
	cfg, err := config.Load()
	if err != nil {
		cfg = config.DefaultConfig()
	}
	apiKey := cfg.GetAccessToken()
	if apiKey == "" {
		http.Error(w, "Not logged in - no API key", http.StatusUnauthorized)
		return
	}

	// Forward to MiniMax using the configured region and preserving query params.
	targetURL := cfg.APIEndpoint() + targetPath
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}

	for k, values := range r.Header {
		if strings.EqualFold(k, "Authorization") || strings.EqualFold(k, "Host") {
			continue
		}
		for _, value := range values {
			req.Header.Add(k, value)
		}
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Upstream API request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxProxyResponseBodyBytes+1))
	if err != nil {
		http.Error(w, "Failed to read response", http.StatusInternalServerError)
		return
	}
	if int64(len(respBody)) > maxProxyResponseBodyBytes {
		http.Error(w, "Response body too large", http.StatusBadGateway)
		return
	}

	// Parse response to extract token usage
	var chatResp ChatCompletionResponse
	if err := json.Unmarshal(respBody, &chatResp); err == nil {
		if chatResp.Usage.TotalTokens > 0 {
			// Log token usage
			go p.logTokenUsage(chatResp.Usage.PromptTokens, chatResp.Usage.CompletionTokens, chatResp.Usage.TotalTokens)
		}
	}

	// Send response back to client
	for k, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(k, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

func (p *Proxy) logTokenUsage(promptTokens, completionTokens, totalTokens int) {
	database, err := db.New(p.configDir + "/usage.db")
	if err != nil {
		return
	}
	defer database.Close()

	// Get user
	cfg, _ := config.Load()
	userID := "local-user"
	if cfg != nil && cfg.UserID != "" {
		userID = cfg.UserID
	} else if cfg != nil && cfg.Email != "" {
		userID = cfg.Email
	}

	// Log token record
	record := &models.TokenRecord{
		Date:         time.Now(),
		PromptTokens: int64(promptTokens),
		OutputTokens: int64(completionTokens),
		TotalTokens:  int64(totalTokens),
		ModelName:    "MiniMax-M*", // We don't have model info from proxy
		CreatedAt:    time.Now(),
	}

	_ = database.LogTokenUsage(userID, record)
}
