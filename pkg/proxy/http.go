package proxy

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
)

// ReadHTTPRequest deserializes an HTTP request from a streaming network connection.
func ReadHTTPRequest(r io.Reader) (*http.Request, error) {
	br := bufio.NewReader(r)
	req, err := http.ReadRequest(br)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTTP request from stream: %w", err)
	}
	return req, nil
}

// WriteHTTPRequest serializes an HTTP request in standard wire format to an io.Writer.
func WriteHTTPRequest(w io.Writer, req *http.Request) error {
	if req == nil {
		return fmt.Errorf("cannot write nil HTTP request")
	}
	if err := req.Write(w); err != nil {
		return fmt.Errorf("failed to write HTTP request: %w", err)
	}
	return nil
}

// ReadHTTPResponse deserializes an HTTP response corresponding to a request from an io.Reader.
func ReadHTTPResponse(r io.Reader, req *http.Request) (*http.Response, error) {
	br := bufio.NewReader(r)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTTP response from stream: %w", err)
	}
	return resp, nil
}

// WriteHTTPResponse serializes an HTTP response to an io.Writer.
func WriteHTTPResponse(w io.Writer, resp *http.Response) error {
	if resp == nil {
		return fmt.Errorf("cannot write nil HTTP response")
	}
	if err := resp.Write(w); err != nil {
		return fmt.Errorf("failed to write HTTP response: %w", err)
	}
	return nil
}
