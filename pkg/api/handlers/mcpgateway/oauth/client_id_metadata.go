package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/handlers"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	maxClientIDMetadataDocumentBytes = 5 * 1024
	clientMetadataFetchTimeout       = 5 * time.Second
	clientMetadataCacheMaxEntries    = 1000
)

var clientIDNativeExceptions = map[string]struct{}{
	"https://claude.ai/oauth/claude-code-client-metadata": {},
}

type clientMetadataCacheEntry struct {
	client    v1.OAuthClient
	expiresAt time.Time
	cachedAt  time.Time
}

type clientIDMetadataDocument struct {
	types.OAuthClientManifest
	ClientID string          `json:"client_id"`
	JWKS     json.RawMessage `json:"jwks,omitempty"`
}

func (h *handler) resolveOAuthClient(ctx context.Context, c kclient.Client, clientID string) (v1.OAuthClient, error) {
	if isClientIDMetadataDocumentURL(clientID) {
		return h.resolveClientIDMetadataDocument(ctx, clientID)
	}

	clientNamespace, clientName, ok := strings.Cut(clientID, ":")
	if !ok {
		return v1.OAuthClient{}, Error{
			Code:        ErrInvalidClient,
			Description: "client_id is invalid",
		}
	}

	var oauthClient v1.OAuthClient
	if err := c.Get(ctx, kclient.ObjectKey{Namespace: clientNamespace, Name: clientName}, &oauthClient); apierrors.IsNotFound(err) {
		return v1.OAuthClient{}, Error{
			Code:        ErrInvalidClient,
			Description: fmt.Sprintf("client_id does not exist: %s", clientID),
		}
	} else if err != nil {
		return v1.OAuthClient{}, Error{
			Code:        ErrServerError,
			Description: fmt.Sprintf("failed to get OAuth client: %v", err),
		}
	}

	return oauthClient, nil
}

func (h *handler) resolveClientIDMetadataDocument(ctx context.Context, clientID string) (v1.OAuthClient, error) {
	if err := validateClientIDMetadataDocumentURL(clientID); err != nil {
		return v1.OAuthClient{}, Error{
			Code:        ErrInvalidClient,
			Description: err.Error(),
		}
	}

	if client, ok := h.getCachedClientMetadata(clientID); ok {
		return client, nil
	}

	doc, expiresAt, err := h.fetchClientIDMetadataDocument(ctx, clientID)
	if err != nil {
		return v1.OAuthClient{}, Error{
			Code:        ErrInvalidClient,
			Description: err.Error(),
		}
	}

	client, err := h.oauthClientFromMetadataDocument(clientID, doc)
	if err != nil {
		return v1.OAuthClient{}, Error{
			Code:        ErrInvalidClient,
			Description: err.Error(),
		}
	}

	h.cacheClientMetadata(clientID, client, expiresAt)

	return client, nil
}

func isClientIDMetadataDocumentURL(clientID string) bool {
	u, err := url.Parse(clientID)
	return err == nil && u.Scheme != "" && u.Host != ""
}

func validateClientIDMetadataDocumentURL(clientID string) error {
	u, err := url.Parse(clientID)
	if err != nil {
		return fmt.Errorf("client_id metadata document URL is invalid: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("client_id metadata document URL must use https")
	}
	if u.Host == "" {
		return fmt.Errorf("client_id metadata document URL must include a host")
	}
	if u.User != nil {
		return fmt.Errorf("client_id metadata document URL must not include userinfo")
	}
	if u.Fragment != "" {
		return fmt.Errorf("client_id metadata document URL must not include a fragment")
	}
	if u.Path == "" || u.Path == "/" {
		return fmt.Errorf("client_id metadata document URL must include a path component")
	}
	for segment := range strings.SplitSeq(u.EscapedPath(), "/") {
		if segment == "." || segment == ".." || strings.EqualFold(segment, "%2e") || strings.EqualFold(segment, "%2e%2e") {
			return fmt.Errorf("client_id metadata document URL path must not contain dot segments")
		}
	}
	return nil
}

func (h *handler) fetchClientIDMetadataDocument(ctx context.Context, clientID string) (clientIDMetadataDocument, time.Time, error) {
	ctx, cancel := context.WithTimeout(ctx, clientMetadataFetchTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, clientID, nil)
	if err != nil {
		return clientIDMetadataDocument{}, time.Time{}, err
	}
	httpReq.Header.Set("Accept", "application/json")

	resp, err := h.clientMetadataHTTPClient.Do(httpReq)
	if err != nil {
		return clientIDMetadataDocument{}, time.Time{}, fmt.Errorf("failed to fetch client metadata document: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return clientIDMetadataDocument{}, time.Time{}, fmt.Errorf("client metadata document returned status %d", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "" {
		mediaType, _, err := mime.ParseMediaType(ct)
		if err != nil || mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json") {
			return clientIDMetadataDocument{}, time.Time{}, fmt.Errorf("client metadata document must be JSON")
		}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxClientIDMetadataDocumentBytes+1))
	if err != nil {
		return clientIDMetadataDocument{}, time.Time{}, fmt.Errorf("failed to read client metadata document: %w", err)
	}
	if len(body) > maxClientIDMetadataDocumentBytes {
		return clientIDMetadataDocument{}, time.Time{}, fmt.Errorf("client metadata document exceeds %d bytes", maxClientIDMetadataDocumentBytes)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return clientIDMetadataDocument{}, time.Time{}, fmt.Errorf("client metadata document is invalid JSON: %w", err)
	}
	if _, ok := raw["client_secret"]; ok {
		return clientIDMetadataDocument{}, time.Time{}, fmt.Errorf("client metadata document must not include client_secret")
	}
	if _, ok := raw["client_secret_expires_at"]; ok {
		return clientIDMetadataDocument{}, time.Time{}, fmt.Errorf("client metadata document must not include client_secret_expires_at")
	}

	var doc clientIDMetadataDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return clientIDMetadataDocument{}, time.Time{}, fmt.Errorf("client metadata document is invalid: %w", err)
	}
	if len(doc.JWKS) > 0 {
		doc.OAuthClientManifest.JWKS = string(doc.JWKS)
	}

	return doc, clientMetadataCacheExpiration(resp.Header), nil
}

