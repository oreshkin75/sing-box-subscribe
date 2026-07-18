package main

import "testing"

func TestGenerationFlags(t *testing.T) {
	clearURLTestEnv(t)
	t.Setenv("GENERATE_URLTEST", "true")
	t.Setenv("GENERATE_SELECTOR", "true")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.GenerateURLTest || !cfg.GenerateSelector {
		t.Fatalf("generation flags were not enabled: %#v", cfg)
	}
}

func TestInvalidGenerationFlag(t *testing.T) {
	clearURLTestEnv(t)
	t.Setenv("GENERATE_URLTEST", "sometimes")
	if _, err := loadConfig(); err == nil {
		t.Fatal("expected invalid boolean error")
	}
}

func TestSubscriptionURLIsNotRequiredAtStartup(t *testing.T) {
	clearURLTestEnv(t)
	t.Setenv("SUBSCRIPTION_URL", "")
	if _, err := loadConfig(); err != nil {
		t.Fatalf("loadConfig() returned an error without SUBSCRIPTION_URL: %v", err)
	}
}

func TestURLTestOptions(t *testing.T) {
	clearURLTestEnv(t)
	t.Setenv("GENERATE_URLTEST", "true")
	t.Setenv("URLTEST_URL", "https://cp.example/generate_204")
	t.Setenv("URLTEST_INTERVAL", "1m")
	t.Setenv("URLTEST_TOLERANCE", "900")
	t.Setenv("URLTEST_IDLE_TIMEOUT", "6h")
	t.Setenv("URLTEST_INTERRUPT_EXIST_CONNECTIONS", "true")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	want := urlTestOptions{
		URL:                       "https://cp.example/generate_204",
		Interval:                  "1m",
		Tolerance:                 900,
		IdleTimeout:               "6h",
		InterruptExistConnections: true,
	}
	if cfg.URLTest != want {
		t.Fatalf("URLTest options = %#v, want %#v", cfg.URLTest, want)
	}
}

func TestURLTestOptionsAreIgnoredWhenGenerationIsDisabled(t *testing.T) {
	clearURLTestEnv(t)
	t.Setenv("GENERATE_URLTEST", "false")
	t.Setenv("URLTEST_URL", "not a URL")
	t.Setenv("URLTEST_INTERVAL", "invalid")
	t.Setenv("URLTEST_TOLERANCE", "invalid")
	t.Setenv("URLTEST_IDLE_TIMEOUT", "invalid")
	t.Setenv("URLTEST_INTERRUPT_EXIST_CONNECTIONS", "invalid")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("disabled URLTest options must be ignored: %v", err)
	}
	if cfg.URLTest != defaultURLTestOptions() {
		t.Fatalf("URLTest options = %#v, want defaults %#v", cfg.URLTest, defaultURLTestOptions())
	}
}

func TestInvalidEnabledURLTestOptions(t *testing.T) {
	for _, test := range []struct {
		name  string
		value string
	}{
		{name: "URLTEST_URL", value: "ftp://example.com/test"},
		{name: "URLTEST_INTERVAL", value: "0s"},
		{name: "URLTEST_TOLERANCE", value: "-1"},
		{name: "URLTEST_IDLE_TIMEOUT", value: "later"},
		{name: "URLTEST_INTERRUPT_EXIST_CONNECTIONS", value: "maybe"},
	} {
		t.Run(test.name, func(t *testing.T) {
			clearURLTestEnv(t)
			t.Setenv("GENERATE_URLTEST", "true")
			t.Setenv(test.name, test.value)
			if _, err := loadConfig(); err == nil {
				t.Fatalf("expected validation error for %s=%q", test.name, test.value)
			}
		})
	}
}

func clearURLTestEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"GENERATE_URLTEST",
		"GENERATE_SELECTOR",
		"URLTEST_URL",
		"URLTEST_INTERVAL",
		"URLTEST_TOLERANCE",
		"URLTEST_IDLE_TIMEOUT",
		"URLTEST_INTERRUPT_EXIST_CONNECTIONS",
		"SELECTOR_INTERRUPT_EXIST_CONNECTIONS",
	} {
		t.Setenv(name, "")
	}
}

func TestSelectorInterruptExistConnections(t *testing.T) {
	clearURLTestEnv(t)
	t.Setenv("GENERATE_SELECTOR", "true")
	t.Setenv("SELECTOR_INTERRUPT_EXIST_CONNECTIONS", "true")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SelectorInterruptExistConnections {
		t.Fatal("SELECTOR_INTERRUPT_EXIST_CONNECTIONS was not enabled")
	}
}

func TestSelectorOptionsAreIgnoredWhenGenerationIsDisabled(t *testing.T) {
	clearURLTestEnv(t)
	t.Setenv("GENERATE_SELECTOR", "false")
	t.Setenv("SELECTOR_INTERRUPT_EXIST_CONNECTIONS", "invalid")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("disabled selector options must be ignored: %v", err)
	}
	if cfg.SelectorInterruptExistConnections {
		t.Fatal("disabled selector option was applied")
	}
}

func TestInvalidEnabledSelectorOption(t *testing.T) {
	clearURLTestEnv(t)
	t.Setenv("GENERATE_SELECTOR", "true")
	t.Setenv("SELECTOR_INTERRUPT_EXIST_CONNECTIONS", "invalid")
	if _, err := loadConfig(); err == nil {
		t.Fatal("expected selector option validation error")
	}
}
