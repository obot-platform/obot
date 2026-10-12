package scim

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/system"
)

const (
	concurrentRequests = 8
)

func TestConcurrencyAndRetriesSQLite(t *testing.T) {
	testConcurrencyAndRetries(t, newSCIMTest(t))
}

func TestConcurrencyAndRetriesPostgres(t *testing.T) {
	testConcurrencyAndRetries(t, newPostgresSCIMTest(t))
}

func testConcurrencyAndRetries(t *testing.T, s *scimTest) {
	t.Helper()

	existing := s.seedUser("00u-existing", "existing@example.com", types2.RoleBasic)
	s.enable()

	t.Run("concurrent creates of an existing user have one winner", func(t *testing.T) {
		statuses := s.concurrently(func(int) (string, string, any) {
			return http.MethodPost, "Users", scimUser("existing@example.com", "00u-existing")
		})
		assertOneWinner(t, statuses, http.StatusConflict)

		binding, err := s.gateway.SCIMUserBindingForUser(t.Context(), existing.ID)
		if err != nil || binding == nil {
			t.Fatalf("existing user not bound: %v", err)
		}
		found := s.do(http.MethodGet, "Users?filter="+url.QueryEscape(`userName eq "existing@example.com"`), nil).expect(t, http.StatusOK).resources()
		if len(found) != 1 || found[0]["id"] != binding.ID {
			t.Fatalf("lookup = %v, want %s", found, binding.ID)
		}
	})

	t.Run("concurrent creates of a new user create one user", func(t *testing.T) {
		users := s.count(new(types.User), "")
		statuses := s.concurrently(func(int) (string, string, any) {
			return http.MethodPost, "Users", scimUser("new@example.com", "00u-new")
		})
		assertOneWinner(t, statuses, http.StatusConflict)
		if got := s.count(new(types.User), ""); got != users+1 {
			t.Fatalf("got %d users, want %d", got, users+1)
		}
		if got := s.count(new(types.Identity), "provider_user_id = ?", "00u-new"); got != 1 {
			t.Fatalf("got %d identities, want 1", got)
		}
	})

	var memberIDs []string
	for i := range concurrentRequests {
		memberIDs = append(memberIDs, s.do(http.MethodPost, "Users", scimUser(fmt.Sprintf("member%d@example.com", i), fmt.Sprintf("00u-member-%d", i))).expect(t, http.StatusCreated).id())
	}

	t.Run("concurrent creates of a group have one winner", func(t *testing.T) {
		groups := s.count(new(types.Group), "")
		statuses := s.concurrently(func(int) (string, string, any) {
			return http.MethodPost, "Groups", scimGroup("Concurrent")
		})
		assertOneWinner(t, statuses, http.StatusConflict)
		if got := s.count(new(types.Group), ""); got != groups+1 {
			t.Fatalf("got %d groups, want %d", got, groups+1)
		}
	})

	t.Run("concurrent membership changes all apply", func(t *testing.T) {
		group := s.do(http.MethodPost, "Groups", scimGroup("Members")).expect(t, http.StatusCreated)
		statuses := s.concurrently(func(i int) (string, string, any) {
			return http.MethodPatch, "Groups/" + group.id(), patchOp(map[string]any{
				"op":   "add",
				"path": "members",
				"value": []any{
					map[string]any{
						"value": memberIDs[i],
					},
				},
			})
		})
		for _, status := range statuses {
			if status != http.StatusNoContent {
				t.Fatalf("statuses = %v", statuses)
			}
		}

		got := s.do(http.MethodGet, "Groups/"+group.id(), nil).expect(t, http.StatusOK).memberIDs()
		slices.Sort(got)
		want := slices.Clone(memberIDs)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Fatalf("members = %v, want %v", got, want)
		}

		// Repeating the adds changes nothing.
		events := s.count(new(types.UserLifecycleEvent), "")
		s.concurrently(func(i int) (string, string, any) {
			return http.MethodPatch, "Groups/" + group.id(), patchOp(map[string]any{
				"op":   "add",
				"path": "members",
				"value": []any{
					map[string]any{
						"value": memberIDs[i],
					},
				},
			})
		})
		if got := s.count(new(types.UserLifecycleEvent), ""); got != events {
			t.Fatalf("repeated adds recorded %d events", got-events)
		}
	})

	t.Run("a first sign-in racing its provisioning yields one bound user", func(t *testing.T) {
		const pairs = 20

		var wg sync.WaitGroup
		start := make(chan struct{})
		signInErrs := make([]error, pairs)
		statuses := make([]int, pairs)
		for i := range pairs {
			nativeID := fmt.Sprintf("00u-race-%d", i)
			email := fmt.Sprintf("race%d@example.com", i)

			wg.Go(func() {
				<-start
				_, signInErrs[i] = s.gateway.EnsureIdentity(s.t.Context(), &types.Identity{
					AuthProviderNamespace: system.DefaultNamespace,
					AuthProviderName:      testOktaProviderName,
					ProviderUsername:      nativeID,
					ProviderUserID:        nativeID,
					Email:                 email,
				}, "", gclient.UserLimit{
					Unlimited: true,
				})
			})

			data, _ := json.Marshal(scimUser(email, nativeID))
			req := httptest.NewRequestWithContext(s.t.Context(), http.MethodPost, s.path("Users"), strings.NewReader(string(data)))
			req.Header.Set("Content-Type", "application/scim+json")
			req.Header.Set("Authorization", "Bearer "+s.token)
			wg.Go(func() {
				rec := httptest.NewRecorder()
				<-start
				s.serve(rec, req)
				statuses[i] = rec.Code
			})
		}
		close(start)
		wg.Wait()

		for i := range pairs {
			nativeID := fmt.Sprintf("00u-race-%d", i)
			if signInErrs[i] != nil {
				t.Errorf("sign-in of %s failed: %v", nativeID, signInErrs[i])
			}
			if statuses[i] != http.StatusCreated {
				t.Errorf("provisioning %s answered %d, want 201", nativeID, statuses[i])
			}

			var userIDs []uint
			if err := s.gorm().Model(new(types.Identity)).Where("provider_user_id = ?", nativeID).Pluck("user_id", &userIDs).Error; err != nil {
				t.Fatal(err)
			}
			if len(userIDs) != 1 {
				t.Fatalf("%s has %d identities, want 1", nativeID, len(userIDs))
			}
			if n := s.count(new(types.User), "username = ? AND deleted_at IS NULL", nativeID); n != 1 {
				t.Errorf("%s has %d users, want 1", nativeID, n)
			}
			if binding, err := s.gateway.SCIMUserBindingForUser(t.Context(), userIDs[0]); err != nil || binding == nil {
				t.Errorf("the user of %s is not bound: %v", nativeID, err)
			}
		}
	})

	t.Run("seat-limit contention admits exactly the free seats", func(t *testing.T) {
		users := s.count(new(types.User), "deleted_at IS NULL")
		s.env.userLimit = gclient.UserLimit{
			Maximum: users + 2,
		}
		defer func() {
			s.env.userLimit = gclient.UserLimit{
				Unlimited: true,
			}
		}()

		statuses := s.concurrently(func(i int) (string, string, any) {
			return http.MethodPost, "Users", scimUser(fmt.Sprintf("seat%d@example.com", i), fmt.Sprintf("00u-seat-%d", i))
		})
		var created, refused int
		for _, status := range statuses {
			switch status {
			case http.StatusCreated:
				created++
			case http.StatusForbidden:
				refused++
			default:
				t.Fatalf("statuses = %v", statuses)
			}
		}
		if created != 2 || refused != concurrentRequests-2 {
			t.Fatalf("created %d and refused %d, want 2 and %d", created, refused, concurrentRequests-2)
		}
	})
}

// concurrently sends one request per index at the same time and returns their statuses.
func (s *scimTest) concurrently(request func(int) (string, string, any)) []int {
	statuses := make([]int, concurrentRequests)
	start := make(chan struct{})

	var wg sync.WaitGroup
	for i := range concurrentRequests {
		method, resource, body := request(i)
		data, _ := json.Marshal(body)

		wg.Go(func() {
			req := httptest.NewRequestWithContext(s.t.Context(), method, s.path(resource), strings.NewReader(string(data)))
			req.Header.Set("Content-Type", "application/scim+json")
			req.Header.Set("Authorization", "Bearer "+s.token)
			rec := httptest.NewRecorder()

			<-start
			s.serve(rec, req)
			statuses[i] = rec.Code
		})
	}
	close(start)
	wg.Wait()
	return statuses
}

func assertOneWinner(t *testing.T, statuses []int, loser int) {
	t.Helper()

	var winners int
	for _, status := range statuses {
		switch status {
		case http.StatusCreated:
			winners++
		case loser:
		default:
			t.Fatalf("statuses = %v", statuses)
		}
	}
	if winners != 1 {
		t.Fatalf("%d requests won, want 1: %v", winners, statuses)
	}
}
