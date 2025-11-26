// Package main implements a runnable Slack events and slash command webhook receiver.
//
// Run:
//   go run ./examples/slack/main.go
// Expose via Portal:
//   portal http 8083 --subdomain slack-test
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var slackSigningSecret = "slack_signing_secret_998877"

// VerifySlackSignature validates Slack's X-Slack-Signature against X-Slack-Request-Timestamp and raw body.
// Format: v0=a2114d57b48eac39b9ad1ea9e66c46437979ac3a87d5603f99760d2435fe3b73
func VerifySlackSignature(payload []byte, timestampHeader, sigHeader, secret string, tolerance time.Duration) bool {
	if timestampHeader == "" || sigHeader == "" || secret == "" {
		return false
	}

	ts, err := strconv.ParseInt(timestampHeader, 10, 64)
	if err != nil {
		return false
	}

	if tolerance > 0 {
		reqTime := time.Unix(ts, 0)
		if time.Since(reqTime).Abs() > tolerance {
			return false
		}
	}

	if !strings.HasPrefix(sigHeader, "v0=") {
		return false
	}

	sigBase := fmt.Sprintf("v0:%s:%s", timestampHeader, string(payload))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(sigBase))
	expectedSig := "v0=" + hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(sigHeader), []byte(expectedSig))
}

func handleSlackWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	payload, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	ts := r.Header.Get("X-Slack-Request-Timestamp")
	sig := r.Header.Get("X-Slack-Signature")

	if !VerifySlackSignature(payload, ts, sig, slackSigningSecret, 0) {
		http.Error(w, "Invalid Slack Signature", http.StatusUnauthorized)
		return
	}

	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Handle Slack url_verification challenge
	if body["type"] == "url_verification" {
		challenge, _ := body["challenge"].(string)
		log.Println("[Slack] Handled url_verification challenge")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"challenge": challenge})
		return
	}

	log.Printf("[Slack] Handled event callback: %v", body["type"])
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func main() {
	if envSecret := os.Getenv("SLACK_SIGNING_SECRET"); envSecret != "" {
		slackSigningSecret = envSecret
	}
	port := "8083"
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}

	http.HandleFunc("/webhook/slack", handleSlackWebhook)
	log.Printf("Slack webhook receiver listening on http://localhost:%s/webhook/slack", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
