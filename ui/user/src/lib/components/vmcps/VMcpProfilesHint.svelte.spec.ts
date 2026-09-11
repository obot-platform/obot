import { VMCP_PROFILES_HINT_STORAGE_KEY } from '$lib/runes/vmcps/vmcpToolFlow.svelte';
import VMcpProfilesHint from './VMcpProfilesHint.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const STORAGE_KEY = VMCP_PROFILES_HINT_STORAGE_KEY;

const HINT_TEXT = 'Click here to begin tailoring access and tools this VMCP.';

function caption() {
	return page.getByText(HINT_TEXT, { exact: false });
}

function dismissButton() {
	return page.getByRole('button', { name: 'Dismiss profiles tip' });
}

function profilesTabAnchor() {
	const el = document.createElement('button');
	el.textContent = 'Profiles';
	el.getBoundingClientRect = () => new DOMRect(100, 40, 96, 32) as DOMRect;
	document.body.appendChild(el);
	return el;
}

describe('VMcpProfilesHint.svelte', () => {
	it('shows the tip on a first visit when queued', async () => {
		render(VMcpProfilesHint, { props: { show: true, anchorEl: profilesTabAnchor() } });

		await expect.element(caption()).toBeVisible();
		await expect.element(page.getByRole('dialog', { name: 'Profiles' })).toBeVisible();
	});

	it('stays hidden once it has been seen', async () => {
		localStorage.setItem(STORAGE_KEY, new Date().toISOString());
		render(VMcpProfilesHint, { props: { show: true, anchorEl: profilesTabAnchor() } });

		await expect.element(caption()).not.toBeInTheDocument();
	});

	it('remembers a dismissal so it does not come back', async () => {
		render(VMcpProfilesHint, { props: { show: true, anchorEl: profilesTabAnchor() } });

		await dismissButton().click();

		await expect.element(caption()).not.toBeInTheDocument();
		expect(localStorage.getItem(STORAGE_KEY)).not.toBeNull();
	});

	it('stays hidden when nothing is queued', async () => {
		render(VMcpProfilesHint, { props: { show: false, anchorEl: profilesTabAnchor() } });

		await expect.element(caption()).not.toBeInTheDocument();
	});
});
