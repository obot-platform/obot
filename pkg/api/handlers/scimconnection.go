package handlers

import (
	"strconv"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	scimsetup "github.com/obot-platform/obot/pkg/scim/setup"
	"github.com/obot-platform/obot/pkg/system"
)

// SCIMConnectionHandler serves the administration of SCIM connections: reviewing a connection, enforcing it, and
// managing its bearer token. Administrators and auditors can review a connection. Only Owners can enforce it, and
// Owners and the bootstrap user can manage its token, so that the identity provider can be set up before any Owner
// has signed in through it.
type SCIMConnectionHandler struct {
	setup *scimsetup.Service
}

func NewSCIMConnectionHandler(service *scimsetup.Service) *SCIMConnectionHandler {
	return &SCIMConnectionHandler{
		setup: service,
	}
}

// GET /api/scim-connections
// Lists the SCIM connections. There is at most one.
func (h *SCIMConnectionHandler) List(req api.Context) error {
	conns, err := h.setup.Connections(req.Context())
	if err != nil {
		return err
	}
	return req.Write(types.SCIMConnectionList{Items: conns})
}

// GET /api/scim-connections/{id}/review
// Reports the state of a SCIM connection: provisioned and unprovisioned users, bound groups, referenced groups not
// pushed yet, the groups enforcing would delete, warnings, what blocks the requesting user from enforcing, and
// recent activity. Each list holds its first page of "limit" items; the list routes below serve the others.
func (h *SCIMConnectionHandler) Review(req api.Context) error {
	review, err := h.setup.Review(req.Context(), req.PathValue("id"), scimActor(req), queryInt(req, "limit"))
	if err != nil {
		return err
	}
	return req.Write(review)
}

// GET /api/scim-connections/{id}/users?provisioned=true|false&offset=&limit=
// Returns a page of the users the connection has provisioned, or of its auth provider's users that it has not.
func (h *SCIMConnectionHandler) Users(req api.Context) error {
	provisioned, err := strconv.ParseBool(req.URL.Query().Get("provisioned"))
	if err != nil {
		return types.NewErrBadRequest("provisioned must be true or false")
	}

	page, err := h.setup.Users(req.Context(), req.PathValue("id"), provisioned, queryPage(req))
	if err != nil {
		return err
	}
	return req.Write(page)
}

// GET /api/scim-connections/{id}/groups?list=bound|unboundReferenced|unreferenced&offset=&limit=
// Returns a page of the groups SCIM manages, the referenced groups the identity provider has not pushed, or the
// unbound groups that nothing references.
func (h *SCIMConnectionHandler) Groups(req api.Context) error {
	page, err := h.setup.Groups(req.Context(), req.PathValue("id"), scimsetup.GroupList(req.URL.Query().Get("list")), queryPage(req))
	if err != nil {
		return err
	}
	return req.Write(page)
}

// GET /api/scim-connections/{id}/failures?offset=&limit=
// Returns a page of the connection's recent failed requests, newest first.
func (h *SCIMConnectionHandler) Failures(req api.Context) error {
	page, err := h.setup.Failures(req.Context(), req.PathValue("id"), queryPage(req))
	if err != nil {
		return err
	}
	return req.Write(page)
}

// POST /api/scim-connections/{id}/enforce
// Enforces SCIM: sign-in with the auth provider requires a SCIM binding from now on, users that SCIM has not
// provisioned are disabled, and unbound groups that nothing references are deleted. It cannot be reversed. Only an
// Owner who signed in through the provider, and is provisioned and active, can enforce, and only while every
// referenced group is bound. The bootstrap user cannot.
func (h *SCIMConnectionHandler) Enforce(req api.Context) error {
	actor := scimActor(req)
	if !actor.Owner || actor.Bootstrap {
		return types.NewErrForbidden("only an owner who signed in through the auth provider can enforce SCIM")
	}

	result, err := h.setup.Enforce(req.Context(), req.PathValue("id"), actor)
	if err != nil {
		return err
	}
	return req.Write(result)
}

// POST /api/scim-connections/{id}/rotate-token
// Issues a new bearer token, including the first token of a connection that has none, and returns it, shown only in
// this response. The previous token is still accepted for a day, or until it is revoked, so the identity provider can
// be switched over without failed requests.
func (h *SCIMConnectionHandler) RotateToken(req api.Context) error {
	if err := requireSCIMTokenManager(req); err != nil {
		return err
	}

	conn, err := h.setup.RotateToken(req.Context(), req.PathValue("id"))
	if err != nil {
		return err
	}
	return req.Write(conn)
}

// POST /api/scim-connections/{id}/revoke-current-token
// Replaces a leaked bearer token in one step: issues a new token, returned only in this response, and stops
// accepting both the current token and the previous one.
func (h *SCIMConnectionHandler) RevokeCurrentToken(req api.Context) error {
	if err := requireSCIMTokenManager(req); err != nil {
		return err
	}

	conn, err := h.setup.RevokeCurrentToken(req.Context(), req.PathValue("id"))
	if err != nil {
		return err
	}
	return req.Write(conn)
}

// POST /api/scim-connections/{id}/revoke-previous-token
// Stops accepting the bearer token that the last rotation replaced.
func (h *SCIMConnectionHandler) RevokePreviousToken(req api.Context) error {
	if err := requireSCIMTokenManager(req); err != nil {
		return err
	}

	conn, err := h.setup.RevokePreviousToken(req.Context(), req.PathValue("id"))
	if err != nil {
		return err
	}
	return req.Write(conn)
}

// requireSCIMTokenManager refuses the request unless it comes from an Owner, which includes the bootstrap user.
// Authorization already limits these routes to Owners, so this is a second line of defense.
func requireSCIMTokenManager(req api.Context) error {
	if !req.UserIsOwner() {
		return types.NewErrForbidden("only an owner can manage the SCIM token")
	}
	return nil
}

// scimActor returns the requesting user and the auth provider they signed in with.
func scimActor(req api.Context) scimsetup.Actor {
	name, namespace := req.AuthProviderNameAndNamespace()
	return scimsetup.Actor{
		UserID:                req.UserID(),
		AuthProviderNamespace: namespace,
		AuthProviderName:      name,
		Owner:                 req.UserIsOwner(),
		Bootstrap:             req.User.GetName() == system.BootstrapName,
	}
}

// queryPage returns the page that the offset and limit query parameters select.
func queryPage(req api.Context) scimsetup.Page {
	return scimsetup.NormalizePage(queryInt(req, "offset"), queryInt(req, "limit"))
}

// queryInt returns a non-negative integer query parameter, or zero when it is absent or invalid.
func queryInt(req api.Context, name string) int {
	value, err := strconv.Atoi(req.URL.Query().Get(name))
	if err != nil || value < 0 {
		return 0
	}
	return value
}
