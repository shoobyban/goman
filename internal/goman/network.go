package goman

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const MaxBodyBytes int64 = 4 << 20

type Response struct {
	StatusCode int
	Reason     string
	Elapsed    time.Duration
	Headers    http.Header
	Body       string
	SizeBytes  int
	IsJSON     bool
	Err        error
}

func NewClient(timeout time.Duration, insecure bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: insecure}
	return &http.Client{Timeout: timeout, Transport: transport}
}

func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("URL cannot be empty")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return "", fmt.Errorf("expected an HTTP or HTTPS URL")
	}
	return raw, nil
}

func FormatBody(raw []byte) (string, bool) {
	if json.Valid(raw) {
		if jsonIndentFits(raw) {
			var formatted bytes.Buffer
			if json.Indent(&formatted, raw, "", "  ") == nil {
				return formatted.String(), true
			}
		}
		return string(raw), true
	}
	return strings.ToValidUTF8(string(raw), "\uFFFD"), false
}

func jsonIndentFits(raw []byte) bool {
	size, depth := int64(len(raw)), int64(0)
	quoted, escaped := false, false
	for _, character := range raw {
		if size > MaxBodyBytes {
			return false
		}
		if quoted {
			if escaped {
				escaped = false
			} else if character == '\\' {
				escaped = true
			} else if character == '"' {
				quoted = false
			}
			continue
		}
		switch character {
		case '"':
			quoted = true
		case '{', '[':
			depth++
			size += 1 + 2*depth
		case '}', ']':
			depth--
			size += 1 + 2*depth
		case ',':
			size += 1 + 2*depth
		case ':':
			size++
		}
	}
	return size <= MaxBodyBytes
}

func Execute(ctx context.Context, client *http.Client, input Request) (result Response) {
	start := time.Now()
	defer func() {
		result.Elapsed = time.Since(start)
		if result.Err != nil {
			result.Body = "Request Failed: " + result.Err.Error()
		}
	}()
	address, err := NormalizeURL(input.URL)
	if err != nil {
		result.Err = err
		return
	}
	method := input.Method
	body := input.Body
	if method == "GET" || method == "HEAD" {
		body = ""
	}
	request, err := http.NewRequestWithContext(ctx, method, address, strings.NewReader(body))
	if err != nil {
		result.Err = err
		return
	}
	for key, value := range input.Headers {
		if strings.EqualFold(key, "Host") {
			request.Host = value
		} else {
			request.Header.Set(key, value)
		}
	}
	if request.Header.Get("User-Agent") == "" {
		request.Header.Set("User-Agent", "goman/0.1.0")
	}
	if body != "" && json.Valid([]byte(body)) && request.Header.Get("Content-Type") == "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		result.Err = err
		return
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, MaxBodyBytes+1))
	if err != nil {
		result.Err = err
		return
	}
	if int64(len(raw)) > MaxBodyBytes {
		result.Err = fmt.Errorf("response exceeds %d bytes", MaxBodyBytes)
		return
	}
	result.StatusCode = response.StatusCode
	result.Reason = strings.TrimPrefix(response.Status, fmt.Sprintf("%d ", response.StatusCode))
	result.Headers = response.Header.Clone()
	result.SizeBytes = len(raw)
	result.Body, result.IsJSON = FormatBody(raw)
	return
}
