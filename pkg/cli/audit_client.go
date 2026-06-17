package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/apiclient/types"
	cliinternal "github.com/obot-platform/obot/pkg/cli/internal"
)

func auditAPIClient(ctx context.Context, rootClient *apiclient.Client, timeout time.Duration) (*apiclient.Client, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if rootClient == nil {
		return nil, fmt.Errorf("audit submission requires an API client")
	}

	token := rootClient.Token
	if token == "" {
		checkCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		var err error
		token, err = cliinternal.ExistingToken(checkCtx, rootClient.BaseURL)
		if err != nil {
			return nil, err
		}
	}

	return &apiclient.Client{
		BaseURL: rootClient.BaseURL,
		Token:   token,
		Cookie:  rootClient.Cookie,
	}, nil
}

func flushAuditSpool(ctx context.Context, client *apiclient.Client, spool auditSpool, limit int) (int, error) {
	records, err := spool.List(limit)
	if err != nil {
		return 0, err
	}
	if len(records) == 0 {
		return 0, nil
	}

	events := make([]types.AuditEvent, 0, len(records))
	for _, record := range records {
		events = append(events, record.Event)
	}

	resp, err := client.SubmitAuditEvents(ctx, events)
	if err != nil {
		return 0, err
	}

	statuses := auditTerminalStatuses(resp, events)
	flushed := 0
	for _, record := range records {
		if !statuses[record.Event.EventID] {
			continue
		}
		if err := spool.Delete(record.Path); err != nil {
			return flushed, err
		}
		flushed++
	}
	return flushed, nil
}

func auditSubmitAccepted(resp *types.AuditEventSubmitResponse, eventID string) bool {
	if resp == nil || len(resp.Items) == 0 {
		return true
	}
	for _, item := range resp.Items {
		if item.EventID == eventID {
			return item.Status == types.AuditEventSubmitStatusAccepted ||
				item.Status == types.AuditEventSubmitStatusDuplicate
		}
	}
	return false
}

func auditTerminalStatuses(resp *types.AuditEventSubmitResponse, events []types.AuditEvent) map[string]bool {
	result := map[string]bool{}
	if resp == nil || len(resp.Items) == 0 {
		for _, event := range events {
			result[event.EventID] = true
		}
		return result
	}
	for _, item := range resp.Items {
		result[item.EventID] = auditStatusTerminal(item.Status)
	}
	return result
}

func auditStatusTerminal(status string) bool {
	switch status {
	case types.AuditEventSubmitStatusAccepted,
		types.AuditEventSubmitStatusDuplicate,
		types.AuditEventSubmitStatusError:
		return true
	default:
		return false
	}
}
