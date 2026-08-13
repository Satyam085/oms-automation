package main

import (
	"bytes"
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"oms-automtion/config"
	"oms-automtion/models"
)

//go:embed index.html
var indexHTML []byte

// runMu serializes /run requests — the OMS API and rate-limit logic assume
// one job at a time.
var runMu sync.Mutex

// profileGuard tracks failed passcode attempts across configured profiles
// and triggers a lockout after too many wrong tries.
type profileGuard struct {
	mu          sync.Mutex
	profiles    []models.UserProfile
	fails       int
	lockedUntil time.Time
}

const (
	maxPasscodeFails = 10
	lockoutDuration  = 15 * time.Minute
)

func (g *profileGuard) authenticate(provided string) (*models.UserProfile, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if remaining := time.Until(g.lockedUntil); remaining > 0 {
		return nil, fmt.Errorf("too many wrong attempts; locked for %s", remaining.Round(time.Second))
	}

	for i := range g.profiles {
		if subtle.ConstantTimeCompare([]byte(provided), []byte(g.profiles[i].Passcode)) == 1 {
			g.fails = 0
			p := g.profiles[i]
			return &p, nil
		}
	}

	g.fails++
	if g.fails >= maxPasscodeFails {
		g.lockedUntil = time.Now().Add(lockoutDuration)
		g.fails = 0
		return nil, fmt.Errorf("too many wrong attempts; locked for %s", lockoutDuration)
	}
	return nil, fmt.Errorf("incorrect passcode (%d/%d before lockout)", g.fails, maxPasscodeFails)
}

type runResponse struct {
	OK     bool       `json:"ok"`
	Error  string     `json:"error,omitempty"`
	Logs   string     `json:"logs,omitempty"`
	Result *RunResult `json:"result,omitempty"`
}

func runServer() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	profiles := config.LoadProfiles()
	if len(profiles) == 0 {
		log.Fatal("No user profiles configured. Please set PASSCODE and OMS credentials in environment variables.")
	}

	for i, p := range profiles {
		if !isSixDigits(p.Passcode) {
			log.Fatalf("Profile %d (EmpNo %s) passcode must be exactly 6 numeric digits (got %q)", i+1, p.EmpNo, p.Passcode)
		}
		log.Printf("Loaded user profile %d: EmpNo=%s, Company=%s", i+1, p.EmpNo, p.CompanyName)
	}

	guard := &profileGuard{profiles: profiles}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/run", makeRunHandler(guard))

	addr := ":" + port
	log.Printf("OMS automation server listening on %s with %d user profile(s)", addr, len(profiles))

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      30 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func isSixDigits(s string) bool {
	if len(s) != 6 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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

func makeRunHandler(guard *profileGuard) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		provided := r.Header.Get("X-Passcode")
		profile, err := guard.authenticate(provided)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, runResponse{
				OK:    false,
				Error: err.Error(),
			})
			return
		}

		limit := 0
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				limit = n
			}
		}

		rulesParam := r.URL.Query().Get("rules")
		var enabledReasonIDs map[int]bool
		if rulesParam != "" {
			enabledReasonIDs = make(map[int]bool)
			for _, part := range strings.Split(rulesParam, ",") {
				part = strings.TrimSpace(part)
				if id, err := strconv.Atoi(part); err == nil {
					enabledReasonIDs[id] = true
				}
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
		result, err := RunAutomation(*profile, limit, enabledReasonIDs, &buf)

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
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
