package copilot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"github.com/obot-platform/obot/pkg/provenance"
)

const (
	// Agent is the actor reported for every Copilot Studio agent.
	Agent = "copilot_studio"
)

var (
	urlHostRe = regexp.MustCompile(`(?i)\bhttps?://([^/\s:?#"']+)`)
	emailRe   = regexp.MustCompile(`(?i)\b[a-z0-9._%+-]+@([a-z0-9.-]+\.[a-z]{2,})\b`)
)

// Engine is what the builders need from the provenance engine.
type Engine interface {
	ToolCapabilities(ctx context.Context, server, tool, description string) provenance.Labels
	IsExternal(ctx context.Context, host string) bool
}

// UserResolver maps a Copilot Studio user (the Microsoft Entra object ID and,
// when the payload carries one, the email or UPN) to an Obot user ID. It
// returns "" when the user is not known to Obot.
type UserResolver func(ctx context.Context, entraObjectID, email string) (string, error)

// Dedupe remembers content already ingested for a conversation so the chat
// history and previous tool outputs, which Copilot Studio resends on every
// call, enter the session once. It is per process; a replica that has not
// seen a conversation ingests its history once more, which only repeats
// labels the session already carries.
type Dedupe struct {
	lock  sync.Mutex
	seen  map[string]struct{}
	order []string
	limit int
}

// ResolveUserID resolves the request's user with resolver, or returns ""
// when there is no resolver or the lookup fails.
func ResolveUserID(ctx context.Context, resolver UserResolver, r *Request) string {
	if resolver == nil || (r.UserID == "" && r.UserEmail == "") {
		return ""
	}
	id, err := resolver(ctx, r.UserID, r.UserEmail)
	if err != nil {
		return ""
	}
	return id
}

// Subject is who the agent acts for. The delegation chain records the Entra
// user and the Copilot Studio agent so a session can be traced to the agent
// even when the user is not an Obot user.
func Subject(r *Request, userID string) provenance.Subject {
	actor := provenance.Actor{Agent: Agent, ClientSessionID: r.ConversationID}
	if principal := first(r.UserEmail, prefixed("entra:", r.UserID)); principal != "" {
		actor.Delegation = append(actor.Delegation, principal)
	}
	if agent := first(r.AgentName, r.AgentID); agent != "" {
		actor.Delegation = append(actor.Delegation, Agent+":"+agent)
	}
	return provenance.Subject{UserID: userID, Email: r.UserEmail, Actor: actor}
}

// Session is the stitching key: the Copilot Studio conversation.
func Session(r *Request) provenance.SessionKey {
	return provenance.SessionKey{ClientSessionID: r.ConversationID, Agent: Agent}
}

// ServerName is the server an action is attributed to: its connector when
// the payload says, otherwise the agent itself.
func ServerName(r *Request, a PlanAction) string {
	return first(a.Connector, r.AgentName, r.AgentID, Agent)
}

// BuildActions turns the planned tool invocations into actions to decide.
// Capabilities come from the engine's labelling (admin, Supervisor or
// heuristic); the sink and source are derived from them, and the sink's
// destination and externality are refined from a URL or email address in
// the inputs when the tool writes.
func BuildActions(ctx context.Context, engine Engine, r *Request) []provenance.Action {
	out := make([]provenance.Action, 0, len(r.Actions))
	for _, a := range r.Actions {
		server := ServerName(r, a)
		caps := engine.ToolCapabilities(ctx, server, a.Name, a.Description)
		action := provenance.Action{
			Surface:      provenance.SurfaceCopilot,
			Kind:         provenance.KindToolCall,
			Tool:         a.Name,
			Server:       server,
			Capabilities: caps,
			Sink:         provenance.SinkFromCapabilities(caps),
			Source:       provenance.SourceFromCapabilities(caps),
			Arguments:    a.Inputs,
		}
		if host := Destination(a.Inputs); host != "" {
			action.Sink.Destination = host
			if action.Sink.Writes {
				action.Sink.External = engine.IsExternal(ctx, host)
			}
		}
		out = append(out, action)
	}
	return out
}

// Destination finds where an action's inputs point: the host of the first
// URL, else the domain of the first email address, else "".
func Destination(inputs map[string]any) string {
	if len(inputs) == 0 {
		return ""
	}
	text := provenance.FlattenArguments(inputs)
	if m := urlHostRe.FindStringSubmatch(text); m != nil {
		return strings.ToLower(m[1])
	}
	if m := emailRe.FindStringSubmatch(text); m != nil {
		return strings.ToLower(m[1])
	}
	return ""
}

// ToolResultCapabilities returns the labels to ingest a tool output with: the
// engine's labelling of the tool, and reads-untrusted when the labelling says
// nothing about what the tool reads, since an unknown Copilot connector may
// return content anyone could have written.
func ToolResultCapabilities(ctx context.Context, engine Engine, server, tool string) provenance.Labels {
	caps := engine.ToolCapabilities(ctx, server, tool, "")
	if !caps.HasAny(provenance.CapReadsPrivate, provenance.CapReadsUntrusted) {
		caps = caps.Add(provenance.CapReadsUntrusted)
	}
	return caps
}

// Summary shortens a prompt to a content-light session title: whitespace
// collapsed, at most 80 characters.
func Summary(prompt string) string {
	fields := strings.FieldsFunc(prompt, unicode.IsSpace)
	s := strings.Join(fields, " ")
	runes := []rune(s)
	if len(runes) > 80 {
		return string(runes[:77]) + "..."
	}
	return s
}

func prefixed(prefix, value string) string {
	if value == "" {
		return ""
	}
	return prefix + value
}

// NewDedupe creates a cache that remembers at most limit entries.
func NewDedupe(limit int) *Dedupe {
	if limit <= 0 {
		limit = 4096
	}
	return &Dedupe{seen: map[string]struct{}{}, limit: limit}
}

// Seen reports whether content was already recorded under scope and kind,
// recording it otherwise.
func (d *Dedupe) Seen(scope, kind, content string) bool {
	if d == nil {
		return false
	}
	sum := sha256.Sum256([]byte(scope + "\x00" + kind + "\x00" + content))
	key := hex.EncodeToString(sum[:])
	d.lock.Lock()
	defer d.lock.Unlock()
	if _, ok := d.seen[key]; ok {
		return true
	}
	d.seen[key] = struct{}{}
	d.order = append(d.order, key)
	if len(d.order) > d.limit {
		drop := len(d.order) / 2
		for _, old := range d.order[:drop] {
			delete(d.seen, old)
		}
		d.order = append([]string(nil), d.order[drop:]...)
	}
	return false
}
