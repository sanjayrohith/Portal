// Package main implements a runnable GitHub webhook receiver server.
//
// Run:
//   go run ./examples/github/main.go
// Expose via Portal:
//   portal http 8082 --subdomain github-test
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

var githubWebhookSecret = "ghsec_test_secret_67890"

// VerifyGitHubSignature validates X-Hub-Signature-256 header (format: sha256=<hex>).
func VerifyGitHubSignature(payload []byte, sigHeader, secret string) bool {
	if sigHeader == "" || secret == "" {
		return false
	}

	if !strings.HasPrefix(sigHeader, "sha256=") {
		return false
	}
	actualSig := strings.TrimPrefix(sigHeader, "sha256=")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(actualSig), []byte(expectedSig))
}

func handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	payload, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	sig := r.Header.Get("X-Hub-Signature-256")
	if !VerifyGitHubSignature(payload, sig, githubWebhookSecret) {
		http.Error(w, "Invalid X-Hub-Signature-256", http.StatusUnauthorized)
		return
	}

	event := r.Header.Get("X-GitHub-Event")
	delivery := r.Header.Get("X-GitHub-Delivery")

	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	action, _ := data["action"].(string)
	log.Printf("[GitHub] Received verified event=%s action=%s delivery=%s", event, action, delivery)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":   "received",
		"event":    event,
		"delivery": delivery,
	})
}

func main() {
	if envSecret := os.Getenv("GITHUB_WEBHOOK_SECRET"); envSecret != "" {
		githubWebhookSecret = envSecret
	}
	port := "8082"
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}

	http.HandleFunc("/webhook/github", handleGitHubWebhook)
	log.Printf("GitHub webhook receiver listening on http://localhost:%s/webhook/github", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
