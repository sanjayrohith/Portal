// Package main implements a runnable Stripe webhook receiver server.
//
// Run:
//   go run ./examples/stripe/main.go
// Expose via Portal:
//   portal http 8081 --subdomain stripe-test
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

var stripeEndpointSecret = "whsec_test_secret_12345"

// VerifyStripeSignature validates Stripe-Signature header against the raw body.
// Stripe-Signature format: t=1699900000,v1=5257a869e7ecebeda32affa62cd...
func VerifyStripeSignature(payload []byte, sigHeader, secret string, tolerance time.Duration) bool {
	if sigHeader == "" || secret == "" {
		return false
	}

	var timestampStr string
	var signatures []string

	pairs := strings.Split(sigHeader, ",")
	for _, pair := range pairs {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) == 2 {
			if parts[0] == "t" {
				timestampStr = parts[1]
			} else if parts[0] == "v1" {
				signatures = append(signatures, parts[1])
			}
		}
	}

	if timestampStr == "" || len(signatures) == 0 {
		return false
	}

	ts, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return false
	}

	// Replay attack prevention check
	if tolerance > 0 {
		eventTime := time.Unix(ts, 0)
		if time.Since(eventTime).Abs() > tolerance {
			return false
		}
	}

	signedPayload := fmt.Sprintf("%s.%s", timestampStr, string(payload))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signedPayload))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	for _, sig := range signatures {
		if hmac.Equal([]byte(sig), []byte(expectedSig)) {
			return true
		}
	}

	return false
}

func handleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	payload, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	sig := r.Header.Get("Stripe-Signature")
	if !VerifyStripeSignature(payload, sig, stripeEndpointSecret, 0) {
		http.Error(w, "Invalid Stripe Signature", http.StatusUnauthorized)
		return
	}

	var event map[string]any
	if err := json.Unmarshal(payload, &event); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	eventType, _ := event["type"].(string)
	log.Printf("[Stripe] Successfully verified and received event: %s", eventType)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"event":   eventType,
		"message": "Webhook processed successfully",
	})
}

func main() {
	if envSecret := os.Getenv("STRIPE_WEBHOOK_SECRET"); envSecret != "" {
		stripeEndpointSecret = envSecret
	}
	port := "8081"
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}

	http.HandleFunc("/webhook/stripe", handleStripeWebhook)
	log.Printf("Stripe webhook receiver listening on http://localhost:%s/webhook/stripe", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
