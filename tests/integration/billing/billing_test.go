//go:build integration

package billing

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/authn"
	"github.com/obot-platform/obot/pkg/api/handlers"
	"github.com/obot-platform/obot/pkg/license"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/tests/integration/harness"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	memoryRefusal = "There isn't room in the plan's hosted MCP server memory for this. Free some up, or ask an owner to change the plan."
	cpuRefusal    = "There isn't room in the plan's hosted MCP server CPU for this. Free some up, or ask an owner to change the plan."
	// A refused sign-in is answered as unauthenticated, with the refusal it was given inside.
	userRefusal   = "error code 403 (Forbidden): Unable to provision your account. Please contact your administrator."
	deviceRefusal = "Unable to enroll your device. Please contact your administrator."

	memberPassword = "Integration-Member-Passw0rd!"

	// Every hosted server in these tests requests this much.
	serverMemoryMiB     = 256
	serverCPUMillicores = 100
)

// version is what the tests read of GET /api/version.
type version struct {
	BillingEnabled bool                `json:"billingEnabled"`
	Cloud          bool                `json:"cloud"`
	UserCount      int64               `json:"userCount"`
	UserLimit      int64               `json:"userLimit"`
	DeviceCount    int64               `json:"deviceCount"`
	DeviceLimit    int64               `json:"deviceLimit"`
	Violations     []license.Violation `json:"licenseEntitlementViolations"`
}

func TestStartsWithBilling(t *testing.T) {
	owner := signInOwner(t)
	granted := types.Entitlements{
		EnterpriseAuthProviders: true,
		Users:                   new(int64(50)),
		Devices:                 new(int64(20)),
		MemoryMiB:               new(int64(4096)),
		CPUMillicores:           new(int64(2000)),
		AuditLogRetentionDays:   new(int64(30)),
	}
	status := setPlan(t, owner, granted)

	want := granted
	want.Cloud = true
	requireJSONEqual(t, "entitlements", status.Entitlements, want)
	// What the plan grants is shown once, so the subscription leaves it out.
	answer := billing.current()
	if got := status.Subscription; got == nil || got.Plan != answer.Plan || got.Status != answer.Status ||
		!got.PeriodEnd.GetTime().Equal(answer.PeriodEnd.GetTime()) || got.CancelAtPeriodEnd != answer.CancelAtPeriodEnd || got.SelfService != answer.SelfService {
		t.Fatalf("subscription = %+v, want billing's answer %+v", got, answer)
	}
	var shown map[string]json.RawMessage
	owner.Get(t, "/api/license", &shown)
	var subscription map[string]json.RawMessage
	if err := json.Unmarshal(shown["subscription"], &subscription); err != nil {
		t.Fatalf("decode the subscription %s: %v", shown["subscription"], err)
	}
	if _, ok := subscription["entitlements"]; ok {
		t.Fatalf("subscription = %s, want it without entitlements", shown["subscription"])
	}
	// A billed environment has no license, so nothing about one is shown.
	for _, field := range []string{"licenseKey", "source", "locked", "enterprise"} {
		if value, ok := shown[field]; ok {
			t.Errorf("billing status has %s = %s, want it left out", field, value)
		}
	}
	if status.SyncError != "" || status.Syncing {
		t.Fatalf("billing status = %+v, want it synced and without a sync error", status)
	}
	if status.Usage == nil || status.Usage.Users < 1 {
		t.Fatalf("usage = %+v, want at least the owner", status.Usage)
	}
	if billing.unauthorizedRequests() != 0 {
		t.Fatalf("billing got %d requests without the billing key", billing.unauthorizedRequests())
	}

	version := getVersion(t, owner)
	if version.BillingEnabled != true || version.Cloud != true || version.UserLimit != 50 || version.DeviceLimit != 20 || len(version.Violations) != 0 {
		t.Fatalf("version = %+v, want billing on, the plan's limits and no violations", version)
	}

	t.Run("an owner opens the billing pages", func(t *testing.T) {
		var session map[string]string
		owner.Post(t, "/api/license/manage", nil, &session)
		if want := billing.server.URL + "/session/one-time"; session["url"] != want {
			t.Fatalf("session URL = %q, want %q", session["url"], want)
		}
		// Obot's own URL is its hostname, which defaults to localhost.
		want := strings.Replace(owner.BaseURL, "127.0.0.1", "localhost", 1) + "/admin/platform?billing=returned"
		if got := billing.lastReturnURL(); got != want {
			t.Fatalf("return URL = %q, want %q", got, want)
		}
	})

	t.Run("the license can't be changed", func(t *testing.T) {
		for _, route := range []struct {
			method string
			path   string
		}{
			{
				method: http.MethodPut,
				path:   "/api/license",
			},
			{
				method: http.MethodDelete,
				path:   "/api/license",
			},
			{
				method: http.MethodPost,
				path:   "/api/license/community",
			},
		} {
			if status, body := owner.Request(t, route.method, route.path, map[string]string{"licenseKey": "key/test"}); status != http.StatusNotFound && status != http.StatusMethodNotAllowed {
				t.Errorf("%s %s = %d %s, want it off", route.method, route.path, status, body)
			}
		}
	})
}

