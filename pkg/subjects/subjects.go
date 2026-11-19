// Package subjects decides whether the subjects of a policy name a caller, for the policy systems whose subjects can
// name agents: vMCP profiles, model access policies and skill access rules, which grant access (Evaluate), and MCP
// guardrail filters, which restrict it (EvaluateRestriction). Access control rules have their own matcher, which
// agents never match. Legacy message policies have none: nothing enforces them.
//
// A subject names its principals by kind:
//
//   - {selector, "*"} (subjects:*) is every user. It never names an agent.
//   - {user, "<id>"} is one user. {user, "*"} names no one, since no user has the ID "*".
//   - {agent, "<id>"} is one agent, by its bare decimal ID. It is the only subject that names an agent, so granting or
//     restricting an agent always names it.
//   - {group, "<id>"} and {obotGroup, "<id>"} are the users in a group. Agents have no groups.
//
// The kind is always part of the comparison, so a user and an agent that share a numeric ID never match each other's
// subjects.
package subjects

import (
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/subjectgroups"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
)

const (
	// PrincipalIndex, GroupIndex and ObotGroupIndex are the indexes of policies by subject that Indexers returns and
	// LookupIndexes reads.
	PrincipalIndex = "subjects-principal"
	GroupIndex     = "subjects-group"
	ObotGroupIndex = "subjects-obot-group"
)

// Groups are the groups a user is in, as subject IDs: the auth provider groups that {group, id} subjects name, and
// the Obot groups that {obotGroup, id} subjects name.
type Groups struct {
	AuthProvider map[string]struct{}
	Obot         map[string]struct{}
}

// Caller is a principal that subjects are evaluated for, with its groups.
type Caller struct {
	Principal principal.Principal
	Groups    Groups
}

// CallerFromUserInfo returns the caller that u is, and false if u is not a well-formed principal of a user or an
// agent (principal.FromUserInfo). Such a caller, like a system principal, an anonymous request, or one built without
// principal.RecordKind, matches no subject at all, so a policy never applies to a caller it cannot identify. An
// agent's caller has no groups, whatever u carries.
func CallerFromUserInfo(u kuser.Info) (Caller, bool) {
	p, ok := principal.FromUserInfo(u)
	if !ok {
		return Caller{}, false
	}
	if p.Kind == principal.KindAgent {
		return Caller{Principal: p}, true
	}
	authProvider, obot := subjectgroups.Sets(u)
	return Caller{Principal: p, Groups: Groups{AuthProvider: authProvider, Obot: obot}}, true
}

// Evaluate reports whether s names caller, who is in groups.
//
// A user matches {selector, "*"}, {user, <their ID>}, and the groups and Obot groups they are in. An agent matches
// only {agent, <its ID>}: never the selector wildcard, nor a user, group or Obot group subject, whatever its ID, and
// groups is ignored for it. A caller of any other kind, or without an ID, matches nothing.
func Evaluate(s types.Subject, caller principal.Principal, groups Groups) bool {
	kindType, ok := subjectType(caller)
	if !ok {
		return false
	}

	switch s.Type {
	case types.SubjectTypeSelector:
		return s.ID == types.SubjectWildcardID && caller.Kind == principal.KindUser
	case kindType:
		// User subjects for a user, agent subjects for an agent: never the other kind's, whatever the ID. No
		// principal has the ID "*", so {user, "*"} and {agent, "*"} name no one.
		return s.ID == caller.ID
	case types.SubjectTypeGroup:
		_, ok := groups.AuthProvider[s.ID]
		return ok && caller.Kind == principal.KindUser
	case types.SubjectTypeObotGroup:
		_, ok := groups.Obot[s.ID]
		return ok && caller.Kind == principal.KindUser
	}
	return false
}

// subjectType returns the subject type that names principals of p's kind: user subjects for a user, and agent
// subjects for an agent. It reports false for a principal that is neither, or that has no ID, which no subject names.
func subjectType(p principal.Principal) (types.SubjectType, bool) {
	if p.ID == "" {
		return "", false
	}
	switch p.Kind {
	case principal.KindUser:
		return types.SubjectTypeUser, true
	case principal.KindAgent:
		return types.SubjectTypeAgent, true
	}
	return "", false
}

// EvaluateRestriction reports whether s names caller for a policy that restricts callers rather than grants them
// anything, such as an MCP guardrail filter. It is Evaluate, except for a caller that is not a user or an agent with
// an ID, such as one without a recorded kind: such a caller still matches the selector wildcard (everyone). So the
// selector wildcard restricts every user and every caller it cannot identify, but no agent: a restriction applies to
// an agent only when it names the agent.
//
// The grant matchers fail closed, so an unidentified caller gets nothing. A restriction fails closed the other way:
// one that applies to everyone must still apply to a caller it cannot identify, or such a caller would escape it. An
// unidentified caller matches no other subject, since nothing says which user, agent or group it is.
func EvaluateRestriction(s types.Subject, caller principal.Principal, groups Groups) bool {
	if _, ok := subjectType(caller); !ok {
		return s.Type == types.SubjectTypeSelector && s.ID == types.SubjectWildcardID
	}
	return Evaluate(s, caller, groups)
}

