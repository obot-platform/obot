package provenance

import (
	"context"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	_ Store = (*MemStore)(nil)
)

// MemStore is an in-memory Store. It backs tests and lets a surface adapter
// run without a database.
type MemStore struct {
	lock        sync.Mutex
	sessions    map[string]*Session
	links       map[string]string
	events      []Event
	policies    map[string]*Policy
	approvals   map[string]*Approval
	kills       map[string]*Kill
	honeytokens map[string]*Honeytoken
	toolLabels  map[string]*ToolLabel
	settings    *Settings
}

// NewMemStore creates an empty store.
func NewMemStore() *MemStore {
	return &MemStore{
		sessions:    map[string]*Session{},
		links:       map[string]string{},
		policies:    map[string]*Policy{},
		approvals:   map[string]*Approval{},
		kills:       map[string]*Kill{},
		honeytokens: map[string]*Honeytoken{},
		toolLabels:  map[string]*ToolLabel{},
	}
}

func (m *MemStore) GetSession(_ context.Context, id string) (*Session, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, ErrNotFound
	}
	c := *s
	return &c, nil
}

func (m *MemStore) ListSessions(_ context.Context, q SessionQuery) ([]Session, int64, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	var out []Session
	for _, s := range m.sessions {
		if q.UserID != "" && s.UserID != q.UserID {
			continue
		}
		if q.Agent != "" && s.Agent != q.Agent {
			continue
		}
		if q.Surface != "" && !slices.Contains(s.State.Surfaces, q.Surface) {
			continue
		}
		if q.MinRisk > 0 && s.State.Risk < q.MinRisk {
			continue
		}
		if q.Label != "" && !s.State.Labels.Has(q.Label) {
			continue
		}
		if q.Killed != nil && s.State.Killed != *q.Killed {
			continue
		}
		if q.Open != nil && (s.EndedAt == nil) != *q.Open {
			continue
		}
		if !q.Start.IsZero() && s.LastActivityAt.Before(q.Start) {
			continue
		}
		if !q.End.IsZero() && s.StartedAt.After(q.End) {
			continue
		}
		if q.Query != "" && !strings.Contains(strings.ToLower(s.Title+" "+s.Email+" "+s.Agent+" "+s.State.LastSummary), strings.ToLower(q.Query)) {
			continue
		}
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		var less bool
		switch q.SortBy {
		case "risk":
			less = out[i].State.Risk < out[j].State.Risk
		case "started_at":
			less = out[i].StartedAt.Before(out[j].StartedAt)
		default:
			less = out[i].LastActivityAt.Before(out[j].LastActivityAt)
		}
		if q.SortDesc {
			return !less && (out[i].ID != out[j].ID)
		}
		return less
	})
	total := int64(len(out))
	return page(out, q.Offset, q.Limit), total, nil
}

func page[T any](items []T, offset, limit int) []T {
	if offset >= len(items) {
		return nil
	}
	items = items[offset:]
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items
}

func (m *MemStore) PutSession(_ context.Context, s *Session) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	c := *s
	m.sessions[s.ID] = &c
	return nil
}

func (m *MemStore) ResolveLink(_ context.Context, key string) (string, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.links[key], nil
}

func (m *MemStore) PutLink(_ context.Context, key, sessionID string) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.links[key] = sessionID
	return nil
}

func (m *MemStore) AppendEvents(_ context.Context, events []Event) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	for _, ev := range events {
		if ev.Action != nil {
			ev.Action = contentFree(ev.Action)
		}
		m.events = append(m.events, ev)
	}
	return nil
}

func (m *MemStore) ListEvents(_ context.Context, q EventQuery) ([]Event, int64, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	var out []Event
	for _, ev := range m.events {
		if q.SessionID != "" && ev.SessionID != q.SessionID {
			continue
		}
		if q.UserID != "" && ev.UserID != q.UserID {
			continue
		}
		if q.Surface != "" && ev.Surface != q.Surface {
			continue
		}
		if q.Kind != "" && ev.Kind != q.Kind {
			continue
		}
		if q.Decided != nil && (ev.Decision != nil) != *q.Decided {
			continue
		}
		if q.Denied != nil && (ev.Decision != nil && !ev.Decision.Allow) != *q.Denied {
			continue
		}
		if !q.Start.IsZero() && ev.At.Before(q.Start) {
			continue
		}
		if !q.End.IsZero() && ev.At.After(q.End) {
			continue
		}
		out = append(out, ev)
	}
	if !q.Ascending {
		slices.Reverse(out)
	}
	total := int64(len(out))
	return page(out, q.Offset, q.Limit), total, nil
}