func TestComputeLimits(t *testing.T) {
	owner := signInOwner(t)
	hosted := createCatalogEntry(t, owner, "hosted", types.MCPServerCatalogEntryManifest{
		Runtime: types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{
			Package: "@modelcontextprotocol/server-everything",
		},
		Resources: &types.MCPResourceRequirements{
			Requests: types.MCPResourceRequests{
				Memory: fmt.Sprintf("%dMi", serverMemoryMiB),
				CPU:    fmt.Sprintf("%dm", serverCPUMillicores),
			},
		},
	})
	remote := createCatalogEntry(t, owner, "remote", types.MCPServerCatalogEntryManifest{
		Runtime: types.RuntimeRemote,
		RemoteConfig: &types.RemoteCatalogConfig{
			FixedURL: "https://mcp.example.com/mcp",
		},
	})
	shared := func(entryID string) types.VMCPComponent {
		return types.VMCPComponent{
			Name:                    "server",
			MCPCatalogID:            system.DefaultCatalog,
			MCPServerCatalogEntryID: entryID,
		}
	}
	perUser := func(entryID string) types.VMCPComponent {
		component := shared(entryID)
		component.ForceSingleUser = true
		return component
	}
	waitForUsage(t, owner, 0, 0)

	// Room for two servers' memory, and CPU isn't limited.
	setPlan(t, owner, types.Entitlements{
		MemoryMiB: new(int64(2 * serverMemoryMiB)),
	})

	// A shared component runs once, for its vMCP.
	first := owner.CreatePersonalVMCP(t, vmcp(owner, "first", shared(hosted)))
	waitForUsage(t, owner, serverMemoryMiB, serverCPUMillicores)

	// A per-user component runs for each connection, so its vMCP uses nothing until someone connects.
	second := owner.CreatePersonalVMCP(t, vmcp(owner, "second", perUser(hosted)))
	third := owner.CreatePersonalVMCP(t, vmcp(owner, "third", perUser(hosted)))
	waitForUsage(t, owner, serverMemoryMiB, serverCPUMillicores)
	owner.CreateVMCPInstance(t, second.ID)
	waitForUsage(t, owner, 2*serverMemoryMiB, 2*serverCPUMillicores)

	t.Run("at the limit, a vMCP with a hosted server is refused", func(t *testing.T) {
		requireRefused(t, http.StatusPaymentRequired, memoryRefusal)(owner.Request(t, http.MethodPost, "/api/vmcps?scope=personal", vmcp(owner, "refused", shared(hosted))))
	})

	t.Run("at the limit, a connection that runs a server is refused", func(t *testing.T) {
		requireRefused(t, http.StatusPaymentRequired, memoryRefusal)(owner.Request(t, http.MethodPost, "/api/vmcp-instances", types.VMCPInstanceManifest{VMCPID: third.ID}))
	})

	t.Run("at the limit, what runs no server is still made", func(t *testing.T) {
		owner.CreatePersonalVMCP(t, vmcp(owner, "remote", shared(remote)))
		createCatalogEntry(t, owner, "another", types.MCPServerCatalogEntryManifest{
			Runtime: types.RuntimeRemote,
			RemoteConfig: &types.RemoteCatalogConfig{
				FixedURL: "https://another.example.com/mcp",
			},
		})
	})

	t.Run("refusals leave usage as it was", func(t *testing.T) {
		waitForUsage(t, owner, 2*serverMemoryMiB, 2*serverCPUMillicores)
		if violations := getVersion(t, owner).Violations; len(violations) != 0 {
			t.Fatalf("violations = %+v, want none at the limit", violations)
		}
	})

	// The plan now has room for one server, and two run.
	setPlan(t, owner, types.Entitlements{
		MemoryMiB: new(int64(serverMemoryMiB)),
	})

	t.Run("over the limit, the violation is reported", func(t *testing.T) {
		eventually(t, "the memory violation", func() error {
			return requireViolations(getVersion(t, owner), license.Violation{
				Type:    "memoryLimit",
				Message: "hosted MCP server memory is over its limit",
			})
		})
	})

	t.Run("over the limit, a raise is refused", func(t *testing.T) {
		requireRefused(t, http.StatusPaymentRequired, memoryRefusal)(owner.Request(t, http.MethodPost, "/api/vmcp-instances", types.VMCPInstanceManifest{VMCPID: third.ID}))
	})

	t.Run("over the limit, a write that doesn't raise usage is made", func(t *testing.T) {
		renamed := vmcp(owner, "first-renamed", shared(hosted))
		renamed.Components[0].ID = first.Components[0].ID
		owner.Put(t, "/api/vmcps/"+first.ID, renamed, nil)
		waitForUsage(t, owner, 2*serverMemoryMiB, 2*serverCPUMillicores)
	})

	t.Run("over the limit, a write that lowers usage is made", func(t *testing.T) {
		owner.Put(t, "/api/vmcps/"+first.ID, vmcp(owner, "first-remote", shared(remote)), nil)
		waitForUsage(t, owner, serverMemoryMiB, serverCPUMillicores)
		eventually(t, "the violation to clear", func() error {
			return requireViolations(getVersion(t, owner))
		})
	})

	// CPU has room for one server, which runs already, and memory isn't limited.
	setPlan(t, owner, types.Entitlements{
		CPUMillicores: new(int64(serverCPUMillicores + serverCPUMillicores/2)),
	})

	t.Run("each limit refuses on its own", func(t *testing.T) {
		requireRefused(t, http.StatusPaymentRequired, cpuRefusal)(owner.Request(t, http.MethodPost, "/api/vmcp-instances", types.VMCPInstanceManifest{VMCPID: third.ID}))
	})

	t.Run("a raised limit makes room", func(t *testing.T) {
		setPlan(t, owner, types.Entitlements{
			MemoryMiB:     new(int64(2 * serverMemoryMiB)),
			CPUMillicores: new(int64(2 * serverCPUMillicores)),
		})
		owner.CreateVMCPInstance(t, third.ID)
		waitForUsage(t, owner, 2*serverMemoryMiB, 2*serverCPUMillicores)
	})
}