// Evaluate reports whether s names the caller.
func (c Caller) Evaluate(s types.Subject) bool {
	return Evaluate(s, c.Principal, c.Groups)
}

// Matches reports whether any of subjects names the caller.
func (c Caller) Matches(subjects []types.Subject) bool {
	return slices.ContainsFunc(subjects, c.Evaluate)
}

// IndexKey returns the key under which a policy index files a subject that names principals rather than groups:
// "*" for {selector, "*"}, and "user:<id>" and "agent:<id>" for one user or agent. The kind is part of every key but
// the selector wildcard's, so a user and an agent that share a numeric ID never find each other's policies. It reports
// false for group subjects, which indexes file under their group IDs, and for subjects that name no principal, such as
// a selector other than "*".
//
// The keys of the subjects that match a principal without a group are exactly LookupKeys of it, and every lookup
// must use LookupKeys, so that indexes and Evaluate agree.
func IndexKey(s types.Subject) (string, bool) {
	switch s.Type {
	case types.SubjectTypeSelector:
		if s.ID == types.SubjectWildcardID {
			return types.SubjectWildcardID, true
		}
	case types.SubjectTypeUser:
		if s.ID != "" {
			return string(principal.KindUser) + ":" + s.ID, true
		}
	case types.SubjectTypeAgent:
		if s.ID != "" {
			return string(principal.KindAgent) + ":" + s.ID, true
		}
	}
	return "", false
}

// IndexKeys returns IndexKey of each of subjects that has one, once each.
func IndexKeys(subjects []types.Subject) []string {
	var keys []string
	for _, s := range subjects {
		if key, ok := IndexKey(s); ok && !slices.Contains(keys, key) {
			keys = append(keys, key)
		}
	}
	return keys
}

// LookupKeys returns the index keys (IndexKey) of the subjects that name p other than through a group: for a user, the
// selector wildcard and its own; for an agent, only its own. It is empty for a principal that is neither a user nor an
// agent, or that has no ID.
func LookupKeys(p principal.Principal) []string {
	if _, ok := subjectType(p); !ok {
		return nil
	}
	own := string(p.Kind) + ":" + p.ID
	if p.Kind == principal.KindAgent {
		return []string{own}
	}
	return []string{types.SubjectWildcardID, own}
}

// Indexers returns the indexes that file a policy by its subjects, which subjectsOf returns: PrincipalIndex under
// IndexKeys of them, and GroupIndex and ObotGroupIndex under the IDs of its group and Obot group subjects, which only
// users match. subjectsOf reports false for a policy to leave out of every index, such as one being deleted.
func Indexers(subjectsOf func(obj any) ([]types.Subject, bool)) gocache.Indexers {
	groupIndex := func(subjectType types.SubjectType) gocache.IndexFunc {
		return func(obj any) ([]string, error) {
			policySubjects, ok := subjectsOf(obj)
			if !ok {
				return nil, nil
			}
			var ids []string
			for _, s := range policySubjects {
				if s.Type == subjectType {
					ids = append(ids, s.ID)
				}
			}
			return ids, nil
		}
	}
	return gocache.Indexers{
		PrincipalIndex: func(obj any) ([]string, error) {
			policySubjects, ok := subjectsOf(obj)
			if !ok {
				return nil, nil
			}
			return IndexKeys(policySubjects), nil
		},
		GroupIndex:     groupIndex(types.SubjectTypeGroup),
		ObotGroupIndex: groupIndex(types.SubjectTypeObotGroup),
	}
}

// LookupIndexes calls lookup with each index of Indexers and key under which a policy whose subjects name caller is
// filed: LookupKeys of its principal, then its groups and Obot groups. A policy can be found under several of them. It
// returns the first error lookup returns.
func LookupIndexes(caller Caller, lookup func(index, key string) error) error {
	for _, key := range LookupKeys(caller.Principal) {
		if err := lookup(PrincipalIndex, key); err != nil {
			return err
		}
	}
	// An agent's caller has no groups, so these find nothing for it.
	for id := range caller.Groups.AuthProvider {
		if err := lookup(GroupIndex, id); err != nil {
			return err
		}
	}
	for id := range caller.Groups.Obot {
		if err := lookup(ObotGroupIndex, id); err != nil {
			return err
		}
	}
	return nil
}
