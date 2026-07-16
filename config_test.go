package main

import "testing"

func TestGenerationFlags(t *testing.T) {
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
	t.Setenv("GENERATE_URLTEST", "sometimes")
	if _, err := loadConfig(); err == nil {
		t.Fatal("expected invalid boolean error")
	}
}

func TestSubscriptionURLIsNotRequiredAtStartup(t *testing.T) {
	t.Setenv("SUBSCRIPTION_URL", "")
	if _, err := loadConfig(); err != nil {
		t.Fatalf("loadConfig() returned an error without SUBSCRIPTION_URL: %v", err)
	}
}
