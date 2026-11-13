package handlers

import (
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
)

// Why a policy's subjects changed, in the details of its admin audit event.
const (
	subjectChangeCreate = "create"
	subjectChangeUpdate = "update"
	subjectChangeDelete = "delete"
)

// auditedSubject is the form a subject takes in an admin audit event.
type auditedSubject struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// isAgentRelevantSubject reports whether a policy that holds the subject can grant something to an agent: an agent
// subject, explicit or the agent wildcard, or the selector wildcard, which names everyone, agents included. User
// subjects, the user wildcard among them, and group and explicit-selector subjects never reach an agent, so changes
// to them are not audited here.
func isAgentRelevantSubject(subject types.Subject) bool {
	return subject.Type == types.SubjectTypeAgent ||
		(subject.Type == types.SubjectTypeSelector && subject.ID == types.SubjectWildcardID)
}

// agentRelevantSubjectChanges returns the agent-relevant subjects that current adds to, and removes from, previous.
func agentRelevantSubjectChanges(previous, current []types.Subject) (added, removed []auditedSubject) {
	for _, subject := range current {
		if isAgentRelevantSubject(subject) && !slices.Contains(previous, subject) {
			added = append(added, auditedSubject{Type: string(subject.Type), ID: subject.ID})
		}
	}
	for _, subject := range previous {
		if isAgentRelevantSubject(subject) && !slices.Contains(current, subject) {
			removed = append(removed, auditedSubject{Type: string(subject.Type), ID: subject.ID})
		}
	}
	return added, removed
}

// recordSubjectChange records, once the change that replaced previous with current has succeeded, the agent-relevant
// subjects it added and removed, as the calling administrator's. A change that adds or removes none records nothing.
// Recording is best effort: it logs, and never fails the request, whose change has been made.
func recordSubjectChange(req api.Context, action, resourceKind, resourceID, parentID, cause string, previous, current []types.Subject) {
	added, removed := agentRelevantSubjectChanges(previous, current)
	if len(added) == 0 && len(removed) == 0 {
		return
	}
	details := map[string]any{"cause": cause}
	if len(added) > 0 {
		details["added"] = added
	}
	if len(removed) > 0 {
		details["removed"] = removed
	}
	recordAdminAudit(req, gclient.AdminAuditEventInput{
		Action:       action,
		ResourceKind: resourceKind,
		ResourceID:   resourceID,
		ParentID:     parentID,
		Details:      details,
	})
}

// recordAdminAudit records an event of a change made to storage the gateway database cannot share a transaction with,
// as the calling administrator's. See gclient.Client.RecordAdminAuditEvent.
func recordAdminAudit(req api.Context, in gclient.AdminAuditEventInput) {
	if req.GatewayClient == nil {
		return
	}
	req.GatewayClient.RecordAdminAuditEvent(adminAuditContext(req), in)
}

// recordProfileSubjectChanges records the agent-relevant subject changes between the profiles of a vMCP before and
// after a change, profile by profile, by profile ID. A profile that is new has all its subjects added, and one that is
// gone has them removed.
func recordProfileSubjectChanges(req api.Context, vmcpID, cause string, previous, current []types.VMCPProfile) {
	for _, profile := range current {
		var before []types.Subject
		if i := slices.IndexFunc(previous, func(p types.VMCPProfile) bool { return p.ID == profile.ID }); i >= 0 {
			before = previous[i].Subjects
		}
		recordSubjectChange(req, gtypes.AdminAuditActionVMCPProfileSubjectsUpdate, gtypes.AdminAuditResourceVMCPProfile, profile.ID, vmcpID, cause, before, profile.Subjects)
	}
	for _, profile := range previous {
		if !slices.ContainsFunc(current, func(p types.VMCPProfile) bool { return p.ID == profile.ID }) {
			recordSubjectChange(req, gtypes.AdminAuditActionVMCPProfileSubjectsUpdate, gtypes.AdminAuditResourceVMCPProfile, profile.ID, vmcpID, cause, profile.Subjects, nil)
		}
	}
}
