package worker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	maxFetchBodyBytes   = 64 << 10 // 64 KiB
	maxFetchBodyPreview = 512
)

func runFetch(ctx context.Context, client *http.Client, payload map[string]any) (any, error) {
	rawURL, ok := payload["url"].(string)
	if !ok || strings.TrimSpace(rawURL) == "" {
		return nil, fmt.Errorf("fetch.url must be a non-empty string")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("fetch.url is invalid: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("fetch.url must use http or https")
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("fetch.url must include a host")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("fetch request failed: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("fetch read body failed: %w", err)
	}

	truncated := len(body) > maxFetchBodyBytes
	if truncated {
		body = body[:maxFetchBodyBytes]
	}

	preview := string(body)
	if len(preview) > maxFetchBodyPreview {
		preview = preview[:maxFetchBodyPreview]
	}

	return map[string]any{
		"url":           parsed.String(),
		"status_code":   resp.StatusCode,
		"bytes":         len(body),
		"truncated":     truncated,
		"content_type":  resp.Header.Get("Content-Type"),
		"body_preview":  preview,
	}, nil
}
