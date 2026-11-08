package provenance

import (
	"context"
	"math/rand/v2"
	"strings"
	"time"
)

const (
	// ContentTagAsync tags content in a goroutine after Ingest returns; the extra
	// labels reach the session taint for SUBSEQUENT decisions. It is the default.
	ContentTagAsync = "async"
	// ContentTagBatch queues content in memory and tags it every
	// ContentTagging.BatchIntervalMinutes.
	ContentTagBatch = "batch"
	// ContentTagSync tags content within the budget before Ingest returns, so the
	// extra labels are in the session taint for a decision in the same request.
	ContentTagSync = "sync"

	// contentCacheCap bounds the content-classification cache.
	contentCacheCap = 4096
	// defaultContentBudgetSeconds bounds a sync classification when the settings
	// name none.
	defaultContentBudgetSeconds = 3
	defaultContentBatchInterval = 15 * time.Minute
	defaultContentBatchMax      = 200
)

// ContentClassifier is the optional, additive AI tagger. It assigns extra
// sensitivity and trust labels to content entering a session, beyond the
// built-in deterministic Classify. It is never required: the deterministic pass
// always runs first and its labels are never removed; this can only ADD labels
// from ContentLabels.
type ContentClassifier interface {
	ClassifyContent(ctx context.Context, in ContentToClassify) (ContentClassification, error)
}

// ContentToClassify is one piece of content to tag. Text is sent to the model
// and is never stored.
type ContentToClassify struct {
	// Text is the content (sent to the model; never stored).
	Text string
	// Tool and Server name the tool that produced the content.
	Tool, Server string
	Surface      Surface
	// Capabilities are the producing tool's capability labels.
	Capabilities Labels
	// Found are the labels the deterministic classifier already found.
	Found Labels
}

// ContentClassification is what the tagger returned.
type ContentClassification struct {
	// Labels are the additional labels; filtered to ContentLabels, minus Found.
	Labels    Labels
	Rationale string
	Model     string
}

// queuedContent is content waiting for the next batch. It is held in memory only and
// dropped once tagged, as the asynchronous mode drops content once its call returns.
type queuedContent struct {
	in      IngestRequest
	s       *Session
	found   Labels
	key     uint64
	eventID string
}

// ContentBatchStatus reports batch mode's queue.
type ContentBatchStatus struct {
	Queued     int       `json:"queued"`
	LastRun    time.Time `json:"lastRun,omitzero"`
	LastTagged int       `json:"lastTagged"`
}

// shouldTagContent reports whether the optional AI tagger should run for an
// ingest. It is pure so the decision is testable; the random sample (0..1) is
// passed in. found is the labels the deterministic pass already produced.
func shouldTagContent(cfg ContentTagging, in IngestRequest, found Labels, sample float64) bool {
	if !cfg.Enabled {
		return false
	}
	if cfg.MinBytes > 0 && len(in.Content) < cfg.MinBytes {
		return false
	}
	rate := cfg.SampleRate
	if rate <= 0 {
		rate = 1
	}
	if rate < 1 && sample >= rate {
		return false
	}
	if cfg.OnlyUnlabeled && found.Confidential() {
		return false
	}
	if len(cfg.Capabilities) > 0 && !in.Capabilities.HasAny(cfg.Capabilities...) {
		return false
	}
	return true
}

// contentCacheKey hashes normalized content for the classification cache,
// reusing the fingerprint helpers so the content itself is never stored.
func contentCacheKey(content string) uint64 {
	return hashExact(strings.Join(normalizeWords(content), " "))
}

// contentLabelsOnly keeps only the labels that are in ContentLabels. It is the
// universal filter the cache stores; the per-ingest "minus Found" step is
// applied when the result is used.
func contentLabelsOnly(labels Labels) Labels {
	var out Labels
	for _, l := range labels {
		if ContentLabels.Has(l) {
			out = out.Add(l)
		}
	}
	return out
}

