package openapiworker

import (
	"net/http"
	"strings"
)

var protectedHeaders = map[string]struct{}{
	"authorization":                       {},
	"x-api-key":                           {},
	"x-obot-api-key":                      {},
	strings.ToLower(workerAuthHeaderName): {},
	"cookie":                              {},
	"set-cookie":                          {},
	"host":                                {},
	"content-length":                      {},
	"transfer-encoding":                   {},
	"connection":                          {},
	"keep-alive":                          {},
	"proxy-authenticate":                  {},
	"proxy-authorization":                 {},
	"proxy-connection":                    {},
	"te":                                  {},
	"trailer":                             {},
	"upgrade":                             {},
	"via":                                 {},
	"forwarded":                           {},
	"x-obot-error-code":                   {},
}

func SanitizeInputHeaders(input http.Header, policies []HeaderPolicy) http.Header {
	result := http.Header{}
	for name, values := range input {
		lower := strings.ToLower(name)
		if IsProtectedHeader(lower) {
			continue
		}
		forwardable := false
		for _, policy := range policies {
			if strings.EqualFold(policy.Name, name) && policy.Forwardable && !policy.Sensitive && !policy.Credential {
				forwardable = true
				break
			}
		}
		if forwardable {
			result[name] = append([]string(nil), values...)
		}
	}
	return result
}

func IsProtectedHeader(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if _, exists := protectedHeaders[lower]; exists {
		return true
	}
	return strings.HasPrefix(lower, "x-obot-") || strings.HasPrefix(lower, "x-forwarded-")
}
