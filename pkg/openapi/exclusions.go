package openapi

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

// Exclusions are validated before deployment; only the wrapper applies them.
var methods = map[string]bool{
	"GET": true, "PUT": true, "POST": true, "DELETE": true,
	"OPTIONS": true, "HEAD": true, "PATCH": true, "TRACE": true,
}

// CompilePathPattern deliberately accepts a portable Go/Python regex subset:
// literals, groups, alternation, classes, quantifiers, anchors, and escaped
// punctuation. Engine-specific escapes, special groups, and POSIX classes are
// rejected so validation does not accept syntax incompatible with FastMCP.
func CompilePathPattern(pattern string) (*regexp.Regexp, error) {
	if strings.Contains(pattern, "(?") || strings.Contains(pattern, "[:") {
		return nil, fmt.Errorf("pathPattern uses unsupported regex syntax")
	}
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '\\' && i+1 < len(pattern) {
			i++
			if (pattern[i] >= 'a' && pattern[i] <= 'z') || (pattern[i] >= 'A' && pattern[i] <= 'Z') || (pattern[i] >= '0' && pattern[i] <= '9') {
				return nil, fmt.Errorf("pathPattern uses an unsupported regex escape")
			}
		}
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pathPattern regular expression")
	}
	return re, nil
}

func validateExclusions(config types.OpenAPIRuntimeConfig) error {
	if _, err := (wrapperSettings{
		BaseURL:    config.BaseURL,
		ToolSearch: config.ToolSearch,
		Exclude:    config.Exclude,
	}).marshal(); err != nil {
		return err
	}
	if len(config.Exclude) > 0 && !config.ToolSearch {
		return fmt.Errorf("exclusions require toolSearch=true")
	}
	for i, rule := range config.Exclude {
		if rule.Method == "" && rule.PathPattern == "" && rule.Tag == "" {
			return fmt.Errorf("exclude[%d] must contain a method, pathPattern, or tag", i)
		}
		if rule.Method != "" && !methods[strings.ToUpper(rule.Method)] {
			return fmt.Errorf("exclude[%d].method must be a supported HTTP method", i)
		}
		if rule.Tag != "" && strings.TrimSpace(rule.Tag) == "" {
			return fmt.Errorf("exclude[%d].tag must not be blank", i)
		}
		if rule.PathPattern != "" {
			if strings.TrimSpace(rule.PathPattern) == "" {
				return fmt.Errorf("exclude[%d].pathPattern must not be blank", i)
			}
			if _, err := CompilePathPattern(rule.PathPattern); err != nil {
				return fmt.Errorf("exclude[%d]: %w", i, err)
			}
		}
	}
	return nil
}
