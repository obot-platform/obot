// Package groupref finds the objects that reference auth provider groups.
package groupref

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	KindAccessControlRule     Kind = "accessControlRule"
	KindModelAccessPolicy     Kind = "modelAccessPolicy"
	KindSkillAccessRule       Kind = "skillAccessRule"
	KindMessagePolicy         Kind = "messagePolicy"
	KindHostedAgentAccessRule Kind = "hostedAgentAccessRule"
	KindGroupRoleAssignment   Kind = "groupRoleAssignment"
	KindVMCPProfile           Kind = "vmcpProfile"
)

var (
	// policyKinds are the access policy types, whose subjects grant access to groups.
	policyKinds = []policyKind{
		{
			kind: KindAccessControlRule,
			list: listAccessControlRules,
		},
		{
			kind: KindModelAccessPolicy,
			list: listModelAccessPolicies,
		},
		{
			kind: KindSkillAccessRule,
			list: listSkillAccessRules,
		},
		{
			kind: KindMessagePolicy,
			list: listMessagePolicies,
		},
		{
			kind: KindHostedAgentAccessRule,
			list: listHostedAgentAccessRules,
		},
	}
)

// Kind is a kind of object that can reference groups.
type Kind string

// Reference is one object that references a group.
type Reference struct {
	Kind Kind
	// Name identifies the object: its name, or the group ID for a group role assignment.
	Name        string
	DisplayName string
	// Detail says where in the object the reference is, such as the version of a published artifact, the profile
	// of a virtual MCP server, or the role a group role assignment grants.
	Detail string
}

// References maps group IDs to the objects that reference them.
type References map[string][]Reference

// RoleAssignmentLister lists group role assignments, which live in the gateway database.
type RoleAssignmentLister interface {
	ListGroupRoleAssignments(ctx context.Context) ([]gatewaytypes.GroupRoleAssignment, error)
}

// Finder finds the objects that reference groups: every access policy type, group role assignments, and the
// profiles of virtual MCP servers.
type Finder struct {
	storage kclient.Reader
	roles   RoleAssignmentLister
}

type policyKind struct {
	kind Kind
	list func(ctx context.Context, c kclient.Reader, namespace string) ([]policyObject, error)
}

type policyObject struct {
	obj         kclient.Object
	displayName string
	subjects    []subjectList
}

type subjectList struct {
	subjects *[]types.Subject
	detail   string
}

// NewFinder returns a Finder that reads objects from storage and group role assignments from roles.
func NewFinder(storage kclient.Reader, roles RoleAssignmentLister) *Finder {
	return &Finder{
		storage: storage,
		roles:   roles,
	}
}

// HasPrefix matches the group IDs that start with prefix.
func HasPrefix(prefix string) func(groupID string) bool {
	return func(groupID string) bool {
		return strings.HasPrefix(groupID, prefix)
	}
}

// Find returns the references in namespace to the groups whose IDs match, sorted by kind, name, and detail.
func (f *Finder) Find(ctx context.Context, namespace string, match func(groupID string) bool) (References, error) {
	refs := make(References)

	for _, kind := range policyKinds {
		objs, err := kind.list(ctx, f.storage, namespace)
		if err != nil {
			return nil, err
		}
		for _, obj := range objs {
			for _, list := range obj.subjects {
				for _, subject := range *list.subjects {
					if subject.Type == types.SubjectTypeGroup && match(subject.ID) {
						refs.add(subject.ID, Reference{
							Kind:        kind.kind,
							Name:        obj.obj.GetName(),
							DisplayName: obj.displayName,
							Detail:      list.detail,
						})
					}
				}
			}
		}
	}

	var vmcps v1.VMCPList
	if err := f.storage.List(ctx, &vmcps, kclient.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list virtual MCP servers: %w", err)
	}
	for _, vmcp := range vmcps.Items {
		for _, profile := range vmcp.Spec.Manifest.Profiles {
			for _, subject := range profile.Subjects {
				if subject.Type == types.SubjectTypeGroup && match(subject.ID) {
					refs.add(subject.ID, Reference{
						Kind:        KindVMCPProfile,
						Name:        vmcp.Name,
						DisplayName: vmcp.Spec.Manifest.DisplayName,
						Detail:      "profile " + profile.Name,
					})
				}
			}
		}
	}

	assignments, err := f.roles.ListGroupRoleAssignments(ctx)
	if err != nil {
		return nil, err
	}
	for _, assignment := range assignments {
		if match(assignment.GroupName) {
			refs.add(assignment.GroupName, Reference{
				Kind:   KindGroupRoleAssignment,
				Name:   assignment.GroupName,
				Detail: roleDetail(assignment.Role),
			})
		}
	}

	for id := range refs {
		slices.SortFunc(refs[id], compareReferences)
		refs[id] = slices.Compact(refs[id])
	}
	return refs, nil
}

