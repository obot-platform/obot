package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/api/server/audit/store"
)

var log = logger.Package()

const (
	AuditLogsModeOff  = "off"
	AuditLogsModeDisk = "disk"
	AuditLogsModeS3   = "s3"
)

type LogEntry struct {
	Time         time.Time `json:"time"`
	UserID       string    `json:"userID"`
	Username     string    `json:"username"`
	UserEmail    string    `json:"userEmail"`
	Method       string    `json:"method"`
	Path         string    `json:"path"`
	UserAgent    string    `json:"userAgent"`
	SourceIP     string    `json:"sourceIP"`
	ResponseCode int       `json:"responseCode"`
	Host         string    `json:"host"`
}

func (e LogEntry) bytes() ([]byte, error) {
	return json.Marshal(e)
}

type Options struct {
	AuditLogsMode             string `usage:"Enable audit logging" default:"off"`
	AuditLogsMaxFileSize      int    `usage:"Audit log max file size in bytes, logs will be flushed when this size is exceeded" default:"1073741824"`
	AuditLogsMaxFlushInterval int    `usage:"Audit log flush interval in seconds regardless of buffer size" default:"30"`
	AuditLogsCompressFile     bool   `usage:"Compress audit log files" default:"true"`

	store.DiskStoreOptions
	store.S3StoreOptions
}

type Logger interface {
	LogEntry(LogEntry) error
	Close() error
}

type persistentLogger struct {
	lock             sync.Mutex
	persistSemaphore chan struct{}
	store            store.Store
	buffer           []byte
}

func New(ctx context.Context, options Options) (Logger, error) {
	if options.AuditLogsMode == AuditLogsModeOff {
		return (*noOpLogger)(nil), nil
	}

	host, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %w", err)
	}

	host = strings.ReplaceAll(strings.ReplaceAll(host, " ", "_"), ".", "_")

	var s store.Store
	switch options.AuditLogsMode {
	case AuditLogsModeDisk:
		s, err = store.NewDiskStore(host, options.AuditLogsCompressFile, options.DiskStoreOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to create audit log store: %w", err)
		}
	case AuditLogsModeS3:
		s, err = store.NewS3Store(host, options.AuditLogsCompressFile, options.S3StoreOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to create audit log store: %w", err)
		}
	default:
		return nil, fmt.Errorf("invalid audit log mode: %s", options.AuditLogsMode)
	}

	l := &persistentLogger{
		lock:             sync.Mutex{},
		persistSemaphore: make(chan struct{}, 1),
		store:            s,
		buffer:           make([]byte, 0, options.AuditLogsMaxFileSize*2),
	}

	go l.startPersistenceLoop(ctx, time.Duration(options.AuditLogsMaxFlushInterval)*time.Second)
	return l, nil
}

func (l *persistentLogger) LogEntry(entry LogEntry) error {
	b, err := entry.bytes()
	if err != nil {
		return err
	}

	l.lock.Lock()
	defer l.lock.Unlock()

	l.buffer = append(l.buffer, b...)
	if len(l.buffer) >= cap(l.buffer)/2 {
		return l.persist()
	}
	return nil
}

func (l *persistentLogger) Close() error {
	return l.persist()
}

func (l *persistentLogger) startPersistenceLoop(ctx context.Context, flushInterval time.Duration) {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	var err error
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err = l.persist(); err != nil {
				log.Errorf("Failed to persist audit log: %v", err)
			}
		}
	}
}

func (l *persistentLogger) persist() error {
	// Allow only one persistence operation at a time.
	// That way, if the ticker or the file size exceeds the limit, only one
	// persistence operation will be triggered.
	select {
	case l.persistSemaphore <- struct{}{}:
		defer func() { <-l.persistSemaphore }()
	default:
		return nil
	}

	l.lock.Lock()
	defer l.lock.Unlock()
	if len(l.buffer) == 0 {
		return nil
	}

	if err := l.store.Persist(l.buffer); err != nil {
		return err
	}

	l.buffer = l.buffer[:0]
	return nil
}

type noOpLogger struct{}

func (l *noOpLogger) LogEntry(_ LogEntry) error {
	return nil
}

func (l *noOpLogger) Close() error {
	return nil
}
