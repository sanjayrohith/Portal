package inspector

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ReplayResult describes the response received from a replay target.
type ReplayResult struct {
	RequestID  string        `json:"request_id"`
	StatusCode int           `json:"status_code"`
	Status     string        `json:"status"`
	Headers    HeaderValues  `json:"headers,omitempty"`
	Body       []byte        `json:"body,omitempty"`
	Duration   time.Duration `json:"duration"`
}

func replayTransaction(transaction *CapturedTransaction, target string) (*ReplayResult, error) {
	if target == "" {
		return nil, fmt.Errorf("replay target is not configured")
	}

	base, err := parseReplayTarget(target)
	if err != nil {
		return nil, err
	}
	requestURI := transaction.Request.Path
	if requestURI == "" {
		requestURI = transaction.Request.URL
	}
	if requestURI == "" {
		requestURI = "/"
	}
	if parsed, err := url.Parse(requestURI); err == nil && parsed.IsAbs() {
		requestURI = parsed.RequestURI()
	}
	if !strings.HasPrefix(requestURI, "/") {
		requestURI = "/" + requestURI
	}

	parsedRequestURI, err := url.Parse(requestURI)
	if err != nil {
		return nil, fmt.Errorf("parse captured request URL: %w", err)
	}
	replayURL := *base
	replayURL.Path = strings.TrimRight(base.Path, "/") + parsedRequestURI.Path
	replayURL.RawQuery = parsedRequestURI.RawQuery

	request, err := http.NewRequest(transaction.Request.Method, replayURL.String(), bytes.NewReader(transaction.Request.Body))
	if err != nil {
		return nil, fmt.Errorf("build replay request: %w", err)
	}
	for key, values := range transaction.Request.Headers {
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}
	if host := request.Header.Get("Host"); host != "" {
		request.Host = host
		request.Header.Del("Host")
	}
	for _, key := range []string{"Connection", "Keep-Alive", "Proxy-Connection", "Transfer-Encoding", "Upgrade"} {
		request.Header.Del(key)
	}

	started := time.Now()
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("replay request: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read replay response: %w", err)
	}

	return &ReplayResult{
		RequestID:  transaction.ID,
		StatusCode: response.StatusCode,
		Status:     response.Status,
		Headers:    headerValues(response.Header),
		Body:       body,
		Duration:   time.Since(started),
	}, nil
}

func validateReplayTarget(target string) error {
	if target == "" {
		return nil
	}
	if _, err := parseReplayTarget(target); err != nil {
		return err
	}
	return nil
}

func parseReplayTarget(target string) (*url.URL, error) {
	if !strings.Contains(target, "://") {
		target = "http://" + target
	}
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid replay target %q", target)
	}
	host := parsed.Hostname()
	if host != "127.0.0.1" && host != "localhost" {
		return nil, fmt.Errorf("replay target must be loopback, got %q", host)
	}
	return parsed, nil
}

func headerValues(headers http.Header) HeaderValues {
	values := make(HeaderValues, len(headers))
	for key, header := range headers {
		values[key] = append([]string(nil), header...)
	}
	return values
}
