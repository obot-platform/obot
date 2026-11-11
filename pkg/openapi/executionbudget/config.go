package executionbudget

import (
	"fmt"
	"time"
)

const (
	MiB = int64(1) << 20
	KiB = int64(1) << 10

	DefaultRequestBodyLimit  = MiB
	DefaultUpstreamBodyLimit = 8 * MiB
	DefaultHeaderLimit       = 64 * KiB
	DefaultAuditBodyLimit    = 32 * KiB
)

type Config struct {
	PlatformMaxInFlight        int
	PlatformMaxWaiting         int
	DefaultPoolMaxInFlight     int
	DedicatedPoolMaxInFlight   int
	DefaultOperationInFlight   int
	DefaultOperationWaiting    int
	MaxWait                    time.Duration
	CallDeadline               time.Duration
	FencingMargin              time.Duration
	ReclaimGrace               time.Duration
	TerminalRetention          time.Duration
	MaxTerminalRequests        int64
	MaxOperationConfigurations int64
	AdmissionInterval          time.Duration
	RequestBodyLimit           int64
	UpstreamBodyLimit          int64
	HeaderLimit                int64
	AuditBodyLimit             int64
}

func DefaultConfig() Config {
	return Config{
		PlatformMaxInFlight:        32,
		PlatformMaxWaiting:         64,
		DefaultPoolMaxInFlight:     16,
		DedicatedPoolMaxInFlight:   16,
		DefaultOperationInFlight:   4,
		DefaultOperationWaiting:    8,
		MaxWait:                    2 * time.Second,
		CallDeadline:               30 * time.Second,
		FencingMargin:              time.Second,
		ReclaimGrace:               time.Second,
		TerminalRetention:          15 * time.Minute,
		MaxTerminalRequests:        50000,
		MaxOperationConfigurations: 4096,
		AdmissionInterval:          20 * time.Millisecond,
		RequestBodyLimit:           DefaultRequestBodyLimit,
		UpstreamBodyLimit:          DefaultUpstreamBodyLimit,
		HeaderLimit:                DefaultHeaderLimit,
		AuditBodyLimit:             DefaultAuditBodyLimit,
	}
}

func (config Config) Validate() error {
	positive := map[string]int{
		"platformMaxInFlight": config.PlatformMaxInFlight,
		"platformMaxWaiting":  config.PlatformMaxWaiting,
		"defaultPool":         config.DefaultPoolMaxInFlight,
		"dedicatedPool":       config.DedicatedPoolMaxInFlight,
		"operationInFlight":   config.DefaultOperationInFlight,
		"operationWaiting":    config.DefaultOperationWaiting,
	}
	for name, value := range positive {
		if value <= 0 {
			return fmt.Errorf("%w: %s must be positive", ErrInvalidConfiguration, name)
		}
	}
	if config.PlatformMaxInFlight > 128 || config.PlatformMaxWaiting > 1024 {
		return fmt.Errorf("%w: platform limits exceed hard cap", ErrInvalidConfiguration)
	}
	if config.DefaultOperationInFlight > config.DefaultPoolMaxInFlight || config.DefaultOperationWaiting > 16 {
		return fmt.Errorf("%w: default operation limits exceed pool or hard cap", ErrInvalidConfiguration)
	}
	durations := []struct {
		name  string
		value time.Duration
		min   time.Duration
		max   time.Duration
	}{
		{"maxWait", config.MaxWait, time.Second, 30 * time.Second},
		{"callDeadline", config.CallDeadline, time.Second, 2 * time.Minute},
		{"fencingMargin", config.FencingMargin, 250 * time.Millisecond, 5 * time.Second},
		{"reclaimGrace", config.ReclaimGrace, 250 * time.Millisecond, 5 * time.Second},
		{"terminalRetention", config.TerminalRetention, time.Minute, 24 * time.Hour},
		{"admissionInterval", config.AdmissionInterval, time.Millisecond, time.Second},
	}
	for _, duration := range durations {
		if duration.value < duration.min || duration.value > duration.max {
			return fmt.Errorf("%w: %s outside allowed range", ErrInvalidConfiguration, duration.name)
		}
	}
	if config.MaxWait >= config.CallDeadline || config.FencingMargin >= config.CallDeadline {
		return fmt.Errorf("%w: deadline relationship invalid", ErrInvalidConfiguration)
	}
	if config.MaxTerminalRequests <= 0 || config.MaxOperationConfigurations <= 0 || config.MaxOperationConfigurations > 4096 {
		return fmt.Errorf("%w: bounded state limits invalid", ErrInvalidConfiguration)
	}
	byteLimits := []struct {
		name  string
		value int64
		min   int64
		max   int64
	}{
		{"requestBodyLimit", config.RequestBodyLimit, KiB, 8 * MiB},
		{"upstreamBodyLimit", config.UpstreamBodyLimit, KiB, 32 * MiB},
		{"headerLimit", config.HeaderLimit, 4 * KiB, 128 * KiB},
		{"auditBodyLimit", config.AuditBodyLimit, 0, 64 * KiB},
	}
	for _, limit := range byteLimits {
		if limit.value < limit.min || limit.value > limit.max {
			return fmt.Errorf("%w: %s outside allowed range", ErrInvalidConfiguration, limit.name)
		}
	}
	return nil
}
