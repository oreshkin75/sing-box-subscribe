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
	ListenAddr                        string
	OutputPath                        string
	FetchTimeout                      time.Duration
	CacheTTL                          time.Duration
	MaxBodyBytes                      int64
	GenerateURLTest                   bool
	GenerateSelector                  bool
	URLTest                           urlTestOptions
	SelectorInterruptExistConnections bool
}

type urlTestOptions struct {
	URL                       string
	Interval                  string
	Tolerance                 int
	IdleTimeout               string
	InterruptExistConnections bool
}

func loadConfig() (config, error) {
	cfg := config{
		ListenAddr:   envOrDefault("LISTEN_ADDR", ":8080"),
		OutputPath:   envOrDefault("OUTPUT_PATH", "/outbounds.json"),
		FetchTimeout: 15 * time.Second,
		CacheTTL:     5 * time.Minute,
		MaxBodyBytes: 8 << 20,
		URLTest:      defaultURLTestOptions(),
	}

	var err error
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
	if cfg.GenerateURLTest {
		if err := loadURLTestOptions(&cfg.URLTest); err != nil {
			return config{}, err
		}
	}
	if cfg.GenerateSelector {
		if cfg.SelectorInterruptExistConnections, err = envBool("SELECTOR_INTERRUPT_EXIST_CONNECTIONS"); err != nil {
			return config{}, err
		}
	}

	return cfg, nil
}

func defaultURLTestOptions() urlTestOptions {
	return urlTestOptions{
		Interval:    "30s",
		Tolerance:   500,
		IdleTimeout: "24h",
	}
}

func loadURLTestOptions(options *urlTestOptions) error {
	if value := strings.TrimSpace(os.Getenv("URLTEST_URL")); value != "" {
		parsed, err := url.Parse(value)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("invalid URLTEST_URL: expected an absolute http(s) URL")
		}
		options.URL = parsed.String()
	}
	if value := strings.TrimSpace(os.Getenv("URLTEST_INTERVAL")); value != "" {
		if duration, err := time.ParseDuration(value); err != nil || duration <= 0 {
			return fmt.Errorf("invalid URLTEST_INTERVAL %q", value)
		}
		options.Interval = value
	}
	if value := strings.TrimSpace(os.Getenv("URLTEST_TOLERANCE")); value != "" {
		tolerance, err := strconv.Atoi(value)
		if err != nil || tolerance < 0 {
			return fmt.Errorf("invalid URLTEST_TOLERANCE %q", value)
		}
		options.Tolerance = tolerance
	}
	if value := strings.TrimSpace(os.Getenv("URLTEST_IDLE_TIMEOUT")); value != "" {
		if duration, err := time.ParseDuration(value); err != nil || duration <= 0 {
			return fmt.Errorf("invalid URLTEST_IDLE_TIMEOUT %q", value)
		}
		options.IdleTimeout = value
	}
	interrupt, err := envBool("URLTEST_INTERRUPT_EXIST_CONNECTIONS")
	if err != nil {
		return err
	}
	options.InterruptExistConnections = interrupt
	return nil
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
