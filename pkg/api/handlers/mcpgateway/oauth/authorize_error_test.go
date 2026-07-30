package oauth

import "testing"

func TestOAuthErrorDescriptionQueryIsRFC6749Safe(t *testing.T) {
	t.Parallel()

	const description = "Obot: failed to get client: tunnel \"mt1vtsmx\" is not connected\n\n" +
		"failed to connect to SSE server url http://127.0.0.1:8080/tunnel/bridge: " +
		"503 Service Unavailable, tunnel \"mt1vtsmx\" is not connected\n"

	got := (oauthError{
		Code:        ErrServerError,
		Description: description,
	}).toQuery().Get("error_description")
	want := "Obot: failed to get client: tunnel 'mt1vtsmx' is not connected " +
		"failed to connect to SSE server url http://127.0.0.1:8080/tunnel/bridge: " +
		"503 Service Unavailable, tunnel 'mt1vtsmx' is not connected"
	if got != want {
		t.Fatalf("error description = %q, want %q", got, want)
	}

	for _, r := range got {
		if r == 0x20 || r == 0x21 || r >= 0x23 && r <= 0x5B || r >= 0x5D && r <= 0x7E {
			continue
		}
		t.Fatalf("error description contains disallowed OAuth character %q", r)
	}
}
