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
		{
			name: "plain text",
			text: "Hello world",
		},
		{
			name: "bold",
			text: "**important** notice",
		},
		{
			name: "italic underscore",
			text: "_emphasis_ here",
		},
		{
			name: "italic asterisk",
			text: "*emphasis* here",
		},
		{
			name: "strikethrough",
			text: "~~gone~~ now",
		},
		{
			name: "plain text without special chars",
			text: "no special chars here",
		},
		{
			name: "http link",
			text: "[click](http://example.com)",
		},
		{
			name: "https link",
			text: "[click](https://example.com/a/b?c=d#e)",
		},
		{
			name: "multiple links",
			text: "[a](https://a.com) and [b](https://b.com)",
		},
		{
			name: "hyphen mid-line is not a list",
			text: "well-known issue is resolved",
		},
		{
			name: "asterisk mid-line",
			text: "5 * 5 equals 25",
		},
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
		{
			name: "fenced code block",
			text: "```code```",
		},
		{
			name: "image",
			text: "![alt](https://example.com/img.png)",
		},
		{
			name: "html tag",
			text: "<b>bold</b>",
		},
		{
			name: "self closing html tag",
			text: "<br/>",
		},
		{
			name: "heading",
			text: "# Heading",
		},
		{
			name: "heading indented",
			text: "   ## Heading",
		},
		{
			name: "blockquote",
			text: "> quoted",
		},
		{
			name: "unordered list dash",
			text: "- item",
		},
		{
			name: "unordered list asterisk",
			text: "* item",
		},
		{
			name: "unordered list plus",
			text: "+ item",
		},
		{
			name: "ordered list",
			text: "1. item",
		},
		{
			name: "horizontal rule dashes",
			text: "---",
		},
		{
			name: "horizontal rule stars",
			text: "***",
		},
		{
			name: "reference style link",
			text: "[text][ref]",
		},
		{
			name: "table row",
			text: "| a | b |",
		},
		{
			name: "link with non http scheme",
			text: "[click](ftp://example.com)",
		},
		{
			name: "link with javascript scheme",
			text: "[click](javascript:alert(1))",
		},
		{
			name: "link with relative url",
			text: "[click](/relative/path)",
		},
		{
			name: "link with empty label",
			text: "[   ](https://example.com)",
		},
		{
			name: "backslash outside link",
			text: "escaped \\* not a list",
		},
		{
			name: "backtick outside link",
			text: "use `code` here",
		},
	}
	for _, tt := range invalidTexts {
		t.Run("invalid/"+tt.name, func(t *testing.T) {
			err := validateBannerText(tt.text)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "banner text only supports")
		})
	}
}
