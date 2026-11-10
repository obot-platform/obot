import { Group } from '$lib/services';
import { profile } from '$lib/stores';
import { createMockProfile } from '../../tests/helpers/pageData';
import { getGatewayStatusResponse } from '../../tests/mocks/data';
import { worker } from '../../tests/mocks/worker';
import ProvidersView from './ProvidersView.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

// Shared setup caps long timeouts; keep these waits longer than the real polling interval.
const realSetTimeout = window.setTimeout.bind(window);
const waitForPoll = () => new Promise<void>((resolve) => realSetTimeout(resolve, 2200));

beforeEach(() => {
	profile.initialize(createMockProfile());
	worker.use(
		http.get('/api/llm-gateway/admin/providers', () =>
			HttpResponse.json({ providers: [{ name: 'bedrock_mantle' }] })
		),
		http.get('/api/llm-gateway/admin/providers/:provider/keys', () =>
			HttpResponse.json({
				keys: [
					{ id: 'iam', name: 'imported-iam' },
					{ id: 'api', name: 'imported-api' }
				]
			})
		),
		http.get('/api/llm-gateway/migration', () =>
			HttpResponse.json({
				sources: {
					iam: { provider: 'bedrock_mantle', status: 'imported' },
					api: { provider: 'bedrock_mantle', status: 'imported' }
				}
			})
		)
	);
});

