package cli

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
	"github.com/obot-platform/obot/apiclient/types"
	keyringlib "github.com/zalando/go-keyring"
)

const auditSpoolKeyUser = "audit-spool-key-v1"

type auditSpool interface {
	Write(types.AuditEvent) error
	List(limit int) ([]auditSpoolRecord, error)
	Delete(path string) error
	Status() (dir string, pending int, keyAvailable bool, err error)
}

type auditSpoolRecord struct {
	Path  string
	Event types.AuditEvent
}

type fileAuditSpool struct {
	dir string
	key auditSpoolKeyStore
}

var defaultAuditSpool = func() auditSpool {
	return fileAuditSpool{key: osAuditSpoolKeyStore{}}
}

func (s fileAuditSpool) spoolDir() (string, error) {
	if s.dir != "" {
		return s.dir, nil
	}
	return xdg.DataFile(filepath.Join("obot", "audit_spool"))
}

func (s fileAuditSpool) Write(event types.AuditEvent) error {
	dir, err := s.spoolDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	key, err := s.key.LoadOrCreate()
	if err != nil {
		return err
	}
	plain, err := json.Marshal(event)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	record := encryptedAuditSpoolRecord{
		Version:    1,
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(aead.Seal(nil, nonce, plain, nil)),
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, spoolFileName(event.EventID))
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func (s fileAuditSpool) List(limit int) ([]auditSpoolRecord, error) {
	dir, err := s.spoolDir()
	if err != nil {
		return nil, err
	}
	key, err := s.key.LoadOrCreate()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	records := make([]auditSpoolRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json.aesgcm") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		record, err := readAuditSpoolRecord(path, key)
		if err != nil {
			auditLog.Warnf("failed to read local audit spool record %s; skipping: %v", path, err)
			continue
		}
		records = append(records, record)
		if limit > 0 && len(records) >= limit {
			break
		}
	}
	return records, nil
}

func (s fileAuditSpool) Delete(path string) error {
	return os.Remove(path)
}

func (s fileAuditSpool) Status() (string, int, bool, error) {
	dir, err := s.spoolDir()
	if err != nil {
		return "", 0, false, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return dir, 0, false, err
	}
	keyAvailable := true
	if _, err := s.key.LoadOrCreate(); err != nil {
		keyAvailable = false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return dir, 0, keyAvailable, err
	}
	pending := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json.aesgcm") {
			pending++
		}
	}
	return dir, pending, keyAvailable, nil
}

type encryptedAuditSpoolRecord struct {
	Version    int    `json:"version"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

func readAuditSpoolRecord(path string, key []byte) (auditSpoolRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return auditSpoolRecord{}, err
	}
	var encrypted encryptedAuditSpoolRecord
	if err := json.Unmarshal(data, &encrypted); err != nil {
		return auditSpoolRecord{}, err
	}
	nonce, err := base64.StdEncoding.DecodeString(encrypted.Nonce)
	if err != nil {
		return auditSpoolRecord{}, err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(encrypted.Ciphertext)
	if err != nil {
		return auditSpoolRecord{}, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return auditSpoolRecord{}, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return auditSpoolRecord{}, err
	}
	plain, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return auditSpoolRecord{}, err
	}
	var event types.AuditEvent
	if err := json.Unmarshal(plain, &event); err != nil {
		return auditSpoolRecord{}, err
	}
	return auditSpoolRecord{Path: path, Event: event}, nil
}

func spoolFileName(eventID string) string {
	sum := sha256.Sum256([]byte(eventID))
	return hex.EncodeToString(sum[:]) + ".json.aesgcm"
}

type auditSpoolKeyStore interface {
	LoadOrCreate() ([]byte, error)
}

type osAuditSpoolKeyStore struct{}

func (osAuditSpoolKeyStore) LoadOrCreate() ([]byte, error) {
	secret, err := keyringlib.Get("obot", auditSpoolKeyUser)
	if err == nil {
		return decodeAuditSpoolKey(secret)
	}
	if !errors.Is(err, keyringlib.ErrNotFound) {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := keyringlib.Set("obot", auditSpoolKeyUser, base64.StdEncoding.EncodeToString(key)); err != nil {
		return nil, err
	}
	return key, nil
}

func decodeAuditSpoolKey(secret string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("audit spool key has %d bytes, want 32", len(key))
	}
	return key, nil
}
