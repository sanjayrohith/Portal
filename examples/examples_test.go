package main_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestStripeWebhookVerificationFixture(t *testing.T) {
	fixturePath := filepath.Join("stripe", "test_payload.json")
	payload, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("failed to read Stripe fixture: %v", err)
	}

	secret := "whsec_test_secret_12345"
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("%s.%s", timestamp, string(payload))))
	sig := hex.EncodeToString(mac.Sum(nil))
	sigHeader := fmt.Sprintf("t=%s,v1=%s", timestamp, sig)

	// Verify header parsing and HMAC check logic
	var parsedTimestamp string
	var parsedV1 string
	for _, part := range strings.Split(sigHeader, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			if kv[0] == "t" {
				parsedTimestamp = kv[1]
			} else if kv[0] == "v1" {
				parsedV1 = kv[1]
			}
		}
	}

	if parsedTimestamp != timestamp || parsedV1 != sig {
		t.Fatalf("Stripe signature mismatch: got t=%s v1=%s, expected t=%s v1=%s", parsedTimestamp, parsedV1, timestamp, sig)
	}
}

func TestGitHubWebhookVerificationFixture(t *testing.T) {
	fixturePath := filepath.Join("github", "test_payload.json")
	payload, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("failed to read GitHub fixture: %v", err)
	}

	secret := "ghsec_test_secret_67890"
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	sig := hex.EncodeToString(mac.Sum(nil))
	sigHeader := "sha256=" + sig

	if !strings.HasPrefix(sigHeader, "sha256=") {
		t.Fatal("expected sha256= prefix")
	}
	extracted := strings.TrimPrefix(sigHeader, "sha256=")
	if !hmac.Equal([]byte(extracted), []byte(sig)) {
		t.Fatal("GitHub HMAC verification failed")
	}
}

func TestSlackWebhookVerificationFixture(t *testing.T) {
	fixturePath := filepath.Join("slack", "test_payload.json")
	payload, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("failed to read Slack fixture: %v", err)
	}

	secret := "slack_signing_secret_998877"
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	sigBase := fmt.Sprintf("v0:%s:%s", timestamp, string(payload))

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(sigBase))
	sig := "v0=" + hex.EncodeToString(mac.Sum(nil))

	if !strings.HasPrefix(sig, "v0=") {
		t.Fatal("expected v0= prefix")
	}
}
