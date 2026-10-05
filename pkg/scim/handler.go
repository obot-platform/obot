// Package scim serves a SCIM 2.0 endpoint for an auth provider's SCIM connection, as a facade over Obot's users,
// identities, groups, and memberships.
package scim

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/scim/adapter"
	"k8s.io/apiserver/pkg/authentication/user"
)

const (
	// PathPrefix is the path under which the SCIM connection's resources are served. There is at most one connection,
	// so its base URL names none.
	PathPrefix = "/scim/v2/"

	contentType = "application/scim+json; charset=utf-8"

	// maxBodyBytes bounds request bodies. A group of 10,000 members is about 1.5 MB.
	maxBodyBytes = 4 << 20
	// maxMemberValues bounds the member values in one request.
	maxMemberValues = 10000
	// requestTimeout keeps every request below Okta's 60-second timeout.
	requestTimeout = 50 * time.Second

	defaultCount = 100
	maxCount     = 200

	// retryAfterSeconds is how soon a client should retry while the endpoint is unavailable.
	retryAfterSeconds = "120"

	// activityTimeout bounds recording a request's outcome after its response is written.
	activityTimeout = 5 * time.Second
)

// Environment answers what a SCIM request needs from outside the gateway database.
type Environment interface {
	// ConfiguredAuthProvider returns the namespace and name of the auth provider that serves sign-ins, or empty strings
	// when none is configured.
	ConfiguredAuthProvider(ctx context.Context) (string, string, error)
	// UserLimit returns the installation's user limit, which SCIM user creation counts against.
	UserLimit(ctx context.Context) (gclient.UserLimit, error)
	// DefaultRole returns the role of new users.
	DefaultRole(ctx context.Context) (types2.Role, error)
}

// Handler serves the SCIM endpoint. The API server authenticates its requests with the SCIM authenticator, which
// accepts only the SCIM connection's bearer token, and authorizes them only for that connection's principal, so no
// cookie, Obot credential, redirect, or just-in-time user creation ever reaches it.
type Handler struct {
	gateway   *gclient.Client
	env       Environment
	serverURL string
}

type request struct {
	*http.Request
	conn    *connection
	segment []string
}

type connection struct {
	*types.SCIMConnection
	baseURL string
	// patchRules are how the connection's identity provider departs from RFC 7644 in PATCH requests.
	patchRules adapter.PatchRules
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	// err is the SCIM error the response carries, if any.
	err *Error
}

// NewHandler returns the SCIM handler. serverURL is Obot's public URL, which resource locations are built from.
func NewHandler(gateway *gclient.Client, env Environment, serverURL string) *Handler {
	return &Handler{
		gateway:   gateway,
		env:       env,
		serverURL: strings.TrimSuffix(serverURL, "/"),
	}
}

// ANY /scim/v2 and /scim/v2/...
// Serves the SCIM endpoint of the SCIM connection, routing each request by its method and path. It writes every response
// itself, as a SCIM response, and never returns an error.
func (h *Handler) Serve(req api.Context) error {
	h.ServeHTTP(req.ResponseWriter, req.Request, req.User)
	return nil
}

// ServeHTTP handles a request that principal made. Only the principal of a SCIM connection, which the SCIM
// authenticator yields, is served, for that connection.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request, principal user.Info) {
	start := time.Now()
	rec := &statusRecorder{
		ResponseWriter: w,
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	r = r.WithContext(ctx)

	rec.Header().Set("Cache-Control", "no-store")
	rec.Header().Set("X-Content-Type-Options", "nosniff")

	connectionID, segments := principal.GetUID(), splitPath(r.URL.Path)
	resource, operation := describeRequest(r.Method, segments)
	var authenticated bool
	defer func() {
		// The token and request body are never logged.
		duration := time.Since(start)
		slog.Info("Handled SCIM request", "connection", connectionID, "method", r.Method, "resource", strings.Join(segments, "/"),
			"status", rec.status, "duration", duration)
		recordMetrics(r.Context(), resource, operation, rec.status, duration)
		if authenticated {
			h.recordActivity(r, rec, connectionID, segments)
		}
	}()

	// Authorization already confined the SCIM endpoint to connection principals. This repeats the check, so the handler
	// never serves a request it was not meant for.
	if !IsConnectionPrincipal(principal) || connectionID == "" {
		writeError(rec, &Error{
			Status: http.StatusForbidden,
			Detail: "this credential cannot access this SCIM connection",
		})
		return
	}
	authenticated = true

	conn, err := h.connection(r.Context(), connectionID)
	if err != nil {
		if errors.Is(err, gclient.ErrSCIMConnectionNotFound) {
			WriteUnauthorized(rec)
			return
		}
		writeError(rec, toError(err))
		return
	}
	if conn == nil {
		unavailable(rec, "the auth provider of this SCIM connection is not configured")
		return
	}

	h.route(rec, &request{
		Request: r,
		conn:    conn,
		segment: segments,
	})
}

