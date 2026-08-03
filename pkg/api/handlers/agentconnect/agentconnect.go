// Package agentconnect proxies browser traffic to a hosted agent sandbox.
//
// It mirrors the MCP gateway's connect route: one stable Obot URL per instance,
// authorized by Obot, forwarding to an address only Obot knows. The sandbox is
// reachable at a cluster-internal address, so this is the only way a user
// reaches their agent, and the only place access is decided.
package agentconnect

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

// DevRouter reaches a sandbox from outside the cluster. It is supplied only in
// development, where Obot runs on a developer's machine and a sandbox's
// cluster-internal address resolves nowhere. In production Obot runs inside the
// cluster and this is nil.
type DevRouter interface {
	// DevRoute returns an address and transport for a backend sandbox ID. The
	// bool reports whether a route was available.
	DevRoute(backendID string) (string, http.RoundTripper, bool)
}

type Handler struct {
	transport http.RoundTripper
	devRouter DevRouter
}

func New(transport http.RoundTripper, devRouter DevRouter) *Handler {
	return &Handler{transport: transport, devRouter: devRouter}
}

// Proxy forwards a request to the instance's sandbox.
//
// For an authenticated caller the authorizer has already established that they
// own this instance and still have access to the agent behind it.
//
// An unauthenticated one reaches here deliberately: this route is opened by a
// person following a link, so the authorizer admits them precisely so that the
// redirect below can send them to sign in. Nothing about the instance is read
// before that happens, so it discloses nothing -- not even whether the instance
// exists.
func (h *Handler) Proxy(req api.Context) error {
	// Unlike the MCP gateway, which challenges an OAuth client, this lands a
	// browser on sign-in rather than a bare 403.
	if !req.UserIsAuthenticated() {
		http.Redirect(req.ResponseWriter, req.Request,
			"/?rd="+url.QueryEscape(req.URL.RequestURI()), http.StatusFound)
		return nil
	}

	// The authorizer has already established ownership for this path; reading
	// the instance again here keeps the proxy target explicit rather than
	// relying on state threaded from authorization.
	var instance v1.HostedAgentInstance
	if err := req.Get(&instance, req.PathValue("hosted_agent_instance_id")); err != nil {
		return err
	}

	target, err := targetURL(&instance)
	if err != nil {
		return err
	}

	// Outside the cluster the sandbox's own address is unreachable, so route
	// through the API server instead. The published address stays canonical;
	// only the hop changes, so what a user sees does not differ by environment.
	transport := h.transport
	basePath := ""
	// The path this proxy is mounted at, which a rewritten redirect has to stay
	// inside.
	prefix := "/agent-connect/" + instance.Name
	if h.devRouter != nil {
		if devURL, devTransport, ok := h.devRouter.DevRoute(instance.Status.BackendID); ok {
			parsed, parseErr := url.Parse(devURL)
			if parseErr != nil {
				return fmt.Errorf("failed to parse development agent address: %w", parseErr)
			}
			target, transport, basePath = parsed, devTransport, parsed.Path
		}
	}

	(&httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(r *httputil.ProxyRequest) {
			// SetXForwarded preserves the X-Forwarded-For handling ReverseProxy
			// applied under the deprecated Director. It also writes
			// X-Forwarded-Host and X-Forwarded-Proto, so the values that matter
			// here are re-applied afterwards.
			r.SetXForwarded()
			r.Out.Header.Set("X-Forwarded-Host", r.In.Host)

			// A proxy that terminated TLS in front of Obot is the only hop that
			// knows the scheme the browser actually used, so its report wins.
			// Absent one, SetXForwarded already derived the scheme from this
			// connection, which is then the best available answer.
			//
			// Assuming https for any host that is not localhost -- as this did
			// previously -- misreports a plain-HTTP deployment. An agent that
			// rebuilds its own public origin from these headers, as ADK does to
			// check Origin, then compares https://host against the browser's
			// http://host and rejects every request it is sent.
			if proto := r.In.Header.Get("X-Forwarded-Proto"); proto != "" {
				r.Out.Header.Set("X-Forwarded-Proto", proto)
			}

			// The stripped prefix, so an agent can build links that work from
			// outside. Without it an agent serving absolute URLs would point at
			// its own root, which is not reachable.
			//
			// Neither X-Forwarded-Prefix nor X-Forwarded-Path is standard --
			// RFC 7239 defines only by, for, host and proto -- but Prefix is the
			// convention frameworks implement, and it says what actually
			// happened here: a prefix was removed.
			r.Out.Header.Set("X-Forwarded-Prefix", prefix)

			// Host as well as URL.Host: the outbound request would otherwise
			// carry Obot's hostname to the sandbox.
			r.Out.Host = target.Host
			r.Out.URL.Scheme = target.Scheme
			r.Out.URL.Host = target.Host

			// Everything after /agent-connect/{id} belongs to the agent, so the
			// prefix is stripped rather than forwarded. An agent serving at its
			// root should not have to know where Obot mounted it. The router
			// captures the remainder, which is more reliable than re-parsing the
			// path here.
			// basePath is empty in production. When routing through the API
			// server it is that proxy's own prefix, which the agent's path is
			// appended to rather than replacing.
			r.Out.URL.Path = basePath + "/"
			if rest := r.In.PathValue("rest"); rest != "" {
				r.Out.URL.Path = basePath + "/" + strings.TrimPrefix(rest, "/")
			}
		},
		// An agent that redirects -- a UI sending "/" to "/dev-ui/", say -- names
		// a path on itself, and the browser resolves it against Obot's host. Left
		// alone it lands outside /agent-connect entirely: under the API server
		// route when this hop goes through it, or on Obot's own root otherwise.
		// Either way the user leaves the agent and gets a 403 from somewhere
		// they never asked for.
		//
		// X-Forwarded-Prefix above is the polite way to prevent this, but it
		// only helps for agents that read it, and the API server proxy rewrites
		// Location on the way through regardless.
		ModifyResponse: func(resp *http.Response) error {
			location := resp.Header.Get("Location")
			if location == "" {
				return nil
			}
			parsed, err := url.Parse(location)
			if err != nil || parsed.IsAbs() {
				// An absolute redirect points somewhere else on purpose -- an
				// OAuth provider, say -- and is not ours to rewrite.
				return nil
			}
			path := strings.TrimPrefix(parsed.Path, basePath)
			// An agent told where it is published -- through publicPath, or
			// X-Forwarded-Prefix -- already emits a prefixed redirect. Adding
			// the prefix again produces /agent-connect/{id}/agent-connect/{id}/…,
			// so the rewrite has to be idempotent: it repairs an agent that does
			// not know, and leaves alone one that does.
			if path != prefix && !strings.HasPrefix(path, prefix+"/") {
				path = prefix + "/" + strings.TrimPrefix(path, "/")
			}
			parsed.Path = path
			resp.Header.Set("Location", parsed.String())
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			// A sandbox that is starting, crashed, or listening on a different
			// port than its agent declares fails here rather than at authorization.
			http.Error(w, fmt.Sprintf("failed to reach agent %s: %v", instance.Name, err), http.StatusBadGateway)
		},
	}).ServeHTTP(req.ResponseWriter, req.Request)

	return nil
}

// targetURL is the sandbox address Obot observed, not one derived from the
// request, so a caller cannot influence where this proxies to.
func targetURL(instance *v1.HostedAgentInstance) (*url.URL, error) {
	if instance.Status.URL == "" {
		if instance.Status.State == types.HostedAgentStateReady {
			// Ready with no address means the agent declares no port.
			return nil, types.NewErrBadRequest("agent %s does not expose a port", instance.Name)
		}
		return nil, types.NewErrHTTP(http.StatusServiceUnavailable,
			fmt.Sprintf("agent %s is not running", instance.Name))
	}

	target, err := url.Parse(instance.Status.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse agent address: %w", err)
	}
	return target, nil
}