// filterContentLabels keeps only ContentLabels that are not already in found,
// then normalizes. The tagger is additive: it can never remove a deterministic
// label and can never add a non-content label.
func filterContentLabels(labels, found Labels) Labels {
	var out Labels
	for _, l := range labels {
		if ContentLabels.Has(l) && !found.Has(l) {
			out = out.Add(l)
		}
	}
	return out.Normalize()
}

func contentBudget(cfg ContentTagging) time.Duration {
	secs := cfg.BudgetSeconds
	if secs <= 0 {
		secs = defaultContentBudgetSeconds
	}
	return time.Duration(secs) * time.Second
}

func contentTagSummary(in IngestRequest) string {
	switch {
	case in.Tool != "":
		return "AI content tag: " + in.Tool + " result"
	case in.Server != "":
		return "AI content tag: " + in.Server + " result"
	case in.Host != "":
		return "AI content tag: content from " + in.Host
	}
	return "AI content tag"
}

func (e *Engine) contentCacheGet(key uint64) (ContentClassification, bool) {
	e.contentCacheLock.Lock()
	defer e.contentCacheLock.Unlock()
	c, ok := e.contentCache[key]
	return c, ok
}

func (e *Engine) contentCachePut(key uint64, c ContentClassification) {
	e.contentCacheLock.Lock()
	defer e.contentCacheLock.Unlock()
	if _, ok := e.contentCache[key]; ok {
		e.contentCache[key] = c
		return
	}
	if len(e.contentCache) >= contentCacheCap && len(e.contentCacheKeys) > 0 {
		oldest := e.contentCacheKeys[0]
		e.contentCacheKeys = e.contentCacheKeys[1:]
		delete(e.contentCache, oldest)
	}
	e.contentCache[key] = c
	e.contentCacheKeys = append(e.contentCacheKeys, key)
}

// tagContent runs the optional, additive AI content tagger after the
// deterministic labels for an ingest have been computed, applied and persisted.
// It never changes the deterministic result: it can only ADD labels from
// ContentLabels to the session taint (and, on the synchronous paths, to the
// returned event). Any tagger error is swallowed so it cannot affect the ingest.
//
// A cache hit is applied immediately on every mode, since it costs nothing. In
// sync mode the model is consulted within the budget before Ingest returns. In
// async mode (the default) the model is consulted in a goroutine and the extra
// labels land in the session taint for SUBSEQUENT decisions; the
// already-persisted event is not rewritten.
func (e *Engine) tagContent(ctx context.Context, in IngestRequest, ev *Event, s *Session, found Labels) {
	if e.ContentClassifier() == nil {
		return
	}
	cfg := e.Settings(ctx).ContentTagging
	if !cfg.Enabled {
		return
	}
	key := contentCacheKey(in.Content)
	if cached, ok := e.contentCacheGet(key); ok {
		e.applyContentTags(ctx, in, ev, s, cached, found, ev.ID)
		return
	}
	if !shouldTagContent(cfg, in, found, rand.Float64()) {
		return
	}
	if cfg.Mode == ContentTagSync {
		sctx, cancel := context.WithTimeout(ctx, contentBudget(cfg))
		defer cancel()
		e.classifyContentNow(sctx, in, ev, s, found, key, ev.ID)
		return
	}
	// Async (default): the returned event keeps the deterministic labels; the
	// extra labels reach the session taint for subsequent decisions.
	eventID := ev.ID
	if cfg.Mode == ContentTagBatch {
		e.queueContent(cfg, queuedContent{in: in, s: s, found: found, key: key, eventID: eventID})
		return
	}
	go func() {
		bg, cancel := context.WithTimeout(context.Background(), contentBudget(cfg))
		defer cancel()
		e.classifyContentNow(bg, in, nil, s, found, key, eventID)
	}()
}

