import { announceOAuthCompletion, onOAuthCompletion } from './oauth-completion';
import { afterEach, describe, expect, it, vi } from 'vitest';

const CHANNEL_NAME = 'obot-mcp-oauth-completion';

describe('OAuth completion signal', () => {
	const cleanups: Array<() => void> = [];
	afterEach(() => {
		cleanups.splice(0).forEach((cleanup) => cleanup());
		vi.restoreAllMocks();
		vi.useRealTimers();
	});

	function listen(minIntervalMs: number) {
		const callback = vi.fn();
		cleanups.push(onOAuthCompletion(callback, minIntervalMs));
		return callback;
	}

	it('ignores messages that are not completion announcements', async () => {
		const callback = listen(0);
		const sender = new BroadcastChannel(CHANNEL_NAME);
		cleanups.push(() => sender.close());
		sender.postMessage('complete');
		sender.postMessage({ type: 'other' });
		sender.postMessage(null);
		announceOAuthCompletion();

		await vi.waitFor(() => expect(callback).toHaveBeenCalledOnce());
		await new Promise((resolve) => setTimeout(resolve, 20));
		expect(callback).toHaveBeenCalledOnce();
	});

	it('checks immediately, then once more for a burst inside the window', async () => {
		const callback = listen(300);
		announceOAuthCompletion();
		await vi.waitFor(() => expect(callback).toHaveBeenCalledOnce());

		announceOAuthCompletion();
		announceOAuthCompletion();
		announceOAuthCompletion();
		await new Promise((resolve) => setTimeout(resolve, 20));
		expect(callback).toHaveBeenCalledOnce();

		await vi.waitFor(() => expect(callback).toHaveBeenCalledTimes(2));
		await new Promise((resolve) => setTimeout(resolve, 350));
		expect(callback).toHaveBeenCalledTimes(2);
	});

	it('keeps checking when the wall clock moves backwards', async () => {
		const callback = listen(100);
		announceOAuthCompletion();
		await vi.waitFor(() => expect(callback).toHaveBeenCalledOnce());
		await new Promise((resolve) => setTimeout(resolve, 150));

		const realNow = Date.now();
		const clock = vi.spyOn(Date, 'now').mockReturnValue(realNow - 60 * 60 * 1000);
		cleanups.push(() => clock.mockRestore());
		announceOAuthCompletion();

		await vi.waitFor(() => expect(callback).toHaveBeenCalledTimes(2));
	});

	it('cancels a pending check when the listener stops', async () => {
		const callback = vi.fn();
		const stop = onOAuthCompletion(callback, 300);
		announceOAuthCompletion();
		await vi.waitFor(() => expect(callback).toHaveBeenCalledOnce());

		announceOAuthCompletion();
		await new Promise((resolve) => setTimeout(resolve, 10));
		stop();
		await new Promise((resolve) => setTimeout(resolve, 350));
		expect(callback).toHaveBeenCalledOnce();
	});

	it.each([false, true])(
		'cancels an overdue trailing check when a new message runs immediately (stop listener: %s)',
		async (stopBeforeTimer) => {
			vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
			const clock = vi.spyOn(performance, 'now').mockReturnValue(0);
			const callback = vi.fn();
			const stop = onOAuthCompletion(callback);
			cleanups.push(stop);

			announceOAuthCompletion();
			await vi.waitFor(() => expect(callback).toHaveBeenCalledOnce());

			clock.mockReturnValue(100);
			announceOAuthCompletion();
			await vi.waitFor(() => expect(vi.getTimerCount()).toBe(1));

			// A background tab can deliver a new message before an overdue timer.
			// Advance the monotonic clock without executing the pending timer.
			clock.mockReturnValue(2100);
			announceOAuthCompletion();
			await vi.waitFor(() => expect(callback).toHaveBeenCalledTimes(2));

			if (stopBeforeTimer) stop();
			vi.runAllTimers();
			expect(callback).toHaveBeenCalledTimes(2);
		}
	);
});
