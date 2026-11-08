package provenance

import (
	"context"
	"testing"
)

const (
	// taggableContent is long enough to pass MinBytes and contains no structured
	// identifiers, so the deterministic Classify finds nothing in it.
	taggableContent = "The project team reviewed the renewal timeline and the roadmap for the next two quarters, " +
		"discussed the migration plan and the staffing changes, walked through the vendor negotiations, " +
		"and agreed on several follow up items for the group to track over the coming weeks."
)

// fakeContentClassifier is a ContentClassifier test double: it records how many
// times it was called and returns canned labels (raw, so the engine's filtering
// is exercised).
type fakeContentClassifier struct {
	labels Labels
	err    error
	calls  int
}

func (f *fakeContentClassifier) ClassifyContent(_ context.Context, _ ContentToClassify) (ContentClassification, error) {
	f.calls++
	if f.err != nil {
		return ContentClassification{}, f.err
	}
	return ContentClassification{Labels: f.labels, Rationale: "test", Model: "fake-tagger"}, nil
}

func setContentTagging(t *testing.T, e *Engine, cfg ContentTagging) {
	t.Helper()
	s, err := e.store.GetSettings(t.Context())
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	s.ContentTagging = cfg
	if err := e.store.PutSettings(t.Context(), s); err != nil {
		t.Fatalf("put settings: %v", err)
	}
	e.Invalidate()
}

// taggableIngest produces a reads-untrusted tool result with benign long
// content: the deterministic pass labels it only "untrusted" (a trust label, not
// a sensitivity one), so OnlyUnlabeled still allows the tagger to run.
func taggableIngest(content string) IngestRequest {
	return IngestRequest{
		Subject: subject, Session: key, Surface: SurfaceMCP, Kind: KindToolResult,
		Tool: "fetch", Server: "web", Capabilities: NewLabels(CapReadsUntrusted),
		Content: content,
	}
}

