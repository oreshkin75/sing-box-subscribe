package main

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type config struct {
	SubscriptionURL  string
	ListenAddr       string
	OutputPath       string
	FetchTimeout     time.Duration
	CacheTTL         time.Duration
	MaxBodyBytes     int64
	GenerateURLTest  bool
	GenerateSelector bool
}

func loadConfig() (config, error) {
	cfg := config{
		SubscriptionURL: strings.TrimSpace(os.Getenv("SUBSCRIPTION_URL")),
		ListenAddr:      envOrDefault("LISTEN_ADDR", ":8080"),
		OutputPath:      envOrDefault("OUTPUT_PATH", "/outbounds.json"),
		FetchTimeout:    15 * time.Second,
		CacheTTL:        5 * time.Minute,
		MaxBodyBytes:    8 << 20,
	}

	if cfg.SubscriptionURL == "" {
		return config{}, fmt.Errorf("SUBSCRIPTION_URL is required")
	}
	u, err := url.Parse(cfg.SubscriptionURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return config{}, fmt.Errorf("SUBSCRIPTION_URL must be a valid http(s) URL")
	}
	if !strings.HasPrefix(cfg.OutputPath, "/") {
		return config{}, fmt.Errorf("OUTPUT_PATH must start with /")
	}
	if cfg.OutputPath == "/healthz" || strings.ContainsAny(cfg.OutputPath, "?#{} \t\r\n") {
		return config{}, fmt.Errorf("invalid OUTPUT_PATH %q", cfg.OutputPath)
	}

	if value := strings.TrimSpace(os.Getenv("FETCH_TIMEOUT")); value != "" {
		cfg.FetchTimeout, err = time.ParseDuration(value)
		if err != nil || cfg.FetchTimeout <= 0 {
			return config{}, fmt.Errorf("invalid FETCH_TIMEOUT %q", value)
		}
	}
	if value := strings.TrimSpace(os.Getenv("CACHE_TTL")); value != "" {
		cfg.CacheTTL, err = time.ParseDuration(value)
		if err != nil || cfg.CacheTTL < 0 {
			return config{}, fmt.Errorf("invalid CACHE_TTL %q", value)
		}
	}
	if value := strings.TrimSpace(os.Getenv("MAX_SUBSCRIPTION_BYTES")); value != "" {
		cfg.MaxBodyBytes, err = strconv.ParseInt(value, 10, 64)
		if err != nil || cfg.MaxBodyBytes <= 0 {
			return config{}, fmt.Errorf("invalid MAX_SUBSCRIPTION_BYTES %q", value)
		}
	}
	if cfg.GenerateURLTest, err = envBool("GENERATE_URLTEST"); err != nil {
		return config{}, err
	}
	if cfg.GenerateSelector, err = envBool("GENERATE_SELECTOR"); err != nil {
		return config{}, err
	}

	return cfg, nil
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envBool(name string) (bool, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return false, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("invalid %s %q: expected true or false", name, value)
	}
	return parsed, nil
}
