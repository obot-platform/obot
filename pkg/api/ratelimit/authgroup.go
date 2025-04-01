// Package ratelimit implements HTTP middleware for rate limiting API requests
package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/sethvargo/go-limiter/httplimit"
	"github.com/sethvargo/go-limiter/memorystore"
	"k8s.io/apiserver/pkg/authentication/user"
)

// Config contains all configuration options for the rate limiter middleware.
// These values can be set via environment variables or command-line flags.
type Config struct {
	// UnauthenticatedRateLimit is the number of requests allowed for unauthenticated users
	UnauthenticatedRateLimit int `usage:"Rate limit for unauthenticated requests (req/sec)" default:"200"`

	// AuthenticatedRateLimit is the number of requests allowed for authenticated non-admin users
	AuthenticatedRateLimit int `usage:"Rate limit for authenticated non-admin requests (req/sec)" default:"400"`

	// UserCacheTTL controls how long authenticated user information is cached
	UserCacheTTL time.Duration `usage:"How long to cache authenticated user information" default:"1hr"`

	// UserCacheCapacity limits the number of authenticated users to store in memory
	UserCacheCapacity int `usage:"Maximum number of users to keep in the auth cache" default:"1000"`
}

// AuthGroupMiddleware implements the Middleware interface with full rate limiting
// functionality for both authenticated and unauthenticated users.
// It uses expirable LRU cache to efficiently track authenticated users
// and different rate limits based on authentication status.
type AuthGroupMiddleware struct {
	// unauthenticatedMiddleware applies unauthenticated rate limits by IP address
	unauthenticatedMiddleware *httplimit.Middleware

	// authenticatedMiddleware applies authenticated rate limits by cached user ID
	authenticatedMiddleware *httplimit.Middleware

	// userCache maps recently used credential hashes to users.
	userCache *expirable.LRU[string, *User]

	// credSources is a set of previously known credential sources
	// used to extract credentials from request headers and cookies.
	credSources *sync.Map
}

// NewAuthGroupMiddleware creates a new rate limiting middleware based on the provided configuration.
// It sets up separate limiters for authenticated and unauthenticated requests,
// and initializes the expirable cache for tracking authenticated users.
func NewAuthGroupMiddleware(config Config) (*AuthGroupMiddleware, error) {
	unauthenticatedStore, err := memorystore.New(&memorystore.Config{
		Tokens:   uint64(config.UnauthenticatedRateLimit),
		Interval: time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create unauthenticated store: %w", err)
	}
	unauthenticatedMiddleware, err := httplimit.NewMiddleware(
		unauthenticatedStore,
		httplimit.IPKeyFunc(
			"X-Forwarded-For",
			"X-Real-IP",
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create unauthenticated middleware: %w", err)
	}

	authenticatedStore, err := memorystore.New(&memorystore.Config{
		Tokens:   uint64(config.AuthenticatedRateLimit),
		Interval: time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create authenticated store: %w", err)
	}
	authenticatedMiddleware, err := httplimit.NewMiddleware(
		authenticatedStore,
		getAuthorizedIDKey,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create authenticated middleware: %w", err)
	}

	return &AuthGroupMiddleware{
		unauthenticatedMiddleware: unauthenticatedMiddleware,
		authenticatedMiddleware:   authenticatedMiddleware,
		userCache: expirable.NewLRU[string, *User](
			config.UserCacheCapacity,
			nil,
			config.UserCacheTTL,
		),
		credSources: &sync.Map{},
	}, nil
}

// registerCredSource registers a new credential source to check on requests.
// This is an internal method called lazily when users are registered,
// ensuring that credential sources are only tracked when actually used.
func (m *AuthGroupMiddleware) registerCredSource(source CredSource) error {
	switch source.Type {
	case CredSourceTypeHeader, CredSourceTypeCookie:
	default:
		return fmt.Errorf("unsupported credential source type: %s", source.Type)
	}

	if source.Name == "" {
		return fmt.Errorf("credential source name cannot be empty")
	}

	// Store this credential source for checking on future requests
	m.credSources.LoadOrStore(source, nil)

	return nil
}

// RegisterUser adds a user to the middleware's cache based on credential info.
// It automatically registers the credential source that was used for authentication,
// so future requests with the same credential can be properly identified.
func (m *AuthGroupMiddleware) RegisterUser(u user.Info) error {
	// Skip invalid or anonymous users
	if u == nil ||
		u.GetUID() == "" ||
		u.GetUID() == "anonymous" {
		return nil
	}

	// Extract credential info from user.Extra
	credHash, source, ok := getUserCredInfo(u)
	if !ok {
		return nil
	}

	// Create and cache the user
	user := &User{
		ID:      u.GetUID(),
		IsAdmin: slices.Contains(u.GetGroups(), authz.AdminGroup),
	}

	// Lazily register the credential source used to authenticate this user
	if err := m.registerCredSource(source); err != nil {
		return fmt.Errorf("failed to register credential source: %w", err)
	}

	// Add user to cache with configured TTL for automatic expiration
	m.userCache.Add(credHash, user)

	return nil
}

type authorizedIDCtxKey struct{}

func contextWithAuthorizedID(ctx context.Context, authorizedID string) context.Context {
	return context.WithValue(ctx, authorizedIDCtxKey{}, authorizedID)
}

func getAuthorizedIDKey(r *http.Request) (string, error) {
	authorizedID, ok := r.Context().Value(authorizedIDCtxKey{}).(string)
	if !ok || authorizedID == "" {
		return "", fmt.Errorf("authorizedID not found in context")
	}
	return authorizedID, nil
}

func (m *AuthGroupMiddleware) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Find authenticated user from request credentials
		cachedUser := m.getCachedUser(r)
		if cachedUser == nil || cachedUser.ID == "" {
			// Apply unauthenticated rate limits to the request by IP
			m.unauthenticatedMiddleware.Handle(next).ServeHTTP(w, r)
			return
		}

		if !cachedUser.IsAdmin {
			// Apply authenticated rate limits to the request by user ID
			r = r.WithContext(contextWithAuthorizedID(r.Context(), cachedUser.ID))
			m.authenticatedMiddleware.Handle(next).ServeHTTP(w, r)
			return
		}

		// Admin users bypass rate limiting completely
		next.ServeHTTP(w, r)
	})
}

// getCachedUser looks for credentials on the request using each registered credential source.
// If it finds a credential, it looks up the user in the cache and returns it.
// If it doesn't find a credential, or a user for that credential, it returns nil.
func (m *AuthGroupMiddleware) getCachedUser(r *http.Request) *User {
	var (
		user  *User
		found bool
	)

	m.credSources.Range(func(key, _ any) bool {
		source := key.(CredSource)
		cred := source.extractCred(r)
		if cred == "" {
			return true
		}

		user, found = m.userCache.Get(hashCred(cred))
		return !found
	})

	return user
}
