package guardrails

import (
	"slices"
	"testing"
)

// findings returns the label and text of every finding in text.
func findings(t *testing.T, c Checker, text string) []string {
	t.Helper()
	result, err := c.Check(t.Context(), CheckInput{Segments: []Segment{{Role: RoleUser, Text: text}}})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, f := range result.Findings {
		for _, s := range f.Spans {
			out = append(out, s.Label+"="+text[s.Start:s.End])
		}
	}
	return out
}

func TestSecretsDetection(t *testing.T) {
	const value = "Zr8Lx2Vw4Tn7Kp3Mb6Xc9Qa1Sd5Fg0Hj8Kl2Pq7Wz"
	tests := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "AWS credentials file",
			text: `aws_secret_access_key = "` + value + `"`,
			want: []string{"GENERIC_SECRET=" + value},
		},
		{
			name: "AWS environment variable",
			text: "AWS_SECRET_ACCESS_KEY=" + value,
			want: []string{"GENERIC_SECRET=" + value},
		},
		{
			name: "AWS secret access key of the real length",
			text: `aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY`,
			want: []string{"AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"},
		},
		{
			name: "client secret",
			text: `client_secret = "` + value + `"`,
			want: []string{"GENERIC_SECRET=" + value},
		},
		{
			name: "access key",
			text: `access_key = "` + value + `"`,
			want: []string{"GENERIC_SECRET=" + value},
		},
		{
			name: "secret in JSON",
			text: `{"client_secret": "` + value + `"}`,
			want: []string{"GENERIC_SECRET=" + value},
		},
		{
			name: "api key",
			text: "api_key: " + value,
			want: []string{"GENERIC_SECRET=" + value},
		},
		{
			name: "short value",
			text: `client_secret = "hunter2"`,
		},
		{
			name: "value with too little entropy",
			text: "max_tokens = 1000000000000000000",
		},
		{
			name: "variable reference",
			text: `aws_secret_access_key = "${AWS_SECRET_ACCESS_KEY}"`,
		},
		{
			name: "code reading the secret from the environment",
			text: `client_secret = os.environ["CLIENT_SECRET"]`,
		},
		{
			name: "prose about secrets",
			text: "Rotate your AWS secret access key and client secret every 90 days.",
		},
		{
			name: "code assigning a field",
			text: "ClientSecret: pendingState.ClientSecret, secret_key = settings.awsSecretKey",
		},
		{
			name: "camelCase name assigned a quoted literal",
			text: `clientSecret = "` + value + `"`,
			want: []string{"GENERIC_SECRET=" + value},
		},
		{
			name: "camelCase name in a Go struct literal",
			text: `ClientSecret: "` + value + `",`,
			want: []string{"GENERIC_SECRET=" + value},
		},
		{
			name: "camelCase name in JSON",
			text: `{"clientSecret": "` + value + `"}`,
			want: []string{"GENERIC_SECRET=" + value},
		},
		{
			name: "field assigned a quoted literal",
			text: `cfg.ClientSecret = "` + value + `"`,
			want: []string{"GENERIC_SECRET=" + value},
		},
		{
			name: "Go short variable declaration",
			text: `authToken := '` + value + `'`,
			want: []string{"GENERIC_SECRET=" + value},
		},
		{
			name: "subscripted environment assignment",
			text: `os.environ["AWS_SECRET_ACCESS_KEY"] = "` + value + `"`,
			want: []string{"GENERIC_SECRET=" + value},
		},
		{
			name: "subscripted assignment with a short name",
			text: `env["API_KEY"] = "` + value + `"`,
			want: []string{"GENERIC_SECRET=" + value},
		},
		{
			name: "camelCase name assigned a name in code",
			text: "clientSecret = Zr8Lx2Vw4Tn7Kp3Mb6Xc9Qa1Sd5Fg0Hj8Kl2Pq7Wz",
		},
		{
			name: "camelCase names assigned calls and fields",
			text: `clientSecret := loadSecret("prod"); cfg.ClientSecret = pendingState.ClientSecret`,
		},
		{
			name: "subscripted environment read",
			text: `apiKey = os.environ["OPENAI_API_KEY"]; env["API_KEY"] = defaultApiKey`,
		},
		{
			name: "camelCase name assigned a URL",
			text: `passwordResetUrl = "https://example.com/reset/a8f3k2"`,
		},
		{
			name: "a URL assigned to a secret-looking name",
			text: "password_reset_url = https://example.com/reset",
		},
	}
	c := &secretsChecker{generic: true, minEntropy: 3.5}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := findings(t, c, tt.text); !slices.Equal(got, tt.want) {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPIIPresets(t *testing.T) {
	tests := []struct {
		name   string
		preset string
		text   string
		want   []string
	}{
		{
			name:   "IBAN printed in groups",
			preset: "IBAN",
			text:   "IBAN GB82 WEST 1234 5698 7654 32 ok",
			want:   []string{"IBAN=GB82 WEST 1234 5698 7654 32"},
		},
		{
			name:   "IBAN without spaces",
			preset: "IBAN",
			text:   "IBAN GB82WEST12345698765432.",
			want:   []string{"IBAN=GB82WEST12345698765432"},
		},
		{
			name:   "grouped IBAN followed by a word in capitals",
			preset: "IBAN",
			text:   "DE89 3704 0044 0532 0130 00 THEN",
			want:   []string{"IBAN=DE89 3704 0044 0532 0130 00"},
		},
		{
			name:   "IBAN with wrong check digits",
			preset: "IBAN",
			text:   "IBAN GB83 WEST 1234 5698 7654 32",
		},
		{
			name:   "capitals that look like an IBAN",
			preset: "IBAN",
			text:   "AB12 THIS WORD SAYS HELLO",
		},
		{
			name:   "international phone number",
			preset: "PHONE",
			text:   "call +44 20 7946 0958 now",
			want:   []string{"PHONE=+44 20 7946 0958"},
		},
		{
			name:   "international phone number without spaces",
			preset: "PHONE",
			text:   "call +4915123456789",
			want:   []string{"PHONE=+4915123456789"},
		},
		{
			name:   "North American phone number",
			preset: "PHONE",
			text:   "call (415) 555-0132 or +1 415 555 0132",
			want:   []string{"PHONE=(415) 555-0132", "PHONE=+1 415 555 0132"},
		},
		{
			name:   "a plus sign before a small number",
			preset: "PHONE",
			text:   "score +12 points, version +3.2.1",
		},
		{
			name:   "a sum",
			preset: "PHONE",
			text:   "3+44207946095",
		},
		{
			name:   "IPv4 address",
			preset: "IP_ADDRESS",
			text:   "host 10.0.0.1",
			want:   []string{"IP_ADDRESS=10.0.0.1"},
		},
		{
			name:   "IPv6 addresses",
			preset: "IP_ADDRESS",
			text:   "or 2001:db8::1, fe80::1ff:fe23:4567:890a and ::1",
			want:   []string{"IP_ADDRESS=2001:db8::1", "IP_ADDRESS=fe80::1ff:fe23:4567:890a", "IP_ADDRESS=::1"},
		},
		{
			name:   "full IPv6 address",
			preset: "IP_ADDRESS",
			text:   "[2001:0db8:85a3:0000:0000:8a2e:0370:7334]:443",
			want:   []string{"IP_ADDRESS=2001:0db8:85a3:0000:0000:8a2e:0370:7334"},
		},
		{
			name:   "times, MAC addresses and C++ names",
			preset: "IP_ADDRESS",
			text:   "at 10:30:00, MAC 00:1a:2b:3c:4d:5e, std::cout, a::b::c, x :: Int",
		},
		{
			name:   "version strings with more than four parts",
			preset: "IP_ADDRESS",
			text:   "version 1.2.3.4.5 released, build 10.20.30.40.50, and 1.2.3.4.5.6.7.8",
		},
		{
			name:   "IPv4 address at the end of a sentence",
			preset: "IP_ADDRESS",
			text:   "Connect to 192.168.1.20. Then log in.",
			want:   []string{"IP_ADDRESS=192.168.1.20"},
		},
		{
			name:   "hex-looking names in code",
			preset: "IP_ADDRESS",
			text:   "use abc::def; and dead::beef, cafe::babe, a::b",
		},
		{
			name:   "short IPv6 addresses with digits",
			preset: "IP_ADDRESS",
			text:   "fe80::1 and 2001:db8::",
			want:   []string{"IP_ADDRESS=fe80::1", "IP_ADDRESS=2001:db8::"},
		},
		{
			name:   "a plus sign before a date",
			preset: "PHONE",
			text:   "he won +1 2026-10-17, and +3 17.10.2026",
		},
		{
			name:   "card number followed by a word",
			preset: "CREDIT_CARD",
			text:   "Pay with 4111 1111 1111 1111 today",
			want:   []string{"CREDIT_CARD=4111 1111 1111 1111"},
		},
		{
			name:   "card number with dashes",
			preset: "CREDIT_CARD",
			text:   "5500-0000-0000-0004 next",
			want:   []string{"CREDIT_CARD=5500-0000-0000-0004"},
		},
		{
			name:   "number that fails the checksum",
			preset: "CREDIT_CARD",
			text:   "1234 5678 9012 3456",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := newRegexChecker(map[string]any{"piiTemplate": []any{tt.preset}}, nil, Deps{})
			if err != nil {
				t.Fatal(err)
			}
			if got := findings(t, c, tt.text); !slices.Equal(got, tt.want) {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCreditCardRedactionKeepsWhitespace(t *testing.T) {
	c, err := newRegexChecker(map[string]any{"piiTemplate": []any{"CREDIT_CARD"}}, nil, Deps{})
	if err != nil {
		t.Fatal(err)
	}
	segments := []Segment{{Role: RoleUser, Text: "Pay with 4111 1111 1111 1111 today please"}}
	result, err := c.Check(t.Context(), CheckInput{Segments: segments})
	if err != nil {
		t.Fatal(err)
	}
	redacted, ok := redactResult(segments, result)
	if !ok {
		t.Fatal("the card number could not be redacted")
	}
	if got := redacted[0].Text; got != "Pay with [REDACTED:CREDIT_CARD] today please" {
		t.Fatalf("got %q", got)
	}
}
