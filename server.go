package main

import (
	"bytes"
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed index.html
var indexHTML []byte

// runMu serializes /run requests — the OMS API and rate-limit logic assume
// one job at a time.
var runMu sync.Mutex

type runResponse struct {
	OK     bool       `json:"ok"`
	Error  string     `json:"error,omitempty"`
	Logs   string     `json:"logs"`
	Result *RunResult `json:"result,omitempty"`
}

func runServer() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	token := os.Getenv("AUTH_TOKEN")
	if token == "" {
		log.Fatal("AUTH_TOKEN env var is required in server mode")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/run", requireAuth(token, handleRun))

	addr := ":" + port
	log.Printf("OMS automation server listening on %s", addr)

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		// /run can take minutes (rate-limited loop). Generous timeouts.
		ReadTimeout:  5 * time.Minute,
		WriteTimeout: 30 * time.Minute,
		IdleTimeout:  2 * time.Minute,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(indexHTML)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

func requireAuth(token string, next http.HandlerFunc) http.HandlerFunc {
	expected := []byte(token)
	return func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), expected) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			limit = n
		}
	}

	if !runMu.TryLock() {
		writeJSON(w, http.StatusConflict, runResponse{
			OK:    false,
			Error: "another run is already in progress",
		})
		return
	}
	defer runMu.Unlock()

	var buf bytes.Buffer
	result, err := RunAutomation(limit, &buf)

	resp := runResponse{
		OK:     err == nil,
		Logs:   buf.String(),
		Result: result,
	}
	if err != nil {
		resp.Error = err.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
