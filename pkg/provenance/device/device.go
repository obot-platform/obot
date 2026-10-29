// Package device maps what Obot Sentry's hooks report from a laptop onto the
// provenance session graph: a pre-tool decision request becomes an Action for
// the engine to decide, and a post-tool audit event becomes an IngestRequest
// whose result content taints the session.
//
// The package is pure: it knows nothing about the engine beyond the two
// lookups a Resolver provides, so the mappings can be tested as tables.
package device

import (
	"encoding/json"
	"net"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/enforcement"
	"github.com/obot-platform/obot/pkg/provenance"
)

const (
	// MaxIngestBytes caps the tool result content handed to the engine. Content
	// is scanned, never stored, and sensitive spans past the cap only lose
	// lineage precision.
	MaxIngestBytes = 1 << 20
)

var (
	// urlRe finds a URL with a scheme anywhere in a command, capturing its host.
	urlRe = regexp.MustCompile(`(?i)\b(?:https?|ftp|ftps|ssh|sftp|wss?)://(?:[^\s/@"'<>]*@)?(\[[0-9a-f:.]+\]|[^\s/:?#"'<>\[\]]+)`)
	// hostRe matches a bare dotted host name with an optional port.
	hostRe = regexp.MustCompile(`^([a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+)(?::\d{1,5})?$`)
	// ipv4Re matches a bare IPv4 address with an optional port.
	ipv4Re = regexp.MustCompile(`^(\d{1,3}(?:\.\d{1,3}){3})(?::\d{1,5})?$`)

	// networkClients are commands whose positional argument is a host. The
	// value reports whether the argument may carry "user@" and ":path" (the
	// ssh family) or is a URL-like host[/path] (the HTTP family).
	networkClients = map[string]bool{
		"curl": false, "wget": false, "http": false, "https": false, "xh": false, "httpie": false,
		"nc": false, "ncat": false, "netcat": false, "telnet": false, "openssl": false,
		"ssh": true, "scp": true, "sftp": true, "rsync": true,
	}

	// fileExtensions are last labels that make a dotted token a file name, not
	// a host, when it has no URL path.
	fileExtensions = map[string]bool{
		"txt": true, "json": true, "jsonl": true, "csv": true, "tsv": true, "log": true, "md": true, "html": true, "htm": true,
		"xml": true, "yaml": true, "yml": true, "toml": true, "ini": true, "conf": true, "cfg": true, "env": true,
		"sh": true, "bash": true, "zsh": true, "py": true, "js": true, "ts": true, "tsx": true, "jsx": true, "go": true, "rs": true,
		"java": true, "rb": true, "php": true, "c": true, "h": true, "cpp": true, "cs": true, "swift": true, "kt": true,
		"tar": true, "gz": true, "tgz": true, "zip": true, "bz2": true, "xz": true, "zst": true, "7z": true,
		"pem": true, "key": true, "crt": true, "cer": true, "p12": true, "pub": true,
		"png": true, "jpg": true, "jpeg": true, "gif": true, "svg": true, "pdf": true, "mp4": true, "mp3": true,
		"sql": true, "db": true, "sqlite": true, "bak": true, "tmp": true, "lock": true, "sum": true, "mod": true,
	}
)

// Resolver provides the two engine lookups the mappings need. A nil function
// falls back to the conservative default: heuristic capability labels, and
// every non-local host external.
type Resolver struct {
	// ToolCapabilities returns the capability labels of an MCP tool
	// (engine.ToolCapabilities with an empty description).
	ToolCapabilities func(server, tool string) provenance.Labels
	// IsExternal reports whether a host is outside the organization's approved
	// destinations (engine.IsExternal).
	IsExternal func(host string) bool
}

func (r Resolver) toolCapabilities(server, tool string) provenance.Labels {
	if r.ToolCapabilities != nil {
		return r.ToolCapabilities(server, tool)
	}
	return provenance.ToolCapabilityHeuristics(server, tool, "")
}

// isExternal is IsExternal with the local machine carved out: the loopback
// interface, link-local and private ranges and ".local" names are the laptop
// itself or its LAN, never an exfiltration destination.
func (r Resolver) isExternal(host string) bool {
	if host == "" || IsLocalHost(host) {
		return false
	}
	if r.IsExternal != nil {
		return r.IsExternal(host)
	}
	return true
}

