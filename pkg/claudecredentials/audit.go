package claudecredentials

import (
	"context"
	"encoding/json"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"
	"uuid"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	ext "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/obot-platform/obot/pkg/clauderunner"
	"github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/substrate"
	"github.com/obot-platform/obot/pkg/system"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	egressBodyLimit = 1 << 20
)

// AuditProcessor receives trusted filter state, never actor-supplied identity headers.
type AuditProcessor struct {
	ext.UnimplementedExternalProcessorServer
	Provider *CredentialProvider
}

func (p *AuditProcessor) attribute(ctx context.Context, identity string, entry *types.EgressAuditLog) error {
	const prefix = "spiffe://substrate-actor.local/ateom-for-actor/"
	ref, ok := strings.CutPrefix(identity, prefix)
	if !ok {
		return status.Error(codes.PermissionDenied, "Missing authenticated actor identity")
	}
	atespace, name, ok := strings.Cut(ref, "/")
	if !ok || strings.Contains(name, "/") {
		return status.Error(codes.PermissionDenied, "Invalid actor identity")
	}
	var instance v1.AgentInstance
	if err := p.Provider.Storage.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: name}, &instance); err != nil {
		return err
	}
	expected := substrate.Ref(&instance)
	if expected.Atespace != atespace || instance.Spec.ClaudeSession == "" {
		return status.Error(codes.PermissionDenied, "Actor does not belong to a Claude session")
	}
	entry.Actor = identity
	entry.UserID = instance.Spec.UserID
	entry.ClaudeAccount = instance.Spec.ClaudeAccount
	entry.EnvironmentID = instance.Spec.ClaudeEnvironment
	entry.ClaudeSession = instance.Spec.ClaudeSession
	return nil
}

func (p *AuditProcessor) Process(stream ext.ExternalProcessor_ProcessServer) error {
	if err := verifyEgressPeer(stream.Context()); err != nil {
		return err
	}
	var entry types.EgressAuditLog
	var requestBody, responseBody []byte
	started := false
	defer func() {
		if !started {
			return
		}
		if entry.Outcome == "running" {
			entry.Outcome = "interrupted"
		}
		entry.DurationMs = time.Since(entry.CreatedAt).Milliseconds()
		if requestBody != nil {
			entry.RequestBody, _ = json.Marshal(requestBody)
		}
		if responseBody != nil {
			entry.ResponseBody, _ = json.Marshal(responseBody)
		}
		p.Provider.Gateway.LogEgressAuditEntry(entry)
	}()

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		reply := &ext.ProcessingResponse{}
		complete := false
		switch data := msg.Request.(type) {
		case *ext.ProcessingRequest_RequestHeaders:
			if started {
				return status.Error(codes.InvalidArgument, "Duplicate request headers")
			}
			identity := ""
			for _, attrs := range msg.Attributes {
				if v := attrs.Fields["filter_state['dev.ate.actor.identity']"]; v != nil {
					identity = v.GetStringValue()
				}
			}
			entry = types.EgressAuditLog{ID: uuid.New().String(), CreatedAt: time.Now().UTC(), Outcome: "running"}
			if err := p.attribute(stream.Context(), identity, &entry); err != nil {
				return status.Error(codes.PermissionDenied, "Cannot attribute egress request")
			}
			headers := headerValues(data.RequestHeaders.Headers)
			entry.Method = firstHeader(headers, ":method")
			entry.Scheme = firstHeader(headers, ":scheme")
			entry.Host = firstHeader(headers, ":authority")
			entry.Path = redactPath(firstHeader(headers, ":path"))
			entry.RequestHeaders = redactedEgressHeaders(headers)
			started = true
			mutation, err := p.secretPlaceholders(stream.Context(), &entry)
			if err != nil {
				return status.Error(codes.Unavailable, "Cannot prepare egress headers")
			}
			reply.Response = &ext.ProcessingResponse_RequestHeaders{RequestHeaders: &ext.HeadersResponse{Response: &ext.CommonResponse{HeaderMutation: mutation}}}
			reply.DynamicMetadata, _ = structpb.NewStruct(map[string]any{"obot.audit": map[string]any{"id": entry.ID}})
		case *ext.ProcessingRequest_ResponseHeaders:
			if !started {
				return status.Error(codes.InvalidArgument, "Missing request headers")
			}
			headers := headerValues(data.ResponseHeaders.Headers)
			entry.ResponseHeaders = redactedEgressHeaders(headers)
			entry.ResponseStatus, _ = strconv.Atoi(firstHeader(headers, ":status"))
			complete = data.ResponseHeaders.EndOfStream
			reply.Response = &ext.ProcessingResponse_ResponseHeaders{ResponseHeaders: &ext.HeadersResponse{Response: &ext.CommonResponse{}}}
		case *ext.ProcessingRequest_RequestBody:
			entry.RequestBytes += int64(len(data.RequestBody.Body))
			requestBody = appendCaptured(requestBody, data.RequestBody.Body)
			entry.RequestTruncated = entry.RequestBytes > egressBodyLimit
			reply.Response = &ext.ProcessingResponse_RequestBody{RequestBody: &ext.BodyResponse{Response: &ext.CommonResponse{}}}
		case *ext.ProcessingRequest_ResponseBody:
			entry.ResponseBytes += int64(len(data.ResponseBody.Body))
			responseBody = appendCaptured(responseBody, data.ResponseBody.Body)
			entry.ResponseTruncated = entry.ResponseBytes > egressBodyLimit
			complete = data.ResponseBody.EndOfStream
			reply.Response = &ext.ProcessingResponse_ResponseBody{ResponseBody: &ext.BodyResponse{Response: &ext.CommonResponse{}}}
		case *ext.ProcessingRequest_RequestTrailers:
			reply.Response = &ext.ProcessingResponse_RequestTrailers{RequestTrailers: &ext.TrailersResponse{}}
		case *ext.ProcessingRequest_ResponseTrailers:
			complete = true
			reply.Response = &ext.ProcessingResponse_ResponseTrailers{ResponseTrailers: &ext.TrailersResponse{}}
		default:
			return status.Error(codes.InvalidArgument, "Unsupported audit event")
		}
		if !started {
			return status.Error(codes.InvalidArgument, "Missing request headers")
		}
		if complete {
			entry.Outcome = egressOutcome(entry.ResponseStatus)
		}
		if err := stream.Send(reply); err != nil {
			return err
		}
	}
}

