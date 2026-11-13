package client

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"k8s.io/apiserver/pkg/storage/value"
)

// Audit body fields written by this version start with a three-byte header. The marker byte can
// never begin valid JSON (an unencrypted legacy field) or base64 (an encrypted legacy field), so
// each field identifies its own format. The marker is per field rather than per row because a
// response can be added to a row by a different Obot version than the one that inserted it.
const (
	auditBodyMarker    byte = 0x00
	auditBodyVersion   byte = 0x01
	auditBodyHeaderLen      = 3

	auditBodyFlagCompressed byte = 1 << 0
	auditBodyFlagEncrypted  byte = 1 << 1
	// auditBodyFlagReference marks a field holding a reference to a shared list-response body
	// rather than the body itself.
	auditBodyFlagReference byte = 1 << 2
	auditBodyKnownFlags         = auditBodyFlagCompressed | auditBodyFlagEncrypted | auditBodyFlagReference

	// auditBodyCompressMinBytes is the smallest body worth compressing; below it the header and
	// zstd framing consume the gain.
	auditBodyCompressMinBytes = 256
	// auditBodyCompressMaxBytes is the largest body that is compressed. Readers refuse to
	// decompress anything larger, which cannot reject a valid field because writers never produce one.
	auditBodyCompressMaxBytes = 64 << 20
	// auditBodyDedupMinBytes is the smallest list result stored as a shared body. Postgres keeps
	// smaller values in the main row, so sharing them saves little.
	auditBodyDedupMinBytes = 2 << 10

	auditBodyHashLen = sha256.Size
	// The first byte hashed says whether the writer encrypts; see auditBodyHash.
	auditBodyHashModePlain     byte = 0x00
	auditBodyHashModeEncrypted byte = 0x01

	// auditBodyReferenceRefresh bounds how stale a shared body's LastReferencedAt may be when a row
	// referencing it is written.
	auditBodyReferenceRefresh = 24 * time.Hour
	// auditBodyCleanupMargin is how far past the audit-row retention cutoff a shared body's
	// LastReferencedAt must be before it is deleted: the refresh interval plus a day, which
	// covers clock skew between replicas and a row's created_at running ahead of its write.
	auditBodyCleanupMargin = auditBodyReferenceRefresh + 24*time.Hour
	// auditBodyKnownFreshFor is how long a replica goes on trusting a stored shared body after
	// reading it, letting it skip reading the stored copy, and compressing and encrypting the
	// same result, again.
	auditBodyKnownFreshFor = auditBodyReferenceRefresh / 2
	auditBodyKnownMax      = 4096

	// unavailableAuditBodyCauseLimit bounds how many distinct causes one read reports.
	unavailableAuditBodyCauseLimit = 5

	auditBodyHashKeyProperty   = "mcp-audit-log-body-hash-key"
	auditBodyHashKeyRetryAfter = time.Minute
	// auditBodyUnusableFor is how long a replica goes on treating a stored copy as unusable before
	// looking at it again. It is short because the cause may be passing, such as a failed call
	// to the encryption provider.
	auditBodyUnusableFor = 10 * time.Minute
	// auditBodyStoredCheckRetryAfter is how long stored copies are left unread after looking one
	// up fails or times out, so that capturing list responses does not wait on the database once
	// per response.
	auditBodyStoredCheckRetryAfter = 30 * time.Second
	auditBodyLookupBatchSize       = 200

	// auditBodyStoredAbsent: nothing is stored under the hash, so the writer stores the body.
	auditBodyStoredAbsent auditBodyStored = 0
	// auditBodyStoredUsable: a copy this writer can read is stored.
	auditBodyStoredUsable auditBodyStored = 1
	// auditBodyStoredUnusable: this writer must not reference what is stored under the hash,
	// either because the stored copy is not usable or because it could not be checked.
	auditBodyStoredUnusable auditBodyStored = 2

	// mcpResponseCallType is the call type of a response-only audit entry.
	mcpResponseCallType = "response"
)

var (
	// auditBodyDedupMethods are the MCP methods whose responses are stored as shared bodies.
	auditBodyDedupMethods = map[string]bool{
		"tools/list":               true,
		"prompts/list":             true,
		"resources/list":           true,
		"resources/templates/list": true,
	}

	auditBodyEncoder = sync.OnceValue(func() *zstd.Encoder {
		encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
		if err != nil {
			panic(fmt.Sprintf("failed to create audit body compressor: %v", err))
		}
		return encoder
	})
	auditBodyDecoder = sync.OnceValue(func() *zstd.Decoder {
		decoder, err := zstd.NewReader(nil, zstd.WithDecoderMaxMemory(auditBodyCompressMaxBytes))
		if err != nil {
			panic(fmt.Sprintf("failed to create audit body decompressor: %v", err))
		}
		return decoder
	})
)

// auditBodyState is the client's in-memory state for shared audit bodies.
type auditBodyState struct {
	lock sync.Mutex
	// hashKey keys the hash that identifies a shared body.
	hashKey        []byte
	hashKeyRetryAt time.Time
	// storedCheckRetryAt is when stored copies may be read again after the database failed.
	storedCheckRetryAt time.Time
	// known records what this replica found when it last read each stored shared body.
	known map[[auditBodyHashLen]byte]auditBodyKnown
}

