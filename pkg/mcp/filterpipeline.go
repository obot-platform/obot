package mcp

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
)

const (
	defaultFilterCallTimeout     = 10 * time.Second
	defaultFilterPipelineTimeout = 25 * time.Second
	maxFilterReasonLength        = 1024

	FilterErrorExecution          = "execution"
	FilterErrorTimeout            = "timeout"
	FilterErrorMalformedResponse  = "malformedResponse"
	FilterErrorInvalidMutation    = "invalidMutation"
	FilterErrorMutationDisallowed = "mutationDisallowed"
	FilterErrorRecording          = "recording"
	FilterErrorCanceled           = "canceled"
)

type FilterPipeline struct {
	executor      FilterExecutor
	recorder      FilterDecisionRecorder
	callTimeout   time.Duration
	totalTimeout  time.Duration
	newEvaluation func() string
}

func NewFilterPipeline(executor FilterExecutor, recorder FilterDecisionRecorder) *FilterPipeline {
	return &FilterPipeline{
		executor:      executor,
		recorder:      recorder,
		callTimeout:   defaultFilterCallTimeout,
		totalTimeout:  defaultFilterPipelineTimeout,
		newEvaluation: rand.Text,
	}
}

func (p *FilterPipeline) Run(ctx context.Context, request FilterPipelineRequest) FilterPipelineResult {
	result := FilterPipelineResult{
		Decision: FilterDecisionAccept,
		Payload:  cloneRawMessage(request.Payload),
	}
	if p == nil || p.executor == nil {
		if len(request.Candidates) == 0 {
			return result
		}
		return infrastructurePipelineResult(result, "", FilterErrorExecution)
	}

	candidates := orderedFilterCandidates(request.Candidates)
	if len(candidates) == 0 {
		return result
	}
	result.EvaluationID = p.newEvaluation()

	totalTimeout := p.totalTimeout
	if totalTimeout <= 0 {
		totalTimeout = defaultFilterPipelineTimeout
	}
	pipelineCtx, cancel := context.WithTimeout(ctx, totalTimeout)
	defer cancel()

	original := cloneRawMessage(request.Payload)
	current := cloneRawMessage(request.Payload)
	for sequence, candidate := range candidates {
		input := cloneRawMessage(current)
		started := time.Now()
		callTimeout := p.callTimeout
		if callTimeout <= 0 {
			callTimeout = defaultFilterCallTimeout
		}
		callCtx, callCancel := context.WithTimeout(pipelineCtx, callTimeout)
		execution, err := p.executor.ExecuteFilter(callCtx, candidate, input)
		callCancel()
		duration := time.Since(started)
		if err == nil {
			switch execution.Decision {
			case FilterDecisionAccept, FilterDecisionReject:
			case FilterDecisionMutate:
				if len(execution.Message) == 0 || !json.Valid(execution.Message) {
					err = newFilterExecutionError(FilterErrorMalformedResponse, "Filter mutation did not contain a complete JSON message")
				}
			default:
				err = newFilterExecutionError(FilterErrorMalformedResponse, "Filter returned an unknown decision")
			}
		}

		outcome, record := normalizeFilterExecution(request, candidate, result.EvaluationID, sequence, input, execution, duration, err)
		if outcome.Decision == FilterDecisionMutate {
			if !candidate.AllowedToMutate || !request.MutationAllowed {
				outcome = infrastructureOutcome(candidate, duration, FilterErrorMutationDisallowed)
				record.Decision = FilterDecisionReject
				record.DecisionKind = FilterDecisionKindInfrastructure
				record.ErrorClass = FilterErrorMutationDisallowed
				record.Diagnostic = "Filter mutation is not permitted for this source"
				record.MutatedPayload = nil
			} else if request.ValidateMutation == nil {
				outcome = infrastructureOutcome(candidate, duration, FilterErrorInvalidMutation)
				record.Decision = FilterDecisionReject
				record.DecisionKind = FilterDecisionKindInfrastructure
				record.ErrorClass = FilterErrorInvalidMutation
				record.Diagnostic = "Filter mutation could not be validated"
				record.MutatedPayload = nil
			} else if validationErr := request.ValidateMutation(original, current, execution.Message); validationErr != nil {
				outcome = infrastructureOutcome(candidate, duration, FilterErrorInvalidMutation)
				record.Decision = FilterDecisionReject
				record.DecisionKind = FilterDecisionKindInfrastructure
				record.ErrorClass = FilterErrorInvalidMutation
				record.Diagnostic = boundedFilterDiagnostic(validationErr)
				record.MutatedPayload = nil
			}
		}

		if p.recorder != nil {
			if err := p.recorder.RecordFilterDecision(pipelineCtx, record); err != nil {
				outcome = infrastructureOutcome(candidate, duration, FilterErrorRecording)
				result.Outcomes = append(result.Outcomes, outcome)
				return terminalPipelineResult(result, current, outcome)
			}
		}

		result.Outcomes = append(result.Outcomes, outcome)
		switch outcome.Decision {
		case FilterDecisionMutate:
			current = cloneRawMessage(execution.Message)
			result.Mutated = true
		case FilterDecisionReject:
			return terminalPipelineResult(result, current, outcome)
		}
	}

	result.Payload = current
	if result.Mutated {
		result.Decision = FilterDecisionMutate
	}
	return result
}