func egressOutcome(code int) string {
	if code == 0 {
		return "interrupted"
	}
	if code == 401 || code == 403 {
		return "denied"
	}
	if code >= 400 {
		return "error"
	}
	return "complete"
}

func appendCaptured(dst, src []byte) []byte {
	return append(dst, src[:min(len(src), egressBodyLimit-len(dst))]...)
}

func headerValues(headers *core.HeaderMap) map[string][]string {
	values := map[string][]string{}
	for _, h := range headers.GetHeaders() {
		val := h.Value
		if len(h.RawValue) > 0 {
			val = string(h.RawValue)
		}
		key := strings.ToLower(h.Key)
		values[key] = append(values[key], val)
	}
	return values
}

func firstHeader(h map[string][]string, key string) string {
	if len(h[key]) == 0 {
		return ""
	}
	return h[key][0]
}

func redactedEgressHeaders(headers map[string][]string) json.RawMessage {
	// An allowlist avoids leaking custom credential headers, URLs, or cookies.
	clean := map[string][]string{}
	for key, values := range headers {
		switch key {
		case "content-type", "content-length", "content-encoding", "accept", "user-agent", ":method", ":scheme", ":authority", ":status":
			clean[key] = values
		case ":path":
			clean[key] = []string{redactPath(firstHeader(headers, key))}
		default:
			clean[key] = []string{"[REDACTED]"}
		}
	}
	result, _ := json.Marshal(clean)
	return result
}

func redactPath(path string) string {
	parsed, err := url.ParseRequestURI(path)
	if err != nil {
		return "[INVALID PATH]"
	}
	query := parsed.Query()
	for key := range query {
		query[key] = []string{"[REDACTED]"}
	}
	parsed.RawQuery = query.Encode()
	parsed.User = nil
	parsed.Fragment = ""
	return parsed.String()
}

// The following Substrate filter replaces existing headers, so supply placeholders
// here before policy enforcement. Secret values never enter the audit stream.
func (p *AuditProcessor) secretPlaceholders(ctx context.Context, entry *types.EgressAuditLog) (*ext.HeaderMutation, error) {
	if entry.Scheme != "https" {
		return nil, nil
	}
	destination, err := url.Parse("https://" + entry.Host)
	if err != nil || (destination.Port() != "" && destination.Port() != "443") {
		return nil, nil
	}
	env, err := clauderunner.AuthorizedEnvironment(ctx, p.Provider.Gateway, entry.EnvironmentID, entry.ClaudeAccount, entry.UserID)
	if clauderunner.AccessDenied(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	mutation := &ext.HeaderMutation{}
	for _, secret := range env.EgressSecrets {
		if secret.Hostname == strings.ToLower(destination.Hostname()) && clauderunner.EgressHostAllowed(env.Runtime, secret.Hostname) {
			mutation.SetHeaders = append(mutation.SetHeaders, &core.HeaderValueOption{
				Header:       &core.HeaderValue{Key: secret.Header, RawValue: []byte("obot-egress-placeholder")},
				AppendAction: core.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
			})
		}
	}
	return mutation, nil
}
