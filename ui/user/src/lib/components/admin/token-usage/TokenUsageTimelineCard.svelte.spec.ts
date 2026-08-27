import { AdminService, UserService, type OrgUser } from '$lib/services';
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

	it('does not refetch token usage or models when supplied users change', async () => {
		vi.spyOn(UserService, 'listUsersIncludeDeleted').mockResolvedValue([]);
		const startDate = new Date('2026-07-27T00:00:00Z');
		const endDate = new Date('2026-08-27T00:00:00Z');
		const result = await render(TokenUsageTimelineCard, {
			startDate,
			endDate,
			users: []
		});
		await vi.waitFor(() => expect(AdminService.listTokenUsage).toHaveBeenCalledOnce());

		await result.rerender({
			startDate,
			endDate,
			users: [{ id: 'user-1' } as OrgUser]
		});

		await new Promise((resolve) => setTimeout(resolve, 0));
		expect(AdminService.listTokenUsage).toHaveBeenCalledOnce();
		expect(AdminService.listModels).toHaveBeenCalledOnce();
	});

	it('refetches when the time range changes', async () => {
		const result = await render(TokenUsageTimelineCard, {
			startDate: new Date('2026-07-27T00:00:00Z'),
			endDate: new Date('2026-08-27T00:00:00Z'),
			users: []
		});
		await vi.waitFor(() => expect(AdminService.listTokenUsage).toHaveBeenCalledOnce());

		await result.rerender({
			startDate: new Date('2026-08-20T00:00:00Z'),
			endDate: new Date('2026-08-27T00:00:00Z'),
			users: []
		});

		await vi.waitFor(() => expect(AdminService.listTokenUsage).toHaveBeenCalledTimes(2));
		expect(AdminService.listModels).toHaveBeenCalledTimes(2);
	});
});