// auditBodyReference is the decoded content of a reference field: which shared body holds the cut
// result, and the rest of the original body to splice it back into.
type auditBodyReference struct {
	hash     []byte
	offset   int
	envelope []byte
}

// namedAuditBodyField is a body field with the name reported when it cannot be read.
type namedAuditBodyField struct {
	name  string
	field *json.RawMessage
}

// pendingAuditBodyReference is a reference field waiting for its shared body to be loaded.
type pendingAuditBodyReference struct {
	log       *types.MCPAuditLog
	name      string
	field     *json.RawMessage
	reference auditBodyReference
	// encrypted records that the reference itself was stored encrypted, in which case the body it
	// points to must be too.
	encrypted bool
}

// unavailableAuditBodies tallies the bodies that could not be read during one read of audit rows,
// grouped by what went wrong.
type unavailableAuditBodies struct {
	count  int
	causes []*unavailableAuditBodyCause
}

// unavailableAuditBodyCause is one reason bodies were unreadable, with the first body it affected.
type unavailableAuditBodyCause struct {
	message      string
	count        int
	firstLogID   uint
	firstPayload string
}

// auditBodyKnown is what a replica found when it last read a stored shared body.
type auditBodyKnown struct {
	at time.Time
	// unusable says the stored copy cannot be read by this replica, or is not protected the way
	// this replica protects bodies, so new rows must not reference it.
	unusable bool
}

// auditBodyStored is what a writer found when it looked for a shared body it is about to reference.
type auditBodyStored int

// auditBodyWrite is a shared body to store alongside the rows that reference it.
type auditBodyWrite struct {
	body types.MCPAuditLogBody
	// replaceIfStored, when set, is the stored copy the writer inspected and decided to replace.
	// The stored body is overwritten only if it is still exactly this.
	replaceIfStored []byte
}

func isEncodedAuditBody(field []byte) bool {
	return len(field) > 0 && field[0] == auditBodyMarker
}

// encodeAuditBody returns plain in the audit body field format: compressed when that makes it
// smaller, then encrypted when a transformer is configured.
func encodeAuditBody(ctx context.Context, transformer value.Transformer, dataCtx value.Context, plain []byte, flags byte) ([]byte, error) {
	payload := plain
	if len(plain) >= auditBodyCompressMinBytes && len(plain) <= auditBodyCompressMaxBytes {
		if compressed := auditBodyEncoder().EncodeAll(plain, make([]byte, 0, len(plain)/4)); len(compressed) < len(plain) {
			payload = compressed
			flags |= auditBodyFlagCompressed
		}
	}
	if transformer != nil {
		flags |= auditBodyFlagEncrypted
		encrypted, err := transformer.TransformToStorage(ctx, payload, auditBodyHeaderCtx(dataCtx, flags))
		if err != nil {
			return nil, err
		}
		payload = encrypted
	}

	field := make([]byte, 0, auditBodyHeaderLen+len(payload))
	field = append(field, auditBodyMarker, auditBodyVersion, flags)
	return append(field, payload...), nil
}

// auditBodyHeaderCtx extends a field's encryption context with its header, so that a flag changed
// in the database no longer matches the ciphertext it sits in front of. Without this a reference
// could be relabelled as an inline body, or a compressed payload as an uncompressed one, and
// still decrypt. It only has that effect with a provider that authenticates the context, and it
// does nothing about a field relabelled as not encrypted, which is not decrypted at all.
//
// The version used is the one this code writes and accepts. A second format version must pass
// the field's own version byte instead.
func auditBodyHeaderCtx(dataCtx value.Context, flags byte) value.Context {
	return value.DefaultContext(fmt.Appendf(bytes.Clone(dataCtx.AuthenticatedData()), "#%d.%d", auditBodyVersion, flags))
}

// decodeAuditBody reverses encodeAuditBody and returns the field's flags so the caller can tell a
// reference from an inline body.
func decodeAuditBody(ctx context.Context, transformer value.Transformer, dataCtx value.Context, field []byte) ([]byte, byte, error) {
	plain, flags, _, err := decodeAuditBodyWithStale(ctx, transformer, dataCtx, field)
	return plain, flags, err
}

