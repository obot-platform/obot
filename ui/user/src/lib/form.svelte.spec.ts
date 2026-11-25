import { completeSave, FormValidation } from '$lib/form.svelte';
import { notifications } from '$lib/stores';
import { goto } from '$lib/url';
import { flushSync } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock(import('$lib/url'), async (importOriginal) => ({
	...(await importOriginal()),
	goto: vi.fn()
}));

afterEach(() => {
	vi.mocked(goto).mockClear();
});

describe('FormValidation', () => {
	it('hides errors until checked, then only clears fixed fields', async () => {
		const cleanup = $effect.root(() => {});
		const values = $state({ name: '', url: '' });
		const validation = new FormValidation<'name' | 'url'>(() => ({
			...(!values.name && { name: 'Name is required.' }),
			...(!values.url && { url: true as const })
		}));

		expect(validation.has('name')).toBe(false);
		expect(validation.valid).toBe(true);

		expect(await validation.check()).toBe(false);
		expect(validation.message('name')).toBe('Name is required.');
		expect(validation.has('url')).toBe(true);
		expect(validation.message('url')).toBeUndefined();
		expect(validation.attrs('name', 'field-name')).toEqual({
			id: 'field-name',
			'aria-invalid': 'true',
			'aria-describedby': 'field-name-error',
			'data-invalid': ''
		});

		values.name = 'Filter';
		flushSync();
		expect(validation.has('name')).toBe(false);
		expect(validation.attrs('name', 'field-name')['data-invalid']).toBeUndefined();

		values.url = 'https://example.com';
		flushSync();
		expect(await validation.check()).toBe(true);
		cleanup();
	});

	it('does not raise errors on other fields while editing', async () => {
		const cleanup = $effect.root(() => {});
		const values = $state({ name: '', url: 'https://example.com' });
		const validation = new FormValidation<'name' | 'url'>(() => ({
			...(!values.name && { name: 'Name is required.' }),
			...(!values.url && { url: 'URL is required.' })
		}));

		expect(await validation.check()).toBe(false);
		values.url = '';
		flushSync();
		expect(validation.has('url')).toBe(false);
		expect(validation.has('name')).toBe(true);

		values.name = 'x';
		flushSync();
		expect(validation.has('name')).toBe(false);
		values.name = '';
		flushSync();
		expect(validation.has('name')).toBe(true); // still flagged until pruned by an input event

		expect(await validation.check()).toBe(false);
		expect(validation.has('url')).toBe(true);
		cleanup();
	});

	it('merges external errors and clears them on reset', async () => {
		const validation = new FormValidation<'condition'>(() => ({}));
		validation.setExternal('condition', 'Invalid condition');
		expect(await validation.check()).toBe(false);
		expect(validation.message('condition')).toBe('Invalid condition');

		validation.setExternal('condition');
		expect(validation.valid).toBe(true);

		validation.setExternal('condition', 'Invalid again');
		validation.reset();
		expect(validation.has('condition')).toBe(false);
	});
});

describe('completeSave', () => {
	it('toasts and stays on the page for an existing object', async () => {
		const add = vi.spyOn(notifications, 'add');
		await completeSave({ created: false, message: 'Saved.', href: '/things/1' });
		expect(add).toHaveBeenCalledWith('Saved.', 'success');
		expect(goto).not.toHaveBeenCalled();
		add.mockRestore();
	});

	it('toasts and replaces the URL with the detail route for a new object', async () => {
		const add = vi.spyOn(notifications, 'add');
		await completeSave({ created: true, message: 'Created.', href: '/things/1' });
		expect(add).toHaveBeenCalledWith('Created.', 'success');
		expect(goto).toHaveBeenCalledWith('/things/1', { replaceState: true });
		add.mockRestore();
	});
});
