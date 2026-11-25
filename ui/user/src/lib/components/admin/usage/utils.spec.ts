import type { McpAuditLogToolCallStatItem, McpAuditLogUsageStats } from '$lib/services';
import {
	mcpUsageCounts,
	transformIndividualToolResponseTimes,
	transformMostActiveUsers,
	transformToolCallErrors,
	transformToolCallErrorsByServer
} from './utils';
import { describe, expect, it } from 'vitest';

function call(
	userID: string,
	overrides: Partial<McpAuditLogToolCallStatItem> = {}
): McpAuditLogToolCallStatItem {
	return {
		createdAt: '2026-11-01T00:00:00Z',
		userID,
		processingTimeMs: 10,
		responseStatus: 200,
		error: '',
		...overrides
	};
}

const stats: McpAuditLogUsageStats = {
	timeStart: '2026-11-01T00:00:00Z',
	timeEnd: '2026-11-02T00:00:00Z',
	totalCalls: 6,
	uniqueUsers: 3,
	items: [
		{
			mcpID: 'ms-1',
			mcpServerInstanceName: '',
			mcpServerName: 'github',
			mcpServerDisplayName: 'GitHub',
			toolCalls: [
				{
					toolName: 'search',
					callCount: 2,
					items: [call('u1'), call('u2', { responseStatus: 500 })]
				},
				{
					toolName: 'create_issue',
					callCount: 1,
					items: [call('u1', { error: 'boom' })]
				}
			]
		},
		{
			mcpID: 'ms-2',
			mcpServerInstanceName: '',
			mcpServerName: 'slack',
			mcpServerDisplayName: 'Slack',
			toolCalls: [
				{ toolName: 'post', callCount: 2, items: [call('u3'), call('u1')] },
				{ toolName: 'empty', callCount: 0 }
			]
		},
		{
			mcpID: 'ms-3',
			mcpServerInstanceName: '',
			mcpServerName: 'idle',
			mcpServerDisplayName: 'Idle'
		}
	]
};

describe('mcpUsageCounts', () => {
	it('matches the row counts of the full transforms', () => {
		expect(mcpUsageCounts(stats)).toEqual({
			individualResponses: transformIndividualToolResponseTimes(stats).length,
			toolErrors: transformToolCallErrors(stats).length,
			serverErrors: transformToolCallErrorsByServer(stats).length,
			activeUsers: transformMostActiveUsers(stats).length
		});
		expect(mcpUsageCounts(stats)).toEqual({
			individualResponses: 5,
			toolErrors: 2,
			serverErrors: 1,
			activeUsers: 3
		});
	});

	it('returns zeros without stats', () => {
		expect(mcpUsageCounts(undefined)).toEqual({
			individualResponses: 0,
			toolErrors: 0,
			serverErrors: 0,
			activeUsers: 0
		});
	});
});