// IsLocalHost reports whether host names the local machine or a private
// network address.
func IsLocalHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "localhost" || host == "0.0.0.0" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}

// Key is the session key for a device hook's report: the agent's own session
// identifier scoped by the agent, falling back to the device and agent.
func Key(in types.EnforcementDecisionRequest, deviceID string) provenance.SessionKey {
	return provenance.SessionKey{
		ClientSessionID: in.SessionID,
		Agent:           in.Agent,
		DeviceID:        deviceID,
	}
}

// Subject is who a device hook's call is performed for. Devices are not tied
// to Obot users, so userID is empty for a device principal and the reported
// email is the only user hint; a user principal (an API key with the
// device-scans scope) carries its user ID.
func Subject(in types.EnforcementDecisionRequest, deviceID, userID string) provenance.Subject {
	return provenance.Subject{
		UserID: userID,
		Email:  strings.TrimSpace(in.UserEmail),
		Actor: provenance.Actor{
			Agent:           in.Agent,
			DeviceID:        deviceID,
			ClientSessionID: in.SessionID,
		},
	}
}

// BuildAction maps a decision request to the normalized action the engine
// decides. The device tool kinds map to event kinds as follows:
//
//	shell        → shell.exec  (Command from the input; external sink when the
//	                            command references a host outside the approved set)
//	read         → file.read   (File from the input)
//	write        → file.write  (File from the input)
//	mcp          → tool.call   (Server and Tool from the resolved call; the tool's
//	                            capability labels decide the sink and source)
//	task/generic → tool.call   (built-in tool capabilities)
//
// Arguments is always the decoded input, so the engine can scan it for
// lineage.
func BuildAction(in types.EnforcementDecisionRequest, call enforcement.NormalizedCall, r Resolver) provenance.Action {
	args := Arguments(in.Input)
	a := provenance.Action{
		Surface:   provenance.SurfaceDevice,
		Tool:      in.Tool,
		Arguments: args,
	}

	switch in.Kind {
	case enforcement.KindShell:
		a.Kind = provenance.KindShellExec
		a.Command = stringArg(args, "command", "cmd")
		a.Capabilities = provenance.BuiltinToolCapabilities(in.Kind)
		a.Source = provenance.SourceFromCapabilities(a.Capabilities)
		a.Sink = provenance.Sink{Writes: true, Exec: true}
		hosts := CommandHosts(a.Command)
		for _, h := range hosts {
			if r.isExternal(h) {
				a.Sink.External = true
				a.Sink.Destination = h
				break
			}
		}
		if a.Sink.Destination == "" && len(hosts) > 0 {
			a.Sink.Destination = hosts[0]
		}
		if a.Sink.Destination != "" {
			a.Host = a.Sink.Destination
		}

	case enforcement.KindRead:
		a.Kind = provenance.KindFileRead
		a.File = stringArg(args, "file_path", "path", "notebook_path")
		a.Capabilities = provenance.BuiltinToolCapabilities(in.Kind)
		a.Source = provenance.SourceFromCapabilities(a.Capabilities)
		a.Sink = provenance.SinkFromCapabilities(a.Capabilities)

	case enforcement.KindWrite:
		a.Kind = provenance.KindFileWrite
		a.File = stringArg(args, "file_path", "path", "notebook_path")
		a.Capabilities = provenance.BuiltinToolCapabilities(in.Kind)
		a.Source = provenance.SourceFromCapabilities(a.Capabilities)
		a.Sink = provenance.SinkFromCapabilities(a.Capabilities)
		a.Sink.Destination = a.File

	case enforcement.KindMCP:
		a.Kind = provenance.KindToolCall
		a.Server = ServerName(call)
		a.Tool = ToolName(in.Tool, call.ServerName)
		a.Capabilities = r.toolCapabilities(a.Server, a.Tool)
		a.Source = provenance.SourceFromCapabilities(a.Capabilities)
		a.Sink = provenance.SinkFromCapabilities(a.Capabilities)
		// A URL server receives the arguments of every call, so the host is the
		// sink regardless of what the tool is labelled to write. An Obot-hosted
		// server is the organization's own gateway and keeps the label-derived
		// judgement; the MCP gateway surface decides the call again with full
		// knowledge of the server.
		if host := callHost(call); host != "" {
			a.Host = host
			a.Sink.Destination = host
			if !call.ObotHosted {
				a.Sink.External = a.Sink.External || r.isExternal(host)
			}
		}

	default:
		a.Kind = provenance.KindToolCall
		a.Capabilities = provenance.BuiltinToolCapabilities(in.Kind)
		a.Source = provenance.SourceFromCapabilities(a.Capabilities)
		a.Sink = provenance.SinkFromCapabilities(a.Capabilities)
	}
	return a
}