// decodeAuditBodyWithStale is decodeAuditBody that also reports whether the transformer considers
// the ciphertext stale, meaning it was not written under the current key.
func decodeAuditBodyWithStale(ctx context.Context, transformer value.Transformer, dataCtx value.Context, field []byte) ([]byte, byte, bool, error) {
	if len(field) < auditBodyHeaderLen || field[0] != auditBodyMarker {
		return nil, 0, false, errors.New("audit body has no format header")
	}
	if field[1] != auditBodyVersion {
		return nil, 0, false, fmt.Errorf("unsupported audit body format version %d", field[1])
	}
	flags := field[2]
	if flags&^auditBodyKnownFlags != 0 {
		return nil, 0, false, fmt.Errorf("unsupported audit body flags %#x", flags)
	}

	var stale bool
	payload := field[auditBodyHeaderLen:]
	if flags&auditBodyFlagEncrypted != 0 {
		if transformer == nil {
			return nil, 0, false, errors.New("audit body is encrypted but encryption is not configured")
		}
		decrypted, isStale, err := transformer.TransformFromStorage(ctx, payload, auditBodyHeaderCtx(dataCtx, flags))
		if err != nil {
			return nil, 0, false, fmt.Errorf("failed to decrypt audit body: %w", err)
		}
		payload, stale = decrypted, isStale
	}
	if flags&auditBodyFlagCompressed != 0 {
		decompressed, err := auditBodyDecoder().DecodeAll(payload, nil)
		if err != nil {
			return nil, 0, false, fmt.Errorf("failed to decompress audit body: %w", err)
		}
		if len(decompressed) > auditBodyCompressMaxBytes {
			return nil, 0, false, errors.New("decompressed audit body exceeds the size limit")
		}
		payload = decompressed
	}
	return payload, flags, stale, nil
}

// splitJSONRPCResult locates the bytes of the top-level "result" value of a JSON-RPC success
// response. It reports false for anything else, including a body that is not exactly one object or
// that names "result" more than once.
func splitJSONRPCResult(body []byte) (start, end int, ok bool) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return 0, 0, false
	}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return 0, 0, false
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return 0, 0, false
		}
		switch key {
		case "result":
			if ok {
				return 0, 0, false
			}
			end = int(decoder.InputOffset())
			start = end - len(raw)
			if start < 0 || !bytes.Equal(body[start:end], raw) {
				return 0, 0, false
			}
			ok = true
		case "method", "error":
			return 0, 0, false
		}
	}
	if _, err := decoder.Token(); err != nil {
		return 0, 0, false
	}
	if _, err := decoder.Token(); err != io.EOF {
		return 0, 0, false
	}
	return start, end, ok
}

func encodeAuditBodyReference(hash []byte, offset int, envelope []byte) []byte {
	record := make([]byte, 0, len(hash)+binary.MaxVarintLen64+len(envelope))
	record = append(record, hash...)
	record = binary.AppendUvarint(record, uint64(offset))
	return append(record, envelope...)
}

func decodeAuditBodyReference(record []byte) (auditBodyReference, error) {
	if len(record) < auditBodyHashLen+1 {
		return auditBodyReference{}, errors.New("audit body reference is truncated")
	}
	offset, n := binary.Uvarint(record[auditBodyHashLen:])
	if n <= 0 {
		return auditBodyReference{}, errors.New("audit body reference has an invalid offset")
	}
	envelope := record[auditBodyHashLen+n:]
	if offset > uint64(len(envelope)) {
		return auditBodyReference{}, errors.New("audit body reference offset is out of range")
	}
	return auditBodyReference{
		hash:     record[:auditBodyHashLen],
		offset:   int(offset),
		envelope: envelope,
	}, nil
}

// splice rebuilds the original body by putting the shared result back where it was cut out.
func (r auditBodyReference) splice(result []byte) []byte {
	body := make([]byte, 0, len(r.envelope)+len(result))
	body = append(body, r.envelope[:r.offset]...)
	body = append(body, result...)
	return append(body, r.envelope[r.offset:]...)
}

// auditBodyDataCtx binds a shared body's ciphertext to the hash that audit rows reference it by.
// A shared body belongs to no single MCP server or user, so it cannot use a row's context.
func auditBodyDataCtx(hash []byte) value.Context {
	return value.DefaultContext(fmt.Sprintf("%s/bodies/%s", mcpAuditLogGroupResource.String(), hex.EncodeToString(hash)))
}

func (c *Client) mcpAuditLogTransformer() value.Transformer {
	if c.encryptionConfig == nil {
		return nil
	}
	return c.encryptionConfig.Transformers[mcpAuditLogGroupResource]
}

// auditBodyHashKey returns the install-wide key for hashing shared bodies, creating it on first
// use and caching it until restart. Keying the hash keeps someone with database access from
// confirming a guessed body. Only writes need the key: rows reference bodies by the stored hash.
//
// If the key cannot be loaded, list responses are stored inline and loading is retried after
// auditBodyHashKeyRetryAfter. A stored key that cannot be read is never replaced.
func (c *Client) auditBodyHashKey(ctx context.Context) ([]byte, error) {
	c.auditBodies.lock.Lock()
	defer c.auditBodies.lock.Unlock()

	if c.auditBodies.hashKey != nil {
		return c.auditBodies.hashKey, nil
	}
	now := time.Now()
	if now.Before(c.auditBodies.hashKeyRetryAt) {
		return nil, errors.New("audit body hash key is unavailable")
	}

	key, err := c.loadAuditBodyHashKey(ctx)
	if err != nil {
		c.auditBodies.hashKeyRetryAt = now.Add(auditBodyHashKeyRetryAfter)
		slog.Warn("MCP list responses will be stored inline until the audit body hash key can be loaded", "error", err)
		return nil, fmt.Errorf("failed to load audit body hash key: %w", err)
	}
	c.auditBodies.hashKey = key
	return key, nil
}

