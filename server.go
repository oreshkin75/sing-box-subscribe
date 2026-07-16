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
	"strings"
	"sync"
	"time"
)

type subscriptionService struct {
	config config
	client *http.Client
	logger *slog.Logger

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	data       []byte
	expiresAt  time.Time
	lastAccess time.Time
}

const (
	maxCachedSubscriptions  = 128
	maxSubscriptionURLBytes = 8192
)

func newSubscriptionService(cfg config, logger *slog.Logger) *subscriptionService {
	return &subscriptionService{
		config: cfg,
		client: &http.Client{Timeout: cfg.FetchTimeout},
		logger: logger,
		cache:  make(map[string]cacheEntry),
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

	subscriptionURL, err := subscriptionURLFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	data, stale, err := s.get(r.Context(), subscriptionURL)
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

func (s *subscriptionService) get(ctx context.Context, subscriptionURL string) ([]byte, bool, error) {
	s.mu.Lock()
	now := time.Now()
	entry, cached := s.cache[subscriptionURL]
	if cached && now.Before(entry.expiresAt) {
		entry.lastAccess = now
		s.cache[subscriptionURL] = entry
		s.mu.Unlock()
		return entry.data, false, nil
	}
	s.mu.Unlock()

	data, err := s.fetchAndConvert(ctx, subscriptionURL)
	if err != nil {
		s.mu.Lock()
		entry, cached = s.cache[subscriptionURL]
		if cached {
			entry.lastAccess = time.Now()
			s.cache[subscriptionURL] = entry
			s.mu.Unlock()
			s.logger.Warn("subscription refresh failed; returning stale cache", "error", err)
			return entry.data, true, nil
		}
		s.mu.Unlock()
		return nil, false, err
	}

	s.mu.Lock()
	s.evictOldestCacheEntry(subscriptionURL)
	s.cache[subscriptionURL] = cacheEntry{
		data:       data,
		expiresAt:  time.Now().Add(s.config.CacheTTL),
		lastAccess: time.Now(),
	}
	s.mu.Unlock()
	return data, false, nil
}

func (s *subscriptionService) fetchAndConvert(ctx context.Context, subscriptionURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, subscriptionURL, nil)
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

func subscriptionURLFromRequest(r *http.Request) (string, error) {
	values, exists := r.URL.Query()["url"]
	if !exists || len(values) == 0 || strings.TrimSpace(values[0]) == "" {
		return "", errors.New("missing required url query parameter")
	}
	if len(values) != 1 {
		return "", errors.New("url query parameter must be specified once")
	}
	rawURL := strings.TrimSpace(values[0])
	if len(rawURL) > maxSubscriptionURLBytes {
		return "", errors.New("subscription URL is too long")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("url must be a valid http(s) subscription URL")
	}
	parsed.Fragment = ""
	return parsed.String(), nil
}

func (s *subscriptionService) evictOldestCacheEntry(incomingURL string) {
	if _, exists := s.cache[incomingURL]; exists || len(s.cache) < maxCachedSubscriptions {
		return
	}
	var oldestURL string
	var oldestAccess time.Time
	for cachedURL, entry := range s.cache {
		if oldestURL == "" || entry.lastAccess.Before(oldestAccess) {
			oldestURL = cachedURL
			oldestAccess = entry.lastAccess
		}
	}
	delete(s.cache, oldestURL)
}
