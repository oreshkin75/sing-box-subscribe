package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type document struct {
	Outbounds []map[string]any `json:"outbounds"`
}

type parsedOutbound struct {
	value map[string]any
	tag   string
}

func parseSubscription(data []byte) (document, []error, error) {
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	result := document{Outbounds: make([]map[string]any, 0)}
	warnings := make([]error, 0)
	tags := make(map[string]int)
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parsed, err := parseProxyURI(line)
		if err != nil {
			warnings = append(warnings, fmt.Errorf("line %d: %w", lineNumber, err))
			continue
		}
		tag := uniqueTag(parsed.tag, parsed.value["type"].(string), tags)
		parsed.value["tag"] = addCountryFlag(tag)
		result.Outbounds = append(result.Outbounds, parsed.value)
	}
	if err := scanner.Err(); err != nil {
		return document{}, warnings, fmt.Errorf("read subscription: %w", err)
	}
	if len(result.Outbounds) == 0 {
		if len(warnings) > 0 {
			return document{}, warnings, fmt.Errorf("subscription contains no valid supported proxy links: %w", warnings[0])
		}
		return document{}, warnings, errors.New("subscription contains no proxy links")
	}
	return result, warnings, nil
}

func parseProxyURI(raw string) (parsedOutbound, error) {
	schemeEnd := strings.Index(raw, "://")
	if schemeEnd < 1 {
		return parsedOutbound{}, errors.New("invalid proxy URI")
	}
	switch strings.ToLower(raw[:schemeEnd]) {
	case "hysteria2", "hy2":
		return parseHysteria2(raw)
	case "ss":
		return parseShadowsocks(raw)
	case "trojan":
		return parseTrojan(raw)
	case "vless":
		return parseVLESS(raw)
	case "vmess":
		return parseVMess(raw)
	default:
		return parsedOutbound{}, fmt.Errorf("unsupported scheme %q", raw[:schemeEnd])
	}
}

func parseHysteria2(raw string) (parsedOutbound, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return parsedOutbound{}, errors.New("invalid hysteria2 URI")
	}
	server, port, err := endpoint(u)
	if err != nil {
		return parsedOutbound{}, err
	}
	password, err := userInfoValue(u.User)
	if err != nil || password == "" {
		return parsedOutbound{}, errors.New("hysteria2 password is missing")
	}

	q := u.Query()
	out := map[string]any{
		"type":        "hysteria2",
		"server":      server,
		"server_port": port,
		"password":    password,
		"tls":         tlsOptions(q, true),
	}
	if obfsType := first(q, "obfs", "obfs-type"); obfsType != "" {
		obfs := map[string]any{"type": obfsType}
		if password := first(q, "obfs-password", "obfsPassword"); password != "" {
			obfs["password"] = password
		}
		out["obfs"] = obfs
	}
	if value := first(q, "upmbps", "up_mbps"); value != "" {
		if number, parseErr := strconv.Atoi(value); parseErr == nil && number > 0 {
			out["up_mbps"] = number
		}
	}
	if value := first(q, "downmbps", "down_mbps"); value != "" {
		if number, parseErr := strconv.Atoi(value); parseErr == nil && number > 0 {
			out["down_mbps"] = number
		}
	}
	return parsedOutbound{value: out, tag: fragmentTag(u)}, nil
}

func parseShadowsocks(raw string) (parsedOutbound, error) {
	payload := raw[len("ss://"):]
	mainPart, rawFragment, _ := strings.Cut(payload, "#")
	mainPart, rawQuery, _ := strings.Cut(mainPart, "?")

	var credentials, address string
	if at := strings.LastIndex(mainPart, "@"); at >= 0 {
		credentials = mainPart[:at]
		address = mainPart[at+1:]
		decoded, err := decodeBase64(credentials)
		if err == nil {
			credentials = string(decoded)
		} else if unescaped, unescapeErr := url.PathUnescape(credentials); unescapeErr == nil {
			credentials = unescaped
		}
	} else {
		decoded, err := decodeBase64(mainPart)
		if err != nil {
			return parsedOutbound{}, errors.New("invalid shadowsocks base64 payload")
		}
		decodedText := string(decoded)
		at := strings.LastIndex(decodedText, "@")
		if at < 0 {
			return parsedOutbound{}, errors.New("shadowsocks endpoint is missing")
		}
		credentials, address = decodedText[:at], decodedText[at+1:]
	}

	method, password, ok := strings.Cut(credentials, ":")
	if !ok || method == "" || password == "" {
		return parsedOutbound{}, errors.New("invalid shadowsocks credentials")
	}
	server, port, err := splitHostPort(address)
	if err != nil {
		return parsedOutbound{}, err
	}
	out := map[string]any{
		"type":        "shadowsocks",
		"server":      server,
		"server_port": port,
		"method":      method,
		"password":    password,
	}
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		return parsedOutbound{}, fmt.Errorf("invalid shadowsocks query: %w", err)
	}
	if pluginValue := q.Get("plugin"); pluginValue != "" {
		plugin, options, _ := strings.Cut(pluginValue, ";")
		out["plugin"] = plugin
		if options != "" {
			out["plugin_opts"] = options
		}
	}
	tag, err := url.QueryUnescape(rawFragment)
	if err != nil {
		tag = rawFragment
	}
	return parsedOutbound{value: out, tag: tag}, nil
}

