package main

import "testing"

func TestAppendGeneratedOutbounds(t *testing.T) {
	doc, warnings, err := parseSubscription([]byte(sampleSubscription))
	if err != nil || len(warnings) != 0 {
		t.Fatalf("parse subscription: warnings=%v err=%v", warnings, err)
	}

	appendGeneratedOutbounds(&doc, true, true)
	// Seven proxies, one country group and five protocol groups for each generated type.
	if len(doc.Outbounds) != 19 {
		t.Fatalf("got %d outbounds, want 19", len(doc.Outbounds))
	}

	countryURLTest := outboundByTag(t, doc, "🇺🇸 US / URLTest")
	if countryURLTest["type"] != "urltest" || countryURLTest["interval"] != "30s" ||
		countryURLTest["tolerance"] != 500 || countryURLTest["idle_timeout"] != "24h" ||
		countryURLTest["interrupt_exist_connections"] != false {
		t.Fatalf("unexpected country urltest: %#v", countryURLTest)
	}
	if outbounds := countryURLTest["outbounds"].([]string); len(outbounds) != 7 {
		t.Fatalf("country urltest contains %d outbounds, want 7", len(outbounds))
	}

	shadowsocksURLTest := outboundByTag(t, doc, "Shadowsocks / URLTest")
	if outbounds := shadowsocksURLTest["outbounds"].([]string); len(outbounds) != 2 {
		t.Fatalf("shadowsocks urltest contains %d outbounds, want 2", len(outbounds))
	}

	countrySelector := outboundByTag(t, doc, "🇺🇸 US / Selector")
	if countrySelector["type"] != "selector" || countrySelector["interrupt_exist_connections"] != false {
		t.Fatalf("unexpected country selector: %#v", countrySelector)
	}
	if outbounds := countrySelector["outbounds"].([]string); len(outbounds) != 7 {
		t.Fatalf("country selector contains %d outbounds, want 7", len(outbounds))
	}
}

func TestURLTestAndSelectorFlagsAreIndependent(t *testing.T) {
	for _, test := range []struct {
		name              string
		urlTest           bool
		selector          bool
		expectedURLTests  int
		expectedSelectors int
	}{
		{name: "disabled", expectedURLTests: 0, expectedSelectors: 0},
		{name: "urltest only", urlTest: true, expectedURLTests: 6, expectedSelectors: 0},
		{name: "selector only", selector: true, expectedURLTests: 0, expectedSelectors: 6},
		{name: "both", urlTest: true, selector: true, expectedURLTests: 6, expectedSelectors: 6},
	} {
		t.Run(test.name, func(t *testing.T) {
			doc, _, err := parseSubscription([]byte(sampleSubscription))
			if err != nil {
				t.Fatal(err)
			}
			appendGeneratedOutbounds(&doc, test.urlTest, test.selector)
			if got := countOutboundType(doc, "urltest"); got != test.expectedURLTests {
				t.Fatalf("got %d urltests, want %d", got, test.expectedURLTests)
			}
			if got := countOutboundType(doc, "selector"); got != test.expectedSelectors {
				t.Fatalf("got %d selectors, want %d", got, test.expectedSelectors)
			}
		})
	}
}

func TestUKAndGBAreCombinedIntoOneCountryGroup(t *testing.T) {
	const subscription = `ss://YWVzLTI1Ni1nY206c2VjcmV0@example.com:8388#UK-LON
ss://YWVzLTI1Ni1nY206c2VjcmV0@example.com:8389#GB-MAN`
	doc, warnings, err := parseSubscription([]byte(subscription))
	if err != nil || len(warnings) != 0 {
		t.Fatalf("parse subscription: warnings=%v err=%v", warnings, err)
	}
	appendGeneratedOutbounds(&doc, true, false)

	group := outboundByTag(t, doc, "🇬🇧 GB / URLTest")
	if outbounds := group["outbounds"].([]string); len(outbounds) != 2 {
		t.Fatalf("GB group contains %d outbounds, want 2", len(outbounds))
	}
}

func outboundByTag(t *testing.T, doc document, tag string) map[string]any {
	t.Helper()
	for _, outbound := range doc.Outbounds {
		if outbound["tag"] == tag {
			return outbound
		}
	}
	t.Fatalf("outbound %q not found", tag)
	return nil
}

func countOutboundType(doc document, outboundType string) int {
	count := 0
	for _, outbound := range doc.Outbounds {
		if outbound["type"] == outboundType {
			count++
		}
	}
	return count
}
