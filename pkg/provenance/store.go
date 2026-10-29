package provenance

import (
	"context"
	"errors"
	"time"
)

const (
	ApprovalPending  = "pending"
	ApprovalApproved = "approved"
	ApprovalDenied   = "denied"
	ApprovalExpired  = "expired"

	KillScopeOrg     = "org"
	KillScopeUser    = "user"
	KillScopeAgent   = "agent"
	KillScopeSession = "session"
)

var (
	// ErrNotFound is returned by stores for a missing record.
	ErrNotFound = errors.New("not found")
)

// SessionQuery filters and pages a session list.
type SessionQuery struct {
	UserID   string
	Agent    string
	Surface  Surface
	MinRisk  int
	Label    Label
	Killed   *bool
	Open     *bool
	Query    string
	Start    time.Time
	End      time.Time
	Limit    int
	Offset   int
	SortBy   string // started_at | last_activity_at | risk
	SortDesc bool
}

// EventQuery filters and pages an event list.
type EventQuery struct {
	SessionID string
	UserID    string
	Surface   Surface
	Kind      EventKind
	Decided   *bool // only events with a decision
	Denied    *bool
	Start     time.Time
	End       time.Time
	Limit     int
	Offset    int
	Ascending bool
}

// Approval is a request for a person to allow an action a policy held.
type Approval struct {
	ID         string    `json:"id"`
	SessionID  string    `json:"sessionID"`
	EventID    string    `json:"eventID"`
	UserID     string    `json:"userID,omitempty"`
	PolicyID   string    `json:"policyID"`
	PolicyName string    `json:"policyName"`
	Action     Action    `json:"action"`
	Reason     string    `json:"reason"`
	Approvers  []string  `json:"approvers,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
	// Status is pending, approved, denied or expired.
	Status     string     `json:"status"`
	ResolvedBy string     `json:"resolvedBy,omitempty"`
	ResolvedAt *time.Time `json:"resolvedAt,omitempty"`
	Note       string     `json:"note,omitempty"`
	// Triage is the Supervisor's recommendation, when it reviewed the request.
	Triage *Triage `json:"triage,omitempty"`
	// Risk is the session's risk at the time.
	Risk int `json:"risk"`
	// Labels are the session's labels at the time.
	Labels Labels `json:"labels,omitempty"`
}

// ApprovalQuery filters approvals.
type ApprovalQuery struct {
	Status    string
	UserID    string
	SessionID string
	Limit     int
	Offset    int
}

// Kill is an active kill switch at some scope. A killed scope denies every
// action on every surface within seconds.
type Kill struct {
	ID string `json:"id"`
	// Scope is org, user, agent or session.
	Scope     string    `json:"scope"`
	Target    string    `json:"target,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	CreatedBy string    `json:"createdBy,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// Honeytoken is a planted canary. Its value is stored hashed; Obot recognizes
// it when it appears in an outbound action.
type Honeytoken struct {
	ID string `json:"id"`
	// Kind is aws-key, api-key, email, record or custom.
	Kind string `json:"kind"`
	// Name says where it was planted ("CRM demo account", "repo .env").
	Name string `json:"name"`
	// Hash is hashExact of the value.
	Hash uint64 `json:"hash"`
	// Hint is the first few characters, for recognizing the token in lists.
	Hint      string    `json:"hint,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	// Tripped counts how many times it was seen leaving.
	Tripped   int        `json:"tripped"`
	LastTrip  *time.Time `json:"lastTrip,omitempty"`
	CreatedBy string     `json:"createdBy,omitempty"`
}

// ToolLabel is the capability labelling of one tool on one server.
type ToolLabel struct {
	// Server is the MCP server display name or catalog entry; Tool is the tool
	// name. Server "*" with Tool kind names labels built-in agent tools.
	Server string `json:"server"`
	Tool   string `json:"tool"`
	Labels Labels `json:"labels"`
	// Source is heuristic, supervisor or admin. Admin labels are never
	// overwritten automatically.
	Source    string    `json:"source"`
	Rationale string    `json:"rationale,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Store persists sessions, events, policies and the rest. The engine keeps
// hot session state in memory and writes through.
type Store interface {
	GetSession(ctx context.Context, id string) (*Session, error)
	ListSessions(ctx context.Context, q SessionQuery) ([]Session, int64, error)
	PutSession(ctx context.Context, s *Session) error
	// ResolveLink returns the session a stitching key points at, or "" when
	// none does.
	ResolveLink(ctx context.Context, key string) (string, error)
	PutLink(ctx context.Context, key, sessionID string) error

	AppendEvents(ctx context.Context, events []Event) error
	ListEvents(ctx context.Context, q EventQuery) ([]Event, int64, error)
	GetEvent(ctx context.Context, id string) (*Event, error)

	ListPolicies(ctx context.Context) ([]Policy, error)
	GetPolicy(ctx context.Context, id string) (*Policy, error)
	PutPolicy(ctx context.Context, p *Policy) error
	DeletePolicy(ctx context.Context, id string) error

	CreateApproval(ctx context.Context, a *Approval) error
	GetApproval(ctx context.Context, id string) (*Approval, error)
	ListApprovals(ctx context.Context, q ApprovalQuery) ([]Approval, int64, error)
	UpdateApproval(ctx context.Context, a *Approval) error

	ListKills(ctx context.Context) ([]Kill, error)
	PutKill(ctx context.Context, k *Kill) error
	DeleteKill(ctx context.Context, id string) error

	ListHoneytokens(ctx context.Context) ([]Honeytoken, error)
	PutHoneytoken(ctx context.Context, h *Honeytoken) error
	DeleteHoneytoken(ctx context.Context, id string) error

	ListToolLabels(ctx context.Context) ([]ToolLabel, error)
	PutToolLabel(ctx context.Context, l *ToolLabel) error

	GetSettings(ctx context.Context) (Settings, error)
	PutSettings(ctx context.Context, s Settings) error
}

// Matches reports whether the kill applies to a session.
func (k Kill) Matches(s *Session, now time.Time) bool {
	if !k.ExpiresAt.IsZero() && now.After(k.ExpiresAt) {
		return false
	}
	switch k.Scope {
	case KillScopeOrg:
		return true
	case KillScopeUser:
		return s != nil && s.UserID != "" && s.UserID == k.Target
	case KillScopeAgent:
		return s != nil && s.Agent == k.Target
	case KillScopeSession:
		return s != nil && s.ID == k.Target
	}
	return false
}
