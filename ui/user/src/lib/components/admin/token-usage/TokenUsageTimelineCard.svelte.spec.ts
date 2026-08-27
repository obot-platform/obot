import { AdminService, UserService } from '$lib/services';
import TokenUsageTimelineCard from './TokenUsageTimelineCard.svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';

describe('TokenUsageTimelineCard', () => {
	beforeEach(() => {
		vi.restoreAllMocks();
		vi.spyOn(AdminService, 'listTokenUsage').mockResolvedValue([]);
		vi.spyOn(AdminService, 'listModels').mockResolvedValue([]);
	});

	it('does not refetch users when an empty users collection is supplied', async () => {
		const listUsers = vi.spyOn(UserService, 'listUsersIncludeDeleted').mockResolvedValue([]);

		render(TokenUsageTimelineCard, {
			startDate: new Date('2026-07-27T00:00:00Z'),
			endDate: new Date('2026-08-27T00:00:00Z'),
			users: []
		});

		await vi.waitFor(() => expect(AdminService.listTokenUsage).toHaveBeenCalledOnce());
		expect(listUsers).not.toHaveBeenCalled();
	});
});
