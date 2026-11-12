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

	// auditBodyReferenceRefresh bounds how stale a shared body's LastReferencedAt may be when a row
	// referencing it is written.
	auditBodyReferenceRefresh = 24 * time.Hour
	// auditBodyCleanupMargin is how far past the audit-row retention cutoff a shared body's
	// LastReferencedAt must be before it is deleted: the refresh interval plus a day for clock
	// skew between replicas.
	auditBodyCleanupMargin = auditBodyReferenceRefresh + 24*time.Hour
	// auditBodyKnownFreshFor is how long a replica trusts its own record that a shared body is
	// stored, letting it skip compressing and encrypting the same result again.
	auditBodyKnownFreshFor = auditBodyReferenceRefresh / 2
	auditBodyKnownMax      = 4096

	auditBodyHashKeyProperty   = "mcp-audit-log-body-hash-key"
	auditBodyHashKeyRetryAfter = time.Minute
	auditBodyLookupBatchSize   = 200
	auditBodyStoreBatchSize    = 10

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
	// knownStored records when this replica last saw each shared body stored and recently referenced.
	knownStored map[[auditBodyHashLen]byte]time.Time
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
		encrypted, err := transformer.TransformToStorage(ctx, payload, dataCtx)
		if err != nil {
			return nil, err
		}
		payload = encrypted
		flags |= auditBodyFlagEncrypted
	}

	field := make([]byte, 0, auditBodyHeaderLen+len(payload))
	field = append(field, auditBodyMarker, auditBodyVersion, flags)
	return append(field, payload...), nil
}

// decodeAuditBody reverses encodeAuditBody and returns the field's flags so the caller can tell a
// reference from an inline body.
func decodeAuditBody(ctx context.Context, transformer value.Transformer, dataCtx value.Context, field []byte) ([]byte, byte, error) {
	if len(field) < auditBodyHeaderLen || field[0] != auditBodyMarker {
		return nil, 0, errors.New("audit body has no format header")
	}
	if field[1] != auditBodyVersion {
		return nil, 0, fmt.Errorf("unsupported audit body format version %d", field[1])
	}
	flags := field[2]
	if flags&^auditBodyKnownFlags != 0 {
		return nil, 0, fmt.Errorf("unsupported audit body flags %#x", flags)
	}

	payload := field[auditBodyHeaderLen:]
	if flags&auditBodyFlagEncrypted != 0 {
		if transformer == nil {
			return nil, 0, errors.New("audit body is encrypted but encryption is not configured")
		}
		decrypted, _, err := transformer.TransformFromStorage(ctx, payload, dataCtx)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to decrypt audit body: %w", err)
		}
		payload = decrypted
	}
	if flags&auditBodyFlagCompressed != 0 {
		decompressed, err := auditBodyDecoder().DecodeAll(payload, nil)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to decompress audit body: %w", err)
		}
		if len(decompressed) > auditBodyCompressMaxBytes {
			return nil, 0, errors.New("decompressed audit body exceeds the size limit")
		}
		payload = decompressed
	}
	return payload, flags, nil
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
// use. A keyed hash keeps someone with database access from confirming a guessed body. The key is
// only needed to find an existing copy when writing: rows reference bodies by the stored hash, so
// reads never depend on it.
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
		c.auditBodies.hashKeyRetryAt = now.Add(auditBodyHashKeyRetryAfter)
		return nil, fmt.Errorf("failed to load audit body hash key: %w", err)
	}

	key, err := base64.StdEncoding.DecodeString(property.Value)
	if err != nil || len(key) != auditBodyHashLen {
		c.auditBodies.hashKeyRetryAt = now.Add(auditBodyHashKeyRetryAfter)
		return nil, errors.New("stored audit body hash key is invalid")
	}
	c.auditBodies.hashKey = key
	return key, nil
}

func (c *Client) auditBodyKnownStored(hash []byte, now time.Time) bool {
	c.auditBodies.lock.Lock()
	defer c.auditBodies.lock.Unlock()

	seen, ok := c.auditBodies.knownStored[[auditBodyHashLen]byte(hash)]
	return ok && now.Sub(seen) < auditBodyKnownFreshFor
}

func (c *Client) rememberAuditBodiesStored(hashes [][]byte, now time.Time) {
	c.auditBodies.lock.Lock()
	defer c.auditBodies.lock.Unlock()

	if c.auditBodies.knownStored == nil || len(c.auditBodies.knownStored)+len(hashes) > auditBodyKnownMax {
		c.auditBodies.knownStored = make(map[[auditBodyHashLen]byte]time.Time, len(hashes))
	}
	for _, hash := range hashes {
		c.auditBodies.knownStored[[auditBodyHashLen]byte(hash)] = now
	}
}

