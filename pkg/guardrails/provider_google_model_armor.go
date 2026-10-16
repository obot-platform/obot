package guardrails

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/sync/errgroup"
)

const (
	modelArmorScope      = "https://www.googleapis.com/auth/cloud-platform"
	modelArmorMatchFound = "MATCH_FOUND"
	// modelArmorConcurrency bounds parallel calls; Model Armor checks one text per call.
	modelArmorConcurrency = 4
)

var (
	// modelArmorLocation matches a Google Cloud location, which the default endpoint's host is
	// built from.
	modelArmorLocation = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

	// modelArmorHost matches the hosts default credentials may be sent to (see isGoogleAPIsEndpoint).
	modelArmorHost = regexp.MustCompile(`^(modelarmor\.googleapis\.com|modelarmor\.[a-z0-9-]+\.rep\.googleapis\.com|[a-z0-9-]+\.p\.googleapis\.com)$`)
)

// modelArmorChecker calls Google Cloud Model Armor's sanitize endpoints.
// https://cloud.google.com/security-command-center/docs/sanitize-prompts-responses
type modelArmorChecker struct {
	base     *http.Client
	endpoint string
	template string
	saJSON   string

	lock   sync.Mutex
	client *http.Client
}

type modelArmorRequest struct {
	UserPromptData    *modelArmorData `json:"userPromptData,omitempty"`
	ModelResponseData *modelArmorData `json:"modelResponseData,omitempty"`
}

type modelArmorData struct {
	Text string `json:"text"`
}

type modelArmorResponse struct {
	SanitizationResult struct {
		FilterMatchState string                      `json:"filterMatchState"`
		InvocationResult string                      `json:"invocationResult"`
		FilterResults    map[string]modelArmorFilter `json:"filterResults"`
	} `json:"sanitizationResult"`
}

// modelArmorFilter is a FilterResult; exactly one field is set.
type modelArmorFilter struct {
	RAI *struct {
		MatchState           string `json:"matchState"`
		RAIFilterTypeResults map[string]struct {
			MatchState      string `json:"matchState"`
			ConfidenceLevel string `json:"confidenceLevel"`
		} `json:"raiFilterTypeResults"`
	} `json:"raiFilterResult"`
	SDP *struct {
		InspectResult *struct {
			MatchState string `json:"matchState"`
			Findings   []struct {
				InfoType   string `json:"infoType"`
				Likelihood string `json:"likelihood"`
				Location   struct {
					ByteRange      *modelArmorRange `json:"byteRange"`
					CodepointRange *modelArmorRange `json:"codepointRange"`
				} `json:"location"`
			} `json:"findings"`
		} `json:"inspectResult"`
		DeidentifyResult *struct {
			MatchState string `json:"matchState"`
			Data       *struct {
				Text string `json:"text"`
			} `json:"data"`
			InfoTypes []string `json:"infoTypes"`
		} `json:"deidentifyResult"`
	} `json:"sdpFilterResult"`
	PIAndJailbreak *struct {
		MatchState      string `json:"matchState"`
		ConfidenceLevel string `json:"confidenceLevel"`
	} `json:"piAndJailbreakFilterResult"`
	MaliciousURI *struct {
		MatchState               string            `json:"matchState"`
		MaliciousURIMatchedItems []json.RawMessage `json:"maliciousUriMatchedItems"`
	} `json:"maliciousUriFilterResult"`
	CSAM *struct {
		MatchState string `json:"matchState"`
	} `json:"csamFilterFilterResult"`
	VirusScan *struct {
		MatchState string `json:"matchState"`
	} `json:"virusScanFilterResult"`
}

// modelArmorRange is a range whose int64 bounds proto3 JSON encodes as strings.
type modelArmorRange struct {
	Start modelArmorInt `json:"start"`
	End   modelArmorInt `json:"end"`
}

type modelArmorInt int

