package main

import "fmt"

type outboundGroup struct {
	name      string
	outbounds []string
}

func appendGeneratedOutbounds(doc *document, generateURLTest, generateSelector bool) {
	if !generateURLTest && !generateSelector {
		return
	}

	countryGroups, protocolGroups := groupOutbounds(doc.Outbounds)
	usedTags := make(map[string]struct{}, len(doc.Outbounds))
	for _, outbound := range doc.Outbounds {
		if tag, ok := outbound["tag"].(string); ok {
			usedTags[tag] = struct{}{}
		}
	}

	if generateURLTest {
		for _, group := range countryGroups {
			doc.Outbounds = append(doc.Outbounds, newURLTest(uniqueGeneratedTag(group.name+" / URLTest", usedTags), group.outbounds))
		}
		for _, group := range protocolGroups {
			doc.Outbounds = append(doc.Outbounds, newURLTest(uniqueGeneratedTag(group.name+" / URLTest", usedTags), group.outbounds))
		}
	}
	if generateSelector {
		for _, group := range countryGroups {
			doc.Outbounds = append(doc.Outbounds, newSelector(uniqueGeneratedTag(group.name+" / Selector", usedTags), group.outbounds))
		}
		for _, group := range protocolGroups {
			doc.Outbounds = append(doc.Outbounds, newSelector(uniqueGeneratedTag(group.name+" / Selector", usedTags), group.outbounds))
		}
	}
}

func groupOutbounds(outbounds []map[string]any) ([]outboundGroup, []outboundGroup) {
	countries := make([]outboundGroup, 0)
	protocols := make([]outboundGroup, 0)
	countryIndexes := make(map[string]int)
	protocolIndexes := make(map[string]int)

	for _, outbound := range outbounds {
		tag, tagOK := outbound["tag"].(string)
		protocol, protocolOK := outbound["type"].(string)
		if !tagOK || tag == "" || !protocolOK || protocol == "" {
			continue
		}

		if code, ok := countryCodeFromTag(tag); ok {
			index, exists := countryIndexes[code]
			if !exists {
				index = len(countries)
				countryIndexes[code] = index
				countries = append(countries, outboundGroup{name: countryFlag(code) + " " + code})
			}
			countries[index].outbounds = append(countries[index].outbounds, tag)
		}

		index, exists := protocolIndexes[protocol]
		if !exists {
			index = len(protocols)
			protocolIndexes[protocol] = index
			protocols = append(protocols, outboundGroup{name: protocolDisplayName(protocol)})
		}
		protocols[index].outbounds = append(protocols[index].outbounds, tag)
	}
	return countries, protocols
}

func newURLTest(tag string, outbounds []string) map[string]any {
	return map[string]any{
		"type":                        "urltest",
		"tag":                         tag,
		"outbounds":                   append([]string(nil), outbounds...),
		"interval":                    "30s",
		"tolerance":                   500,
		"idle_timeout":                "24h",
		"interrupt_exist_connections": false,
	}
}

func newSelector(tag string, outbounds []string) map[string]any {
	return map[string]any{
		"type":                        "selector",
		"tag":                         tag,
		"outbounds":                   append([]string(nil), outbounds...),
		"interrupt_exist_connections": false,
	}
}

func uniqueGeneratedTag(base string, used map[string]struct{}) string {
	if _, exists := used[base]; !exists {
		used[base] = struct{}{}
		return base
	}
	for number := 2; ; number++ {
		candidate := fmt.Sprintf("%s (%d)", base, number)
		if _, exists := used[candidate]; !exists {
			used[candidate] = struct{}{}
			return candidate
		}
	}
}

func protocolDisplayName(protocol string) string {
	switch protocol {
	case "hysteria2":
		return "Hysteria2"
	case "shadowsocks":
		return "Shadowsocks"
	case "trojan":
		return "Trojan"
	case "vless":
		return "VLESS"
	case "vmess":
		return "VMess"
	default:
		return protocol
	}
}
