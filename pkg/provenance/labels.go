package provenance

import (
	"slices"
	"strings"
)

const (
	// Capability labels describe what a tool can do. They are assigned to catalog
	// tools, automatically from schemas and observed traffic and confirmed by an
	// administrator, and to built-in agent tools by kind.

	// CapReadsPrivate: the tool reads organization-private data (CRM, code,
	// documents, email, databases).
	CapReadsPrivate Label = "reads-private"
	// CapReadsUntrusted: the tool reads content an outsider could have written
	// (web pages, inbound email, public issues and PRs, tickets, chat from
	// external users).
	CapReadsUntrusted Label = "reads-untrusted"
	// CapWritesExternal: the tool sends data somewhere outside the
	// organization's control (HTTP to arbitrary hosts, email, public posts).
	CapWritesExternal Label = "writes-external"
	// CapWritesInternal: the tool writes to an organization system.
	CapWritesInternal Label = "writes-internal"
	// CapExec: the tool executes code or shell commands.
	CapExec Label = "exec"
	// CapDestructive: the tool can delete or irreversibly change data.
	CapDestructive Label = "destructive"
	// CapCredentialBearing: the tool carries or returns credentials.
	CapCredentialBearing Label = "credential-bearing"

	// Sensitivity labels describe content that entered a session.
	LabelPII          Label = "pii"
	LabelSecret       Label = "secret"
	LabelCustomerData Label = "customer-data"
	LabelSourceCode   Label = "source-code"
	LabelFinancial    Label = "financial"
	LabelHealth       Label = "health"
	LabelConfidential Label = "confidential" // any organization-private data

	// Trust labels describe where content came from.

	// LabelUntrusted: the content was authored, or could have been authored,
	// by someone outside the organization. Once it is in the context the
	// model's instructions can no longer be assumed to be the user's.
	LabelUntrusted Label = "untrusted"
	// LabelHoneytoken: the content contained a planted canary.
	LabelHoneytoken Label = "honeytoken"
)

var (
	// CapabilityLabels lists every capability label, for validation and UIs.
	CapabilityLabels = Labels{
		CapReadsPrivate, CapReadsUntrusted, CapWritesExternal, CapWritesInternal,
		CapExec, CapDestructive, CapCredentialBearing,
	}

	// ContentLabels lists every sensitivity and trust label.
	ContentLabels = Labels{
		LabelPII, LabelSecret, LabelCustomerData, LabelSourceCode, LabelFinancial,
		LabelHealth, LabelConfidential, LabelUntrusted, LabelHoneytoken,
	}

	// ConfidentialLabels are the sensitivity labels that imply confidential.
	ConfidentialLabels = Labels{
		LabelPII, LabelSecret, LabelCustomerData, LabelSourceCode, LabelFinancial, LabelHealth,
	}
)

// Label is a tag on a tool (a capability label), on content (a sensitivity or
// trust label) or on a session (the union of everything that entered it).
type Label string

// Labels is a set of labels, kept sorted and unique.
type Labels []Label

// NewLabels builds a sorted, de-duplicated set.
func NewLabels(labels ...Label) Labels {
	var out Labels
	return out.Add(labels...)
}

// Add returns the set with labels added.
func (l Labels) Add(labels ...Label) Labels {
	out := slices.Clone(l)
	for _, label := range labels {
		if label == "" {
			continue
		}
		if !slices.Contains(out, label) {
			out = append(out, label)
		}
	}
	slices.Sort(out)
	return out
}

// Has reports whether the set contains label.
func (l Labels) Has(label Label) bool {
	return slices.Contains(l, label)
}

// HasAny reports whether the set contains any of labels.
func (l Labels) HasAny(labels ...Label) bool {
	return slices.ContainsFunc(labels, l.Has)
}

// Strings returns the labels as strings, for CEL and JSON.
func (l Labels) Strings() []string {
	out := make([]string, 0, len(l))
	for _, label := range l {
		out = append(out, string(label))
	}
	return out
}

// Anys returns the labels as []any, which is what CEL list variables want.
func (l Labels) Anys() []any {
	out := make([]any, 0, len(l))
	for _, label := range l {
		out = append(out, string(label))
	}
	return out
}

