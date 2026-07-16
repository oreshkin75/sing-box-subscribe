package main

import (
	"encoding/base64"
	"strings"
	"testing"
)

const sampleSubscription = `hysteria2://1866d8b8-5e21-4e88-b21f-22d5a9781693@192.166.82.244:443/?sni=us-slc-hysteria1.xeovo.net#US-SLC%20/%20Hysteria2

ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTo2ZmhEQUZLYzRvaURqbUxIV3lNOGFMcHpNVDdp@us-slc-global1.xeovo.eu:9000#US-SLC%20/%20Shadowsocks

ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTo2ZmhEQUZLYzRvaURqbUxIV3lNOGFMcHpNVDdp@us-slc-global1.xeovo.eu:9002?plugin=v2ray-plugin%3Btls%3Bhost%3Dus-slc-global1.xeovo.net%3Bpath%3D/%3Bmux%3D0#US-SLC%20/%20Shadowsocks%20%28v2ray%29

trojan://jmLHWyM8aLpzMT7i@us-slc-global1.xeovo.eu:444#US-SLC%20/%20Trojan%20%28TLS%29

trojan://jmLHWyM8aLpzMT7i@us-slc-global1.xeovo.eu:443?path=%2Fcolcha&host=us-slc-global1.xeovo.eu&sni=us-slc-global1.xeovo.eu&type=ws#US-SLC%20/%20Trojan%20%28WS%2BTLS%29

vless://1866d8b8-5e21-4e88-b21f-22d5a9781693@us-slc-global1.xeovo.eu:443/potosi?type=ws&encryption=none&security=tls&sni=us-slc-global1.xeovo.eu&host=us-slc-global1.xeovo.eu&path=%2Fpotosi#US-SLC%20/%20VLESS%20%28WS%2BTLS%29

vmess://eyJhZGQiOiAidXMtc2xjLWdsb2JhbDEueGVvdm8uZXUiLCAiYWlkIjogIjAiLCAiaG9zdCI6ICJ1cy1zbGMtZ2xvYmFsMS54ZW92by5ldSIsICJpZCI6ICIxODY2ZDhiOC01ZTIxLTRlODgtYjIxZi0yMmQ1YTk3ODE2OTMiLCAibmV0IjogIndzIiwgInBhdGgiOiAiL3NhY2FjYSIsICJwb3J0IjogIjQ0MyIsICJwcyI6ICJVUy1TTEMgLyBWTWVzcyAoV1MrVExTKSIsICJzY3kiOiAibm9uZSIsICJzbmkiOiAidXMtc2xjLWdsb2JhbDEueGVvdm8uZXUiLCAidGxzIjogInRscyIsICJ0eXBlIjogIiIsICJ2IjogIjIifQ==`

func TestParseSampleSubscription(t *testing.T) {
	doc, warnings, err := parseSubscription([]byte(sampleSubscription))
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(doc.Outbounds) != 7 {
		t.Fatalf("got %d outbounds, want 7", len(doc.Outbounds))
	}

	hy2 := doc.Outbounds[0]
	if hy2["type"] != "hysteria2" || hy2["password"] != "1866d8b8-5e21-4e88-b21f-22d5a9781693" {
		t.Fatalf("unexpected hysteria2 outbound: %#v", hy2)
	}
	if hy2["tag"] != "🇺🇸 US-SLC / Hysteria2" {
		t.Fatalf("unexpected country flag in tag: %q", hy2["tag"])
	}
	if tls := hy2["tls"].(map[string]any); tls["server_name"] != "us-slc-hysteria1.xeovo.net" {
		t.Fatalf("unexpected hysteria2 TLS: %#v", tls)
	}

	ssPlugin := doc.Outbounds[2]
	if ssPlugin["plugin"] != "v2ray-plugin" || ssPlugin["plugin_opts"] != "tls;host=us-slc-global1.xeovo.net;path=/;mux=0" {
		t.Fatalf("unexpected shadowsocks plugin: %#v", ssPlugin)
	}

	trojanWS := doc.Outbounds[4]
	transport := trojanWS["transport"].(map[string]any)
	if transport["type"] != "ws" || transport["path"] != "/colcha" {
		t.Fatalf("unexpected trojan transport: %#v", transport)
	}

	vless := doc.Outbounds[5]
	if vless["uuid"] != "1866d8b8-5e21-4e88-b21f-22d5a9781693" {
		t.Fatalf("unexpected vless outbound: %#v", vless)
	}

	vmess := doc.Outbounds[6]
	if vmess["security"] != "none" || vmess["alter_id"] != 0 {
		t.Fatalf("unexpected vmess outbound: %#v", vmess)
	}
	vmessTransport := vmess["transport"].(map[string]any)
	if vmessTransport["path"] != "/sacaca" {
		t.Fatalf("unexpected vmess transport: %#v", vmessTransport)
	}
}

func TestParseFullBase64ShadowsocksAndDuplicateTags(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte("aes-256-gcm:secret@example.com:8388"))
	input := "ss://" + payload + "#same\nss://" + payload + "#same\nunsupported://x\n"
	doc, warnings, err := parseSubscription([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1", len(warnings))
	}
	if doc.Outbounds[0]["tag"] != "same" || doc.Outbounds[1]["tag"] != "same (2)" {
		t.Fatalf("duplicate tags were not disambiguated: %#v", doc.Outbounds)
	}
}

func TestNoValidLinksIsError(t *testing.T) {
	_, warnings, err := parseSubscription([]byte("not a link\nss://broken\n"))
	if err == nil || len(warnings) != 2 {
		t.Fatalf("err=%v warnings=%v", err, warnings)
	}
	if !strings.Contains(err.Error(), "no valid supported") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddCountryFlag(t *testing.T) {
	tests := map[string]string{
		"US-SLC / Hysteria2":    "🇺🇸 US-SLC / Hysteria2",
		"UK-LON / VLESS":        "🇬🇧 UK-LON / VLESS",
		"GB-LON / VLESS":        "🇬🇧 GB-LON / VLESS",
		"DE / Shadowsocks":      "🇩🇪 DE / Shadowsocks",
		"🇺🇸 US-SLC / Hysteria2": "🇺🇸 US-SLC / Hysteria2",
		"vmess":                 "vmess",
		"Unknown proxy":         "Unknown proxy",
	}
	for input, expected := range tests {
		if actual := addCountryFlag(input); actual != expected {
			t.Errorf("addCountryFlag(%q) = %q, want %q", input, actual, expected)
		}
	}
}