// RemoveGroupSubjects removes the group subjects that match from every access policy in namespace, and returns
// the number of objects of each kind that it changed.
//
// It changes neither group role assignments, which live in the gateway database, nor the profiles of virtual MCP
// servers, which must keep at least one subject. A subject naming a group that no longer exists matches nobody.
func RemoveGroupSubjects(ctx context.Context, c kclient.Client, namespace string, match func(groupID string) bool) (map[Kind]int, error) {
	counts := make(map[Kind]int, len(policyKinds))
	for _, kind := range policyKinds {
		objs, err := kind.list(ctx, c, namespace)
		if err != nil {
			return counts, err
		}

		for _, obj := range objs {
			changed := false
			for _, list := range obj.subjects {
				if subjects, ok := RemoveSubjects(*list.subjects, match); ok {
					*list.subjects = subjects
					changed = true
				}
			}
			if !changed {
				continue
			}

			if err = c.Update(ctx, obj.obj); err != nil {
				return counts, fmt.Errorf("failed to remove group subjects from %s %s: %w", kind.kind, obj.obj.GetName(), err)
			}
			counts[kind.kind]++
		}
	}
	return counts, nil
}

// RemoveSubjects returns subjects without the group subjects that match, and whether any were removed. The order
// of the remaining subjects is kept.
func RemoveSubjects(subjects []types.Subject, match func(groupID string) bool) ([]types.Subject, bool) {
	result := make([]types.Subject, 0, len(subjects))
	for _, subject := range subjects {
		if subject.Type == types.SubjectTypeGroup && match(subject.ID) {
			continue
		}
		result = append(result, subject)
	}
	if len(result) == len(subjects) {
		return subjects, false
	}
	return result, true
}

func (r References) add(groupID string, ref Reference) {
	r[groupID] = append(r[groupID], ref)
}

func policyObjects[T any, PT interface {
	*T
	kclient.Object
}](items []T, subjects func(PT) (string, []subjectList)) []policyObject {
	objs := make([]policyObject, 0, len(items))
	for i := range items {
		obj := PT(&items[i])
		displayName, lists := subjects(obj)
		objs = append(objs, policyObject{
			obj:         obj,
			displayName: displayName,
			subjects:    lists,
		})
	}
	return objs
}

// roleDetail names the role a group role assignment grants.
func roleDetail(role types.Role) string {
	var names []string
	switch role.ExtractBaseRole() {
	case types.RoleOwner:
		names = append(names, "Owner")
	case types.RoleAdmin:
		names = append(names, "Admin")
	case types.RolePowerUserPlus:
		names = append(names, "Power User Plus")
	case types.RolePowerUser:
		names = append(names, "Power User")
	case types.RoleBasic:
		names = append(names, "Basic User")
	}
	if role.HasAuditorRole() {
		names = append(names, "Auditor")
	}
	if len(names) == 0 {
		return ""
	}
	return "role " + strings.Join(names, ", ")
}

func compareReferences(a, b Reference) int {
	return cmp.Or(
		cmp.Compare(a.Kind, b.Kind),
		cmp.Compare(a.Name, b.Name),
		cmp.Compare(a.Detail, b.Detail),
	)
}

func listAccessControlRules(ctx context.Context, c kclient.Reader, namespace string) ([]policyObject, error) {
	var list v1.AccessControlRuleList
	if err := c.List(ctx, &list, kclient.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list access control rules: %w", err)
	}
	return policyObjects(list.Items, func(rule *v1.AccessControlRule) (string, []subjectList) {
		return rule.Spec.Manifest.DisplayName, []subjectList{
			{
				subjects: &rule.Spec.Manifest.Subjects,
			},
		}
	}), nil
}

func listModelAccessPolicies(ctx context.Context, c kclient.Reader, namespace string) ([]policyObject, error) {
	var list v1.ModelAccessPolicyList
	if err := c.List(ctx, &list, kclient.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list model access policies: %w", err)
	}
	return policyObjects(list.Items, func(policy *v1.ModelAccessPolicy) (string, []subjectList) {
		return policy.Spec.Manifest.DisplayName, []subjectList{
			{
				subjects: &policy.Spec.Manifest.Subjects,
			},
		}
	}), nil
}

func listSkillAccessRules(ctx context.Context, c kclient.Reader, namespace string) ([]policyObject, error) {
	var list v1.SkillAccessRuleList
	if err := c.List(ctx, &list, kclient.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list skill access rules: %w", err)
	}
	return policyObjects(list.Items, func(rule *v1.SkillAccessRule) (string, []subjectList) {
		return rule.Spec.Manifest.DisplayName, []subjectList{
			{
				subjects: &rule.Spec.Manifest.Subjects,
			},
		}
	}), nil
}

func listMessagePolicies(ctx context.Context, c kclient.Reader, namespace string) ([]policyObject, error) {
	var list v1.MessagePolicyList
	if err := c.List(ctx, &list, kclient.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list message policies: %w", err)
	}
	return policyObjects(list.Items, func(policy *v1.MessagePolicy) (string, []subjectList) {
		return policy.Spec.Manifest.DisplayName, []subjectList{
			{
				subjects: &policy.Spec.Manifest.Subjects,
			},
		}
	}), nil
}

func listHostedAgentAccessRules(ctx context.Context, c kclient.Reader, namespace string) ([]policyObject, error) {
	var list v1.HostedAgentAccessRuleList
	if err := c.List(ctx, &list, kclient.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list hosted agent access rules: %w", err)
	}
	return policyObjects(list.Items, func(rule *v1.HostedAgentAccessRule) (string, []subjectList) {
		return rule.Spec.Manifest.DisplayName, []subjectList{
			{
				subjects: &rule.Spec.Manifest.Subjects,
			},
		}
	}), nil
}
