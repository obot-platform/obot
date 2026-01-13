package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// TokenRefreshAttempts tracks token refresh attempts by provider and result
	TokenRefreshAttempts = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "obot_auth_token_refresh_attempts_total",
			Help: "Total number of token refresh attempts by provider and result (success/failure)",
		},
		[]string{"provider", "result"},
	)

	// TokenRefreshDuration tracks token refresh latency by provider
	TokenRefreshDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "obot_auth_token_refresh_duration_seconds",
			Help:    "Token refresh latency in seconds by provider",
			Buckets: prometheus.DefBuckets, // [0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10]
		},
		[]string{"provider"},
	)

	// AuthenticationAttempts tracks authentication attempts by provider and result
	AuthenticationAttempts = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "obot_auth_authentication_attempts_total",
			Help: "Total number of authentication attempts by provider and result (success/failure/error)",
		},
		[]string{"provider", "result"},
	)

	// SessionStorageErrors tracks session storage errors by provider and operation
	SessionStorageErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "obot_auth_session_storage_errors_total",
			Help: "Total number of session storage errors by provider and operation type",
		},
		[]string{"provider", "operation"},
	)

	// CookieDecryptionErrors tracks cookie decryption failures by provider
	CookieDecryptionErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "obot_auth_cookie_decryption_errors_total",
			Help: "Total number of cookie decryption failures by provider",
		},
		[]string{"provider"},
	)

	// ActiveSessions tracks the number of active user sessions by provider
	ActiveSessions = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "obot_auth_active_sessions",
			Help: "Current number of active user sessions by provider",
		},
		[]string{"provider"},
	)
)

// RecordTokenRefreshSuccess records a successful token refresh
func RecordTokenRefreshSuccess(provider string, duration float64) {
	TokenRefreshAttempts.WithLabelValues(provider, "success").Inc()
	TokenRefreshDuration.WithLabelValues(provider).Observe(duration)
}

// RecordTokenRefreshFailure records a failed token refresh
func RecordTokenRefreshFailure(provider string, duration float64) {
	TokenRefreshAttempts.WithLabelValues(provider, "failure").Inc()
	TokenRefreshDuration.WithLabelValues(provider).Observe(duration)
}

// RecordAuthenticationSuccess records a successful authentication
func RecordAuthenticationSuccess(provider string) {
	AuthenticationAttempts.WithLabelValues(provider, "success").Inc()
}

// RecordAuthenticationFailure records a failed authentication
func RecordAuthenticationFailure(provider string) {
	AuthenticationAttempts.WithLabelValues(provider, "failure").Inc()
}

// RecordAuthenticationError records an authentication error
func RecordAuthenticationError(provider string) {
	AuthenticationAttempts.WithLabelValues(provider, "error").Inc()
}

// RecordSessionStorageError records a session storage error
func RecordSessionStorageError(provider string, operation string) {
	SessionStorageErrors.WithLabelValues(provider, operation).Inc()
}

// RecordCookieDecryptionError records a cookie decryption error
func RecordCookieDecryptionError(provider string) {
	CookieDecryptionErrors.WithLabelValues(provider).Inc()
}

// SetActiveSessions sets the current number of active sessions
func SetActiveSessions(provider string, count float64) {
	ActiveSessions.WithLabelValues(provider).Set(count)
}
