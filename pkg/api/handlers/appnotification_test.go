package handlers

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateBanner(t *testing.T) {
	tests := []struct {
		name    string
		banner  types.BannerNotification
		wantErr bool
	}{
		{
			name:    "disabled banner still rejects invalid text",
			banner:  types.BannerNotification{Enabled: false, Text: "# heading with ```code```"},
			wantErr: true,
		},
		{
			name:    "disabled banner allows empty text",
			banner:  types.BannerNotification{Enabled: false, Text: ""},
			wantErr: false,
		},
		{
			name:    "disabled banner allows valid text without type",
			banner:  types.BannerNotification{Enabled: false, Text: "**heads up** [docs](https://example.com)"},
			wantErr: false,
		},
		{
			name:    "enabled banner requires text",
			banner:  types.BannerNotification{Enabled: true, Type: types.BannerTypeInfo, Text: ""},
			wantErr: true,
		},
		{
			name:    "enabled banner requires whitespace-only text be treated as empty",
			banner:  types.BannerNotification{Enabled: true, Type: types.BannerTypeInfo, Text: "   "},
			wantErr: true,
		},
		{
			name:    "enabled banner requires type",
			banner:  types.BannerNotification{Enabled: true, Type: "", Text: "hello"},
			wantErr: true,
		},
		{
			name:    "valid plain text",
			banner:  types.BannerNotification{Enabled: true, Type: types.BannerTypeWarning, Text: "Scheduled maintenance tonight."},
			wantErr: false,
		},
		{
			name:    "valid inline formatting and http link",
			banner:  types.BannerNotification{Enabled: true, Type: types.BannerTypeInfo, Text: "**Bold** _italic_ ~~strike~~ see [docs](https://example.com)"},
			wantErr: false,
		},
		{
			name:    "valid http link",
			banner:  types.BannerNotification{Enabled: true, Type: types.BannerTypeInfo, Text: "Read [more](http://example.com/path?x=1)"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBanner(tt.banner)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateBannerText(t *testing.T) {
	validTexts := []struct {
		name string
		text string
	}{
		{"plain text", "Hello world"},
		{"bold", "**important** notice"},
		{"italic underscore", "_emphasis_ here"},
		{"italic asterisk", "*emphasis* here"},
		{"strikethrough", "~~gone~~ now"},
		{"plain text without special chars", "no special chars here"},
		{"http link", "[click](http://example.com)"},
		{"https link", "[click](https://example.com/a/b?c=d#e)"},
		{"multiple links", "[a](https://a.com) and [b](https://b.com)"},
		{"hyphen mid-line is not a list", "well-known issue is resolved"},
		{"asterisk mid-line", "5 * 5 equals 25"},
	}
	for _, tt := range validTexts {
		t.Run("valid/"+tt.name, func(t *testing.T) {
			assert.NoError(t, validateBannerText(tt.text))
		})
	}

	invalidTexts := []struct {
		name string
		text string
	}{
		{"fenced code block", "```code```"},
		{"image", "![alt](https://example.com/img.png)"},
		{"html tag", "<b>bold</b>"},
		{"self closing html tag", "<br/>"},
		{"heading", "# Heading"},
		{"heading indented", "   ## Heading"},
		{"blockquote", "> quoted"},
		{"unordered list dash", "- item"},
		{"unordered list asterisk", "* item"},
		{"unordered list plus", "+ item"},
		{"ordered list", "1. item"},
		{"horizontal rule dashes", "---"},
		{"horizontal rule stars", "***"},
		{"reference style link", "[text][ref]"},
		{"table row", "| a | b |"},
		{"link with non http scheme", "[click](ftp://example.com)"},
		{"link with javascript scheme", "[click](javascript:alert(1))"},
		{"link with relative url", "[click](/relative/path)"},
		{"link with empty label", "[   ](https://example.com)"},
		{"backslash outside link", "escaped \\* not a list"},
		{"backtick outside link", "use `code` here"},
	}
	for _, tt := range invalidTexts {
		t.Run("invalid/"+tt.name, func(t *testing.T) {
			err := validateBannerText(tt.text)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "banner text only supports")
		})
	}
}
