package provenance

import (
	"net/mail"
	"net/url"
	"slices"
	"strings"
)

// DestinationHosts extracts explicit routing fields in a stable order. It does
// not scan message bodies, records, queries or arbitrary nested payloads: an
// address mentioned in content is not necessarily where an action sends data.
// Every recipient is retained so an approved To address cannot hide an
// unapproved CC, BCC or later entry in a recipient list.
func DestinationHosts(args map[string]any) []string {
	var hosts []string
	collectDestinationHosts(args, &hosts, 0)
	return hosts
}

func collectDestinationHosts(args map[string]any, hosts *[]string, depth int) {
	if depth > 8 {
		return
	}
	keys := make([]string, 0, len(args))
	for key := range args {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, field := range []string{
		"destinationurl", "destinationuri", "destination", "url", "uri", "endpoint", "webhook", "host", "hostname",
		"to", "recipient", "recipients", "cc", "bcc", "email", "emailaddress", "address", "channel", "contact",
	} {
		for _, key := range keys {
			normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(key))
			if normalized == field {
				collectDestinationValue(args[key], hosts, depth+1)
			}
		}
	}
}

func collectDestinationValue(value any, hosts *[]string, depth int) {
	if depth > 8 {
		return
	}
	switch value := value.(type) {
	case string:
		value = strings.TrimSpace(value)
		if strings.HasPrefix(strings.ToLower(value), "mailto:") {
			value = strings.TrimPrefix(value[len("mailto:"):], "//")
		}
		// Parse address lists before individual hosts. This retains every
		// recipient, including display-name forms and comma-separated lists.
		if addresses, err := mail.ParseAddressList(strings.ReplaceAll(value, ";", ",")); err == nil && len(addresses) > 0 {
			for _, address := range addresses {
				_, domain, ok := strings.Cut(address.Address, "@")
				if ok {
					appendDestinationHost(hosts, domain)
				}
			}
			return
		}
		explicitURL := strings.Contains(value, "://")
		if !explicitURL {
			value = "//" + value
		}
		if u, err := url.Parse(value); err == nil && u.Hostname() != "" && !strings.ContainsAny(u.Hostname(), " \t\n") {
			host := strings.ToLower(u.Hostname())
			if explicitURL || strings.Contains(host, ".") || host == "localhost" || strings.Contains(host, ":") {
				appendDestinationHost(hosts, host)
			}
		}
	case []any:
		for _, item := range value {
			collectDestinationValue(item, hosts, depth+1)
		}
	case []string:
		for _, item := range value {
			collectDestinationValue(item, hosts, depth+1)
		}
	case map[string]any:
		collectDestinationHosts(value, hosts, depth+1)
	}
}

func appendDestinationHost(hosts *[]string, host string) {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host != "" && !slices.Contains(*hosts, host) {
		*hosts = append(*hosts, host)
	}
}
