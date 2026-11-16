//go:build integration

package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
)

// NewSession returns a Harness that keeps cookies and doesn't follow redirects, for an app with authentication. It's
// anonymous until SignIn.
func NewSession(t *testing.T) *Harness {
	t.Helper()
	h := New(t)
	h.HTTP = newSessionClient()
	return h
}

// SignIn signs the session in with local auth and returns the status and body of the session's first request as the
// user, GET /api/me. That request is where Obot creates the user's account, so it's where a sign-in is refused.
func (h *Harness) SignIn(t *testing.T, email, password string) (int, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), requestTimeout)
	defer cancel()
	if err := signIn(ctx, h.HTTP, h.BaseURL, email, password); err != nil {
		t.Fatalf("sign in as %s: %v", email, err)
	}
	return h.Request(t, http.MethodGet, "/api/me", nil)
}

// ActivateOwner sets the initial local-auth owner's password with its setup token. TestMain calls it once, before
// any session signs in as the owner.
func ActivateOwner(ctx context.Context, baseURL, setupToken, password string) error {
	client := newSessionClient()
	for _, step := range []struct {
		path string
		body map[string]string
	}{
		{
			path: "/api/local-auth/activate",
			body: map[string]string{"setupToken": setupToken},
		},
		{
			path: "/api/local-auth/change-password",
			body: map[string]string{"password": password},
		},
	} {
		data, err := json.Marshal(step.body)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+step.path, bytes.NewReader(data))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("POST %s: %d %s", step.path, resp.StatusCode, body)
		}
	}
	return nil
}

func newSessionClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// signIn submits the local-auth login form, which sets the session cookie in client's jar.
func signIn(ctx context.Context, client *http.Client, baseURL, email, password string) error {
	start, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/oauth2/start?rd=/", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(start)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	form := url.Values{"email": {email}, "password": {password}, "rd": {"/"}}
	submit, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/oauth2/start", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	submit.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err = client.Do(submit)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return nil
}
