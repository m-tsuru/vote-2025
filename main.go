package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	editors   = []string{"vi", "vim", "nano", "emacs", "vscode", "sublime", "atom", "neovim", "ed", "sakura", "hidemaru", "emeditor", "other"}
	votes     []int
	mu        sync.Mutex
	votesFile = "votes.json"
)

func init() {
	votes = make([]int, len(editors))
}

func loadVotes() {
	mu.Lock()
	defer mu.Unlock()
	data, err := os.ReadFile(votesFile)
	if err != nil {

		return
	}
	var loaded map[string]int
	if err := json.Unmarshal(data, &loaded); err == nil {
		for i, editor := range editors {
			if v, ok := loaded[editor]; ok {
				votes[i] = v
			}
		}
	}
}

type VoteRequest struct {
	Editor    string `json:"editor"`
	Turnstile string `json:"turnstile"`
}

func verifyTurnstile(token string, remoteip string) bool {
	secret := os.Getenv("TURNSTILE_SECRET")
	if secret == "" || token == "" {
		return false
	}
	client := &http.Client{Timeout: 5 * time.Second}
	reqBody := "secret=" + secret + "&response=" + token
	if remoteip != "" {
		reqBody += "&remoteip=" + remoteip
	}
	req, err := http.NewRequest("POST", "https://challenges.cloudflare.com/turnstile/v0/siteverify", io.NopCloser(strings.NewReader(reqBody)))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var result struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false
	}
	return result.Success
}

type VoteResponse struct {
	Editors []string `json:"editors"`
	Votes   []int    `json:"votes"`
	Total   int      `json:"total"`
}

func getVotesHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	total := 0
	for _, v := range votes {
		total += v
	}
	resp := VoteResponse{
		Editors: editors,
		Votes:   votes,
		Total:   total,
	}
	mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func postVoteHandler(w http.ResponseWriter, r *http.Request) {

	if cookie, err := r.Cookie("voted"); err == nil && cookie.Value == "true" {
		http.Error(w, "already voted", http.StatusForbidden)
		return
	}

	var req VoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if !verifyTurnstile(req.Turnstile, r.RemoteAddr) {
		http.Error(w, "invalid turnstile token", http.StatusForbidden)
		return
	}
	mu.Lock()
	idx := -1
	for i, e := range editors {
		if e == req.Editor {
			idx = i
			break
		}
	}
	if idx == -1 {
		mu.Unlock()
		http.Error(w, "unknown editor", http.StatusBadRequest)
		return
	}
	votes[idx]++

	m := make(map[string]int)
	for i, editor := range editors {
		m[editor] = votes[i]
	}
	data, err := json.Marshal(m)
	if err == nil {
		_ = os.WriteFile(votesFile, data, 0644)
	}
	total := 0
	for _, v := range votes {
		total += v
	}
	resp := VoteResponse{
		Editors: editors,
		Votes:   votes,
		Total:   total,
	}
	mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     "voted",
		Value:    "true",
		Path:     "/",
		MaxAge:   60 * 60 * 24 * 365,
		HttpOnly: false,
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func main() {

	loadVotes()

	http.HandleFunc("/api/votes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			getVotesHandler(w, r)
		} else if r.Method == http.MethodPost {
			postVoteHandler(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	fs := http.FileServer(http.Dir("."))
	http.Handle("/", fs)

	log.Println("Listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