// connection returns the connection a request is for, or nil when its auth provider is not the configured one. Only
// the authenticated connection learns that the auth provider is not serving sign-ins.
func (h *Handler) connection(ctx context.Context, id string) (*connection, error) {
	conn, err := h.gateway.SCIMConnection(ctx, id)
	if err != nil {
		return nil, err
	}

	namespace, name, err := h.env.ConfiguredAuthProvider(ctx)
	if err != nil {
		slog.Error("Failed to check the configured auth provider for a SCIM request", "error", err)
		return nil, nil
	}
	if namespace != conn.AuthProviderNamespace || name != conn.AuthProviderName {
		return nil, nil
	}

	a, ok := adapter.Lookup(conn.AdapterType)
	if !ok {
		return nil, fmt.Errorf("SCIM connection %s has unknown adapter type %q", conn.ID, conn.AdapterType)
	}
	return &connection{
		SCIMConnection: conn,
		baseURL:        BaseURL(h.serverURL),
		patchRules:     a.PatchRules(),
	}, nil
}

// recordActivity records the outcome of a request that the connection's token authenticated as the connection's
// activity. Unauthenticated requests never reach the handler, so anyone who knows the base URL cannot fill the failure
// log. A failure to record it is logged, and never changes the response.
func (h *Handler) recordActivity(r *http.Request, rec *statusRecorder, connectionID string, segments []string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), activityTimeout)
	defer cancel()

	outcome := gclient.SCIMRequestOutcome{
		Method:   r.Method,
		Resource: strings.Join(segments, "/"),
		Status:   rec.status,
	}
	if rec.err != nil {
		outcome.SCIMType = rec.err.ScimType
		outcome.Detail = rec.err.Detail
	}
	if err := h.gateway.RecordSCIMRequest(ctx, connectionID, outcome); err != nil {
		slog.Warn("Failed to record SCIM request activity", "connection", connectionID, "error", err)
	}
}

func (h *Handler) route(w http.ResponseWriter, r *request) {
	if len(r.segment) == 0 {
		writeError(w, notFound("unknown SCIM endpoint"))
		return
	}

	resource, rest := r.segment[0], r.segment[1:]
	switch {
	case strings.EqualFold(resource, "ServiceProviderConfig") && len(rest) == 0:
		if allow(w, r, http.MethodGet) {
			writeJSON(w, http.StatusOK, serviceProviderConfig(r.conn.baseURL))
		}
	case strings.EqualFold(resource, "ResourceTypes") && len(rest) <= 1:
		if allow(w, r, http.MethodGet) {
			serveDiscovery(w, resourceTypes(r.conn.baseURL), rest)
		}
	case strings.EqualFold(resource, "Schemas") && len(rest) <= 1:
		if allow(w, r, http.MethodGet) {
			serveDiscovery(w, schemas(r.conn.baseURL), rest)
		}
	case strings.EqualFold(resource, "Users") && len(rest) == 0:
		switch r.Method {
		case http.MethodGet:
			h.listUsers(w, r)
		case http.MethodPost:
			h.createUser(w, r)
		default:
			methodNotAllowed(w, http.MethodGet, http.MethodPost)
		}
	case strings.EqualFold(resource, "Users") && len(rest) == 1 && rest[0] != ".search":
		switch r.Method {
		case http.MethodGet:
			h.getUser(w, r, rest[0])
		case http.MethodPut:
			h.replaceUser(w, r, rest[0])
		case http.MethodPatch:
			h.patchUser(w, r, rest[0])
		case http.MethodDelete:
			// Okta deprovisions users by setting active to false. Deleting a user is an Obot action.
			writeError(w, &Error{
				Status: http.StatusNotImplemented,
				Detail: "deleting users is not supported; deprovision a user by setting active to false",
			})
		default:
			methodNotAllowed(w, http.MethodGet, http.MethodPut, http.MethodPatch)
		}
	case strings.EqualFold(resource, "Groups") && len(rest) == 0:
		switch r.Method {
		case http.MethodGet:
			h.listGroups(w, r)
		case http.MethodPost:
			h.createGroup(w, r)
		default:
			methodNotAllowed(w, http.MethodGet, http.MethodPost)
		}
	case strings.EqualFold(resource, "Groups") && len(rest) == 1 && rest[0] != ".search":
		switch r.Method {
		case http.MethodGet:
			h.getGroup(w, r, rest[0])
		case http.MethodPut:
			h.replaceGroup(w, r, rest[0])
		case http.MethodPatch:
			h.patchGroup(w, r, rest[0])
		case http.MethodDelete:
			h.deleteGroup(w, r, rest[0])
		default:
			methodNotAllowed(w, http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete)
		}
	case strings.EqualFold(resource, "Bulk"), strings.EqualFold(resource, "Me"), resource == ".search", len(rest) == 1 && rest[0] == ".search":
		writeError(w, &Error{
			Status: http.StatusNotImplemented,
			Detail: fmt.Sprintf("%s is not supported", strings.Join(r.segment, "/")),
		})
	default:
		writeError(w, notFound("unknown SCIM endpoint"))
	}
}