func (c *Client) loadAuditBodyHashKey(ctx context.Context) ([]byte, error) {
	var stored types.Property
	err := c.db.WithContext(ctx).Where("key = ?", auditBodyHashKeyProperty).First(&stored).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return c.createAuditBodyHashKey(ctx)
	}
	if err != nil {
		return nil, err
	}

	key, stale, err := c.openAuditBodyHashKey(ctx, stored)
	if err != nil {
		// Not a reason to replace the key. The failure may be passing, or this replica may hold
		// the wrong encryption key, and other replicas are using the stored one.
		return nil, err
	}
	if c.propertyTransformer() == nil || (stored.Encrypted && !stale) {
		// Either nothing this replica could store would be better protected than what is there,
		// or it is already stored as this replica would store it.
		return key, nil
	}

	// Store the key again under the current encryption key. The update only applies while the
	// stored value is the one read above, so replicas doing this at once all end up reading the
	// one that won.
	if !stored.Encrypted {
		// A key stored unencrypted was readable in the database and may have been copied, so it
		// is replaced rather than reused. This goes by the stored flag: with a configuration
		// that also accepts unencrypted data, a value flagged encrypted is taken to be.
		key = make([]byte, auditBodyHashLen)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
	}
	replacement := types.Property{Key: auditBodyHashKeyProperty, Value: base64.StdEncoding.EncodeToString(key)}
	if err := c.encryptProperty(ctx, &replacement); err != nil {
		return nil, err
	}
	if err := c.db.WithContext(ctx).Model(&types.Property{}).
		Where("key = ? AND value = ?", auditBodyHashKeyProperty, stored.Value).
		Updates(map[string]any{"value": replacement.Value, "encrypted": replacement.Encrypted, "updated_at": time.Now()}).Error; err != nil {
		return nil, err
	}
	if err := c.db.WithContext(ctx).Where("key = ?", auditBodyHashKeyProperty).First(&stored).Error; err != nil {
		return nil, err
	}
	key, _, err = c.openAuditBodyHashKey(ctx, stored)
	return key, err
}

