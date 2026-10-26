import { beforeNavigate } from '$app/navigation';
import { filterUnsavedChanges } from './filterUnsavedChanges.svelte';
import { onMount } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('$app/navigation', () => ({ beforeNavigate: vi.fn() }));
vi.mock('svelte', () => ({ onMount: vi.fn() }));
vi.mock('$lib/i18n', () => ({ m: { mcps_filters_discard_changes: () => 'Discard changes?' } }));

function setup() {
	const confirm = vi.fn().mockReturnValue(true);
	const listeners = new Map<
		string,
		(event: { preventDefault: () => void; returnValue?: string }) => void
	>();
	vi.stubGlobal('window', {
		confirm,
		addEventListener: vi.fn((name, callback) => listeners.set(name, callback)),
		removeEventListener: vi.fn()
	});
	const guard = filterUnsavedChanges(() => true);
	const before = vi.mocked(beforeNavigate).mock.calls[0][0];
	vi.mocked(onMount).mock.calls[0][0]();
	return { guard, confirm, before, listeners };
}
beforeEach(() => {
	vi.clearAllMocks();
	vi.unstubAllGlobals();
});
describe('filter unsaved changes', () => {
	it('uses only the native warning for full-page exits', () => {
		const { before, confirm, listeners } = setup();
		const cancel = vi.fn();
		before({
			type: 'leave',
			from: null,
			to: null,
			complete: Promise.resolve(),
			willUnload: true,
			cancel
		});
		expect(confirm).not.toHaveBeenCalled();
		expect(cancel).not.toHaveBeenCalled();
		const event = { preventDefault: vi.fn(), returnValue: undefined as string | undefined };
		listeners.get('beforeunload')!(event);
		expect(event.preventDefault).toHaveBeenCalledOnce();
	});
	it('confirms internal navigation and can cancel it', () => {
		const { before, confirm } = setup();
		const cancel = vi.fn();
		confirm.mockReturnValue(false);
		before({
			type: 'goto',
			from: null,
			to: null,
			complete: Promise.resolve(),
			willUnload: false,
			cancel
		});
		expect(confirm).toHaveBeenCalledOnce();
		expect(cancel).toHaveBeenCalledOnce();
	});
	it('bypasses both warnings after a successful confirmed deletion', () => {
		const { guard, before, confirm, listeners } = setup();
		guard.allowNavigation();
		const cancel = vi.fn();
		before({
			type: 'goto',
			from: null,
			to: null,
			complete: Promise.resolve(),
			willUnload: false,
			cancel
		});
		const event = { preventDefault: vi.fn() };
		listeners.get('beforeunload')!(event);
		expect(confirm).not.toHaveBeenCalled();
		expect(cancel).not.toHaveBeenCalled();
		expect(event.preventDefault).not.toHaveBeenCalled();
	});
});