// GET /scim/v2/Users
// Lists the users that SCIM has provisioned, optionally filtered by userName or id.
func (h *Handler) listUsers(w http.ResponseWriter, r *request) {
	q := r.URL.Query()
	f, err := parseListFilter(userResourceSchema, q.Get("filter"), "userName", "id")
	if err != nil {
		writeError(w, toError(err))
		return
	}
	startIndex, count, err := pageParams(q)
	if err != nil {
		writeError(w, toError(err))
		return
	}
	proj, err := parseProjection(userResourceSchema, q)
	if err != nil {
		writeError(w, toError(err))
		return
	}

	if f != nil && f.Value == "" {
		// No user has an empty userName or id.
		writeList(w, []any{}, 0, startIndex)
		return
	}

	var filter gclient.SCIMUserFilter
	if f != nil {
		switch f.Attribute {
		case "userName":
			filter.UserName = f.Value
		case "id":
			filter.ID = f.Value
		}
	}

	users, total, err := h.gateway.ListSCIMUsers(r.Context(), r.conn.ID, filter, gclient.SCIMPage{
		Offset: startIndex - 1,
		Limit:  count,
	})
	if err != nil {
		writeError(w, toError(err))
		return
	}

	resources := make([]any, 0, len(users))
	for i := range users {
		resources = append(resources, proj.apply(userResource(&users[i], r.conn.baseURL)))
	}
	writeList(w, resources, total, startIndex)
}

// POST /scim/v2/Users
// Provisions a user, binding it to the existing user with the same native user ID or creating one.
func (h *Handler) createUser(w http.ResponseWriter, r *request) {
	proj, err := parseProjection(userResourceSchema, r.URL.Query())
	if err != nil {
		writeError(w, toError(err))
		return
	}
	body, err := readBody(r)
	if err != nil {
		writeError(w, toError(err))
		return
	}
	input, err := userInputFromResource(body)
	if err != nil {
		writeError(w, toError(err))
		return
	}

	userLimit, err := h.env.UserLimit(r.Context())
	if err != nil {
		writeError(w, toError(fmt.Errorf("failed to resolve user limit: %w", err)))
		return
	}
	if !userLimit.Unlimited && userLimit.Maximum <= 0 {
		writeError(w, toError(fmt.Errorf("invalid user limit %d", userLimit.Maximum)))
		return
	}
	defaultRole, err := h.env.DefaultRole(r.Context())
	if err != nil {
		writeError(w, toError(fmt.Errorf("failed to get default role: %w", err)))
		return
	}

	user, err := h.gateway.CreateSCIMUser(r.Context(), r.conn.SCIMConnection, input, gclient.SCIMUserCreateOptions{
		UserLimit:   userLimit,
		DefaultRole: defaultRole,
	})
	if err != nil {
		writeError(w, toError(err))
		return
	}

	w.Header().Set("Location", r.conn.baseURL+"/Users/"+user.ID)
	writeJSON(w, http.StatusCreated, proj.apply(userResource(user, r.conn.baseURL)))
}