describe('ProvidersView migration results', () => {
	it.each(['imported', 'empty'] as const)(
		'retries the provider refresh before accepting an initial %s snapshot',
		async (snapshot) => {
			const migration = vi.fn(() =>
				HttpResponse.json({
					sources:
						snapshot === 'empty' ? {} : { iam: { provider: 'bedrock_mantle', status: 'imported' } }
				})
			);
			let requests = 0;
			const providers = vi.fn(() => {
				requests++;
				return requests === 1
					? new HttpResponse(null, { status: 500 })
					: HttpResponse.json({ providers: [{ name: 'bedrock_mantle' }] });
			});
			worker.use(
				http.get('/api/llm-gateway/migration', migration),
				http.get('/api/llm-gateway/admin/providers', providers)
			);
			render(ProvidersView);
			await expect.element(page.getByText('No providers configured')).toBeVisible();
			await vi.waitFor(() => expect(providers).toHaveBeenCalledTimes(2), { timeout: 3500 });
			await expect.element(page.getByText('2 keys', { exact: true })).toBeVisible();
			await expect.element(page.getByRole('button', { name: 'Modify', exact: true })).toBeVisible();
			expect(
				page.getByRole('heading', { name: 'AWS Bedrock', exact: true }).elements()
			).toHaveLength(1);
			await expect.element(page.getByText('No providers configured')).not.toBeInTheDocument();
			await waitForPoll();
			expect(migration).toHaveBeenCalledTimes(2);
			expect(providers).toHaveBeenCalledTimes(2);
		},
		7000
	);
	it.each(['error', 'pending'] as const)(
		'keeps the previous %s snapshot through provider refresh failures until recovery',
		async (status) => {
			let reads = 0;
			let requests = 0;
			let release!: () => void;
			// Hold the successful provider refresh across a tick to check the in-flight guard.
			const refreshed = new Promise<void>((resolve) => (release = resolve));
			const providers = vi.fn(async () => {
				requests++;
				if (requests === 1) return HttpResponse.json({ providers: [{ name: 'openai' }] });
				if (requests === 2) return new HttpResponse(null, { status: 500 });
				if (requests === 3) return HttpResponse.error();
				await refreshed;
				return HttpResponse.json({ providers: [{ name: 'bedrock_mantle' }] });
			});
			worker.use(
				http.get('/api/llm-gateway/admin/providers', providers),
				http.get('/api/llm-gateway/migration', () => {
					reads++;
					return HttpResponse.json({
						sources: {
							iam: {
								name: 'AWS Bedrock',
								provider: 'bedrock_mantle',
								status: reads === 1 ? status : 'imported'
							}
						}
					});
				})
			);
			const { unmount } = await render(ProvidersView);
			const warning = page.getByText('Some legacy model providers need attention.');
			try {
				await expect.element(page.getByText('2 keys', { exact: true })).toBeVisible();
				await vi.waitFor(() => expect(providers).toHaveBeenCalledTimes(2), { timeout: 3500 });
				if (status === 'error') {
					await expect.element(warning).toBeVisible();
					await expect
						.element(page.getByText('AWS Bedrock: The import failed.', { exact: false }))
						.toBeVisible();
				} else {
					await expect.element(warning).not.toBeInTheDocument();
				}
				await vi.waitFor(() => expect(providers).toHaveBeenCalledTimes(4), { timeout: 5500 });
				await expect
					.element(page.getByRole('heading', { name: 'OpenAI', exact: true }))
					.toBeVisible();
				await expect.element(page.getByText('2 keys', { exact: true })).toBeVisible();
				await waitForPoll();
				expect(providers).toHaveBeenCalledTimes(4);
				expect(reads).toBe(4);
				if (status === 'error') await expect.element(warning).toBeVisible();
				release();
				await expect
					.element(
						page
							.getByCSS('div.rounded-lg')
							.filter({ hasText: 'AWS Bedrock' })
							.getByText('2 keys', { exact: true })
					)
					.toBeVisible();
				await expect
					.element(
						page
							.getByCSS('div.rounded-lg')
							.filter({ hasText: 'OpenAI' })
							.getByRole('button', { name: 'Modify', exact: true })
					)
					.not.toBeInTheDocument();
				await expect.element(warning).not.toBeInTheDocument();
				await waitForPoll();
				expect(providers).toHaveBeenCalledTimes(4);
				expect(reads).toBe(4);
			} finally {
				release();
				await unmount();
			}
			await waitForPoll();
			expect(providers).toHaveBeenCalledTimes(4);
			expect(reads).toBe(4);
		},
		16000
	);
	it('waits until the gateway is ready and providers refresh before accepting a terminal snapshot', async () => {
		let reads = 0;
		const status = vi.fn(() => {
			reads++;
			return HttpResponse.json({ ...getGatewayStatusResponse, ready: reads > 1 });
		});
		const migration = vi.fn(() => HttpResponse.json({ sources: {} }));
		const providers = vi.fn(() => HttpResponse.json({ providers: [{ name: 'bedrock_mantle' }] }));
		worker.use(
			http.get('/api/llm-gateway/status', status),
			http.get('/api/llm-gateway/migration', migration),
			http.get('/api/llm-gateway/admin/providers', providers)
		);
		render(ProvidersView);
		await expect.element(page.getByText('The LLM gateway is starting…')).toBeVisible();
		expect(providers).not.toHaveBeenCalled();
		await vi.waitFor(() => expect(providers).toHaveBeenCalledOnce(), { timeout: 3500 });
		await expect.element(page.getByText('2 keys', { exact: true })).toBeVisible();
		await waitForPoll();
		expect(status).toHaveBeenCalledTimes(2);
		expect(migration).toHaveBeenCalledTimes(2);
		expect(providers).toHaveBeenCalledOnce();
	}, 7000);
	it('retries initial fetch failures, shows recovered results and stops after completion', async () => {
		let reads = 0;
		worker.use(
			http.get('/api/llm-gateway/admin/providers', () =>
				HttpResponse.json({ providers: reads >= 3 ? [{ name: 'bedrock_mantle' }] : [] })
			),
			http.get('/api/llm-gateway/migration', () => {
				reads++;
				if (reads === 1) return new HttpResponse(null, { status: 500 });
				if (reads === 2) return HttpResponse.error();
				return HttpResponse.json({
					sources: {
						iam: {
							name: 'AWS Bedrock',
							provider: 'bedrock_mantle',
							status: reads === 3 ? 'error' : 'imported'
						}
					}
				});
			})
		);
		render(ProvidersView);
		await expect.element(page.getByText('No providers configured')).toBeVisible();
		await vi.waitFor(() => expect(reads).toBe(3), { timeout: 5500 });
		await expect
			.element(page.getByText('AWS Bedrock: The import failed.', { exact: false }))
			.toBeVisible();
		await expect.element(page.getByText('2 keys', { exact: true })).toBeVisible();
		await vi.waitFor(() => expect(reads).toBe(4), { timeout: 3500 });
		await expect
			.element(page.getByText('Some legacy model providers need attention.'))
			.not.toBeInTheDocument();
		await waitForPoll();
		expect(reads).toBe(4);
	}, 12000);
	it('does not poll after a successful empty snapshot', async () => {
		const migration = vi.fn(() => HttpResponse.json({ sources: {} }));
		worker.use(http.get('/api/llm-gateway/migration', migration));
		render(ProvidersView);
		await expect.element(page.getByText('2 keys', { exact: true })).toBeVisible();
		await waitForPoll();
		expect(migration).toHaveBeenCalledTimes(1);
	}, 5000);
	it('does not request migration status for readonly users, including on polling ticks', async () => {
		profile.initialize(createMockProfile([Group.AUDITOR]));
		const migration = vi.fn(() => new HttpResponse(null, { status: 500 }));
		worker.use(http.get('/api/llm-gateway/migration', migration));
		render(ProvidersView);
		await expect.element(page.getByText('2 keys', { exact: true })).toBeVisible();
		await waitForPoll();
		expect(migration).not.toHaveBeenCalled();
	}, 5000);
	it('waits for initial and polling loads, then stops requests after unmount', async () => {
		let reads = 0;
		let releaseInitial!: () => void;
		let releaseRetry!: () => void;
		const initial = new Promise<void>((resolve) => (releaseInitial = resolve));
		const retry = new Promise<void>((resolve) => (releaseRetry = resolve));
		const status = vi.fn(() => HttpResponse.json(getGatewayStatusResponse));
		const providers = vi.fn(() => HttpResponse.json({ providers: [] }));
		worker.use(
			http.get('/api/llm-gateway/status', status),
			http.get('/api/llm-gateway/admin/providers', providers),
			http.get('/api/llm-gateway/migration', async () => {
				reads++;
				await (reads === 1 ? initial : retry);
				return new HttpResponse(null, { status: 500 });
			})
		);
		const { unmount } = await render(ProvidersView);
		try {
			await vi.waitFor(() => expect(reads).toBe(1));
			await waitForPoll();
			expect(reads).toBe(1);
			expect(status).toHaveBeenCalledTimes(1);
			releaseInitial();
			await expect.element(page.getByText('No providers configured')).toBeVisible();
			await vi.waitFor(() => expect(reads).toBe(2), { timeout: 3500 });
			await waitForPoll();
			expect(reads).toBe(2);
			expect(status).toHaveBeenCalledTimes(2);
		} finally {
			await unmount();
			releaseInitial();
			releaseRetry();
		}
		// Let the outstanding load finish before checking that no new requests start.
		await vi.waitFor(() => expect(providers).toHaveBeenCalledTimes(2));
		await waitForPoll();
		expect(reads).toBe(2);
		expect(status).toHaveBeenCalledTimes(2);
	}, 14000);
	it('does not show a migration warning for a completed automatic pool', async () => {
		render(ProvidersView);
		await expect.element(page.getByText('2 keys', { exact: true })).toBeVisible();
		await expect
			.element(page.getByText('Some legacy model providers need attention.'))
			.not.toBeInTheDocument();
	});
	it('shows the imported Mantle provider as configured AWS Bedrock without a duplicate', async () => {
		render(ProvidersView);
		await expect.element(page.getByRole('button', { name: 'Modify', exact: true })).toBeVisible();
		await expect
			.element(page.getByRole('heading', { name: 'bedrock_mantle', exact: true }))
			.not.toBeInTheDocument();
		expect(page.getByRole('heading', { name: 'AWS Bedrock', exact: true }).elements()).toHaveLength(
			1
		);
		await expect
			.element(page.getByRole('img', { name: 'AWS Bedrock', exact: true }))
			.toHaveAttribute('src', '/admin/assets/amazon-bedrock.png');
	});
	it('does not require a choice while automatic imports are pending', async () => {
		worker.use(
			http.get('/api/llm-gateway/migration', () =>
				HttpResponse.json({
					sources: {
						iam: { provider: 'bedrock_mantle', status: 'pending' },
						api: { provider: 'bedrock_mantle', status: 'pending' }
					}
				})
			)
		);
		render(ProvidersView);
		await expect.element(page.getByText('2 keys', { exact: true })).toBeVisible();
		await expect
			.element(page.getByText('Some legacy model providers need attention.'))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Review migration' }))
			.not.toBeInTheDocument();
	});
	it('keeps failed imports visible without offering a choice', async () => {
		worker.use(
			http.get('/api/llm-gateway/migration', () =>
				HttpResponse.json({
					sources: {
						openai: { name: 'OpenAI', provider: 'openai', status: 'error' }
					}
				})
			)
		);
		render(ProvidersView);
		await expect
			.element(page.getByText('Some legacy model providers need attention.'))
			.toBeVisible();
		await expect
			.element(page.getByText('OpenAI: The import failed.', { exact: false }))
			.toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Review migration' }))
			.not.toBeInTheDocument();
	});
	it('clears a failure warning after automatic reconciliation succeeds', async () => {
		let reads = 0;
		worker.use(
			http.get('/api/llm-gateway/migration', () => {
				reads++;
				return HttpResponse.json({
					sources: {
						iam: { provider: 'bedrock_mantle', status: reads === 1 ? 'error' : 'imported' },
						api: { provider: 'bedrock_mantle', status: reads === 1 ? 'error' : 'imported' }
					}
				});
			})
		);
		render(ProvidersView);
		await expect
			.element(page.getByText('Some legacy model providers need attention.'))
			.toBeVisible();
		await vi.waitFor(() => expect(reads).toBeGreaterThanOrEqual(2), { timeout: 3500 });
		await expect
			.element(page.getByText('Some legacy model providers need attention.'))
			.not.toBeInTheDocument();
	}, 5000);
	it.each(['error', 'pending'] as const)(
		'preserves the last %s snapshot and keeps polling after a transient fetch failure',
		async (status) => {
			let reads = 0;
			let imported = false;
			let release!: () => void;
			// Hold reconciliation so the retained warning can be checked before success clears it.
			const reconciled = new Promise<void>((resolve) => (release = resolve));
			const providers = vi.fn(() =>
				HttpResponse.json({ providers: imported ? [{ name: 'bedrock_mantle' }] : [] })
			);
			worker.use(
				http.get('/api/llm-gateway/admin/providers', providers),
				http.get('/api/llm-gateway/migration', async () => {
					reads++;
					if (reads === 1) {
						return HttpResponse.json({
							sources: { iam: { name: 'AWS Bedrock', provider: 'bedrock_mantle', status } }
						});
					}
					if (reads === 2) {
						return status === 'error'
							? new HttpResponse(null, { status: 500 })
							: HttpResponse.error();
					}
					await reconciled;
					imported = true;
					return HttpResponse.json({
						sources: { iam: { provider: 'bedrock_mantle', status: 'imported' } }
					});
				})
			);
			const { unmount } = await render(ProvidersView);
			const warning = page.getByText('Some legacy model providers need attention.');
			try {
				await expect.element(page.getByText('No providers configured')).toBeVisible();
				await vi.waitFor(() => expect(providers).toHaveBeenCalledTimes(2), { timeout: 3500 });
				if (status === 'error') {
					await expect.element(warning).toBeVisible();
					await expect
						.element(page.getByText('AWS Bedrock: The import failed.', { exact: false }))
						.toBeVisible();
				} else {
					await expect.element(warning).not.toBeInTheDocument();
				}
				await vi.waitFor(() => expect(reads).toBe(3), { timeout: 3500 });
				release();
				await expect.element(page.getByText('2 keys', { exact: true })).toBeVisible();
				await expect.element(warning).not.toBeInTheDocument();
				await expect
					.element(page.getByRole('button', { name: 'Review migration' }))
					.not.toBeInTheDocument();
			} finally {
				release();
				await unmount();
			}
		},
		8000
	);
});
