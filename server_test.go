package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestOutboundsHandler(t *testing.T) {
	cfg := config{
		OutputPath:   "/outbounds.json",
		FetchTimeout: time.Second,
		CacheTTL:     time.Minute,
		MaxBodyBytes: 1 << 20,
	}
	service := newSubscriptionService(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	service.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://subscription.example/config?token=secret" {
			t.Fatalf("unexpected upstream URL %q", request.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewBufferString(sampleSubscription)),
			Request:    request,
		}, nil
	})
	request := newOutboundsRequest(t, "https://subscription.example/config?token=secret")
	response := httptest.NewRecorder()
	service.handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("unexpected content type %q", got)
	}
	var doc document
	if err := json.Unmarshal(response.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Outbounds) != 7 {
		t.Fatalf("got %d outbounds, want 7", len(doc.Outbounds))
	}
}

func TestMethodNotAllowed(t *testing.T) {
	cfg := config{OutputPath: "/outbounds.json"}
	service := newSubscriptionService(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	request := httptest.NewRequest(http.MethodPost, "/outbounds.json", nil)
	response := httptest.NewRecorder()
	service.handler().ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("got status %d", response.Code)
	}
}

func TestStaleCacheIsReturnedWhenRefreshFails(t *testing.T) {
	cfg := config{
		OutputPath:   "/outbounds.json",
		FetchTimeout: time.Second,
		CacheTTL:     0,
		MaxBodyBytes: 1 << 20,
	}
	service := newSubscriptionService(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	calls := 0
	service.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if calls > 1 {
			return nil, errors.New("temporary network failure")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewBufferString(sampleSubscription)),
			Request:    request,
		}, nil
	})

	firstRequest := newOutboundsRequest(t, "https://subscription.example/config/secret-token")
	firstResponse := httptest.NewRecorder()
	service.handler().ServeHTTP(firstResponse, firstRequest)
	if firstResponse.Code != http.StatusOK {
		t.Fatalf("initial status=%d", firstResponse.Code)
	}

	secondRequest := newOutboundsRequest(t, "https://subscription.example/config/secret-token")
	secondResponse := httptest.NewRecorder()
	service.handler().ServeHTTP(secondResponse, secondRequest)
	if secondResponse.Code != http.StatusOK {
		t.Fatalf("stale status=%d", secondResponse.Code)
	}
	if secondResponse.Header().Get("Warning") == "" {
		t.Fatal("stale response has no Warning header")
	}
	if secondResponse.Body.String() != firstResponse.Body.String() {
		t.Fatal("stale response differs from cached response")
	}
}

func TestSubscriptionURLValidation(t *testing.T) {
	service := newSubscriptionService(config{OutputPath: "/outbounds.json"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, requestTarget := range []string{
		"/outbounds.json",
		"/outbounds.json?url=",
		"/outbounds.json?url=ftp%3A%2F%2Fexample.com%2Fconfig",
		"/outbounds.json?url=https%3A%2F%2Fexample.com&url=https%3A%2F%2Fexample.org",
	} {
		request := httptest.NewRequest(http.MethodGet, requestTarget, nil)
		response := httptest.NewRecorder()
		service.handler().ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Errorf("target %q returned status %d, want 400", requestTarget, response.Code)
		}
	}
}

func TestCacheIsSeparatedBySubscriptionURL(t *testing.T) {
	cfg := config{
		OutputPath:   "/outbounds.json",
		FetchTimeout: time.Second,
		CacheTTL:     time.Minute,
		MaxBodyBytes: 1 << 20,
	}
	service := newSubscriptionService(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	calls := make(map[string]int)
	service.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls[request.URL.String()]++
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewBufferString(sampleSubscription)),
			Request:    request,
		}, nil
	})

	for _, subscriptionURL := range []string{
		"https://one.example/config",
		"https://two.example/config",
		"https://one.example/config",
	} {
		response := httptest.NewRecorder()
		service.handler().ServeHTTP(response, newOutboundsRequest(t, subscriptionURL))
		if response.Code != http.StatusOK {
			t.Fatalf("URL %q returned status %d", subscriptionURL, response.Code)
		}
	}
	if calls["https://one.example/config"] != 1 || calls["https://two.example/config"] != 1 {
		t.Fatalf("unexpected upstream call counts: %#v", calls)
	}
}

func newOutboundsRequest(t *testing.T, subscriptionURL string) *http.Request {
	t.Helper()
	return httptest.NewRequest(http.MethodGet, "/outbounds.json?url="+url.QueryEscape(subscriptionURL), nil)
}
