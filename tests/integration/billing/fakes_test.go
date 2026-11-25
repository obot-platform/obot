//go:build integration

package billing

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
)

// fakeBilling answers obot-billing's API for Obot with a subscription the tests change.
type fakeBilling struct {
	server *httptest.Server
	key    string

	mu           sync.Mutex
	subscription types.Subscription
	plans        int
	// failure, if set, answers every request instead.
	failure http.HandlerFunc
	// unauthorized counts requests without the billing key, and returnURL is the last session's.
	unauthorized int
	returnURL    string
}

func startFakeBilling(key string) *fakeBilling {
	f := &fakeBilling{
		key: key,
	}
	f.set(types.Entitlements{})
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	return f
}

// set makes entitlements what billing grants from now on, for a period that ends in the future.
func (f *fakeBilling) set(entitlements types.Entitlements) string {
	return f.answer(types.Subscription{
		Status:       "active",
		PeriodEnd:    types.NewTime(time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)),
		SelfService:  true,
		Entitlements: entitlements,
	})
}

// answer makes subscription what billing answers from now on, under a plan name of its own, which it returns. Obot
// shows the name once it has synced the answer.
func (f *fakeBilling) answer(subscription types.Subscription) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.plans++
	f.failure = nil
	subscription.Plan = fmt.Sprintf("Plan %d", f.plans)
	f.subscription = subscription
	return subscription.Plan
}

// fail makes billing answer every request with failure until the next set.
func (f *fakeBilling) fail(failure http.HandlerFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failure = failure
}

func (f *fakeBilling) current() types.Subscription {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.subscription
}

func (f *fakeBilling) unauthorizedRequests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.unauthorized
}

func (f *fakeBilling) lastReturnURL() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.returnURL
}

func (f *fakeBilling) serve(rw http.ResponseWriter, req *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if req.Header.Get("Authorization") != "Bearer "+f.key {
		f.unauthorized++
		http.Error(rw, `{"error":{"message":"unknown billing key"}}`, http.StatusUnauthorized)
		return
	}
	if f.failure != nil {
		f.failure(rw, req)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	switch req.Method + " " + req.URL.Path {
	case "GET /api/v1/subscription":
		_ = json.NewEncoder(rw).Encode(f.subscription)
	case "POST /api/v1/sessions":
		var in struct {
			ReturnURL string `json:"returnUrl"`
		}
		_ = json.NewDecoder(req.Body).Decode(&in)
		f.returnURL = in.ReturnURL
		rw.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(rw).Encode(map[string]string{"url": f.server.URL + "/session/one-time"})
	default:
		http.NotFound(rw, req)
	}
}