// createAuditBodyHashKey stores a new hash key, or returns the one another replica stored first.
func (c *Client) createAuditBodyHashKey(ctx context.Context) ([]byte, error) {
	generated := make([]byte, auditBodyHashLen)
	if _, err := rand.Read(generated); err != nil {
		return nil, err
	}

	// Another replica can create the key between this one's lookup and insert, which fails the
	// insert on the primary key. The retry then reads the key that replica stored.
	var (
		property types.Property
		err      error
	)
	for range 2 {
		property, err = c.GetOrCreateProperty(ctx, auditBodyHashKeyProperty, base64.StdEncoding.EncodeToString(generated))
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	return decodeAuditBodyHashKey(property.Value)
}

// openAuditBodyHashKey returns the hash key held in a stored property, and whether it is
// encrypted under a key that is no longer the current one.
func (c *Client) openAuditBodyHashKey(ctx context.Context, stored types.Property) ([]byte, bool, error) {
	if !stored.Encrypted {
		key, err := decodeAuditBodyHashKey(stored.Value)
		return key, false, err
	}
	transformer := c.propertyTransformer()
	if transformer == nil {
		return nil, false, errors.New("stored key is encrypted and this server has no encryption configured")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(stored.Value)
	if err != nil {
		return nil, false, err
	}
	plain, stale, err := transformer.TransformFromStorage(ctx, ciphertext, propertyDataCtx(&stored))
	if err != nil {
		return nil, false, err
	}
	key, err := decodeAuditBodyHashKey(string(plain))
	return key, stale, err
}

func decodeAuditBodyHashKey(encoded string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	if len(key) != auditBodyHashLen {
		return nil, errors.New("stored key has the wrong length")
	}
	return key, nil
}

func (c *Client) propertyTransformer() value.Transformer {
	if c.encryptionConfig == nil {
		return nil
	}
	return c.encryptionConfig.Transformers[propertyGroupResource]
}

// auditBodyStoredState reports whether a row may reference the shared body stored under hash. A
// stored copy is usable only if this writer can read it, it is a body rather than a reference,
// and it is encrypted exactly when this writer encrypts. Referencing an unusable copy would keep
// retention from removing it and make every later row with that result unreadable.
//
// A usable answer is remembered for auditBodyKnownFreshFor and an unusable one for
// auditBodyUnusableFor. If the lookup fails, the answer is unusable and lookups are skipped for
// auditBodyStoredCheckRetryAfter.
func (c *Client) auditBodyStoredState(ctx context.Context, transformer value.Transformer, hash []byte, now time.Time) auditBodyStored {
	id := [auditBodyHashLen]byte(hash)

	c.auditBodies.lock.Lock()
	known, ok := c.auditBodies.known[id]
	backingOff := now.Before(c.auditBodies.storedCheckRetryAt)
	c.auditBodies.lock.Unlock()
	if ok {
		if known.unusable && now.Sub(known.at) < auditBodyUnusableFor {
			return auditBodyStoredUnusable
		}
		if !known.unusable && now.Sub(known.at) < auditBodyKnownFreshFor {
			return auditBodyStoredUsable
		}
	}
	if backingOff {
		return auditBodyStoredUnusable
	}

	var stored types.MCPAuditLogBody
	err := c.db.WithContext(ctx).Select("body").Where("hash = ?", hash).First(&stored).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return auditBodyStoredAbsent
	}
	if err != nil {
		// Unknown, so do not rely on it for this entry.
		c.auditBodies.lock.Lock()
		c.auditBodies.storedCheckRetryAt = now.Add(auditBodyStoredCheckRetryAfter)
		c.auditBodies.lock.Unlock()
		return auditBodyStoredUnusable
	}

	_, flags, err := decodeAuditBody(ctx, transformer, auditBodyDataCtx(hash), stored.Body)
	if err == nil && (flags&auditBodyFlagReference != 0 || (flags&auditBodyFlagEncrypted != 0) != (transformer != nil)) {
		err = errors.New("stored copy is not protected the way this server protects bodies")
	}
	if err != nil && ctx.Err() != nil {
		// The check was cut short, which says nothing about the copy.
		return auditBodyStoredUnusable
	}
	c.rememberAuditBody(hash, now, err != nil)
	if err != nil {
		slog.Warn("A shared MCP audit log body cannot be used by this server; storing matching list responses inline",
			"hash", hex.EncodeToString(hash), "error", err)
		return auditBodyStoredUnusable
	}
	return auditBodyStoredUsable
}

// rememberAuditBody records what this replica found when it read the copy stored under hash.
// Everything remembered is dropped once auditBodyKnownMax copies have been, which keeps the
// memory bounded and only costs reading those copies again.
func (c *Client) rememberAuditBody(hash []byte, now time.Time, unusable bool) {
	c.auditBodies.lock.Lock()
	defer c.auditBodies.lock.Unlock()

	if c.auditBodies.known == nil || len(c.auditBodies.known) >= auditBodyKnownMax {
		c.auditBodies.known = make(map[[auditBodyHashLen]byte]auditBodyKnown)
	}
	c.auditBodies.known[[auditBodyHashLen]byte(hash)] = auditBodyKnown{at: now, unusable: unusable}
}

// auditBodyHash identifies a shared body by its content and by whether the writer encrypts.
// Writers that encrypt and writers that do not therefore never share a stored body: enabling
// encryption stores a new, encrypted copy, and a replica running without the encryption
// configuration never writes to or references a copy that is encrypted.
func auditBodyHash(key []byte, encrypted bool, result []byte) []byte {
	mode := auditBodyHashModePlain
	if encrypted {
		mode = auditBodyHashModeEncrypted
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte{mode})
	mac.Write(result)
	return mac.Sum(nil)
}

// encodeAuditBodyField replaces a body field with its stored form. It must be given the captured
// body, exactly once: it does not recognize a field that is already encoded, so that no captured
// content can pass itself off as one.
func encodeAuditBodyField(ctx context.Context, transformer value.Transformer, dataCtx value.Context, field *json.RawMessage) error {
	if len(*field) == 0 {
		return nil
	}
	encoded, err := encodeAuditBody(ctx, transformer, dataCtx, *field, 0)
	if err != nil {
		return err
	}
	*field = encoded
	return nil
}

// encodeAuditListResponseField stores a list-method response as a reference to a shared body when
// it is a JSON-RPC success response with a large enough result. Anything else is stored inline.
func (c *Client) encodeAuditListResponseField(ctx context.Context, transformer value.Transformer, dataCtx value.Context, log *types.MCPAuditLog, field *json.RawMessage) error {
	body := []byte(*field)
	if len(body) == 0 {
		return nil
	}
	start, end, ok := splitJSONRPCResult(body)
	if !ok || end-start < auditBodyDedupMinBytes {
		return encodeAuditBodyField(ctx, transformer, dataCtx, field)
	}
	key, err := c.auditBodyHashKey(ctx)
	if err != nil {
		// Sharing is an optimization; the audit entry must still be recorded. auditBodyHashKey
		// has already logged why.
		return encodeAuditBodyField(ctx, transformer, dataCtx, field)
	}

	result := body[start:end]
	hash := auditBodyHash(key, transformer != nil, result)

	pending := types.PendingMCPAuditLogBody{Hash: hash}
	switch c.auditBodyStoredState(ctx, transformer, hash, time.Now()) {
	case auditBodyStoredUnusable:
		return encodeAuditBodyField(ctx, transformer, dataCtx, field)
	case auditBodyStoredUsable:
		// Kept in case the stored copy is gone by the time this entry is written.
		pending.Plain = result
	case auditBodyStoredAbsent:
		if pending.Encoded, err = encodeAuditBody(ctx, transformer, auditBodyDataCtx(hash), result, 0); err != nil {
			return err
		}
	}

	envelope := make([]byte, 0, len(body)-len(result))
	envelope = append(envelope, body[:start]...)
	envelope = append(envelope, body[end:]...)
	encoded, err := encodeAuditBody(ctx, transformer, dataCtx, encodeAuditBodyReference(hash, start, envelope), auditBodyFlagReference)
	if err != nil {
		return err
	}

	*field = encoded
	log.PendingBodies = append(log.PendingBodies, pending)
	return nil
}

// encodeMCPAuditLogBodies replaces every body field of log with its stored form.
func (c *Client) encodeMCPAuditLogBodies(ctx context.Context, transformer value.Transformer, dataCtx value.Context, log *types.MCPAuditLog) error {
	if mcp := log.MCP(); mcp != nil {
		for _, field := range []*json.RawMessage{&mcp.RequestBody, &mcp.MutatedRequestBody} {
			if err := encodeAuditBodyField(ctx, transformer, dataCtx, field); err != nil {
				return err
			}
		}

		method := mcp.CallType
		if method == mcpResponseCallType {
			method = mcp.ResponseMethod
		}
		for _, field := range []*json.RawMessage{&mcp.ResponseBody, &mcp.OriginalResponseBody} {
			var err error
			if auditBodyDedupMethods[method] {
				err = c.encodeAuditListResponseField(ctx, transformer, dataCtx, log, field)
			} else {
				err = encodeAuditBodyField(ctx, transformer, dataCtx, field)
			}
			if err != nil {
				return err
			}
		}
	}

	if local := log.LocalAgentToolCallFields; local != nil {
		for _, field := range []*json.RawMessage{&local.RequestBody, &local.ResponseBody, &local.RawEvent} {
			if err := encodeAuditBodyField(ctx, transformer, dataCtx, field); err != nil {
				return err
			}
		}
	}
	return nil
}

// auditBodyFields lists the body fields of log with the names reported when one cannot be read.
func auditBodyFields(log *types.MCPAuditLog) []namedAuditBodyField {
	var fields []namedAuditBodyField
	if mcp := log.MCP(); mcp != nil {
		fields = append(fields,
			namedAuditBodyField{name: "request.body", field: &mcp.RequestBody},
			namedAuditBodyField{name: "request.mutatedBody", field: &mcp.MutatedRequestBody},
			namedAuditBodyField{name: "response.body", field: &mcp.ResponseBody},
			namedAuditBodyField{name: "response.originalBody", field: &mcp.OriginalResponseBody},
		)
	}
	if local := log.LocalAgentToolCallFields; local != nil {
		fields = append(fields,
			namedAuditBodyField{name: "request.body", field: &local.RequestBody},
			namedAuditBodyField{name: "response.body", field: &local.ResponseBody},
			namedAuditBodyField{name: "rawEvent", field: &local.RawEvent},
		)
	}
	return fields
}

// mark drops a body that is stored but cannot be read, and records that on the row so one bad
// body does not fail a whole page or export.
func (u *unavailableAuditBodies) mark(log *types.MCPAuditLog, name string, field *json.RawMessage, err error) {
	*field = nil
	log.UnavailablePayloads = append(log.UnavailablePayloads, name)
	u.count++

	message := err.Error()
	for _, cause := range u.causes {
		if cause.message == message {
			cause.count++
			return
		}
	}
	if len(u.causes) == unavailableAuditBodyCauseLimit {
		// Keep the entry bounded. The total still counts these, and report lists them as other
		// causes.
		return
	}
	u.causes = append(u.causes, &unavailableAuditBodyCause{message: message, count: 1, firstLogID: log.ID, firstPayload: name})
}

// report logs the unreadable bodies of one read as a single entry, with a count and an example
// for each distinct cause. A bad key makes every body on a page unreadable, and a line per body
// would bury the log.
func (u *unavailableAuditBodies) report() {
	if u.count == 0 {
		return
	}
	causes := make([]string, 0, len(u.causes)+1)
	listed := 0
	for _, cause := range u.causes {
		causes = append(causes, fmt.Sprintf("%d: %s (first: %s of audit log %d)", cause.count, cause.message, cause.firstPayload, cause.firstLogID))
		listed += cause.count
	}
	if listed < u.count {
		causes = append(causes, fmt.Sprintf("%d: other causes", u.count-listed))
	}
	slog.Error("MCP audit log bodies are unavailable", "count", u.count, "causes", causes)
}

// decodeMCPAuditLogBodies restores the body fields of log. Fields in the legacy format are
// decrypted as before and their errors returned. Fields in the current format that cannot be read
// are dropped and reported on the row. References to shared bodies are returned for the caller to
// resolve in one batch.
func decodeMCPAuditLogBodies(ctx context.Context, transformer value.Transformer, dataCtx value.Context, log *types.MCPAuditLog, unavailable *unavailableAuditBodies) ([]pendingAuditBodyReference, error) {
	var (
		references []pendingAuditBodyReference
		errs       []error
	)
	for _, f := range auditBodyFields(log) {
		if len(*f.field) == 0 {
			continue
		}
		if !isEncodedAuditBody(*f.field) {
			if log.Encrypted && transformer != nil {
				errs = append(errs, decryptRawMessageField(ctx, transformer, dataCtx, f.field))
			}
			continue
		}

		plain, flags, err := decodeAuditBody(ctx, transformer, dataCtx, *f.field)
		if err != nil {
			unavailable.mark(log, f.name, f.field, err)
			continue
		}
		if flags&auditBodyFlagReference == 0 {
			*f.field = plain
			continue
		}
		reference, err := decodeAuditBodyReference(plain)
		if err != nil {
			unavailable.mark(log, f.name, f.field, err)
			continue
		}
		references = append(references, pendingAuditBodyReference{
			log:       log,
			name:      f.name,
			field:     f.field,
			reference: reference,
			encrypted: flags&auditBodyFlagEncrypted != 0,
		})
	}
	return references, errors.Join(errs...)
}

// resolveAuditBodyReferences loads the shared bodies that references point to, each once, and
// rebuilds the original fields.
func (c *Client) resolveAuditBodyReferences(ctx context.Context, transformer value.Transformer, references []pendingAuditBodyReference, unavailable *unavailableAuditBodies) error {
	if len(references) == 0 {
		return nil
	}

	seen := make(map[string]bool, len(references))
	var hashes [][]byte
	for _, pending := range references {
		if key := string(pending.reference.hash); !seen[key] {
			seen[key] = true
			hashes = append(hashes, pending.reference.hash)
		}
	}

	stored := make(map[string][]byte, len(hashes))
	for batch := range slices.Chunk(hashes, auditBodyLookupBatchSize) {
		var bodies []types.MCPAuditLogBody
		if err := c.db.WithContext(ctx).Where("hash IN ?", batch).Find(&bodies).Error; err != nil {
			return fmt.Errorf("failed to load shared audit log bodies: %w", err)
		}
		for _, body := range bodies {
			stored[string(body.Hash)] = body.Body
		}
	}

	type decoded struct {
		result []byte
		flags  byte
		err    error
	}
	results := make(map[string]decoded, len(stored))
	for _, pending := range references {
		key := string(pending.reference.hash)
		result, ok := results[key]
		if !ok {
			if encoded, found := stored[key]; !found {
				result.err = errors.New("shared audit log body is missing")
			} else {
				result.result, result.flags, result.err = decodeAuditBody(ctx, transformer, auditBodyDataCtx(pending.reference.hash), encoded)
				if result.err == nil && result.flags&auditBodyFlagReference != 0 {
					result.err = errors.New("shared audit log body is itself a reference")
				}
			}
			results[key] = result
		}
		err := result.err
		if err == nil && pending.encrypted && result.flags&auditBodyFlagEncrypted == 0 {
			// Writers that encrypt only ever reference encrypted bodies, so this copy was put
			// there by something else and its content cannot be trusted.
			err = errors.New("shared audit log body is not encrypted but the row referencing it is")
		}
		if err != nil {
			unavailable.mark(pending.log, pending.name, pending.field, err)
			continue
		}
		*pending.field = pending.reference.splice(result.result)
	}
	return nil
}

// prepareAuditBodiesForStore returns the shared bodies that must be written with logs: those not
// stored yet and those whose LastReferencedAt needs refreshing. It runs before the transaction so
// that any encoding still required, and the check of stored copies, happen outside it.
func (c *Client) prepareAuditBodiesForStore(ctx context.Context, logs []types.MCPAuditLog, now time.Time) (toStore []auditBodyWrite, err error) {
	pendingByHash := make(map[string]types.PendingMCPAuditLogBody)
	for _, log := range logs {
		for _, pending := range log.PendingBodies {
			key := string(pending.Hash)
			if existing, ok := pendingByHash[key]; !ok || (existing.Encoded == nil && pending.Encoded != nil) {
				pendingByHash[key] = pending
			}
		}
	}
	if len(pendingByHash) == 0 {
		return nil, nil
	}

	referenced := make([][]byte, 0, len(pendingByHash))
	for _, pending := range pendingByHash {
		referenced = append(referenced, pending.Hash)
	}
	// A stable order keeps concurrent transactions that write the same bodies from deadlocking.
	slices.SortFunc(referenced, bytes.Compare)

	lastReferenced := make(map[string]time.Time, len(referenced))
	for batch := range slices.Chunk(referenced, auditBodyLookupBatchSize) {
		var existing []types.MCPAuditLogBody
		if err := c.db.WithContext(ctx).Select("hash", "last_referenced_at").Where("hash IN ?", batch).Find(&existing).Error; err != nil {
			return nil, fmt.Errorf("failed to look up shared audit log bodies: %w", err)
		}
		for _, body := range existing {
			lastReferenced[string(body.Hash)] = body.LastReferencedAt
		}
	}

	var due [][]byte
	for _, hash := range referenced {
		// A recently referenced body is at least auditBodyCleanupMargin away from deletion, far
		// longer than this write can take, so it needs neither a write nor a lock.
		if seen, ok := lastReferenced[string(hash)]; ok && now.Sub(seen) < auditBodyReferenceRefresh {
			continue
		}
		due = append(due, hash)
	}

	transformer := c.mcpAuditLogTransformer()
	outdated, err := c.outdatedAuditBodies(ctx, transformer, due)
	if err != nil {
		return nil, err
	}
	for _, hash := range due {
		pending := pendingByHash[string(hash)]
		if pending.Encoded == nil {
			if pending.Encoded, err = encodeAuditBody(ctx, transformer, auditBodyDataCtx(hash), pending.Plain, 0); err != nil {
				return nil, fmt.Errorf("failed to encode shared audit log body: %w", err)
			}
		}
		toStore = append(toStore, auditBodyWrite{
			body: types.MCPAuditLogBody{
				Hash:             hash,
				Body:             pending.Encoded,
				CreatedAt:        now,
				LastReferencedAt: now,
			},
			replaceIfStored: outdated[string(hash)],
		})
	}
	return toStore, nil
}

// outdatedAuditBodies returns, as currently stored, the bodies named by hashes that should be
// replaced with a freshly encoded copy: those this writer can decrypt and that its transformer
// reports as stale, meaning encrypted under a key that is no longer the current one. Checking
// this when a body's refresh is due moves a long-lived body off a rotated-out key while that
// key can still be read.
//
// Any other stored copy is left as it is. One this writer cannot read may be what another
// replica's rows depend on, and a replica holding the wrong key must not be able to overwrite
// what the others can read. Such a copy is remembered as unusable, so that auditBodyStoredState
// keeps later rows from referencing it.
func (c *Client) outdatedAuditBodies(ctx context.Context, transformer value.Transformer, hashes [][]byte) (map[string][]byte, error) {
	outdated := make(map[string][]byte)
	if transformer == nil {
		return outdated, nil
	}
	for batch := range slices.Chunk(hashes, auditBodyLookupBatchSize) {
		var stored []types.MCPAuditLogBody
		if err := c.db.WithContext(ctx).Select("hash", "body").Where("hash IN ?", batch).Find(&stored).Error; err != nil {
			return nil, fmt.Errorf("failed to load shared audit log bodies: %w", err)
		}
		for _, body := range stored {
			_, _, stale, err := decodeAuditBodyWithStale(ctx, transformer, auditBodyDataCtx(body.Hash), body.Body)
			switch {
			case err == nil && stale:
				outdated[string(body.Hash)] = body.Body
			case err != nil && ctx.Err() == nil:
				// The entries being written already reference it, but later ones should not.
				c.rememberAuditBody(body.Hash, time.Now(), true)
			}
		}
	}
	return outdated, nil
}

// storeAuditBodies inserts shared bodies, or refreshes LastReferencedAt on those that already
// exist. It must run in the transaction that writes the rows referencing them: taking the row lock
// here is what stops retention cleanup from deleting a body a new row is about to reference.
//
// A stored copy is replaced only where the write names the copy it inspected, and only if that is
// still what is stored. The inspection happened before the transaction, and another replica may
// have replaced the body since with one this writer has not judged.
//
// The writes must be in hash order, which keeps concurrent transactions from deadlocking.
func storeAuditBodies(tx *gorm.DB, writes []auditBodyWrite) error {
	for _, write := range writes {
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "hash"}},
			DoUpdates: clause.AssignmentColumns([]string{"last_referenced_at"}),
		}).Create(&write.body).Error; err != nil {
			return err
		}
		if write.replaceIfStored == nil {
			continue
		}
		if err := tx.Model(&types.MCPAuditLogBody{}).
			Where("hash = ? AND body = ?", write.body.Hash, write.replaceIfStored).
			Update("body", write.body.Body).Error; err != nil {
			return err
		}
	}
	return nil
}