// BuildIngest maps a completed local-agent tool call to the content entering
// the session: the tool's result, labelled by what the tool can read. A shell
// command that talked to a host outside the machine read untrusted web
// content from that host; any other shell command, and a file read, read
// private data; an MCP tool carries its own capability labels.
func BuildIngest(ev types.LocalAgentToolCallAuditLogInput, deviceID, userID string, r Resolver) provenance.IngestRequest {
	provider := string(ev.Details.Agent.Provider)
	sessionID := ev.Details.Trace.SessionID
	content, size := ResultContent(ev.Details.Response.Body)
	req := provenance.IngestRequest{
		Subject: provenance.Subject{
			UserID: userID,
			Email:  strings.TrimSpace(ev.Details.Environment.ReportedUserEmail),
			Actor: provenance.Actor{
				Agent:           provider,
				AgentVersion:    ev.Details.Agent.Version,
				DeviceID:        deviceID,
				ClientSessionID: sessionID,
			},
		},
		Session: provenance.SessionKey{
			ClientSessionID: sessionID,
			Agent:           provider,
			DeviceID:        deviceID,
		},
		Surface: provenance.SurfaceDevice,
		Kind:    provenance.KindToolResult,
		Tool:    firstNonEmpty(ev.Target.Name, ev.Action.Name),
		Content: content,
		Bytes:   size,
	}

	switch {
	case ev.Target.TargetType == types.AuditLogTargetTypeMCPTool:
		server := ""
		if ev.Target.Parent != nil {
			server = ev.Target.Parent.Name
		}
		req.Server = server
		req.Tool = ToolName(req.Tool, server)
		req.Capabilities = r.toolCapabilities(server, req.Tool)
	case ev.Action.Kind == enforcement.KindShell:
		command := stringArg(Arguments(ev.Details.Request.Body), "command", "cmd")
		if hosts := CommandHosts(command); len(hosts) > 0 {
			req.Host = hosts[0]
			req.Capabilities = provenance.NewLabels(provenance.CapReadsUntrusted)
		} else {
			req.Capabilities = provenance.NewLabels(provenance.CapReadsPrivate)
		}
	case ev.Action.Kind == enforcement.KindRead:
		req.Kind = provenance.KindFileRead
		req.Capabilities = provenance.NewLabels(provenance.CapReadsPrivate)
	default:
		req.Capabilities = provenance.BuiltinToolCapabilities(ev.Action.Kind)
	}
	return req
}

// ResultContent renders a reported tool result for scanning: a JSON string is
// its text, a JSON object or array is flattened with its keys, anything else
// is taken as is. The returned size is the reported body's length; the
// content is cut at MaxIngestBytes.
func ResultContent(body json.RawMessage) (content string, size int) {
	size = len(body)
	raw := body
	if len(raw) > MaxIngestBytes {
		raw = raw[:MaxIngestBytes]
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return "", size
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return trimmed, size
	}
	switch t := v.(type) {
	case string:
		return t, size
	case map[string]any, []any:
		return provenance.FlattenArguments(t), size
	default:
		return trimmed, size
	}
}