// GET /scim/v2/Users/{id}
// Returns a provisioned user, including active and the read-only groups.
func (h *Handler) getUser(w http.ResponseWriter, r *request, id string) {
	proj, err := parseProjection(userResourceSchema, r.URL.Query())
	if err != nil {
		writeError(w, toError(err))
		return
	}

	user, err := h.gateway.GetSCIMUser(r.Context(), r.conn.ID, id)
	if err != nil {
		writeError(w, toError(err))
		return
	}
	writeJSON(w, http.StatusOK, proj.apply(userResource(user, r.conn.baseURL)))
}

// PUT /scim/v2/Users/{id}
// Replaces a provisioned user's writable attributes. An omitted active keeps the current state, and id, meta, and
// groups are ignored. There is no upsert.
func (h *Handler) replaceUser(w http.ResponseWriter, r *request, id string) {
	proj, err := parseProjection(userResourceSchema, r.URL.Query())
	if err != nil {
		writeError(w, toError(err))
		return
	}
	body, err := readBody(r)
	if err != nil {
		writeError(w, toError(err))
		return
	}
	input, err := userInputFromResource(body)
	if err != nil {
		writeError(w, toError(err))
		return
	}

	user, err := h.gateway.UpdateSCIMUser(r.Context(), r.conn.SCIMConnection, id, func(gclient.SCIMUser) (gclient.SCIMUserInput, error) {
		return input, nil
	})
	if err != nil {
		writeError(w, toError(err))
		return
	}
	writeJSON(w, http.StatusOK, proj.apply(userResource(user, r.conn.baseURL)))
}

// PATCH /scim/v2/Users/{id}
// Applies PatchOp operations to a provisioned user, including active with and without a path.
func (h *Handler) patchUser(w http.ResponseWriter, r *request, id string) {
	proj, err := parseProjection(userResourceSchema, r.URL.Query())
	if err != nil {
		writeError(w, toError(err))
		return
	}
	body, err := readBody(r)
	if err != nil {
		writeError(w, toError(err))
		return
	}
	ops, err := decodePatchOperations(body)
	if err != nil {
		writeError(w, toError(err))
		return
	}

	user, err := h.gateway.UpdateSCIMUser(r.Context(), r.conn.SCIMConnection, id, func(current gclient.SCIMUser) (gclient.SCIMUserInput, error) {
		resource := userResource(&current, r.conn.baseURL)
		if err := applyPatch(userResourceSchema, resource, ops, r.conn.patchRules); err != nil {
			return gclient.SCIMUserInput{}, err
		}
		// A user's externalId is the identity evidence it was bound by. A PUT that omits it keeps it, but a PATCH that
		// removes it asks for what cannot be done.
		if current.ExternalID != "" && stringValue(resource, "externalId") == "" {
			return gclient.SCIMUserInput{}, badRequest(scimTypeMutability, "externalId identifies the user and cannot be removed")
		}
		return userInputFromResource(resource)
	})
	if err != nil {
		writeError(w, toError(err))
		return
	}
	writeJSON(w, http.StatusOK, proj.apply(userResource(user, r.conn.baseURL)))
}

// GET /scim/v2/Groups
// Lists the groups that SCIM has bound or created, optionally filtered by displayName or id.
func (h *Handler) listGroups(w http.ResponseWriter, r *request) {
	q := r.URL.Query()
	f, err := parseListFilter(groupResourceSchema, q.Get("filter"), "displayName", "id")
	if err != nil {
		writeError(w, toError(err))
		return
	}
	startIndex, count, err := pageParams(q)
	if err != nil {
		writeError(w, toError(err))
		return
	}
	proj, err := parseProjection(groupResourceSchema, q)
	if err != nil {
		writeError(w, toError(err))
		return
	}

	if f != nil && f.Value == "" {
		// No group has an empty displayName or id.
		writeList(w, []any{}, 0, startIndex)
		return
	}

	var filter gclient.SCIMGroupFilter
	if f != nil {
		switch f.Attribute {
		case "displayName":
			filter.DisplayName = f.Value
		case "id":
			filter.ID = f.Value
		}
	}

	groups, total, err := h.gateway.ListSCIMGroups(r.Context(), r.conn.ID, filter, gclient.SCIMPage{
		Offset: startIndex - 1,
		Limit:  count,
	}, proj.includes("members"))
	if err != nil {
		writeError(w, toError(err))
		return
	}

	resources := make([]any, 0, len(groups))
	for i := range groups {
		resources = append(resources, proj.apply(groupResource(&groups[i], r.conn.baseURL)))
	}
	writeList(w, resources, total, startIndex)
}