func (h *handler) oauthClientFromMetadataDocument(clientID string, doc clientIDMetadataDocument) (v1.OAuthClient, error) {
	if doc.ClientID != clientID {
		return v1.OAuthClient{}, fmt.Errorf("client metadata document client_id must exactly match the document URL")
	}
	if doc.ClientName == "" {
		return v1.OAuthClient{}, fmt.Errorf("client metadata document must include client_name")
	}
	if len(doc.RedirectURIs) == 0 {
		return v1.OAuthClient{}, fmt.Errorf("client metadata document must include redirect_uris")
	}
	if doc.ApplicationType == "" {
		if _, ok := clientIDNativeExceptions[clientID]; ok {
			doc.ApplicationType = "native"
		} else {
			doc.ApplicationType = "web"
		}
	}
	if doc.ApplicationType != "web" && doc.ApplicationType != "native" {
		return v1.OAuthClient{}, fmt.Errorf("client metadata document application_type must be web or native")
	}
	if isSharedSecretAuthMethod(doc.TokenEndpointAuthMethod) {
		return v1.OAuthClient{}, fmt.Errorf("client metadata document must not use shared-secret token endpoint authentication")
	}
	if doc.TokenEndpointAuthMethod == "" {
		doc.TokenEndpointAuthMethod = "none"
	}
	if doc.JWKSURI != "" && doc.JWKS != nil {
		return v1.OAuthClient{}, fmt.Errorf("client metadata document must not include both jwks_uri and jwks")
	}
	if doc.TokenEndpointAuthMethod == "private_key_jwt" && doc.JWKSURI == "" && doc.JWKS == nil {
		return v1.OAuthClient{}, fmt.Errorf("client metadata document using private_key_jwt must include jwks_uri or jwks")
	}

	client := v1.OAuthClient{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: system.DefaultNamespace,
			Name:      clientID,
		},
		Spec: v1.OAuthClientSpec{
			Manifest: doc.OAuthClientManifest,
		},
	}

	if err := handlers.ValidateClientConfig(&client, h.oauthConfig); err != nil {
		return v1.OAuthClient{}, err
	}

	return client, nil
}

func isSharedSecretAuthMethod(method string) bool {
	return strings.Contains(method, "client_secret")
}

func (h *handler) getCachedClientMetadata(clientID string) (v1.OAuthClient, bool) {
	h.clientMetadataCacheLock.Lock()
	defer h.clientMetadataCacheLock.Unlock()

	entry, ok := h.clientMetadataCache[clientID]
	if !ok || time.Now().After(entry.expiresAt) {
		delete(h.clientMetadataCache, clientID)
		return v1.OAuthClient{}, false
	}
	return entry.client, true
}

func (h *handler) cacheClientMetadata(clientID string, client v1.OAuthClient, expiresAt time.Time) {
	if expiresAt.IsZero() {
		return
	}

	h.clientMetadataCacheLock.Lock()
	defer h.clientMetadataCacheLock.Unlock()

	now := time.Now()
	for cachedClientID, entry := range h.clientMetadataCache {
		if now.After(entry.expiresAt) {
			delete(h.clientMetadataCache, cachedClientID)
		}
	}

	if _, ok := h.clientMetadataCache[clientID]; !ok && len(h.clientMetadataCache) >= clientMetadataCacheMaxEntries {
		h.evictOldestClientMetadata()
	}

	h.clientMetadataCache[clientID] = clientMetadataCacheEntry{
		client:    client,
		expiresAt: expiresAt,
		cachedAt:  now,
	}
}

func (h *handler) evictOldestClientMetadata() {
	var oldestClientID string
	var oldestCachedAt time.Time
	for clientID, entry := range h.clientMetadataCache {
		if oldestClientID == "" || entry.cachedAt.Before(oldestCachedAt) {
			oldestClientID = clientID
			oldestCachedAt = entry.cachedAt
		}
	}
	delete(h.clientMetadataCache, oldestClientID)
}

func clientMetadataCacheExpiration(header http.Header) time.Time {
	var maxAge time.Duration
	for _, cacheControl := range header.Values("Cache-Control") {
		cacheControl = strings.ToLower(cacheControl)
		for directive := range strings.SplitSeq(cacheControl, ",") {
			directive = strings.TrimSpace(directive)
			if directive == "no-store" || directive == "no-cache" {
				return time.Time{}
			}
			if value, ok := strings.CutPrefix(directive, "max-age="); ok {
				seconds, err := strconv.Atoi(strings.Trim(value, `"`))
				if err == nil {
					if seconds <= 0 {
						return time.Time{}
					}
					maxAge = time.Duration(seconds) * time.Second
				}
			}
		}
	}
	if maxAge > 0 {
		return time.Now().Add(maxAge)
	}

	if expires := header.Get("Expires"); expires != "" {
		t, err := http.ParseTime(expires)
		if err == nil && time.Now().Before(t) {
			return t
		}
	}

	return time.Time{}
}