func init() {
	Register(ProviderInfo{
		Type:        "google-model-armor",
		Label:       "Google Cloud Model Armor",
		Category:    "Integrations",
		Description: "Screens prompts and responses with a Google Cloud Model Armor template: responsible AI, prompt injection and jailbreak, malicious URL, CSAM and Sensitive Data Protection filters. Sensitive data is redacted when the template inspects or de-identifies it.",
		Redacts:     true,
		Fields: []Field{
			{Name: "project", Label: "Project ID", Type: FieldText, Required: true, Description: "Google Cloud project that holds the template."},
			{Name: "location", Label: "Location", Type: FieldText, Destination: true, Required: true, Description: "The template's location, such as us-central1."},
			{Name: "template", Label: "Template ID", Type: FieldText, Required: true},
			{Name: "serviceAccountJSON", Label: "Service account key (JSON)", Type: FieldTextarea, Sensitive: true, Description: "Leave empty to use application default credentials. The account needs the Model Armor User role."},
			{Name: "endpoint", Label: "Endpoint", Type: FieldText, Destination: true, Description: "Overrides the Model Armor endpoint. Defaults to https://modelarmor.<location>.rep.googleapis.com. Without a service account key, only Model Armor endpoints on googleapis.com are allowed."},
		},
	}, newModelArmorChecker)
}

// isGoogleAPIsEndpoint reports whether endpoint is an https URL on a Model Armor host:
// modelarmor.googleapis.com, a regional modelarmor.<location>.rep.googleapis.com, or a
// Private Service Connect endpoint (*.p.googleapis.com). Other googleapis.com hosts are
// refused too: some, such as Cloud Storage buckets, are named by their users.
func isGoogleAPIsEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return false
	}
	return modelArmorHost.MatchString(strings.TrimSuffix(strings.ToLower(u.Hostname()), "."))
}

func newModelArmorChecker(config map[string]any, secrets map[string]string, deps Deps) (Checker, error) {
	project := ConfigString(config, "project", "")
	location := ConfigString(config, "location", "")
	template := ConfigString(config, "template", "")
	if project == "" || location == "" || template == "" {
		return nil, fmt.Errorf("project, location and template are required")
	}
	if !modelArmorLocation.MatchString(location) {
		return nil, fmt.Errorf("location %q is not a Google Cloud location, such as us-central1", location)
	}
	saJSON := strings.TrimSpace(secrets["serviceAccountJSON"])
	if saJSON != "" && !json.Valid([]byte(saJSON)) {
		return nil, fmt.Errorf("the service account key is not valid JSON")
	}
	endpoint := strings.TrimRight(ConfigString(config, "endpoint", "https://modelarmor."+location+".rep.googleapis.com"), "/")
	if saJSON == "" && !isGoogleAPIsEndpoint(endpoint) {
		// Application default credentials are the Obot server's own Google Cloud identity,
		// which must only ever be sent to Google.
		return nil, fmt.Errorf("a custom endpoint outside googleapis.com requires a service account key; application default credentials are only sent to Model Armor's own endpoints")
	}
	return &modelArmorChecker{
		base:     providerHTTPClient(deps),
		endpoint: endpoint,
		template: fmt.Sprintf("projects/%s/locations/%s/templates/%s", url.PathEscape(project), url.PathEscape(location), url.PathEscape(template)),
		saJSON:   saJSON,
	}, nil
}