func (m *MemStore) GetEvent(_ context.Context, id string) (*Event, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	for _, ev := range m.events {
		if ev.ID == id {
			c := ev
			return &c, nil
		}
	}
	return nil, ErrNotFound
}

func (m *MemStore) ListPolicies(context.Context) ([]Policy, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	out := make([]Policy, 0, len(m.policies))
	for _, p := range m.policies {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Priority > out[j].Priority })
	return out, nil
}

func (m *MemStore) GetPolicy(_ context.Context, id string) (*Policy, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	p, ok := m.policies[id]
	if !ok {
		return nil, ErrNotFound
	}
	c := *p
	return &c, nil
}

func (m *MemStore) PutPolicy(_ context.Context, p *Policy) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	if p.ID == "" {
		p.ID = NewID("pol")
	}
	c := *p
	m.policies[p.ID] = &c
	return nil
}

func (m *MemStore) DeletePolicy(_ context.Context, id string) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	delete(m.policies, id)
	return nil
}

func (m *MemStore) CreateApproval(_ context.Context, a *Approval) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	c := *a
	m.approvals[a.ID] = &c
	return nil
}

func (m *MemStore) GetApproval(_ context.Context, id string) (*Approval, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	a, ok := m.approvals[id]
	if !ok {
		return nil, ErrNotFound
	}
	c := *a
	c.Status = c.EffectiveStatus(time.Now())
	return &c, nil
}

func (m *MemStore) ListApprovals(_ context.Context, q ApprovalQuery) ([]Approval, int64, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	var out []Approval
	now := time.Now()
	for _, a := range m.approvals {
		current := *a
		current.Status = current.EffectiveStatus(now)
		if q.Status != "" && current.Status != q.Status {
			continue
		}
		if q.UserID != "" && a.UserID != q.UserID {
			continue
		}
		if q.SessionID != "" && a.SessionID != q.SessionID {
			continue
		}
		out = append(out, current)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	total := int64(len(out))
	return page(out, q.Offset, q.Limit), total, nil
}

func (m *MemStore) UpdateApproval(_ context.Context, a *Approval) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	if _, ok := m.approvals[a.ID]; !ok {
		return ErrNotFound
	}
	c := *a
	m.approvals[a.ID] = &c
	return nil
}

func (m *MemStore) ListKills(context.Context) ([]Kill, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	out := make([]Kill, 0, len(m.kills))
	for _, k := range m.kills {
		out = append(out, *k)
	}
	return out, nil
}

func (m *MemStore) PutKill(_ context.Context, k *Kill) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	c := *k
	m.kills[k.ID] = &c
	return nil
}

func (m *MemStore) DeleteKill(_ context.Context, id string) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	delete(m.kills, id)
	return nil
}

func (m *MemStore) ListHoneytokens(context.Context) ([]Honeytoken, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	out := make([]Honeytoken, 0, len(m.honeytokens))
	for _, h := range m.honeytokens {
		out = append(out, *h)
	}
	return out, nil
}

func (m *MemStore) PutHoneytoken(_ context.Context, h *Honeytoken) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	c := *h
	m.honeytokens[h.ID] = &c
	return nil
}

func (m *MemStore) DeleteHoneytoken(_ context.Context, id string) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	delete(m.honeytokens, id)
	return nil
}

func (m *MemStore) ListToolLabels(context.Context) ([]ToolLabel, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	out := make([]ToolLabel, 0, len(m.toolLabels))
	for _, l := range m.toolLabels {
		out = append(out, *l)
	}
	return out, nil
}

func (m *MemStore) PutToolLabel(_ context.Context, l *ToolLabel) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	c := *l
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = time.Now()
	}
	m.toolLabels[toolLabelKey(l.Server, l.Tool)] = &c
	return nil
}

func (m *MemStore) GetSettings(context.Context) (Settings, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.settings == nil {
		return Settings{}, ErrNotFound
	}
	return *m.settings, nil
}

func (m *MemStore) PutSettings(_ context.Context, s Settings) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.settings = &s
	return nil
}
