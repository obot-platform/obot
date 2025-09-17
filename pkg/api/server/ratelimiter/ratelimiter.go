package ratelimiter

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/server/requestinfo"
	"github.com/sethvargo/go-limiter"
	"github.com/sethvargo/go-limiter/memorystore"
	"k8s.io/apiserver/pkg/authentication/user"
)

const (
	// HeaderRateLimitLimit, HeaderRateLimitRemaining, and HeaderRateLimitReset
	// are the recommended return header values from IETF on rate limiting. Reset
	// is in UTC time.
	headerRateLimitLimit     = "X-RateLimit-Limit"
	headerRateLimitRemaining = "X-RateLimit-Remaining"
	headerRateLimitReset     = "X-RateLimit-Reset"

	// HeaderRetryAfter is the header used to indicate when a client should retry
	// requests (when the rate limit expires), in UTC time.
	headerRetryAfter = "Retry-After"
)

var ErrRateLimitExceeded = errors.New("rate limit exceeded, please try again later")

type Options struct {
	UnauthenticatedRateLimit int `usage:"Rate limit for unauthenticated requests (req/sec)" default:"100"`
	AuthenticatedRateLimit   int `usage:"Rate limit for authenticated non-admin requests (req/sec)" default:"200"`
}

// RateLimiter limits the number of HTTP requests per second a user can make.
// It tracks limits for unauthenticated and authenticated users separately:
// - Authenticated requests are tracked by user ID or name.
// - Unauthenticated requests are tracked by IP address.
// - Admins are exempt from rate limiting.
type RateLimiter struct {
	unauthenticatedStore limiter.Store
	authenticatedStore   limiter.Store
}

func New(opts Options) (*RateLimiter, error) {
	unauthenticatedStore, err := memorystore.New(&memorystore.Config{
		Tokens:   uint64(opts.UnauthenticatedRateLimit),
		Interval: time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create unauthenticated store: %w", err)
	}

	authenticatedStore, err := memorystore.New(&memorystore.Config{
		Tokens:   uint64(opts.AuthenticatedRateLimit),
		Interval: time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create authenticated store: %w", err)
	}

	return &RateLimiter{
		unauthenticatedStore: unauthenticatedStore,
		authenticatedStore:   authenticatedStore,
	}, nil
}

// ApplyLimit applies the user's rate limit to the request, sets the rate limit headers, and returns a ErrRateLimitExceeded error if the limit has been exceeded.
// It returns nil if the user is exempt from rate limiting or if the user has not exceeded their limit.
func (l *RateLimiter) ApplyLimit(u user.Info, rw http.ResponseWriter, req *http.Request) error {
	groups := u.GetGroups()

	if slices.Contains(groups, types.GroupAdmin) {
		// Admins are exempt from rate limiting
		return nil
	}

	var store limiter.Store
	key := u.GetUID()
	if key == "" {
		key = u.GetName()
	}

	if slices.Contains(groups, types.GroupAuthenticated) && key != "" {
		store = l.authenticatedStore
	} else {
		// Get the source IP address from the request.
		key = requestinfo.GetSourceIP(req)

		// Strip the port from the IP address if present.
		if ip, _, err := net.SplitHostPort(key); err == nil {
			key = ip
		}

		store = l.unauthenticatedStore
	}

	limit, remaining, reset, ok, err := store.Take(req.Context(), key)
	if err != nil {
		return fmt.Errorf("failed to take rate limit tokens: %w", err)
	}

	resetTime := time.Unix(0, int64(reset)).UTC().Format(time.RFC1123)

	// Always set the rate limit response headers
	rw.Header().Set(headerRateLimitLimit, strconv.FormatUint(limit, 10))
	rw.Header().Set(headerRateLimitRemaining, strconv.FormatUint(remaining, 10))
	rw.Header().Set(headerRateLimitReset, resetTime)

	if !ok {
		// Rate limit exceeded.
		rw.Header().Set(headerRetryAfter, resetTime)
		return ErrRateLimitExceeded
	}

	return nil
}