// POST /scim/v2/Groups
// Binds a pushed group to the unbound group with the same name, or creates one, with the complete member list.
func (h *Handler) createGroup(w http.ResponseWriter, r *request) {
	proj, err := parseProjection(groupResourceSchema, r.URL.Query())
	if err != nil {
		writeError(w, toError(err))
		return
	}
	body, err := readBody(r)
	if err != nil {
		writeError(w, toError(err))
		return
	}
	input, err := groupInputFromResource(body)
	if err != nil {
		writeError(w, toError(err))
		return
	}
	if len(input.MemberIDs) > maxMemberValues {
		writeError(w, tooManyMembers())
		return
	}

	group, err := h.gateway.CreateSCIMGroup(r.Context(), r.conn.SCIMConnection, input)
	if err != nil {
		writeError(w, toError(err))
		return
	}

	w.Header().Set("Location", r.conn.baseURL+"/Groups/"+group.ID)
	writeJSON(w, http.StatusCreated, proj.apply(groupResource(group, r.conn.baseURL)))
}

// GET /scim/v2/Groups/{id}
// Returns a bound group with its full member list, unless attributes are projected.
func (h *Handler) getGroup(w http.ResponseWriter, r *request, id string) {
	proj, err := parseProjection(groupResourceSchema, r.URL.Query())
	if err != nil {
		writeError(w, toError(err))
		return
	}

	group, err := h.gateway.GetSCIMGroup(r.Context(), r.conn.ID, id, proj.includes("members"))
	if err != nil {
		writeError(w, toError(err))
		return
	}
	writeJSON(w, http.StatusOK, proj.apply(groupResource(group, r.conn.baseURL)))
}

// PUT /scim/v2/Groups/{id}
// Replaces a bound group's display name and members. The members are the complete set.
func (h *Handler) replaceGroup(w http.ResponseWriter, r *request, id string) {
	proj, err := parseProjection(groupResourceSchema, r.URL.Query())
	if err != nil {
		writeError(w, toError(err))
		return
	}
	body, err := readBody(r)
	if err != nil {
		writeError(w, toError(err))
		return
	}
	input, err := groupInputFromResource(body)
	if err != nil {
		writeError(w, toError(err))
		return
	}
	if len(input.MemberIDs) > maxMemberValues {
		writeError(w, tooManyMembers())
		return
	}

	// The replacement does not depend on the current members, so it is applied without reading them, and the response
	// reads them once the write is committed, only if it includes them.
	if err := h.gateway.PatchSCIMGroup(r.Context(), r.conn.SCIMConnection, id, gclient.SCIMGroupPatch{
		DisplayName:    input.DisplayName,
		ReplaceMembers: true,
		MemberIDs:      input.MemberIDs,
	}); err != nil {
		writeError(w, toError(err))
		return
	}
	group, err := h.gateway.GetSCIMGroup(r.Context(), r.conn.ID, id, proj.includes("members"))
	if err != nil {
		writeError(w, toError(err))
		return
	}
	writeJSON(w, http.StatusOK, proj.apply(groupResource(group, r.conn.baseURL)))
}

