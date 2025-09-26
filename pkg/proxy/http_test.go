package proxy

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestReadWriteHTTPRequest(t *testing.T) {
	payload := `{"event":"ping"}`
	rawReq := "POST /api/webhooks HTTP/1.1\r\n" +
		"Host: myapp.tunnel.dev\r\n" +
		"Content-Type: application/json\r\n" +
		"Content-Length: 16\r\n" +
		"\r\n" +
		payload

	buf := bytes.NewBufferString(rawReq)
	req, err := ReadHTTPRequest(buf)
	if err != nil {
		t.Fatalf("ReadHTTPRequest failed: %v", err)
	}

	if req.Method != "POST" {
		t.Errorf("expected POST, got %s", req.Method)
	}
	if req.URL.Path != "/api/webhooks" {
		t.Errorf("expected /api/webhooks, got %s", req.URL.Path)
	}
	if req.Host != "myapp.tunnel.dev" {
		t.Errorf("expected myapp.tunnel.dev, got %s", req.Host)
	}

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("failed reading request body: %v", err)
	}
	if string(bodyBytes) != payload {
		t.Errorf("expected body %s, got %s", payload, string(bodyBytes))
	}

	// Test write back
	req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	outBuf := &bytes.Buffer{}
	if err := WriteHTTPRequest(outBuf, req); err != nil {
		t.Fatalf("WriteHTTPRequest failed: %v", err)
	}

	if !strings.Contains(outBuf.String(), payload) {
		t.Errorf("written request missing body payload")
	}
}

func TestReadWriteHTTPResponse(t *testing.T) {
	rawResp := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: text/plain\r\n" +
		"Content-Length: 12\r\n" +
		"\r\n" +
		"hello world!"

	dummyReq := &http.Request{
		Method: "GET",
		URL:    &url.URL{Path: "/"},
	}

	buf := bytes.NewBufferString(rawResp)
	resp, err := ReadHTTPResponse(buf, dummyReq)
	if err != nil {
		t.Fatalf("ReadHTTPResponse failed: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body failed: %v", err)
	}
	if string(body) != "hello world!" {
		t.Errorf("expected 'hello world!', got %q", string(body))
	}

	// Test write response
	resp.Body = io.NopCloser(bytes.NewBuffer(body))
	outBuf := &bytes.Buffer{}
	if err := WriteHTTPResponse(outBuf, resp); err != nil {
		t.Fatalf("WriteHTTPResponse failed: %v", err)
	}

	if !strings.Contains(outBuf.String(), "200 OK") {
		t.Errorf("written response missing status code line")
	}
}