// classifyContentNow calls the content classifier and applies the result. It is
// unexported so tests can drive the async work synchronously. ev is the event to
// also tag in memory on the synchronous paths; it is nil for async work, which
// must not touch the already-returned event.
func (e *Engine) classifyContentNow(ctx context.Context, in IngestRequest, ev *Event, s *Session, found Labels, key uint64, eventID string) {
	classifier := e.ContentClassifier()
	if classifier == nil {
		return
	}
	out, err := classifier.ClassifyContent(ctx, ContentToClassify{
		Text: in.Content, Tool: in.Tool, Server: in.Server, Surface: in.Surface,
		Capabilities: in.Capabilities, Found: found,
	})
	if err != nil {
		e.logger.Debug("provenance: content tagging failed", "tool", in.Tool, "server", in.Server, "error", err)
		return
	}
	out.Labels = contentLabelsOnly(out.Labels)
	e.contentCachePut(key, out)
	e.applyContentTags(ctx, in, ev, s, out, found, eventID)
}

// applyContentTags adds a classification's labels to the session taint (and the
// event, when one is given). It is additive and idempotent: labels already
// present are no-ops, and deterministic labels are never removed.
func (e *Engine) applyContentTags(ctx context.Context, in IngestRequest, ev *Event, s *Session, class ContentClassification, found Labels, eventID string) {
	add := filterContentLabels(class.Labels, found)
	if len(add) == 0 {
		return
	}
	origin := LabelOrigin{EventID: eventID, Summary: contentTagSummary(in), At: e.now()}
	e.lock.Lock()
	s.State.AddLabels(add, origin)
	s.State.Recompute()
	snapshot := *s
	e.lock.Unlock()
	if ev != nil {
		ev.Labels = ev.Labels.Add(add...)
	}
	if err := e.store.PutSession(ctx, &snapshot); err != nil {
		e.logger.Warn("provenance: persisting AI content tags", "error", err)
	}
}

func (e *Engine) queueContent(cfg ContentTagging, item queuedContent) {
	limit := cfg.BatchMaxItems
	if limit <= 0 {
		limit = defaultContentBatchMax
	}
	e.contentBatchLock.Lock()
	defer e.contentBatchLock.Unlock()
	e.contentBatch = append(e.contentBatch, item)
	if over := len(e.contentBatch) - limit; over > 0 {
		e.contentBatch = e.contentBatch[over:]
	}
}

// ContentBatchStatus reports how much content is waiting for the next batch.
func (e *Engine) ContentBatchStatus() ContentBatchStatus {
	e.contentBatchLock.Lock()
	defer e.contentBatchLock.Unlock()
	return ContentBatchStatus{Queued: len(e.contentBatch), LastRun: e.contentBatchRun, LastTagged: e.contentBatchTagged}
}

// RunContentBatches tags batch mode's queue every BatchIntervalMinutes until ctx ends.
func (e *Engine) RunContentBatches(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		cfg := e.Settings(ctx).ContentTagging
		interval := time.Duration(cfg.BatchIntervalMinutes) * time.Minute
		if interval <= 0 {
			interval = defaultContentBatchInterval
		}
		e.contentBatchLock.Lock()
		due := len(e.contentBatch) > 0 && e.now().Sub(e.contentBatchRun) >= interval
		e.contentBatchLock.Unlock()
		if due {
			e.FlushContentBatch(ctx)
		}
	}
}

// FlushContentBatch tags everything queued now and returns how many items gained labels.
func (e *Engine) FlushContentBatch(ctx context.Context) int {
	e.contentBatchLock.Lock()
	items := e.contentBatch
	e.contentBatch = nil
	e.contentBatchRun = e.now()
	e.contentBatchLock.Unlock()

	cfg := e.Settings(ctx).ContentTagging
	tagged := 0
	for _, item := range items {
		if ctx.Err() != nil {
			break
		}
		cached, ok := e.contentCacheGet(item.key)
		if !ok {
			cctx, cancel := context.WithTimeout(ctx, contentBudget(cfg))
			e.classifyContentNow(cctx, item.in, nil, item.s, item.found, item.key, item.eventID)
			cancel()
			cached, ok = e.contentCacheGet(item.key)
		} else {
			e.applyContentTags(ctx, item.in, nil, item.s, cached, item.found, item.eventID)
		}
		if ok && len(filterContentLabels(cached.Labels, item.found)) > 0 {
			tagged++
		}
	}
	e.contentBatchLock.Lock()
	e.contentBatchTagged = tagged
	e.contentBatchLock.Unlock()
	return tagged
}