// PATCH /scim/v2/Groups/{id}
// Renames a bound group, and adds, removes, or replaces its members. The response has no body, as RFC 7644 section
// 3.5.2 allows: returning the group would mean reading every member of it. A request with the attributes parameter
// gets the group, with the attributes it selects, as that section requires.
func (h *Handler) patchGroup(w http.ResponseWriter, r *request, id string) {
	proj, err := parseProjection(groupResourceSchema, r.URL.Query())
	if err != nil {
		writeError(w, toError(err))
		return
	}
	body, err := readBody(r)
	if err != nil {
		writeError(w, toError(err))
		return
	}
	ops, err := decodePatchOperations(body)
	if err != nil {
		writeError(w, toError(err))
		return
	}

	var values int
	for _, op := range ops {
		values += countValues(op.Value)
	}
	if values > maxMemberValues {
		writeError(w, tooManyMembers())
		return
	}

	// The operations Okta sends name the members they change, and apply without reading the group's other members.
	// Any other operation applies to the whole group.
	if patch, ok := planGroupPatch(id, ops); ok {
		err = h.gateway.PatchSCIMGroup(r.Context(), r.conn.SCIMConnection, id, patch)
	} else {
		err = h.gateway.UpdateSCIMGroup(r.Context(), r.conn.SCIMConnection, id, func(current gclient.SCIMGroup) (gclient.SCIMGroupInput, error) {
			resource := groupResource(&current, r.conn.baseURL)
			if err := applyPatch(groupResourceSchema, resource, ops, r.conn.patchRules); err != nil {
				return gclient.SCIMGroupInput{}, err
			}
			return groupInputFromResource(resource)
		})
	}
	if err != nil {
		writeError(w, toError(err))
		return
	}

	if !proj.selected {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	group, err := h.gateway.GetSCIMGroup(r.Context(), r.conn.ID, id, proj.includes("members"))
	if err != nil {
		writeError(w, toError(err))
		return
	}
	writeJSON(w, http.StatusOK, proj.apply(groupResource(group, r.conn.baseURL)))
}

// DELETE /scim/v2/Groups/{id}
// Retires the group's binding and removes its memberships. Until SCIM is enforced, the Obot group and its references
// remain, unbound. Once it is enforced, the group is deleted, with its role assignments and policy subjects.
func (h *Handler) deleteGroup(w http.ResponseWriter, r *request, id string) {
	if err := h.gateway.DeleteSCIMGroup(r.Context(), r.conn.SCIMConnection, id); err != nil {
		writeError(w, toError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// toError converts an error into the SCIM error response for it. Errors that are not about the request are logged and
// answered with a generic 500.
func toError(err error) *Error {
	if e, ok := errors.AsType[*Error](err); ok {
		return e
	}
	if e, ok := errors.AsType[*gclient.SCIMNotFoundError](err); ok {
		return notFound("%s", e.Error())
	}
	if errors.Is(err, gclient.ErrSCIMConnectionNotFound) {
		// The connection was deleted while the request was handled, so its token no longer authenticates.
		return &Error{
			Status: http.StatusUnauthorized,
			Detail: unauthorizedDetail,
		}
	}
	if e, ok := errors.AsType[*gclient.SCIMConflictError](err); ok {
		return conflict(e.Message)
	}
	if e, ok := errors.AsType[*gclient.SCIMInvalidValueError](err); ok {
		return badRequest(scimTypeInvalidValue, "%s", e.Message)
	}
	if e, ok := errors.AsType[*gclient.SCIMMutabilityError](err); ok {
		return badRequest(scimTypeMutability, "%s", e.Message)
	}
	if e, ok := errors.AsType[*types2.ErrHTTP](err); ok && e.Code < http.StatusInternalServerError {
		// Such as the user limit, which refuses a new user with 403.
		return &Error{
			Status: e.Code,
			Detail: e.Message,
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &Error{
			Status: http.StatusServiceUnavailable,
			Detail: "the request took too long; retry it",
		}
	}

	slog.Error("Failed to handle SCIM request", "error", err)
	return &Error{
		Status: http.StatusInternalServerError,
		Detail: "internal error",
	}
}

// countValues returns the number of list elements in a PATCH value, including lists nested in the object of an
// operation without a path.
func countValues(v any) int {
	switch v := v.(type) {
	case []any:
		return len(v)
	case map[string]any:
		var n int
		for _, nested := range v {
			n += countValues(nested)
		}
		return n
	}
	return 0
}

func tooManyMembers() *Error {
	return &Error{
		Status:   http.StatusRequestEntityTooLarge,
		ScimType: scimTypeTooMany,
		Detail:   fmt.Sprintf("a request can list at most %d members", maxMemberValues),
	}
}

// readBody reads a JSON object request body, bounded in size.
func readBody(r *request) (map[string]any, error) {
	if ct := r.Header.Get("Content-Type"); ct != "" {
		mediaType, _, _ := strings.Cut(strings.ToLower(ct), ";")
		if mediaType = strings.TrimSpace(mediaType); mediaType != "application/scim+json" && mediaType != "application/json" {
			return nil, &Error{
				Status: http.StatusUnsupportedMediaType,
				Detail: "requests must be application/scim+json",
			}
		}
	}

	data, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil {
		return nil, badRequest(scimTypeInvalidSyntax, "failed to read request body")
	}
	if len(data) > maxBodyBytes {
		return nil, &Error{
			Status: http.StatusRequestEntityTooLarge,
			Detail: fmt.Sprintf("request bodies are limited to %d bytes", maxBodyBytes),
		}
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var body map[string]any
	if err := decoder.Decode(&body); err != nil || body == nil {
		return nil, badRequest(scimTypeInvalidSyntax, "request body must be a JSON object")
	}
	// The object must be the whole body. A second value or trailing garbage makes the body invalid JSON, which must
	// not be half applied.
	if err := decoder.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
		return nil, badRequest(scimTypeInvalidSyntax, "request body must be a single JSON object")
	}
	return body, nil
}

// pageParams returns the one-based startIndex and the count of a list request. A startIndex below 1 is treated as 1,
// and a count above the maximum is reduced to it.
func pageParams(q url.Values) (int, int, error) {
	startIndex, count := 1, defaultCount
	if s := q.Get("startIndex"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil {
			return 0, 0, badRequest(scimTypeInvalidValue, "startIndex must be an integer")
		}
		startIndex = max(v, 1)
	}
	if s := q.Get("count"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil {
			return 0, 0, badRequest(scimTypeInvalidValue, "count must be an integer")
		}
		count = min(max(v, 0), maxCount)
	}
	return startIndex, count, nil
}

// IsSCIMPath reports whether a request path is served by the SCIM endpoint: the base URL, and every path below it.
func IsSCIMPath(path string) bool {
	return path == strings.TrimSuffix(PathPrefix, "/") || strings.HasPrefix(path, PathPrefix)
}

// BaseURL returns the base URL of the SCIM connection, which every SCIM request is relative to. The server URL is
// configured by hand, so any trailing slashes on it are dropped.
func BaseURL(serverURL string) string {
	return strings.TrimRight(serverURL, "/") + strings.TrimSuffix(PathPrefix, "/")
}

// splitPath returns the path segments of a SCIM request path below the connection's base URL.
func splitPath(path string) []string {
	rest, ok := strings.CutPrefix(path, PathPrefix)
	if !ok {
		return nil
	}
	return slices.DeleteFunc(strings.Split(rest, "/"), func(s string) bool { return s == "" })
}

func bearerToken(r *http.Request) (string, bool) {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}

func allow(w http.ResponseWriter, r *request, method string) bool {
	if r.Method != method {
		methodNotAllowed(w, method)
		return false
	}
	return true
}

func methodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	writeError(w, &Error{
		Status: http.StatusMethodNotAllowed,
		Detail: "method not allowed; allowed methods: " + strings.Join(methods, ", "),
	})
}

func unavailable(w http.ResponseWriter, detail string) {
	w.Header().Set("Retry-After", retryAfterSeconds)
	writeError(w, &Error{
		Status: http.StatusServiceUnavailable,
		Detail: detail,
	})
}

func writeError(w http.ResponseWriter, err *Error) {
	if rec, ok := w.(*statusRecorder); ok {
		rec.err = err
	}
	if err.Status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", wwwAuthenticate)
	}
	writeJSON(w, err.Status, err.response())
}

func writeList(w http.ResponseWriter, resources []any, total int64, startIndex int) {
	writeJSON(w, http.StatusOK, map[string]any{
		"schemas":      []string{listResponseSchema},
		"totalResults": total,
		"startIndex":   startIndex,
		"itemsPerPage": len(resources),
		"Resources":    resources,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		slog.Error("Failed to encode SCIM response", "error", err)
		status = http.StatusInternalServerError
		data = []byte(`{"schemas":["` + errorSchema + `"],"status":"500","detail":"internal error"}`)
	}

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}