// encodeAuditBodyField replaces a body field with its stored form.
func encodeAuditBodyField(ctx context.Context, transformer value.Transformer, dataCtx value.Context, field *json.RawMessage) error {
	if len(*field) == 0 || isEncodedAuditBody(*field) {
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
	if len(body) == 0 || isEncodedAuditBody(body) {
		return nil
	}
	start, end, ok := splitJSONRPCResult(body)
	if !ok || end-start < auditBodyDedupMinBytes {
		return encodeAuditBodyField(ctx, transformer, dataCtx, field)
	}
	key, err := c.auditBodyHashKey(ctx)
	if err != nil {
		// Sharing is an optimization; the audit entry must still be recorded.
		slog.Warn("Storing MCP list response inline", "error", err)
		return encodeAuditBodyField(ctx, transformer, dataCtx, field)
	}

	result := body[start:end]
	mac := hmac.New(sha256.New, key)
	mac.Write(result)
	hash := mac.Sum(nil)

	pending := types.PendingMCPAuditLogBody{Hash: hash}
	if c.auditBodyKnownStored(hash, time.Now()) {
		pending.Plain = result
	} else if pending.Encoded, err = encodeAuditBody(ctx, transformer, auditBodyDataCtx(hash), result, 0); err != nil {
		return err
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

// markAuditBodyUnavailable drops a body that is stored but cannot be read, and records that on the
// row so one bad body does not fail a whole page or export.
func markAuditBodyUnavailable(log *types.MCPAuditLog, name string, field *json.RawMessage, err error) {
	slog.Error("MCP audit log body is unavailable", "auditLogID", log.ID, "payload", name, "error", err)
	*field = nil
	log.UnavailablePayloads = append(log.UnavailablePayloads, name)
}

// decodeMCPAuditLogBodies restores the body fields of log. Fields in the legacy format are
// decrypted as before and their errors returned. Fields in the current format that cannot be read
// are dropped and reported on the row. References to shared bodies are returned for the caller to
// resolve in one batch.
func decodeMCPAuditLogBodies(ctx context.Context, transformer value.Transformer, dataCtx value.Context, log *types.MCPAuditLog) ([]pendingAuditBodyReference, error) {
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
			markAuditBodyUnavailable(log, f.name, f.field, err)
			continue
		}
		if flags&auditBodyFlagReference == 0 {
			*f.field = plain
			continue
		}
		reference, err := decodeAuditBodyReference(plain)
		if err != nil {
			markAuditBodyUnavailable(log, f.name, f.field, err)
			continue
		}
		references = append(references, pendingAuditBodyReference{log: log, name: f.name, field: f.field, reference: reference})
	}
	return references, errors.Join(errs...)
}

// resolveAuditBodyReferences loads the shared bodies that references point to, each once, and
// rebuilds the original fields.
func (c *Client) resolveAuditBodyReferences(ctx context.Context, transformer value.Transformer, references []pendingAuditBodyReference) error {
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
				var flags byte
				result.result, flags, result.err = decodeAuditBody(ctx, transformer, auditBodyDataCtx(pending.reference.hash), encoded)
				if result.err == nil && flags&auditBodyFlagReference != 0 {
					result.err = errors.New("shared audit log body is itself a reference")
				}
			}
			results[key] = result
		}
		if result.err != nil {
			markAuditBodyUnavailable(pending.log, pending.name, pending.field, result.err)
			continue
		}
		*pending.field = pending.reference.splice(result.result)
	}
	return nil
}

// prepareAuditBodiesForStore returns the shared bodies that must be written with logs: those not
// stored yet and those whose LastReferencedAt needs refreshing. It runs before the transaction so
// that any encoding still required happens outside it.
func (c *Client) prepareAuditBodiesForStore(ctx context.Context, logs []types.MCPAuditLog, now time.Time) (toStore []types.MCPAuditLogBody, referenced [][]byte, err error) {
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
		return nil, nil, nil
	}

	referenced = make([][]byte, 0, len(pendingByHash))
	for _, pending := range pendingByHash {
		referenced = append(referenced, pending.Hash)
	}
	// A stable order keeps concurrent transactions that write the same bodies from deadlocking.
	slices.SortFunc(referenced, bytes.Compare)

	lastReferenced := make(map[string]time.Time, len(referenced))
	for batch := range slices.Chunk(referenced, auditBodyLookupBatchSize) {
		var existing []types.MCPAuditLogBody
		if err := c.db.WithContext(ctx).Select("hash", "last_referenced_at").Where("hash IN ?", batch).Find(&existing).Error; err != nil {
			return nil, nil, fmt.Errorf("failed to look up shared audit log bodies: %w", err)
		}
		for _, body := range existing {
			lastReferenced[string(body.Hash)] = body.LastReferencedAt
		}
	}

	transformer := c.mcpAuditLogTransformer()
	for _, hash := range referenced {
		// A recently referenced body is at least auditBodyCleanupMargin away from deletion, far
		// longer than this write can take, so it needs neither a write nor a lock.
		if seen, ok := lastReferenced[string(hash)]; ok && now.Sub(seen) < auditBodyReferenceRefresh {
			continue
		}
		pending := pendingByHash[string(hash)]
		if pending.Encoded == nil {
			if pending.Encoded, err = encodeAuditBody(ctx, transformer, auditBodyDataCtx(hash), pending.Plain, 0); err != nil {
				return nil, nil, fmt.Errorf("failed to encode shared audit log body: %w", err)
			}
		}
		toStore = append(toStore, types.MCPAuditLogBody{
			Hash:             hash,
			Body:             pending.Encoded,
			CreatedAt:        now,
			LastReferencedAt: now,
		})
	}
	return toStore, referenced, nil
}

// storeAuditBodies inserts shared bodies, or refreshes LastReferencedAt on those that already
// exist. It must run in the transaction that writes the rows referencing them: taking the row lock
// here is what stops retention cleanup from deleting a body a new row is about to reference.
func storeAuditBodies(tx *gorm.DB, bodies []types.MCPAuditLogBody, now time.Time) error {
	if len(bodies) == 0 {
		return nil
	}
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "hash"}},
		DoUpdates: clause.Assignments(map[string]any{"last_referenced_at": now}),
	}).CreateInBatches(bodies, auditBodyStoreBatchSize).Error
}

// deleteUnreferencedMCPAuditLogBodies deletes shared bodies that no remaining audit row can
// reference. It relies on retention being the only thing that deletes audit rows, and on every row
// write leaving its bodies' LastReferencedAt within auditBodyReferenceRefresh of the write: a row
// that survives retention was created after the cutoff, so its bodies were referenced no earlier
// than the cutoff minus that interval.
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