// Arguments decodes a reported tool input. A JSON object is returned as is;
// any other JSON value, or text that is not JSON, is wrapped as {"input": v}
// so it is still scanned.
func Arguments(input json.RawMessage) map[string]any {
	trimmed := strings.TrimSpace(string(input))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	var v any
	if err := json.Unmarshal(input, &v); err != nil {
		return map[string]any{"input": trimmed}
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{"input": v}
}

// ToolName strips an agent's MCP tool prefix ("mcp__<server>__<tool>") so the
// tool is named as the server knows it.
func ToolName(runtimeTool, server string) string {
	rest, ok := strings.CutPrefix(runtimeTool, "mcp__")
	if !ok {
		return runtimeTool
	}
	if server != "" {
		if tool, ok := strings.CutPrefix(rest, server+"__"); ok {
			return tool
		}
	}
	if _, tool, ok := strings.Cut(rest, "__"); ok {
		return tool
	}
	return runtimeTool
}

// ServerName is the display name of a resolved MCP server: the agent's own
// name for it, else the host, package, connector or command it resolved to.
func ServerName(call enforcement.NormalizedCall) string {
	if call.ServerName != "" {
		return call.ServerName
	}
	if host := callHost(call); host != "" {
		return host
	}
	if call.Server.Package != nil && call.Server.Package.Name != "" {
		return call.Server.Package.Name
	}
	if call.Server.Connector != "" {
		return call.Server.Connector
	}
	if fields := strings.Fields(call.Server.Command); len(fields) > 0 {
		return path.Base(fields[0])
	}
	return ""
}

func callHost(call enforcement.NormalizedCall) string {
	if call.Server.Hostname != "" {
		return strings.ToLower(call.Server.Hostname)
	}
	if call.Server.URL == "" {
		return ""
	}
	u, err := url.Parse(call.Server.URL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// CommandHosts returns the hosts a shell command talks to: the host of every
// URL in it, and the host argument of a network client (curl, wget, ssh,
// scp, ...). Hosts are lower-cased, unique, in order of appearance.
func CommandHosts(command string) []string {
	var hosts []string
	add := func(h string) {
		h = strings.ToLower(strings.TrimSuffix(h, "."))
		if h != "" && !slices.Contains(hosts, h) {
			hosts = append(hosts, h)
		}
	}
	for _, m := range urlRe.FindAllStringSubmatch(command, -1) {
		add(m[1])
	}

	tokens := strings.Fields(command)
	for i := 0; i < len(tokens); i++ {
		client, ok := networkClients[path.Base(strings.Trim(tokens[i], `"'`))]
		if !ok {
			continue
		}
		for j := i + 1; j < len(tokens); j++ {
			arg := strings.Trim(tokens[j], `"'`)
			if isShellOperator(arg) {
				i = j
				break
			}
			if strings.HasPrefix(arg, "-") || strings.Contains(arg, "://") {
				continue
			}
			if h := bareHost(arg, client); h != "" {
				add(h)
			}
		}
	}
	return hosts
}

// bareHost extracts a host from a positional network-client argument, or
// returns "" when the argument is not one. HTTP clients take host[:port][/path];
// the ssh family takes [user@]host[:path].
func bareHost(arg string, sshStyle bool) string {
	if sshStyle {
		if _, rest, ok := strings.Cut(arg, "@"); ok {
			arg = rest
		}
		if host, _, ok := strings.Cut(arg, ":"); ok && host != "" {
			arg = host
		}
	}
	if strings.ContainsAny(arg, "@=,;$`\\") {
		return ""
	}
	host, pathPart, hasPath := strings.Cut(arg, "/")
	host = strings.ToLower(host)
	if m := ipv4Re.FindStringSubmatch(host); m != nil {
		return m[1]
	}
	m := hostRe.FindStringSubmatch(host)
	if m == nil {
		return ""
	}
	host = m[1]
	last := host[strings.LastIndex(host, ".")+1:]
	if !hasPath || pathPart == "" {
		if fileExtensions[last] || len(last) < 2 || strings.ContainsAny(last, "0123456789") {
			return ""
		}
	}
	return host
}

func isShellOperator(tok string) bool {
	switch tok {
	case "|", "||", "&&", ";", "&", ">", ">>", "<", "2>", "2>>", "&>", "|&":
		return true
	}
	return false
}

func stringArg(args map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := args[k].(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
