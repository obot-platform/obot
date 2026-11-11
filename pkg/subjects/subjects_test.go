package subjects

import (
	"slices"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/principal"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

// The callers of the matrix. The user and the agent share the numeric ID 5, so that a subject naming one never
// matches the other only because their IDs are equal.
var (
	user5      = Caller{Principal: principal.User(5)}
	agent5     = Caller{Principal: principal.Agent(5)}
	groupUser5 = Caller{Principal: principal.User(5), Groups: Groups{
		AuthProvider: map[string]struct{}{"eng": {}},
		Obot:         map[string]struct{}{"team": {}},
	}}
	admin7 = Caller{Principal: principal.User(7), Groups: Groups{
		Obot: map[string]struct{}{types.GroupAdmin: {}, types.GroupOwner: {}},
	}}
	// agentWithGroups is an agent whose user info somehow carried groups. Groups never match an agent.
	agentWithGroups = Caller{Principal: principal.Agent(5), Groups: Groups{
		AuthProvider: map[string]struct{}{"eng": {}},
		Obot:         map[string]struct{}{"team": {}, types.GroupAdmin: {}},
	}}
)

func TestEvaluate(t *testing.T) {
	for _, tt := range []struct {
		name    string
		subject types.Subject
		want    map[string]Match
	}{
		{
			name:    "selector wildcard matches users and agents",
			subject: types.Subject{Type: types.SubjectTypeSelector, ID: "*"},
			want: map[string]Match{
				"user":             MatchWildcardAll,
				"agent":            MatchWildcardAll,
				"group user":       MatchWildcardAll,
				"admin":            MatchWildcardAll,
				"agent with group": MatchWildcardAll,
			},
		},
		{
			name:    "selector other than the wildcard matches nobody",
			subject: types.Subject{Type: types.SubjectTypeSelector, ID: "all"},
			want:    map[string]Match{},
		},
		{
			name:    "user wildcard matches users only",
			subject: types.Subject{Type: types.SubjectTypeUser, ID: "*"},
			want: map[string]Match{
				"user":       MatchWildcardKind,
				"group user": MatchWildcardKind,
				"admin":      MatchWildcardKind,
			},
		},
		{
			name:    "agent wildcard matches agents only",
			subject: types.Subject{Type: types.SubjectTypeAgent, ID: "*"},
			want: map[string]Match{
				"agent":            MatchWildcardKind,
				"agent with group": MatchWildcardKind,
			},
		},
		{
			name:    "user 5 matches user 5 and not agent 5",
			subject: types.Subject{Type: types.SubjectTypeUser, ID: "5"},
			want: map[string]Match{
				"user":       MatchExplicit,
				"group user": MatchExplicit,
			},
		},
		{
			name:    "agent 5 matches agent 5 and not user 5",
			subject: types.Subject{Type: types.SubjectTypeAgent, ID: "5"},
			want: map[string]Match{
				"agent":            MatchExplicit,
				"agent with group": MatchExplicit,
			},
		},
		{
			name:    "user subject with an agent's storage encoding matches nobody",
			subject: types.Subject{Type: types.SubjectTypeUser, ID: "agent:5"},
			want:    map[string]Match{},
		},
		{
			name:    "agent subject with an agent's storage encoding matches nobody",
			subject: types.Subject{Type: types.SubjectTypeAgent, ID: "agent:5"},
			want:    map[string]Match{},
		},
		{
			name:    "agent subject with a non-canonical ID matches nobody",
			subject: types.Subject{Type: types.SubjectTypeAgent, ID: "05"},
			want:    map[string]Match{},
		},
		{
			name:    "other user matches nobody here",
			subject: types.Subject{Type: types.SubjectTypeUser, ID: "6"},
			want:    map[string]Match{},
		},
		{
			name:    "group matches its members, never an agent",
			subject: types.Subject{Type: types.SubjectTypeGroup, ID: "eng"},
			want: map[string]Match{
				"group user": MatchGroup,
			},
		},
		{
			name:    "Obot group matches its members, never an agent",
			subject: types.Subject{Type: types.SubjectTypeObotGroup, ID: "team"},
			want: map[string]Match{
				"group user": MatchGroup,
			},
		},
		{
			name:    "admin Obot group matches admins, never an agent",
			subject: types.Subject{Type: types.SubjectTypeObotGroup, ID: types.GroupAdmin},
			want: map[string]Match{
				"admin": MatchGroup,
			},
		},
		{
			name:    "group wildcard ID is not a wildcard",
			subject: types.Subject{Type: types.SubjectTypeGroup, ID: "*"},
			want:    map[string]Match{},
		},
		{
			name:    "unknown type matches nobody",
			subject: types.Subject{Type: "service", ID: "*"},
			want:    map[string]Match{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for name, caller := range map[string]Caller{
				"user":             user5,
				"agent":            agent5,
				"group user":       groupUser5,
				"admin":            admin7,
				"agent with group": agentWithGroups,
			} {
				if got := caller.Evaluate(tt.subject); got != tt.want[name] {
					t.Errorf("%s: Evaluate(%v) = %v, want %v", name, tt.subject, got, tt.want[name])
				}
			}
		})
	}
}

func TestEvaluateRejectsCallersThatAreNotPrincipals(t *testing.T) {
	everything := []types.Subject{
		{Type: types.SubjectTypeSelector, ID: "*"},
		{Type: types.SubjectTypeUser, ID: "*"},
		{Type: types.SubjectTypeAgent, ID: "*"},
		{Type: types.SubjectTypeUser, ID: ""},
		{Type: types.SubjectTypeAgent, ID: ""},
	}
	for _, tt := range []struct {
		name   string
		caller principal.Principal
	}{
		{
			name:   "zero principal",
			caller: principal.Principal{},
		},
		{
			name:   "unknown kind",
			caller: principal.Principal{Kind: "device", ID: "5"},
		},
		{
			name:   "user without an ID",
			caller: principal.Principal{Kind: principal.KindUser},
		},
		{
			name:   "agent without an ID",
			caller: principal.Principal{Kind: principal.KindAgent},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, s := range everything {
				if got := Evaluate(s, tt.caller, Groups{}); got != NoMatch {
					t.Errorf("Evaluate(%v) = %v, want NoMatch", s, got)
				}
			}
		})
	}
}

