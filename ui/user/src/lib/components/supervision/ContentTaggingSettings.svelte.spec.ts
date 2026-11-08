import { Group } from '$lib/services';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import ContentTaggingSettings from './ContentTaggingSettings.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const settings = {
	supervisorEnabled: true,
	retentionDays: 30,
	contentTagging: { enabled: false, onlyUnlabeled: true, mode: 'async', minBytes: 200 }
};

function mockSettings(saved = vi.fn(), flushed = vi.fn()) {
	worker.use(
		http.get('/api/provenance/settings', () => HttpResponse.json(settings)),
		http.get('/api/provenance/content-tagging', () =>
			HttpResponse.json({ queued: 4, lastTagged: 0 })
		),
		http.put('/api/provenance/settings', async ({ request }) => {
			const body = await request.json();
			saved(body);
			return HttpResponse.json(body);
		}),
		http.post('/api/provenance/content-tagging/flush', () => {
			flushed();
			return HttpResponse.json({ queued: 0, lastRun: new Date().toISOString(), lastTagged: 3 });
		})
	);
	return { saved, flushed };
}

async function renderSettings(groups: string[] = [Group.ADMIN]) {
	await preparePageData({ profile: createMockProfile(groups) });
	return render(ContentTaggingSettings);
}

describe('ContentTaggingSettings', () => {
	it('saves batch mode with a sample rate and interval, keeping other settings', async () => {
		const { saved } = mockSettings();
		await renderSettings();

		await page.getByRole('checkbox', { name: 'Tag content with AI' }).click();
		await page.getByRole('radio', { name: 'In batches on an interval' }).click();
		await page.getByRole('spinbutton', { name: /Sample rate/ }).fill('25');
		await page.getByRole('spinbutton', { name: /Batch every/ }).fill('30');
		await page.getByRole('button', { name: 'Save' }).click();

		await vi.waitFor(() => expect(saved).toHaveBeenCalled());
		const body = saved.mock.calls[0][0];
		expect(body.retentionDays).toBe(30);
		expect(body.contentTagging).toMatchObject({
			enabled: true,
			mode: 'batch',
			sampleRate: 0.25,
			batchIntervalMinutes: 30,
			onlyUnlabeled: true,
			minBytes: 200
		});
	});

	it('runs the batch queue on demand', async () => {
		const { flushed } = mockSettings();
		await renderSettings();

		await page.getByRole('checkbox', { name: 'Tag content with AI' }).click();
		await page.getByRole('radio', { name: 'In batches on an interval' }).click();
		await expect.element(page.getByText('4 items waiting for the next batch')).toBeInTheDocument();
		await page.getByRole('button', { name: 'Tag the queue now' }).click();

		await vi.waitFor(() => expect(flushed).toHaveBeenCalled());
		await expect.element(page.getByText(/tagged 3 items/)).toBeInTheDocument();
	});

	it('treats a full sample as every eligible item', async () => {
		const { saved } = mockSettings();
		await renderSettings();

		await page.getByRole('checkbox', { name: 'Tag content with AI' }).click();
		await page.getByRole('button', { name: 'Save' }).click();

		await vi.waitFor(() => expect(saved).toHaveBeenCalled());
		expect(saved.mock.calls[0][0].contentTagging.sampleRate).toBe(0);
	});

	it('is read-only for auditors', async () => {
		mockSettings();
		await renderSettings([Group.USER, Group.AUDITOR]);

		await expect
			.element(page.getByRole('checkbox', { name: 'Tag content with AI' }))
			.toBeDisabled();
		await expect.element(page.getByRole('button', { name: 'Save' })).not.toBeInTheDocument();
	});
});
