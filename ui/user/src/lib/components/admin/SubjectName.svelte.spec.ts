import SubjectName from './SubjectName.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

describe('SubjectName', () => {
	it('shows only the name of a subject that is not disabled', async () => {
		render(SubjectName, { name: 'Alice' });

		await expect.element(page.getByText('Alice')).toBeVisible();
		await expect.element(page.getByText('Disabled')).not.toBeInTheDocument();
	});

	it('marks a disabled user', async () => {
		render(SubjectName, { name: 'Dan', disabled: true });

		await expect.element(page.getByText('Dan')).toBeVisible();
		await expect.element(page.getByText('Disabled', { exact: true })).toBeVisible();
	});
});