// String joins the labels for display.
func (l Labels) String() string {
	return strings.Join(l.Strings(), ",")
}

// ParseLabels splits a comma-separated label list.
func ParseLabels(s string) Labels {
	var out Labels
	for part := range strings.SplitSeq(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = out.Add(Label(part))
		}
	}
	return out
}

// Confidential reports whether the set carries any confidential content.
func (l Labels) Confidential() bool {
	return l.Has(LabelConfidential) || l.HasAny(ConfidentialLabels...)
}

// Normalize adds the implied labels: any sensitivity label implies
// confidential.
func (l Labels) Normalize() Labels {
	if l.HasAny(ConfidentialLabels...) {
		return l.Add(LabelConfidential)
	}
	return l
}

// SourceFromCapabilities derives what an action reads from its tool's
// capability labels.
func SourceFromCapabilities(caps Labels) Source {
	return Source{
		Private:   caps.Has(CapReadsPrivate),
		Untrusted: caps.Has(CapReadsUntrusted),
	}
}

// SinkFromCapabilities derives where an action writes from its tool's
// capability labels. The destination and the external judgement are refined
// by the adapter when it knows the host.
func SinkFromCapabilities(caps Labels) Sink {
	return Sink{
		External: caps.Has(CapWritesExternal),
		Writes:   caps.HasAny(CapWritesExternal, CapWritesInternal, CapDestructive),
		Exec:     caps.Has(CapExec),
	}
}

// ToolCapabilityHeuristics assigns capability labels to a tool from its name
// and description alone. It is the cold-start classifier; an administrator
// can confirm or override the result and an LLM-backed classifier refines it.
func ToolCapabilityHeuristics(server, tool, description string) Labels {
	name := strings.ToLower(server + " " + tool)
	desc := strings.ToLower(description)
	all := name + " " + desc
	var caps Labels

	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(all, w) {
				return true
			}
		}
		return false
	}
	nameHas := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(name, w) {
				return true
			}
		}
		return false
	}

	switch {
	case nameHas("shell", "bash", "exec", "run_command", "terminal", "execute", "python", "eval"):
		caps = caps.Add(CapExec)
	}
	if nameHas("delete", "remove", "drop", "destroy", "purge", "rm_") || has("permanently delete") {
		caps = caps.Add(CapDestructive)
	}
	if nameHas("send", "post", "publish", "email", "mail", "tweet", "upload", "fetch", "http", "curl", "webhook", "notify", "message") {
		caps = caps.Add(CapWritesExternal)
	}
	if nameHas("create", "update", "write", "edit", "insert", "put", "patch", "set_", "comment", "reply") {
		caps = caps.Add(CapWritesInternal)
	}
	if nameHas("search", "fetch", "browse", "web", "scrape", "crawl", "issue", "ticket", "inbox", "read_email", "list_messages", "pull_request", "review") || has("public", "internet", "web page", "inbound") {
		caps = caps.Add(CapReadsUntrusted)
	}
	if nameHas("query", "get", "read", "list", "search", "salesforce", "crm", "hubspot", "database", "sql", "customer", "contact", "drive", "sharepoint", "confluence", "notion", "jira", "gmail", "outlook", "slack", "github", "gitlab", "repo", "file") || has("private", "internal", "customer") {
		caps = caps.Add(CapReadsPrivate)
	}
	if nameHas("secret", "credential", "token", "password", "apikey", "api_key", "vault", "keychain") {
		caps = caps.Add(CapCredentialBearing)
	}
	return caps
}

// BuiltinToolCapabilities maps a device tool kind (pkg/enforcement) to the
// capability labels of the built-in agent tool.
func BuiltinToolCapabilities(kind string) Labels {
	switch kind {
	case "shell":
		return NewLabels(CapExec, CapReadsPrivate, CapWritesExternal, CapWritesInternal, CapDestructive)
	case "read":
		return NewLabels(CapReadsPrivate)
	case "write":
		return NewLabels(CapWritesInternal, CapDestructive)
	case "task":
		return NewLabels(CapExec)
	}
	return nil
}
