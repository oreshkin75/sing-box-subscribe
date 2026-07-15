package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type subscriptionService struct {
	config config
	client *http.Client
	logger *slog.Logger

	mu        sync.Mutex
	cached    []byte
	expiresAt time.Time
}

func newSubscriptionService(cfg config, logger *slog.Logger) *subscriptionService {
	return &subscriptionService{
		config: cfg,
		client: &http.Client{Timeout: cfg.FetchTimeout},
		logger: logger,
	}
}

func (s *subscriptionService) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(s.config.OutputPath, s.handleOutbounds)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok\n")
	})
	return mux
}

func (s *subscriptionService) handleOutbounds(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data, stale, err := s.get(r.Context())
	if err != nil {
		s.logger.Error("cannot build outbounds", "error", err)
		http.Error(w, "cannot load subscription", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="outbounds.json"`)
	w.Header().Set("Cache-Control", "no-cache")
	if stale {
		w.Header().Set("Warning", `110 - "upstream unavailable; stale subscription returned"`)
	}
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write(data)
	}
}

func (s *subscriptionService) get(ctx context.Context) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if s.cached != nil && now.Before(s.expiresAt) {
		return s.cached, false, nil
	}

	data, err := s.fetchAndConvert(ctx)
	if err != nil {
		if s.cached != nil {
			s.logger.Warn("subscription refresh failed; returning stale cache", "error", err)
			return s.cached, true, nil
		}
		return nil, false, err
	}
	s.cached = data
	s.expiresAt = now.Add(s.config.CacheTTL)
	return data, false, nil
}

func (s *subscriptionService) fetchAndConvert(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.config.SubscriptionURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create upstream request: %w", err)
	}
	req.Header.Set("User-Agent", "sing-box-subscribe/1.0")
	req.Header.Set("Accept", "text/plain, application/octet-stream;q=0.9, */*;q=0.1")

	response, err := s.client.Do(req)
	if err != nil {
		var urlError *url.Error
		if errors.As(err, &urlError) {
			return nil, fmt.Errorf("fetch upstream subscription: %w", urlError.Err)
		}
		return nil, errors.New("fetch upstream subscription failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("upstream returned HTTP %d", response.StatusCode)
	}

	limited := io.LimitReader(response.Body, s.config.MaxBodyBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read upstream subscription: %w", err)
	}
	if int64(len(body)) > s.config.MaxBodyBytes {
		return nil, errors.New("upstream subscription is too large")
	}

	doc, warnings, err := parseSubscription(body)
	if err != nil {
		return nil, err
	}
	appendGeneratedOutbounds(&doc, s.config.GenerateURLTest, s.config.GenerateSelector)
	for _, warning := range warnings {
		s.logger.Warn("skipped subscription entry", "error", warning)
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode outbounds JSON: %w", err)
	}
	return append(data, '\n'), nil
}