func TestBest(t *testing.T) {
	for _, tt := range []struct {
		name     string
		caller   Caller
		subjects []types.Subject
		want     Match
	}{
		{
			name:   "explicit beats wildcards",
			caller: agent5,
			subjects: []types.Subject{
				{Type: types.SubjectTypeSelector, ID: "*"},
				{Type: types.SubjectTypeAgent, ID: "5"},
				{Type: types.SubjectTypeAgent, ID: "*"},
			},
			want: MatchExplicit,
		},
		{
			name:   "kind wildcard beats selector wildcard",
			caller: user5,
			subjects: []types.Subject{
				{Type: types.SubjectTypeSelector, ID: "*"},
				{Type: types.SubjectTypeUser, ID: "*"},
			},
			want: MatchWildcardKind,
		},
		{
			name:   "group beats wildcards for a user",
			caller: groupUser5,
			subjects: []types.Subject{
				{Type: types.SubjectTypeUser, ID: "*"},
				{Type: types.SubjectTypeGroup, ID: "eng"},
			},
			want: MatchGroup,
		},
		{
			name:   "agent matches none of user subjects",
			caller: agentWithGroups,
			subjects: []types.Subject{
				{Type: types.SubjectTypeUser, ID: "*"},
				{Type: types.SubjectTypeUser, ID: "5"},
				{Type: types.SubjectTypeGroup, ID: "eng"},
				{Type: types.SubjectTypeObotGroup, ID: types.GroupAdmin},
			},
			want: NoMatch,
		},
		{
			name:     "no subjects",
			caller:   user5,
			subjects: nil,
			want:     NoMatch,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.caller.Best(tt.subjects); got != tt.want {
				t.Errorf("Best() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCallerFromUserInfo(t *testing.T) {
	userExtra := func(kind principal.Kind) map[string][]string {
		extra := map[string][]string{
			"auth_provider_groups": {"eng"},
			"obot_groups":          {types.GroupAdmin},
		}
		principal.RecordKind(extra, kind)
		return extra
	}
	for _, tt := range []struct {
		name   string
		info   kuser.Info
		want   Caller
		wantOK bool
	}{
		{
			name: "user with groups",
			info: &kuser.DefaultInfo{UID: "5", Extra: userExtra(principal.KindUser)},
			want: Caller{Principal: principal.User(5), Groups: Groups{
				AuthProvider: map[string]struct{}{"eng": {}},
				Obot:         map[string]struct{}{types.GroupAdmin: {}},
			}},
			wantOK: true,
		},
		{
			name:   "agent drops any groups",
			info:   &kuser.DefaultInfo{UID: "agent:5", Extra: userExtra(principal.KindAgent)},
			want:   Caller{Principal: principal.Agent(5)},
			wantOK: true,
		},
		{
			name: "user without a recorded kind",
			info: &kuser.DefaultInfo{UID: "5", Extra: map[string][]string{"obot_groups": {types.GroupAdmin}}},
		},
		{
			name: "agent UID recorded as a user",
			info: &kuser.DefaultInfo{UID: "agent:5", Extra: userExtra(principal.KindUser)},
		},
		{
			name: "user UID recorded as an agent",
			info: &kuser.DefaultInfo{UID: "5", Extra: userExtra(principal.KindAgent)},
		},
		{
			name: "anonymous",
			info: &kuser.DefaultInfo{Name: "anonymous", UID: "anonymous"},
		},
		{
			name: "nil",
			info: nil,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := CallerFromUserInfo(tt.info)
			if ok != tt.wantOK {
				t.Fatalf("CallerFromUserInfo() ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				if got := got.Best([]types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}}); got != NoMatch {
					t.Errorf("Best() of a caller that is not a principal = %v, want NoMatch", got)
				}
				return
			}
			if got.Principal != tt.want.Principal {
				t.Errorf("Principal = %v, want %v", got.Principal, tt.want.Principal)
			}
			if len(got.Groups.AuthProvider) != len(tt.want.Groups.AuthProvider) || len(got.Groups.Obot) != len(tt.want.Groups.Obot) {
				t.Errorf("Groups = %v, want %v", got.Groups, tt.want.Groups)
			}
			for g := range tt.want.Groups.AuthProvider {
				if _, ok := got.Groups.AuthProvider[g]; !ok {
					t.Errorf("missing auth provider group %q", g)
				}
			}
			for g := range tt.want.Groups.Obot {
				if _, ok := got.Groups.Obot[g]; !ok {
					t.Errorf("missing Obot group %q", g)
				}
			}
		})
	}
}

// TestIndexKeysAgreeWithEvaluate checks that the index keys that LookupKeys returns for a principal are exactly the
// IndexKey of the subjects that Evaluate matches it to without a group, so that an index lookup finds the policies
// Evaluate would, and no others.
func TestIndexKeysAgreeWithEvaluate(t *testing.T) {
	subjectForms := []types.Subject{
		{Type: types.SubjectTypeSelector, ID: "*"},
		{Type: types.SubjectTypeSelector, ID: "all"},
		{Type: types.SubjectTypeUser, ID: "*"},
		{Type: types.SubjectTypeUser, ID: "5"},
		{Type: types.SubjectTypeUser, ID: "6"},
		{Type: types.SubjectTypeUser, ID: "agent:5"},
		{Type: types.SubjectTypeAgent, ID: "*"},
		{Type: types.SubjectTypeAgent, ID: "5"},
		{Type: types.SubjectTypeAgent, ID: "6"},
		{Type: types.SubjectTypeAgent, ID: "agent:5"},
		{Type: types.SubjectTypeGroup, ID: "eng"},
		{Type: types.SubjectTypeObotGroup, ID: "team"},
	}
	for _, p := range []principal.Principal{principal.User(5), principal.Agent(5), principal.User(6), principal.Agent(6)} {
		lookup := LookupKeys(p)
		for _, s := range subjectForms {
			match := Evaluate(s, p, Groups{})
			key, indexed := IndexKey(s)
			found := indexed && slices.Contains(lookup, key)
			if found != (match != NoMatch) {
				t.Errorf("%v: subject %v has key %q (indexed %v), lookup %v finds it = %v, but Evaluate = %v", p, s, key, indexed, lookup, found, match)
			}
		}
	}

	if keys := LookupKeys(principal.Principal{}); keys != nil {
		t.Errorf("LookupKeys(zero principal) = %v, want none", keys)
	}
	if keys := LookupKeys(principal.Principal{Kind: "device", ID: "5"}); keys != nil {
		t.Errorf("LookupKeys(unknown kind) = %v, want none", keys)
	}
	if _, ok := IndexKey(types.Subject{Type: types.SubjectTypeGroup, ID: "eng"}); ok {
		t.Error("IndexKey(group) is indexed, want not")
	}
}

func TestIndexKeys(t *testing.T) {
	got := IndexKeys([]types.Subject{
		{Type: types.SubjectTypeUser, ID: "5"},
		{Type: types.SubjectTypeAgent, ID: "5"},
		{Type: types.SubjectTypeUser, ID: "5"},
		{Type: types.SubjectTypeGroup, ID: "eng"},
		{Type: types.SubjectTypeSelector, ID: "*"},
	})
	want := []string{"user:5", "agent:5", "*"}
	if !slices.Equal(got, want) {
		t.Errorf("IndexKeys() = %v, want %v", got, want)
	}
}

// TestEvaluateRestriction checks that a restriction matches as a grant does for users and agents, and that a caller
// that is not a principal still matches the selector wildcard, but nothing else.
func TestEvaluateRestriction(t *testing.T) {
	forms := []types.Subject{
		{Type: types.SubjectTypeSelector, ID: "*"},
		{Type: types.SubjectTypeUser, ID: "*"},
		{Type: types.SubjectTypeUser, ID: "5"},
		{Type: types.SubjectTypeAgent, ID: "*"},
		{Type: types.SubjectTypeAgent, ID: "5"},
		{Type: types.SubjectTypeGroup, ID: "eng"},
		{Type: types.SubjectTypeObotGroup, ID: "team"},
	}
	for _, caller := range []Caller{user5, agent5, groupUser5, agentWithGroups} {
		for _, s := range forms {
			if got, want := EvaluateRestriction(s, caller.Principal, caller.Groups), caller.Evaluate(s); got != want {
				t.Errorf("%v: EvaluateRestriction(%v) = %v, want Evaluate's %v", caller.Principal, s, got, want)
			}
		}
	}
	for _, p := range []principal.Principal{{}, {Kind: "device", ID: "5"}, {Kind: principal.KindUser}} {
		for _, s := range forms {
			want := NoMatch
			if s.Type == types.SubjectTypeSelector {
				want = MatchWildcardAll
			}
			if got := EvaluateRestriction(s, p, groupUser5.Groups); got != want {
				t.Errorf("%v: EvaluateRestriction(%v) = %v, want %v", p, s, got, want)
			}
		}
	}
}