func TestUserLimit(t *testing.T) {
	owner := signInOwner(t)
	users := getLicense(t, owner).Usage.Users

	setPlan(t, owner, types.Entitlements{
		Users: new(users),
	})
	// Adding a local account doesn't take a seat. Signing in with it does.
	email := createLocalUser(t, owner, "over-the-limit")
	member := harness.NewSession(t)
	requireRefused(t, http.StatusUnauthorized, userRefusal)(member.SignIn(t, email, memberPassword))
	if got := getLicense(t, owner).Usage.Users; got != users {
		t.Fatalf("users = %d, want %d after a refused sign-in", got, users)
	}
	if version := getVersion(t, owner); version.UserCount != users || version.UserLimit != users || len(version.Violations) != 0 {
		t.Fatalf("version = %+v, want %d of %d users and no violations", version, users, users)
	}

	setPlan(t, owner, types.Entitlements{
		Users: new(users + 1),
	})
	if status, body := harness.NewSession(t).SignIn(t, email, memberPassword); status != http.StatusOK {
		t.Fatalf("sign-in with a seat free = %d %s, want 200", status, body)
	}
	if got := getLicense(t, owner).Usage.Users; got != users+1 {
		t.Fatalf("users = %d, want %d", got, users+1)
	}

	// Everyone who has a seat keeps it when the plan shrinks.
	setPlan(t, owner, types.Entitlements{
		Users: new(users),
	})
	if status, body := harness.NewSession(t).SignIn(t, email, memberPassword); status != http.StatusOK {
		t.Fatalf("sign-in of a user who has a seat = %d %s, want 200", status, body)
	}
	if err := requireViolations(getVersion(t, owner), license.Violation{
		Type:    "userLimit",
		Message: "user count is over its limit",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestDeviceLimit(t *testing.T) {
	owner := signInOwner(t)
	devices := getLicense(t, owner).Usage.Devices

	var configuration types.MDMConfiguration
	owner.Post(t, "/api/mdm/configurations", types.MDMConfiguration{}, &configuration)
	var key types.MDMEnrollmentKeyCreateResponse
	owner.Post(t, fmt.Sprintf("/api/mdm/configurations/%d/enrollment-keys", configuration.ID), types.MDMEnrollmentKeyCreateRequest{Name: owner.RunID}, &key)
	enroller := harness.New(t)
	enroller.BearerToken = key.EnrollmentCredential
	enroll := func(t *testing.T, name string) (int, string) {
		t.Helper()
		public, _, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := x509.MarshalPKIXPublicKey(public)
		if err != nil {
			t.Fatal(err)
		}
		return enroller.Request(t, http.MethodPost, "/api/mdm/enroll", types.DeviceEnrollRequest{
			DeviceID:  owner.RunID + "-" + name,
			PublicKey: encoded,
			Hostname:  name,
		})
	}

	setPlan(t, owner, types.Entitlements{
		Devices: new(devices + 1),
	})
	if status, body := enroll(t, "first"); status != http.StatusCreated {
		t.Fatalf("enrolling within the limit = %d %s, want 201", status, body)
	}
	requireRefused(t, http.StatusForbidden, deviceRefusal)(enroll(t, "second"))
	if got := getLicense(t, owner).Usage.Devices; got != devices+1 {
		t.Fatalf("devices = %d, want %d", got, devices+1)
	}
	if version := getVersion(t, owner); version.DeviceCount != devices+1 || version.DeviceLimit != devices+1 {
		t.Fatalf("version = %+v, want %d of %d devices", version, devices+1, devices+1)
	}

	setPlan(t, owner, types.Entitlements{
		Devices: new(int64(0)),
	})
	requireRefused(t, http.StatusForbidden, deviceRefusal)(enroll(t, "third"))
	if err := requireViolations(getVersion(t, owner), license.Violation{
		Type:    "deviceLimit",
		Message: "device count is over its limit",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRestriction(t *testing.T) {
	owner := signInOwner(t)
	setPlan(t, owner, types.Entitlements{})
	email := createLocalUser(t, owner, "member")
	member := harness.NewSession(t)
	if status, body := member.SignIn(t, email, memberPassword); status != http.StatusOK {
		t.Fatalf("member sign-in = %d %s, want 200", status, body)
	}
	users := getLicense(t, owner).Usage.Users

	// One user too many for the plan.
	setPlan(t, owner, types.Entitlements{
		Users: new(users - 1),
	})
	var since metav1.Time
	eventually(t, "the over-limit clock to start", func() error {
		subscription := getSubscription(t)
		if subscription.Status.OverLimitSince == nil {
			return fmt.Errorf("the clock hasn't started")
		}
		since = *subscription.Status.OverLimitSince
		return nil
	})
	if age := time.Since(since.Time); age < 0 || age > time.Minute {
		t.Fatalf("the clock started %s ago, want just now", age)
	}

	t.Run("during the grace period everyone keeps working", func(t *testing.T) {
		if status, body := member.Request(t, http.MethodGet, "/api/me", nil); status != http.StatusOK {
			t.Fatalf("member during the grace period = %d %s, want 200", status, body)
		}
		if err := requireViolations(getVersion(t, owner), license.Violation{
			Type:    "userLimit",
			Message: "user count is over its limit",
		}); err != nil {
			t.Fatal(err)
		}
	})

	// The grace period is 3 days, so the clock is set back instead of waited out.
	setOverLimitSince(t, time.Now().Add(-3*24*time.Hour-time.Minute))

	t.Run("after the grace period members are signed out", func(t *testing.T) {
		requireSignedOut(t, member)
		// The session is gone, so signing in again is what's refused from here on.
		requireRefused(t, http.StatusPaymentRequired, authn.ErrRestricted.Message)(harness.NewSession(t).SignIn(t, email, memberPassword))
	})

	t.Run("after the grace period admins keep working", func(t *testing.T) {
		status := getLicense(t, owner)
		if status.Usage == nil || status.Usage.Users != users {
			t.Fatalf("usage = %+v, want %d users", status.Usage, users)
		}
		if err := requireViolations(getVersion(t, owner),
			license.Violation{
				Type:    "userLimit",
				Message: "user count is over its limit",
			},
			license.Violation{
				Type: "restricted",
			},
		); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("a plan with room lifts the restriction", func(t *testing.T) {
		setPlan(t, owner, types.Entitlements{
			Users: new(users),
		})
		eventually(t, "the over-limit clock to clear", func() error {
			if since := getSubscription(t).Status.OverLimitSince; since != nil {
				return fmt.Errorf("the clock still runs from %s", since)
			}
			return nil
		})
		eventually(t, "the member to sign in", func() error {
			if status, body := harness.NewSession(t).SignIn(t, email, memberPassword); status != http.StatusOK {
				return fmt.Errorf("member = %d %s, want 200", status, body)
			}
			return nil
		})
		if err := requireViolations(getVersion(t, owner)); err != nil {
			t.Fatal(err)
		}
	})
}

func TestBillingUnreachable(t *testing.T) {
	owner := signInOwner(t)
	hosted := createCatalogEntry(t, owner, "hosted", types.MCPServerCatalogEntryManifest{
		Runtime: types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{
			Package: "@modelcontextprotocol/server-everything",
		},
	})
	// The member needs a seat, whatever plan the last test left.
	setPlan(t, owner, types.Entitlements{})
	email := createLocalUser(t, owner, "member")
	member := harness.NewSession(t)
	if status, body := member.SignIn(t, email, memberPassword); status != http.StatusOK {
		t.Fatalf("member sign-in = %d %s, want 200", status, body)
	}

	// No hosted server fits this plan.
	granted := types.Entitlements{
		MemoryMiB: new(int64(0)),
	}
	want := granted
	want.Cloud = true
	failures := []struct {
		name      string
		failure   http.HandlerFunc
		syncError string
	}{
		{
			name: "billing answers with an error",
			failure: func(rw http.ResponseWriter, _ *http.Request) {
				http.Error(rw, `{"error":{"message":"billing is down for maintenance"}}`, http.StatusInternalServerError)
			},
			syncError: "billing responded 500: billing is down for maintenance",
		},
		{
			name: "billing hangs up",
			failure: func(rw http.ResponseWriter, _ *http.Request) {
				conn, _, err := rw.(http.Hijacker).Hijack()
				if err == nil {
					_ = conn.Close()
				}
			},
			syncError: "failed to reach billing: ",
		},
	}
	failSync := func(t *testing.T, failure http.HandlerFunc, syncError string) handlers.BillingStatus {
		t.Helper()
		billing.fail(failure)
		return checkNow(t, owner, func(status handlers.BillingStatus) bool {
			return strings.HasPrefix(status.SyncError, syncError)
		})
	}
	requireMemberWorks := func(t *testing.T) {
		t.Helper()
		// The restriction is read from a cache, so give it a moment to be wrong.
		for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
			if status, body := member.Request(t, http.MethodGet, "/api/me", nil); status != http.StatusOK {
				t.Fatalf("member = %d %s, want 200", status, body)
			}
		}
		if err := requireViolations(getVersion(t, owner)); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("within the period, the last answer stays in force", func(t *testing.T) {
		setPlan(t, owner, granted)
		plan := billing.current().Plan
		for _, tt := range failures {
			t.Run(tt.name, func(t *testing.T) {
				status := failSync(t, tt.failure, tt.syncError)
				requireJSONEqual(t, "entitlements", status.Entitlements, want)
				if status.Subscription == nil || status.Subscription.Plan != plan {
					t.Fatalf("subscription = %+v, want the last answer, %s", status.Subscription, plan)
				}
				requireRefused(t, http.StatusPaymentRequired, memoryRefusal)(owner.Request(t, http.MethodPost, "/api/vmcps?scope=personal", vmcp(owner, "refused", types.VMCPComponent{
					Name:                    "server",
					MCPCatalogID:            system.DefaultCatalog,
					MCPServerCatalogEntryID: hosted,
				})))
				requireMemberWorks(t)

				// Members see the entitlements, and nothing about billing.
				requireJSONEqual(t, "a member's billing status", getLicense(t, member), handlers.BillingStatus{
					Entitlements: want,
				})
			})
		}
	})

	t.Run("a subscription without a period never runs out", func(t *testing.T) {
		setAnswer(t, owner, types.Subscription{
			Status:       "active",
			Entitlements: granted,
		})
		failSync(t, failures[0].failure, failures[0].syncError)
		requireMemberWorks(t)
	})

	lapsed := types.Subscription{
		Status:       "active",
		PeriodEnd:    types.NewTime(time.Now().Add(-3*24*time.Hour - time.Minute).Truncate(time.Second)),
		SelfService:  true,
		Entitlements: granted,
	}

	t.Run("while billing answers, a period that ended restricts no one", func(t *testing.T) {
		setAnswer(t, owner, lapsed)
		requireMemberWorks(t)
	})

	t.Run("3 days past the period without an answer, members are signed out", func(t *testing.T) {
		status := failSync(t, failures[0].failure, failures[0].syncError)
		requireJSONEqual(t, "entitlements", status.Entitlements, want)
		requireSignedOut(t, member)
		requireRefused(t, http.StatusPaymentRequired, authn.ErrRestricted.Message)(harness.NewSession(t).SignIn(t, email, memberPassword))

		// Admins keep working, and are told.
		if err := requireViolations(getVersion(t, owner), license.Violation{
			Type: "restricted",
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("billing answers again", func(t *testing.T) {
		status := setPlan(t, owner, types.Entitlements{})
		if status.SyncError != "" {
			t.Fatalf("sync error = %q, want none once billing answers", status.SyncError)
		}
		eventually(t, "the member to sign in", func() error {
			if status, body := harness.NewSession(t).SignIn(t, email, memberPassword); status != http.StatusOK {
				return fmt.Errorf("member = %d %s, want 200", status, body)
			}
			return nil
		})
		if err := requireViolations(getVersion(t, owner)); err != nil {
			t.Fatal(err)
		}
	})
}

func signInOwner(t *testing.T) *harness.Harness {
	t.Helper()
	owner := harness.NewSession(t)
	if status, body := owner.SignIn(t, ownerEmail, ownerPassword); status != http.StatusOK {
		t.Fatalf("owner sign-in = %d %s, want 200", status, body)
	}
	return owner
}

// setPlan makes entitlements what billing grants and has Obot sync them.
func setPlan(t *testing.T, owner *harness.Harness, entitlements types.Entitlements) handlers.BillingStatus {
	t.Helper()
	plan := billing.set(entitlements)
	return checkNow(t, owner, func(status handlers.BillingStatus) bool {
		return status.Subscription != nil && status.Subscription.Plan == plan && status.SyncError == ""
	})
}

// setAnswer makes answer what billing answers and has Obot sync it.
func setAnswer(t *testing.T, owner *harness.Harness, answer types.Subscription) handlers.BillingStatus {
	t.Helper()
	plan := billing.answer(answer)
	return checkNow(t, owner, func(status handlers.BillingStatus) bool {
		return status.Subscription != nil && status.Subscription.Plan == plan && status.SyncError == ""
	})
}

// checkNow asks Obot to sync with billing once, as the license page does when an owner comes back from the billing
// pages, and waits for the leader to record the answer. synced reports whether that answer is the one expected.
func checkNow(t *testing.T, owner *harness.Harness, synced func(handlers.BillingStatus) bool) handlers.BillingStatus {
	t.Helper()
	var status handlers.BillingStatus
	owner.Post(t, "/api/license", nil, &status)
	if !status.Syncing {
		t.Fatalf("billing status = %+v, want a sync under way after asking for one", status)
	}
	eventually(t, "the sync with billing", func() error {
		if status = getLicense(t, owner); status.Syncing {
			return fmt.Errorf("still syncing")
		}
		return nil
	})
	if !synced(status) {
		t.Fatalf("billing status = %+v, want the answer billing gives now", status)
	}
	return status
}

func getLicense(t *testing.T, as *harness.Harness) handlers.BillingStatus {
	t.Helper()
	var status handlers.BillingStatus
	as.Get(t, "/api/license", &status)
	return status
}

func getVersion(t *testing.T, as *harness.Harness) version {
	t.Helper()
	var v version
	as.Get(t, "/api/version", &v)
	return v
}

// waitForUsage waits for the leader to count the compute that hosted servers request.
func waitForUsage(t *testing.T, owner *harness.Harness, memoryMiB, cpuMillicores int64) {
	t.Helper()
	eventually(t, "the compute usage", func() error {
		usage := getLicense(t, owner).Usage
		if usage == nil || usage.MemoryMiB != memoryMiB || usage.CPUMillicores != cpuMillicores {
			return fmt.Errorf("usage = %+v, want %d MiB and %dm", usage, memoryMiB, cpuMillicores)
		}
		return nil
	})
}

func createCatalogEntry(t *testing.T, owner *harness.Harness, name string, manifest types.MCPServerCatalogEntryManifest) string {
	t.Helper()
	manifest.Name = owner.MCPServerName(name)
	manifest.ShortDescription = "billing integration test server"
	manifest.Description = "billing integration test server"
	entry := owner.CreateMCPCatalogEntry(t, system.DefaultCatalog, manifest)
	if entry.ID == "" {
		t.Fatalf("catalog entry create returned no ID: %+v", entry)
	}
	return entry.ID
}

func vmcp(owner *harness.Harness, name string, components ...types.VMCPComponent) types.VMCPManifest {
	return types.VMCPManifest{
		DisplayName: owner.MCPServerName(name),
		Components:  components,
	}
}

// createLocalUser adds a local account that can sign in with memberPassword, and returns its email.
func createLocalUser(t *testing.T, owner *harness.Harness, name string) string {
	t.Helper()
	email := fmt.Sprintf("%s-%s@integration.example.com", name, owner.RunID)
	owner.Post(t, "/api/local-auth/users", map[string]any{
		"email":                 email,
		"password":              memberPassword,
		"requirePasswordChange": false,
	}, nil)
	return email
}

func getSubscription(t *testing.T) v1.Subscription {
	t.Helper()
	var subscription v1.Subscription
	if err := storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: system.SubscriptionName}, &subscription); err != nil {
		t.Fatalf("read the subscription: %v", err)
	}
	return subscription
}

// setOverLimitSince moves the running over-limit clock to since, as if usage had been over the limits since then.
func setOverLimitSince(t *testing.T, since time.Time) {
	t.Helper()
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		subscription := getSubscription(t)
		if subscription.Status.OverLimitSince == nil {
			return fmt.Errorf("the over-limit clock isn't running")
		}
		subscription.Status.OverLimitSince = &metav1.Time{Time: since}
		return storage.Status().Update(t.Context(), &subscription)
	}); err != nil {
		t.Fatalf("set the over-limit clock: %v", err)
	}
}

// requireSignedOut waits for the restriction to reach a signed-in member, and checks how their session ends. The
// refusal is told once: it ends the session, so the member is anonymous from then on.
func requireSignedOut(t *testing.T, member *harness.Harness) {
	t.Helper()
	var (
		status int
		body   string
	)
	eventually(t, "the member to be refused", func() error {
		if status, body = member.Request(t, http.MethodGet, "/api/me", nil); status == http.StatusOK {
			return fmt.Errorf("the member still has access")
		}
		return nil
	})
	requireRefused(t, http.StatusPaymentRequired, authn.ErrRestricted.Message)(status, body)
	requireRefused(t, http.StatusUnauthorized, "unauthorized")(member.Request(t, http.MethodGet, "/api/me", nil))
}

// requireRefused returns a check that a response is the refusal with status and message, and nothing more.
func requireRefused(t *testing.T, status int, message string) func(int, string) {
	t.Helper()
	return func(gotStatus int, body string) {
		t.Helper()
		if gotStatus != status || strings.TrimSpace(body) != message {
			t.Fatalf("response = %d %q, want %d %q", gotStatus, body, status, message)
		}
	}
}

func requireViolations(v version, want ...license.Violation) error {
	got, _ := json.Marshal(v.Violations)
	wanted, _ := json.Marshal(want)
	if len(v.Violations) != len(want) || len(want) > 0 && string(got) != string(wanted) {
		return fmt.Errorf("violations = %s, want %s", got, wanted)
	}
	return nil
}

func requireJSONEqual(t *testing.T, what string, got, want any) {
	t.Helper()
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("%s = %s, want %s", what, gotJSON, wantJSON)
	}
}

// eventually retries check until it passes, and fails the test with its last error after 20 seconds.
func eventually(t *testing.T, what string, check func() error) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		err := check()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s: %v", what, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