func TestShouldTagContent(t *testing.T) {
	cases := []struct {
		name   string
		cfg    ContentTagging
		in     IngestRequest
		found  Labels
		sample float64
		want   bool
	}{
		{
			name:   "disabled",
			cfg:    ContentTagging{Enabled: false, MinBytes: 200, OnlyUnlabeled: true},
			in:     IngestRequest{Content: taggableContent},
			sample: 0,
			want:   false,
		},
		{
			name:   "too short",
			cfg:    ContentTagging{Enabled: true, MinBytes: 200, OnlyUnlabeled: true},
			in:     IngestRequest{Content: "short"},
			sample: 0,
			want:   false,
		},
		{
			name:   "eligible unlabeled",
			cfg:    ContentTagging{Enabled: true, MinBytes: 200, OnlyUnlabeled: true},
			in:     IngestRequest{Content: taggableContent},
			found:  NewLabels(LabelUntrusted),
			sample: 0,
			want:   true,
		},
		{
			name:   "only unlabeled skips sensitivity",
			cfg:    ContentTagging{Enabled: true, MinBytes: 200, OnlyUnlabeled: true},
			in:     IngestRequest{Content: taggableContent},
			found:  NewLabels(LabelPII),
			sample: 0,
			want:   false,
		},
		{
			name:   "labeled allowed when OnlyUnlabeled off",
			cfg:    ContentTagging{Enabled: true, MinBytes: 200, OnlyUnlabeled: false},
			in:     IngestRequest{Content: taggableContent},
			found:  NewLabels(LabelPII),
			sample: 0,
			want:   true,
		},
		{
			name:   "sample at or above rate skips",
			cfg:    ContentTagging{Enabled: true, MinBytes: 200, SampleRate: 0.25},
			in:     IngestRequest{Content: taggableContent},
			sample: 0.5,
			want:   false,
		},
		{
			name:   "sample below rate runs",
			cfg:    ContentTagging{Enabled: true, MinBytes: 200, SampleRate: 0.75},
			in:     IngestRequest{Content: taggableContent},
			sample: 0.5,
			want:   true,
		},
		{
			name:   "capabilities must intersect",
			cfg:    ContentTagging{Enabled: true, MinBytes: 200, Capabilities: []Label{CapReadsPrivate}},
			in:     IngestRequest{Content: taggableContent, Capabilities: NewLabels(CapReadsUntrusted)},
			sample: 0,
			want:   false,
		},
		{
			name:   "capabilities intersect",
			cfg:    ContentTagging{Enabled: true, MinBytes: 200, Capabilities: []Label{CapReadsPrivate, CapReadsUntrusted}},
			in:     IngestRequest{Content: taggableContent, Capabilities: NewLabels(CapReadsUntrusted)},
			sample: 0,
			want:   true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldTagContent(tc.cfg, tc.in, tc.found, tc.sample); got != tc.want {
				t.Fatalf("shouldTagContent = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestContentTaggingDisabledNeverCalls(t *testing.T) {
	ctx := t.Context()
	e, _ := newTestEngine(t)
	cl := &fakeContentClassifier{labels: NewLabels(LabelCustomerData)}
	e.SetContentClassifier(cl)
	// ContentTagging defaults to disabled, so even with a classifier installed the
	// model is never consulted.
	if _, err := e.Ingest(ctx, taggableIngest(taggableContent)); err != nil {
		t.Fatal(err)
	}
	if cl.calls != 0 {
		t.Fatalf("disabled tagger must not be called, calls=%d", cl.calls)
	}
}

func TestContentTaggingSyncAddsLabelForNextDecision(t *testing.T) {
	ctx := t.Context()
	e, _ := newTestEngine(t)
	cl := &fakeContentClassifier{labels: NewLabels(LabelCustomerData)}
	e.SetContentClassifier(cl)
	setContentTagging(t, e, ContentTagging{Enabled: true, MinBytes: 200, OnlyUnlabeled: true, Mode: ContentTagSync, BudgetSeconds: 3})

	ev, err := e.Ingest(ctx, taggableIngest(taggableContent))
	if err != nil {
		t.Fatal(err)
	}
	if cl.calls != 1 {
		t.Fatalf("sync tagger should be called once, calls=%d", cl.calls)
	}
	if !ev.Labels.Has(LabelCustomerData) {
		t.Fatalf("sync should merge AI labels into the returned event, got %v", ev.Labels)
	}
	if !ev.Labels.Has(LabelUntrusted) {
		t.Fatalf("deterministic labels must be preserved (additive), got %v", ev.Labels)
	}

	s, err := e.Resolve(ctx, subject, key, SurfaceMCP)
	if err != nil {
		t.Fatal(err)
	}
	if !s.State.Labels.Has(LabelCustomerData) || !s.State.Labels.Has(LabelConfidential) {
		t.Fatalf("session taint should carry the AI label and its implied confidential, got %v", s.State.Labels)
	}

	// A following decision sees the AI label through the session taint.
	_, d, err := e.Decide(ctx, DecideRequest{Subject: subject, Session: key, Action: Action{
		Surface: SurfaceMCP, Kind: KindToolCall, Server: "slack", Tool: "post", Sink: Sink{External: true, Writes: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Labels.Has(LabelCustomerData) {
		t.Fatalf("a following decision should see the AI label via the session taint, got %v", d.Labels)
	}
}

func TestContentTaggingOnlyUnlabeledSkips(t *testing.T) {
	ctx := t.Context()
	e, _ := newTestEngine(t)
	cl := &fakeContentClassifier{labels: NewLabels(LabelCustomerData)}
	e.SetContentClassifier(cl)
	setContentTagging(t, e, ContentTagging{Enabled: true, MinBytes: 50, OnlyUnlabeled: true, Mode: ContentTagSync, BudgetSeconds: 3})

	// The content carries an email, so the deterministic pass already labels it
	// PII (a sensitivity label). OnlyUnlabeled must then skip the model.
	labeled := taggableIngest("Please contact ada@customer.example about the renewal. " + taggableContent)
	if _, err := e.Ingest(ctx, labeled); err != nil {
		t.Fatal(err)
	}
	if cl.calls != 0 {
		t.Fatalf("OnlyUnlabeled should skip content the regex already labelled, calls=%d", cl.calls)
	}
}

func TestContentTaggingCachesByContent(t *testing.T) {
	ctx := t.Context()
	e, _ := newTestEngine(t)
	cl := &fakeContentClassifier{labels: NewLabels(LabelCustomerData)}
	e.SetContentClassifier(cl)
	setContentTagging(t, e, ContentTagging{Enabled: true, MinBytes: 200, OnlyUnlabeled: true, Mode: ContentTagSync, BudgetSeconds: 3})

	if _, err := e.Ingest(ctx, taggableIngest(taggableContent)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Ingest(ctx, taggableIngest(taggableContent)); err != nil {
		t.Fatal(err)
	}
	if cl.calls != 1 {
		t.Fatalf("identical content should hit the cache and call the model once, calls=%d", cl.calls)
	}
}

func TestContentTaggingFiltersToContentLabels(t *testing.T) {
	ctx := t.Context()
	e, _ := newTestEngine(t)
	// The model returns a content label, a capability label and an unknown label.
	cl := &fakeContentClassifier{labels: Labels{LabelCustomerData, CapReadsPrivate, Label("bogus")}}
	e.SetContentClassifier(cl)
	setContentTagging(t, e, ContentTagging{Enabled: true, MinBytes: 200, OnlyUnlabeled: true, Mode: ContentTagSync, BudgetSeconds: 3})

	ev, err := e.Ingest(ctx, taggableIngest(taggableContent))
	if err != nil {
		t.Fatal(err)
	}
	if !ev.Labels.Has(LabelCustomerData) {
		t.Fatalf("content label should be added, got %v", ev.Labels)
	}
	if ev.Labels.Has(CapReadsPrivate) || ev.Labels.Has(Label("bogus")) {
		t.Fatalf("non-content labels must be dropped, got %v", ev.Labels)
	}
	if !ev.Labels.Has(LabelUntrusted) {
		t.Fatalf("deterministic label must remain (additive), got %v", ev.Labels)
	}
}

func TestContentTaggingAsyncUpdatesSessionTaint(t *testing.T) {
	ctx := t.Context()
	e, _ := newTestEngine(t)
	cl := &fakeContentClassifier{labels: NewLabels(LabelCustomerData)}
	e.SetContentClassifier(cl)
	setContentTagging(t, e, ContentTagging{Enabled: true, MinBytes: 200, OnlyUnlabeled: true, Mode: ContentTagAsync, BudgetSeconds: 3})

	in := taggableIngest(taggableContent)
	s, err := e.Resolve(ctx, in.Subject, in.Session, in.Surface)
	if err != nil {
		t.Fatal(err)
	}
	// Drive the async work synchronously, exactly as the engine's goroutine does.
	e.classifyContentNow(ctx, in, nil, s, NewLabels(LabelUntrusted), contentCacheKey(in.Content), "evt-async")
	if cl.calls != 1 {
		t.Fatalf("classifier should be called once, calls=%d", cl.calls)
	}
	got, err := e.Resolve(ctx, in.Subject, in.Session, in.Surface)
	if err != nil {
		t.Fatal(err)
	}
	if !got.State.Labels.Has(LabelCustomerData) {
		t.Fatalf("async tagging should land in the session taint, got %v", got.State.Labels)
	}
}

func TestContentTaggingBatchQueuesUntilFlush(t *testing.T) {
	ctx := t.Context()
	e, _ := newTestEngine(t)
	cl := &fakeContentClassifier{labels: NewLabels(LabelCustomerData)}
	e.SetContentClassifier(cl)
	setContentTagging(t, e, ContentTagging{Enabled: true, MinBytes: 200, OnlyUnlabeled: true, Mode: ContentTagBatch, BudgetSeconds: 3})

	in := taggableIngest(taggableContent)
	if _, err := e.Ingest(ctx, in); err != nil {
		t.Fatal(err)
	}
	if cl.calls != 0 {
		t.Fatalf("batch mode must not classify on ingest, calls=%d", cl.calls)
	}
	if status := e.ContentBatchStatus(); status.Queued != 1 {
		t.Fatalf("expected one queued item, got %+v", status)
	}

	if tagged := e.FlushContentBatch(ctx); tagged != 1 {
		t.Fatalf("expected one tagged item, got %d", tagged)
	}
	if cl.calls != 1 {
		t.Fatalf("flush should classify once, calls=%d", cl.calls)
	}
	if status := e.ContentBatchStatus(); status.Queued != 0 || status.LastRun.IsZero() || status.LastTagged != 1 {
		t.Fatalf("unexpected status after flush: %+v", status)
	}
	got, err := e.Resolve(ctx, in.Subject, in.Session, in.Surface)
	if err != nil {
		t.Fatal(err)
	}
	if !got.State.Labels.Has(LabelCustomerData) {
		t.Fatalf("batch tagging should land in the session taint, got %v", got.State.Labels)
	}
}

func TestContentTaggingBatchDropsOldestBeyondCap(t *testing.T) {
	e, _ := newTestEngine(t)
	cfg := ContentTagging{Mode: ContentTagBatch, BatchMaxItems: 2}
	for i := range 3 {
		e.queueContent(cfg, queuedContent{eventID: string(rune('a' + i))})
	}
	e.contentBatchLock.Lock()
	defer e.contentBatchLock.Unlock()
	if len(e.contentBatch) != 2 || e.contentBatch[0].eventID != "b" || e.contentBatch[1].eventID != "c" {
		t.Fatalf("expected the two newest items, got %+v", e.contentBatch)
	}
}

func TestContentTaggingValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ContentTagging
		wantErr bool
	}{
		{name: "defaults", cfg: ContentTagging{}},
		{name: "batch", cfg: ContentTagging{Mode: ContentTagBatch, BatchIntervalMinutes: 30, SampleRate: 0.25}},
		{name: "sample rate above one", cfg: ContentTagging{SampleRate: 1.5}, wantErr: true},
		{name: "unknown mode", cfg: ContentTagging{Mode: "hourly"}, wantErr: true},
		{name: "interval too long", cfg: ContentTagging{Mode: ContentTagBatch, BatchIntervalMinutes: 5000}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