// httpClient returns a client that authenticates as the configured service account, or with
// application default credentials. Credentials are loaded on first use.
func (c *modelArmorChecker) httpClient(ctx context.Context) (*http.Client, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.client != nil {
		return c.client, nil
	}
	// Token requests use the base client; the context is kept for token refreshes.
	tokenCtx := context.WithValue(context.Background(), oauth2.HTTPClient, c.base)
	var (
		creds *google.Credentials
		err   error
	)
	if c.saJSON != "" {
		creds, err = google.CredentialsFromJSONWithType(tokenCtx, []byte(c.saJSON), google.ServiceAccount, modelArmorScope)
	} else {
		creds, err = google.FindDefaultCredentials(tokenCtx, modelArmorScope)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load Google Cloud credentials: %w", err)
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	c.client = &http.Client{
		Transport: &oauth2.Transport{Source: creds.TokenSource, Base: providerTransport(c.base)},
		Timeout:   c.base.Timeout,
	}
	return c.client, nil
}

func (c *modelArmorChecker) Check(ctx context.Context, in CheckInput) (Result, error) {
	indexes := checkableSegments(in.Segments)
	if len(indexes) == 0 {
		return Result{}, nil
	}
	client, err := c.httpClient(ctx)
	if err != nil {
		return Result{}, err
	}

	responses := make([]modelArmorResponse, len(indexes))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(modelArmorConcurrency)
	for n, i := range indexes {
		g.Go(func() error {
			return c.sanitize(gctx, client, in.Stage, in.Segments[i].Text, &responses[n])
		})
	}
	if err := g.Wait(); err != nil {
		return Result{}, err
	}

	var (
		fs           = newMergedFindings()
		replacements map[int]string
		// unlocated reports whether any finding can't be redacted, such as a prompt injection.
		// Replacements are then dropped, because the engine ignores findings without spans
		// when a checker returns replacements, and redacting policies must block instead.
		unlocated bool
	)
	for n, seg := range indexes {
		text, replaced, ok := modelArmorFindings(in.Segments, seg, responses[n], fs)
		unlocated = unlocated || !ok
		if replaced && text != in.Segments[seg].Text {
			if replacements == nil {
				replacements = map[int]string{}
			}
			replacements[seg] = text
		}
	}
	if unlocated {
		replacements = nil
	}
	return Result{Findings: fs.list(), Replacements: replacements}, nil
}

func (c *modelArmorChecker) sanitize(ctx context.Context, client *http.Client, stage, text string, out *modelArmorResponse) error {
	method := "sanitizeUserPrompt"
	body := modelArmorRequest{UserPromptData: &modelArmorData{Text: text}}
	if stage == StageOutput {
		method = "sanitizeModelResponse"
		body = modelArmorRequest{ModelResponseData: &modelArmorData{Text: text}}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/v1/%s:%s", c.endpoint, c.template, method), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if err := providerDo(client, req, "Google Cloud Model Armor", out); err != nil {
		return err
	}
	if out.SanitizationResult.InvocationResult == "FAILURE" {
		return fmt.Errorf("google Cloud Model Armor could not evaluate the content: invocation result FAILURE")
	}
	return nil
}

// modelArmorFindings adds a segment's findings to fs. It returns the de-identified text when
// the template's Sensitive Data Protection filter rewrote the content, and whether every
// finding can be redacted, by a span or by that text.
func modelArmorFindings(segments []Segment, seg int, resp modelArmorResponse, fs *mergedFindings) (replacement string, replaced, redactable bool) {
	sr := resp.SanitizationResult
	if sr.FilterMatchState != modelArmorMatchFound {
		return "", false, true
	}
	var (
		found bool
		// unlocated reports a finding with neither a span nor de-identified text.
		unlocated bool
	)
	keys := make([]string, 0, len(sr.FilterResults))
	for k := range sr.FilterResults {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	add := func(f Finding) {
		found = true
		fs.add(f)
	}
	addUnlocated := func(f Finding) {
		unlocated = true
		add(f)
	}
	for _, k := range keys {
		r := sr.FilterResults[k]
		switch {
		case r.RAI != nil && r.RAI.MatchState == modelArmorMatchFound:
			types := make([]string, 0, len(r.RAI.RAIFilterTypeResults))
			for t, tr := range r.RAI.RAIFilterTypeResults {
				if tr.MatchState == modelArmorMatchFound {
					types = append(types, t)
				}
			}
			slices.Sort(types)
			for _, t := range types {
				msg := strings.ReplaceAll(t, "_", " ") + " content"
				if cl := r.RAI.RAIFilterTypeResults[t].ConfidenceLevel; cl != "" {
					msg += " (" + modelArmorConfidence(cl) + " confidence)"
				}
				addUnlocated(Finding{Category: "responsible_ai", Message: msg, Count: 1})
			}
			if len(types) == 0 {
				addUnlocated(Finding{Category: "responsible_ai", Message: "Responsible AI filter matched", Count: 1})
			}
		case r.PIAndJailbreak != nil && r.PIAndJailbreak.MatchState == modelArmorMatchFound:
			msg := "Prompt injection or jailbreak attempt"
			if cl := r.PIAndJailbreak.ConfidenceLevel; cl != "" {
				msg += " (" + modelArmorConfidence(cl) + " confidence)"
			}
			addUnlocated(Finding{Category: "prompt_injection", Message: msg, Count: 1})
		case r.MaliciousURI != nil && r.MaliciousURI.MatchState == modelArmorMatchFound:
			addUnlocated(Finding{Category: "malicious_uri", Message: "Malicious URL detected", Count: max(1, len(r.MaliciousURI.MaliciousURIMatchedItems))})
		case r.CSAM != nil && r.CSAM.MatchState == modelArmorMatchFound:
			addUnlocated(Finding{Category: "csam", Message: "Child sexual abuse material detected", Count: 1})
		case r.VirusScan != nil && r.VirusScan.MatchState == modelArmorMatchFound:
			addUnlocated(Finding{Category: "malware", Message: "Malware detected", Count: 1})
		case r.SDP != nil && r.SDP.DeidentifyResult != nil && r.SDP.DeidentifyResult.MatchState == modelArmorMatchFound:
			d := r.SDP.DeidentifyResult
			for _, t := range d.InfoTypes {
				add(Finding{Category: "sensitive_data", Message: t + " detected", Count: 1})
			}
			if len(d.InfoTypes) == 0 {
				add(Finding{Category: "sensitive_data", Message: "Sensitive data detected", Count: 1})
			}
			if d.Data != nil {
				replacement, replaced = d.Data.Text, true
			} else {
				unlocated = true
			}
		case r.SDP != nil && r.SDP.InspectResult != nil && r.SDP.InspectResult.MatchState == modelArmorMatchFound:
			for _, sf := range r.SDP.InspectResult.Findings {
				f := Finding{Category: "sensitive_data", Message: sf.InfoType + " detected", Count: 1}
				if span, ok := modelArmorSpan(segments, seg, sf.Location.ByteRange, sf.Location.CodepointRange, sf.InfoType); ok {
					f.Spans = []Span{span}
				} else {
					unlocated = true
				}
				add(f)
			}
			if len(r.SDP.InspectResult.Findings) == 0 {
				addUnlocated(Finding{Category: "sensitive_data", Message: "Sensitive data detected", Count: 1})
			}
		}
	}
	if !found {
		// A match in a filter this checker doesn't know about.
		addUnlocated(Finding{Category: "model_armor", Message: "Model Armor filter matched", Count: 1})
	}
	return replacement, replaced, !unlocated
}

// modelArmorSpan locates an inspection finding, preferring the byte range and checking it
// against the code point range when both are present.
func modelArmorSpan(segments []Segment, seg int, byteRange, cpRange *modelArmorRange, label string) (Span, bool) {
	text := segments[seg].Text
	if cpRange != nil {
		if span, ok := codepointSpan(segments, seg, int(cpRange.Start), int(cpRange.End), "", label); ok {
			return span, true
		}
	}
	if byteRange != nil {
		start, end := int(byteRange.Start), int(byteRange.End)
		if start >= 0 && end > start && end <= len(text) {
			return Span{Segment: seg, Start: start, End: end, Label: label}, true
		}
	}
	return Span{}, false
}

func modelArmorConfidence(level string) string {
	return strings.ToLower(strings.ReplaceAll(level, "_", " "))
}

// UnmarshalJSON accepts a number or a string holding one.
func (n *modelArmorInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*n = 0
		return nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	*n = modelArmorInt(v)
	return nil
}
