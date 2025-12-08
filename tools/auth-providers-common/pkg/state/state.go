package state

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	oauth2proxy "github.com/oauth2-proxy/oauth2-proxy/v7"
)

type GroupInfo struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	IconURL *string `json:"iconURL,omitempty"`
}

type GroupInfoList []GroupInfo

func (a GroupInfoList) IDs() []string {
	ids := make([]string, len(a))
	for i, group := range a {
		ids[i] = group.ID
	}
	return ids
}

type SerializableRequest struct {
	Method string              `json:"method"`
	URL    string              `json:"url"`
	Header map[string][]string `json:"header"`
}

type SerializableState struct {
	ExpiresOn         *time.Time    `json:"expiresOn"`
	AccessToken       string        `json:"accessToken"`
	IDToken           string        `json:"idToken"`
	PreferredUsername string        `json:"preferredUsername"`
	User              string        `json:"user"`
	Email             string        `json:"email"`
	Groups            []string      `json:"groups"`
	GroupInfos        GroupInfoList `json:"groupInfos"`
	SetCookies        []string      `json:"setCookies"`
}

func ObotGetState(p *oauth2proxy.OAuthProxy) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var sr SerializableRequest
		if err := json.NewDecoder(r.Body).Decode(&sr); err != nil {
			http.Error(w, fmt.Sprintf("failed to decode request body: %v", err), http.StatusBadRequest)
			return
		}

		reqObj, err := http.NewRequest(sr.Method, sr.URL, nil)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create request object: %v", err), http.StatusBadRequest)
			return
		}

		reqObj.Header = sr.Header

		ss, err := GetSerializableState(p, reqObj)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get state: %v", err), http.StatusInternalServerError)
			return
		}

		if err = json.NewEncoder(w).Encode(ss); err != nil {
			http.Error(w, fmt.Sprintf("failed to encode state: %v", err), http.StatusInternalServerError)
			return
		}
	}
}

func GetSerializableState(p *oauth2proxy.OAuthProxy, r *http.Request) (SerializableState, error) {
	state, err := p.LoadCookiedSession(r)
	if err != nil {
		return SerializableState{}, fmt.Errorf("failed to load cookied session: %v", err)
	}

	if state == nil {
		return SerializableState{}, fmt.Errorf("state is nil")
	}

	var setCookies []string
	if state.IsExpired() || (p.CookieOptions.Refresh != 0 && state.Age() > p.CookieOptions.Refresh) {
		setCookies, err = refreshToken(p, r)
		if err != nil {
			return SerializableState{}, fmt.Errorf("failed to refresh token: %v", err)
		}
	}

	return SerializableState{
		ExpiresOn:         state.ExpiresOn,
		AccessToken:       state.AccessToken,
		IDToken:           state.IDToken,
		PreferredUsername: state.PreferredUsername,
		User:              state.User,
		Email:             state.Email,
		Groups:            state.Groups,
		GroupInfos:        GroupInfoList{},
		SetCookies:        setCookies,
	}, nil
}

func refreshToken(p *oauth2proxy.OAuthProxy, r *http.Request) ([]string, error) {
	w := &response{
		headers: make(http.Header),
	}

	req, err := http.NewRequest(r.Method, "/oauth2/auth", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh request object: %v", err)
	}

	req.Header = r.Header
	p.ServeHTTP(w, req)

	switch w.status {
	case http.StatusOK, http.StatusAccepted:
		var headers []string
		for _, v := range w.Header().Values("Set-Cookie") {
			headers = append(headers, v)
		}
		return headers, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("refreshing token returned %d: %s", w.status, w.body)
	default:
		return nil, fmt.Errorf("refreshing token returned unexpected status %d: %s", w.status, w.body)
	}
}

type response struct {
	headers http.Header
	body    []byte
	status  int
}

func (r *response) Header() http.Header {
	return r.headers
}

func (r *response) Write(b []byte) (int, error) {
	r.body = append(r.body, b...)
	return len(b), nil
}

func (r *response) WriteHeader(status int) {
	r.status = status
}