func normalizeFilterExecution(request FilterPipelineRequest, candidate FilterCandidate, evaluationID string, sequence int, input json.RawMessage, execution FilterExecutionResult, duration time.Duration, err error) (FilterOutcome, FilterDecisionRecord) {
	record := FilterDecisionRecord{
		EvaluationID:  evaluationID,
		Sequence:      sequence,
		FilterID:      candidate.ResourceID,
		FilterName:    candidate.DisplayName,
		Source:        request.Source,
		Event:         request.Event,
		Phase:         request.Phase,
		DurationMs:    max(duration.Milliseconds(), 1),
		SourceContext: cloneRawMessage(request.SourceContext),
		Input:         cloneRawMessage(input),
		RawResponse:   cloneRawMessage(execution.RawResponse),
	}
	if err != nil {
		class := filterExecutionErrorClass(err)
		outcome := infrastructureOutcome(candidate, duration, class)
		record.Decision = outcome.Decision
		record.DecisionKind = outcome.DecisionKind
		record.ErrorClass = class
		record.Diagnostic = boundedFilterDiagnostic(err)
		return outcome, record
	}

	reason := boundedFilterReason(execution.Reason)
	outcome := FilterOutcome{
		Candidate:    candidate,
		Decision:     execution.Decision,
		DecisionKind: FilterDecisionKindPolicy,
		Reason:       reason,
		Duration:     duration,
	}
	record.Decision = execution.Decision
	record.DecisionKind = FilterDecisionKindPolicy
	record.Reason = reason
	if execution.Decision == FilterDecisionMutate {
		record.MutatedPayload = cloneRawMessage(execution.Message)
	}
	return outcome, record
}

func orderedFilterCandidates(candidates []FilterCandidate) []FilterCandidate {
	ordered := slices.Clone(candidates)
	slices.SortStableFunc(ordered, func(a, b FilterCandidate) int {
		return strings.Compare(filterCandidateKey(a), filterCandidateKey(b))
	})
	seen := make(map[string]struct{}, len(ordered))
	result := ordered[:0]
	for _, candidate := range ordered {
		key := filterCandidateKey(candidate)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, candidate)
	}
	return result
}

func filterCandidateKey(candidate FilterCandidate) string {
	if candidate.ResourceName != "" {
		return candidate.ResourceName
	}
	if candidate.ResourceID != "" {
		return candidate.ResourceID
	}
	return candidate.Target
}

func infrastructureOutcome(candidate FilterCandidate, duration time.Duration, class string) FilterOutcome {
	return FilterOutcome{
		Candidate:    candidate,
		Decision:     FilterDecisionReject,
		DecisionKind: FilterDecisionKindInfrastructure,
		Reason:       "Filter evaluation failed",
		ErrorClass:   class,
		Duration:     duration,
	}
}

func infrastructurePipelineResult(result FilterPipelineResult, reason, class string) FilterPipelineResult {
	result.Decision = FilterDecisionReject
	result.Reason = reason
	if result.Reason == "" {
		result.Reason = "Filter evaluation failed"
	}
	result.ErrorClass = class
	return result
}

func terminalPipelineResult(result FilterPipelineResult, current json.RawMessage, outcome FilterOutcome) FilterPipelineResult {
	result.Decision = FilterDecisionReject
	result.Payload = cloneRawMessage(current)
	result.Reason = outcome.Reason
	result.ErrorClass = outcome.ErrorClass
	return result
}

func filterExecutionErrorClass(err error) string {
	var executionErr *FilterExecutionError
	if errors.As(err, &executionErr) && executionErr.Class != "" {
		return executionErr.Class
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return FilterErrorTimeout
	}
	if errors.Is(err, context.Canceled) {
		return FilterErrorCanceled
	}
	return FilterErrorExecution
}

func boundedFilterReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if len(reason) <= maxFilterReasonLength {
		return reason
	}
	return reason[:maxFilterReasonLength]
}

func boundedFilterDiagnostic(err error) string {
	if err == nil {
		return ""
	}
	diagnostic := err.Error()
	if len(diagnostic) > maxFilterReasonLength {
		diagnostic = diagnostic[:maxFilterReasonLength]
	}
	return diagnostic
}

func cloneRawMessage(message json.RawMessage) json.RawMessage {
	return slices.Clone(message)
}
