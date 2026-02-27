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
	ModeOff  = "off"
	ModeDisk = "disk"
	ModeS3   = "s3"
)

type LogEntry struct {
	Time         time.Time `json:"time"`
	UserID       string    `json:"userID"`
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
	AuditLogsMaxFlushInterval int    `usage:"Audit log flush interval in seconds regardless of buffer size" default:"120"`
	AuditLogsCompressFile     bool   `usage:"Compress audit log files" default:"true"`

	store.DiskStoreOptions
	store.S3StoreOptions
}

type Logger interface {
	LogEntry(LogEntry) error
	Close() error
}

type persistentLogger struct {
	lock        sync.Mutex
	kickPersist chan struct{}
	store       store.Store
	buffer      []byte
	spare       []byte // reusable buffer from previous successful persist
	maxSize     int    // flush trigger threshold (= AuditLogsMaxFileSize)
}

func New(ctx context.Context, options Options) (Logger, error) {
	if options.AuditLogsMode == ModeOff {
		return (*noOpLogger)(nil), nil
	}

	host, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %w", err)
	}

	host = strings.ReplaceAll(strings.ReplaceAll(host, " ", "_"), ".", "_")

	var s store.Store
	switch options.AuditLogsMode {
	case ModeDisk:
		s, err = store.NewDiskStore(host, options.AuditLogsCompressFile, options.DiskStoreOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to create audit log store: %w", err)
		}
	case ModeS3:
		s, err = store.NewS3Store(host, options.AuditLogsCompressFile, options.S3StoreOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to create audit log store: %w", err)
		}
	default:
		return nil, fmt.Errorf("invalid audit log mode: %s", options.AuditLogsMode)
	}

	if options.AuditLogsMaxFileSize <= 0 {
		return nil, fmt.Errorf("audit log max file size must be greater than 0")
	}

	l := &persistentLogger{
		kickPersist: make(chan struct{}),
		store:       s,
		maxSize:     options.AuditLogsMaxFileSize,
	}

	go l.runPersistenceLoop(ctx, time.Duration(options.AuditLogsMaxFlushInterval)*time.Second)
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
	if len(l.buffer) >= l.maxSize {
		select {
		case l.kickPersist <- struct{}{}:
		default:
		}
	}

	return nil
}

func (l *persistentLogger) Close() error {
	return l.persist()
}

func (l *persistentLogger) runPersistenceLoop(ctx context.Context, flushInterval time.Duration) {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	var err error
	for {
		select {
		case <-ctx.Done():
			return
		case <-l.kickPersist:
			ticker.Stop()
			if err = l.persist(); err != nil {
				log.Errorf("Failed to persist audit log: %v", err)
			}
			ticker.Reset(flushInterval)
		case <-ticker.C:
			if err = l.persist(); err != nil {
				log.Errorf("Failed to persist audit log: %v", err)
			}
		}
	}
}

func (l *persistentLogger) persist() error {
	l.lock.Lock()
	if len(l.buffer) == 0 {
		l.lock.Unlock()
		return nil
	}

	buf := l.buffer
	if l.spare != nil {
		l.buffer = l.spare[:0]
	} else {
		l.buffer = nil
	}
	l.spare = nil
	l.lock.Unlock()

	if err := l.store.Persist(buf); err != nil {
		l.lock.Lock()
		l.buffer = append(buf, l.buffer...)
		l.lock.Unlock()
		return err
	}

	l.lock.Lock()
	if cap(buf) <= l.maxSize {
		l.spare = buf[:0]
	} else {
		l.spare = nil
	}
	l.lock.Unlock()

	return nil
}

type noOpLogger struct{}

func (*noOpLogger) LogEntry(LogEntry) error {
	return nil
}

func (*noOpLogger) Close() error {
	return nil
}