// deleteUnreferencedMCPAuditLogBodies deletes shared bodies that no remaining audit row can
// reference. Every write of a referencing row leaves its bodies' LastReferencedAt within
// auditBodyReferenceRefresh of the write, and a row is normally written no earlier than its
// created_at. A row that survives retention was created after the cutoff, so its bodies were
// referenced no earlier than the cutoff minus that interval, which the margin keeps.
//
// This holds as long as every row that references a shared body is written through
// insertMCPAuditLogs, nothing else deletes shared bodies, and how far a row's created_at is ahead
// of its write plus the clock skew between replicas stays within
// auditBodyCleanupMargin - auditBodyReferenceRefresh. Rows with an old created_at, and rows
// deleted by other means, are harmless: their bodies are refreshed at write time and age out by
// timestamp.
func (c *Client) deleteUnreferencedMCPAuditLogBodies(ctx context.Context, now time.Time, retentionDays int) error {
	if retentionDays <= 0 {
		return nil
	}

	cutoff := now.Truncate(24*time.Hour).AddDate(0, 0, -retentionDays).Add(-auditBodyCleanupMargin)

	for {
		// The outer condition repeats the cutoff so that a body refreshed by a concurrent writer
		// after the subquery ran is re-checked, and skipped, once that writer commits.
		result := c.db.WithContext(ctx).Exec(
			"DELETE FROM mcp_audit_log_bodies WHERE last_referenced_at < ? AND hash IN (SELECT hash FROM mcp_audit_log_bodies WHERE last_referenced_at < ? LIMIT ?)",
			cutoff, cutoff, c.auditLogDeleteBatchSize,
		)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected < int64(c.auditLogDeleteBatchSize) {
			return nil
		}
	}
}
