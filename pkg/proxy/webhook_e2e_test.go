package proxy

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sanjayrohith/portal/pkg/mux"
)

// webhookSecret is the shared HMAC secret between the simulated provider and
// the local receiver in this end-to-end test.
const webhookSecret = "whsec_e2e_test_secret"

// signWebhook returns the hex HMAC-SHA256 of rawBody under webhookSecret,
// mimicking provider signature schemes (e.g. Stripe's v1 signatures).
func signWebhook(rawBody []byte) string {
	mac := hmac.New(sha256.New, []byte(webhookSecret))
	_, _ = mac.Write(rawBody)
	return hex.EncodeToString(mac.Sum(nil))
}

// buildSignedMultipart assembles a multipart webhook payload and its signature.
func buildSignedMultipart(t *testing.T, eventType, fileName, fileBody string) (contentType, signature string, raw []byte) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("event_type", eventType); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("payload", fileName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, fileBody); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	raw = body.Bytes()
	return writer.FormDataContentType(), signWebhook(raw), raw
}

// roundTripWebhook sends one signed multipart POST through a fresh tunneled
// stream and returns the upstream's status code and decoded JSON body.
func roundTripWebhook(t *testing.T, edgeSession *mux.Session, contentType, signature string, raw []byte) (int, map[string]any) {
	t.Helper()
	stream, err := edgeSession.OpenStream()
	if err != nil {
		t.Fatalf("open webhook stream: %v", err)
	}
	defer stream.Close()

	request, err := http.NewRequest(http.MethodPost, "http://webhook.portal.test/webhooks/acme", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("create webhook request: %v", err)
	}
	request.Host = "webhook.portal.test"
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("X-Webhook-Signature", signature)
	if err := WriteHTTPRequest(stream, request); err != nil {
		t.Fatalf("write webhook request: %v", err)
	}

	response, err := ReadHTTPResponse(stream, request)
	if err != nil {
		t.Fatalf("read webhook response: %v", err)
	}
	defer response.Body.Close()
	respBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read webhook response body: %v", err)
	}
	decoded := map[string]any{}
	if response.StatusCode == http.StatusOK && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, &decoded); err != nil {
			t.Fatalf("decode webhook response %q: %v", respBody, err)
		}
	}
	return response.StatusCode, decoded
}

func TestEndToEndSignedWebhookDelivery(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawBody, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "unreadable body", http.StatusBadRequest)
			return
		}
		provided := r.Header.Get("X-Webhook-Signature")
		expected := signWebhook(rawBody)
		if !hmac.Equal([]byte(provided), []byte(expected)) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
		r2, err := http.NewRequest(r.Method, r.URL.String(), bytes.NewReader(rawBody))
		if err != nil {
			http.Error(w, "rebuild failed", http.StatusInternalServerError)
			return
		}
		r2.Header.Set("Content-Type", r.Header.Get("Content-Type"))
		if err := r2.ParseMultipartForm(1 << 20); err != nil {
			http.Error(w, "multipart parse failed", http.StatusBadRequest)
			return
		}
		file, header, err := r2.FormFile("payload")
		if err != nil {
			http.Error(w, "missing payload file", http.StatusBadRequest)
			return
		}
		defer file.Close()
		fileBytes, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "unreadable payload file", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"verified":   true,
			"event_type": r2.FormValue("event_type"),
			"filename":   header.Filename,
			"bytes":      len(fileBytes),
		})
	}))
	defer upstream.Close()

	edgeConn, clientConn := net.Pipe()
	edgeSession := mux.NewSession(edgeConn, true)
	clientSession := mux.NewSession(clientConn, false)
	defer edgeSession.Close()
	defer clientSession.Close()

	bridge := NewStreamBridge(NewLocalDialer(upstream.Listener.Addr().String(), 2*time.Second), nil, 2*time.Second)
	go func() {
		for {
			stream, err := clientSession.AcceptStream()
			if err != nil {
				return
			}
			go func(st *mux.Stream) {
				_ = bridge.BridgeStream(t.Context(), st)
			}(stream)
		}
	}()

	t.Run("valid signature and multipart payload", func(t *testing.T) {
		contentType, signature, raw := buildSignedMultipart(t, "invoice.paid", "invoice.json", `{"id":"in_123","amount":2000}`)
		status, decoded := roundTripWebhook(t, edgeSession, contentType, signature, raw)
		if status != http.StatusOK {
			t.Fatalf("webhook status = %d, want 200", status)
		}
		if decoded["verified"] != true || decoded["event_type"] != "invoice.paid" {
			t.Fatalf("unexpected webhook ack: %v", decoded)
		}
		if decoded["filename"] != "invoice.json" || int(decoded["bytes"].(float64)) != len(`{"id":"in_123","amount":2000}`) {
			t.Fatalf("multipart fidelity lost: %v", decoded)
		}
	})

	t.Run("tampered body rejected", func(t *testing.T) {
		contentType, signature, raw := buildSignedMultipart(t, "invoice.paid", "invoice.json", `{"id":"in_123"}`)
		raw = append(raw, ' ')
		status, _ := roundTripWebhook(t, edgeSession, contentType, signature, raw)
		if status != http.StatusUnauthorized {
			t.Fatalf("tampered webhook status = %d, want 401", status)
		}
	})

	t.Run("missing signature rejected", func(t *testing.T) {
		contentType, _, raw := buildSignedMultipart(t, "invoice.paid", "invoice.json", `{"id":"in_123"}`)
		status, _ := roundTripWebhook(t, edgeSession, contentType, "", raw)
		if status != http.StatusUnauthorized {
			t.Fatalf("unsigned webhook status = %d, want 401", status)
		}
	})
}