func parseTrojan(raw string) (parsedOutbound, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return parsedOutbound{}, errors.New("invalid trojan URI")
	}
	server, port, err := endpoint(u)
	if err != nil {
		return parsedOutbound{}, err
	}
	password, err := userInfoValue(u.User)
	if err != nil || password == "" {
		return parsedOutbound{}, errors.New("trojan password is missing")
	}
	q := u.Query()
	out := map[string]any{
		"type":        "trojan",
		"server":      server,
		"server_port": port,
		"password":    password,
		"tls":         tlsOptions(q, true),
	}
	if transport := transportOptions(first(q, "type", "network"), q, u.Path); transport != nil {
		out["transport"] = transport
	}
	return parsedOutbound{value: out, tag: fragmentTag(u)}, nil
}

func parseVLESS(raw string) (parsedOutbound, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return parsedOutbound{}, errors.New("invalid vless URI")
	}
	server, port, err := endpoint(u)
	if err != nil {
		return parsedOutbound{}, err
	}
	uuid, err := userInfoValue(u.User)
	if err != nil || uuid == "" {
		return parsedOutbound{}, errors.New("vless UUID is missing")
	}
	q := u.Query()
	out := map[string]any{
		"type":        "vless",
		"server":      server,
		"server_port": port,
		"uuid":        uuid,
	}
	if flow := q.Get("flow"); flow != "" {
		out["flow"] = flow
	}
	if strings.EqualFold(q.Get("security"), "tls") {
		out["tls"] = tlsOptions(q, true)
	}
	if transport := transportOptions(first(q, "type", "network"), q, u.Path); transport != nil {
		out["transport"] = transport
	}
	return parsedOutbound{value: out, tag: fragmentTag(u)}, nil
}

func parseVMess(raw string) (parsedOutbound, error) {
	payload := strings.TrimSpace(raw[len("vmess://"):])
	decoded, err := decodeBase64(payload)
	if err != nil {
		return parsedOutbound{}, errors.New("invalid vmess base64 payload")
	}
	decoder := json.NewDecoder(bytes.NewReader(decoded))
	decoder.UseNumber()
	var source map[string]any
	if err := decoder.Decode(&source); err != nil {
		return parsedOutbound{}, fmt.Errorf("invalid vmess JSON: %w", err)
	}
	server := valueString(source["add"])
	port, err := strconv.Atoi(valueString(source["port"]))
	if server == "" || err != nil || port < 1 || port > 65535 {
		return parsedOutbound{}, errors.New("invalid vmess endpoint")
	}
	uuid := valueString(source["id"])
	if uuid == "" {
		return parsedOutbound{}, errors.New("vmess UUID is missing")
	}
	out := map[string]any{
		"type":        "vmess",
		"server":      server,
		"server_port": port,
		"uuid":        uuid,
		"security":    defaultString(valueString(source["scy"]), "auto"),
	}
	if alterID, parseErr := strconv.Atoi(defaultString(valueString(source["aid"]), "0")); parseErr == nil {
		out["alter_id"] = alterID
	}
	if tlsMode := valueString(source["tls"]); tlsMode != "" && !strings.EqualFold(tlsMode, "none") {
		q := url.Values{}
		q.Set("sni", valueString(source["sni"]))
		if insecure := valueString(source["allowInsecure"]); insecure != "" {
			q.Set("allowInsecure", insecure)
		}
		out["tls"] = tlsOptions(q, true)
	}
	q := url.Values{}
	q.Set("path", valueString(source["path"]))
	q.Set("host", valueString(source["host"]))
	if transport := transportOptions(valueString(source["net"]), q, ""); transport != nil {
		out["transport"] = transport
	}
	return parsedOutbound{value: out, tag: valueString(source["ps"])}, nil
}

func endpoint(u *url.URL) (string, int, error) {
	if u.Hostname() == "" || u.Port() == "" {
		return "", 0, errors.New("proxy endpoint must contain host and port")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return "", 0, errors.New("invalid proxy port")
	}
	return u.Hostname(), port, nil
}

