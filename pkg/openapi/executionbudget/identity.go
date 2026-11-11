package executionbudget

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/idna"
	"golang.org/x/text/unicode/norm"
)

type CatalogReference struct {
	SourceURL string
	EntryKey  string
}

type BudgetIdentity struct {
	BudgetSourceHash string
	OperationKeyHash string
	BudgetKeyHash    string
}

func ResolveBudgetIdentity(primary, snapshotFallback []CatalogReference, operationKey string) (BudgetIdentity, error) {
	references := primary
	if len(references) == 0 {
		references = snapshotFallback
	}
	if len(references) == 0 {
		return BudgetIdentity{}, ErrBudgetIdentityUnavailable
	}

	seen := make(map[string]struct{}, len(references))
	var canonical string
	for _, reference := range references {
		current, err := canonicalCatalogReference(reference)
		if err != nil {
			return BudgetIdentity{}, err
		}
		if _, exists := seen[current]; exists {
			continue
		}
		seen[current] = struct{}{}
		if canonical == "" {
			canonical = current
			continue
		}
		if current != canonical {
			return BudgetIdentity{}, fmt.Errorf("%w: multiple catalog references", ErrBudgetIdentityUnavailable)
		}
	}
	if canonical == "" {
		return BudgetIdentity{}, ErrBudgetIdentityUnavailable
	}

	sourceHash := hashPrefixed("openapi-budget-source-v1", canonical)
	operationHash := hashHex(operationKey)
	return BudgetIdentity{
		BudgetSourceHash: sourceHash,
		OperationKeyHash: operationHash,
		BudgetKeyHash:    hashPrefixed("openapi-budget-key-v1", sourceHash+"\n"+operationHash),
	}, nil
}

func canonicalCatalogReference(reference CatalogReference) (string, error) {
	entryKey, err := canonicalEntryKey(reference.EntryKey)
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(reference.SourceURL)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("%w: invalid source URL", ErrBudgetIdentityUnavailable)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("%w: userinfo, query, and fragment are not identity data", ErrBudgetIdentityUnavailable)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("%w: source scheme must be http or https", ErrBudgetIdentityUnavailable)
	}
	host, err := canonicalHost(parsed.Hostname())
	if err != nil {
		return "", err
	}
	port := parsed.Port()
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
	}
	if port != "" {
		host += ":" + port
	}
	path := canonicalPath(parsed.EscapedPath())
	return scheme + "://" + host + "/" + path + "::" + entryKey, nil
}

func canonicalEntryKey(value string) (string, error) {
	entryKey := norm.NFC.String(strings.TrimSpace(value))
	if entryKey == "" || strings.ContainsAny(entryKey, "\r\n") || strings.Contains(entryKey, "::") {
		return "", fmt.Errorf("%w: invalid catalog entry key", ErrBudgetIdentityUnavailable)
	}
	return entryKey, nil
}

func canonicalHost(value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("%w: source host is empty", ErrBudgetIdentityUnavailable)
	}
	host, err := idna.Lookup.ToASCII(strings.TrimSuffix(strings.ToLower(value), "."))
	if err != nil || strings.Contains(host, " ") {
		return "", fmt.Errorf("%w: invalid source host", ErrBudgetIdentityUnavailable)
	}
	return host, nil
}

func canonicalPath(value string) string {
	segments := strings.Split(value, "/")
	var output []string
	for _, segment := range segments {
		segment = canonicalPathSegment(segment)
		if strings.EqualFold(segment, ".") || segment == "" {
			continue
		}
		if strings.EqualFold(segment, "..") {
			if len(output) > 0 {
				output = output[:len(output)-1]
			}
			continue
		}
		output = append(output, segment)
	}
	if len(output) == 0 {
		return ""
	}
	return strings.Join(output, "/")
}

func canonicalPathSegment(value string) string {
	decoded := make([]byte, 0, len(value))
	for index := 0; index < len(value); {
		char := value[index]
		if char != '%' {
			decoded = append(decoded, char)
			index++
			continue
		}
		if index+2 >= len(value) || !isHexDigit(value[index+1]) || !isHexDigit(value[index+2]) {
			return value
		}
		byteValue := hexByteValue(value[index+1])<<4 | hexByteValue(value[index+2])
		if isUnreserved(byteValue) {
			decoded = append(decoded, byteValue)
		} else {
			decoded = append(decoded, '%', upperHexDigit(byteValue>>4), upperHexDigit(byteValue&0x0f))
		}
		index += 3
	}

	encoded := make([]byte, 0, len(decoded))
	for _, byteValue := range decoded {
		if isPathAllowed(byteValue) {
			encoded = append(encoded, byteValue)
			continue
		}
		encoded = append(encoded, '%', upperHexDigit(byteValue>>4), upperHexDigit(byteValue&0x0f))
	}
	return string(encoded)
}

func isHexDigit(char byte) bool {
	return char >= '0' && char <= '9' || char >= 'a' && char <= 'f' || char >= 'A' && char <= 'F'
}

func hexByteValue(char byte) byte {
	switch {
	case char >= '0' && char <= '9':
		return char - '0'
	case char >= 'a' && char <= 'f':
		return char - 'a' + 10
	default:
		return char - 'A' + 10
	}
}

func upperHexDigit(value byte) byte {
	if value < 10 {
		return '0' + value
	}
	return 'A' + value - 10
}

func isUnreserved(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z' ||
		value >= '0' && value <= '9' || value == '-' || value == '.' || value == '_' || value == '~'
}

func isPathAllowed(value byte) bool {
	return isUnreserved(value) || value == '/' || value == ':' || value == '@' ||
		strings.IndexByte("!$&'()*+,;=", value) >= 0
}

func hashPrefixed(prefix, value string) string {
	return hashHex(prefix + "\x00" + value)
}

func hashHex(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