func splitHostPort(address string) (string, int, error) {
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return "", 0, fmt.Errorf("invalid proxy endpoint: %w", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 || host == "" {
		return "", 0, errors.New("invalid proxy endpoint")
	}
	return host, port, nil
}

func userInfoValue(info *url.Userinfo) (string, error) {
	if info == nil {
		return "", errors.New("userinfo is missing")
	}
	username := info.Username()
	if password, ok := info.Password(); ok {
		return username + ":" + password, nil
	}
	return username, nil
}

func tlsOptions(q url.Values, enabled bool) map[string]any {
	tls := map[string]any{"enabled": enabled}
	if serverName := first(q, "sni", "peer", "serverName"); serverName != "" {
		tls["server_name"] = serverName
	}
	if parseBool(first(q, "insecure", "allowInsecure", "skip-cert-verify")) {
		tls["insecure"] = true
	}
	if alpn := q.Get("alpn"); alpn != "" {
		parts := strings.FieldsFunc(alpn, func(r rune) bool { return r == ',' })
		if len(parts) > 0 {
			tls["alpn"] = parts
		}
	}
	return tls
}

func transportOptions(kind string, q url.Values, fallbackPath string) map[string]any {
	switch strings.ToLower(kind) {
	case "ws", "websocket":
		path := defaultString(q.Get("path"), fallbackPath)
		path = defaultString(path, "/")
		transport := map[string]any{"type": "ws", "path": path}
		if host := q.Get("host"); host != "" {
			transport["headers"] = map[string]string{"Host": host}
		}
		return transport
	case "grpc":
		serviceName := first(q, "serviceName", "service_name")
		if serviceName == "" {
			serviceName = strings.TrimPrefix(defaultString(q.Get("path"), fallbackPath), "/")
		}
		transport := map[string]any{"type": "grpc"}
		if serviceName != "" {
			transport["service_name"] = serviceName
		}
		return transport
	case "http", "h2":
		transport := map[string]any{"type": "http"}
		if path := defaultString(q.Get("path"), fallbackPath); path != "" {
			transport["path"] = path
		}
		if host := q.Get("host"); host != "" {
			transport["host"] = strings.Split(host, ",")
		}
		return transport
	default:
		return nil
	}
}

func decodeBase64(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	}
	var lastErr error
	for _, encoding := range encodings {
		decoded, err := encoding.DecodeString(value)
		if err == nil {
			return decoded, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func valueString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return ""
	}
}

func fragmentTag(u *url.URL) string { return strings.TrimSpace(u.Fragment) }

func uniqueTag(tag, scheme string, seen map[string]int) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		tag = scheme
	}
	seen[tag]++
	if seen[tag] == 1 {
		return tag
	}
	return fmt.Sprintf("%s (%d)", tag, seen[tag])
}

func addCountryFlag(tag string) string {
	if startsWithFlag(tag) {
		return tag
	}
	code, ok := countryCodeFromTag(tag)
	if !ok {
		return tag
	}
	return countryFlag(code) + " " + tag
}

func startsWithFlag(tag string) bool {
	runes := []rune(tag)
	if len(runes) < 2 {
		return false
	}
	const (
		regionalIndicatorA = rune(0x1F1E6)
		regionalIndicatorZ = rune(0x1F1FF)
	)
	return runes[0] >= regionalIndicatorA && runes[0] <= regionalIndicatorZ &&
		runes[1] >= regionalIndicatorA && runes[1] <= regionalIndicatorZ
}

func countryCodeFromTag(tag string) (string, bool) {
	runes := []rune(tag)
	if len(runes) >= 2 && isRegionalIndicator(runes[0]) && isRegionalIndicator(runes[1]) {
		const regionalIndicatorA = rune(0x1F1E6)
		return string([]byte{
			byte('A' + runes[0] - regionalIndicatorA),
			byte('A' + runes[1] - regionalIndicatorA),
		}), true
	}

	if len(tag) < 2 || tag[0] < 'A' || tag[0] > 'Z' || tag[1] < 'A' || tag[1] > 'Z' {
		return "", false
	}
	if len(tag) > 2 && !strings.ContainsRune("-_ /", rune(tag[2])) {
		return "", false
	}
	code := tag[:2]
	if code == "UK" {
		code = "GB"
	}
	return code, true
}

func countryFlag(code string) string {
	const regionalIndicatorA = rune(0x1F1E6)
	return string([]rune{
		regionalIndicatorA + rune(code[0]-'A'),
		regionalIndicatorA + rune(code[1]-'A'),
	})
}

func isRegionalIndicator(value rune) bool {
	return value >= rune(0x1F1E6) && value <= rune(0x1F1FF)
}

func first(values url.Values, keys ...string) string {
	for _, key := range keys {
		if value := values.Get(key); value != "" {
			return value
		}
	}
	return ""
}

func defaultString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func parseBool(value string) bool {
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
