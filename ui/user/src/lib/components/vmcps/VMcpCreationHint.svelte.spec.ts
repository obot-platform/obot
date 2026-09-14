import { VMCP_CREATION_HINT_STORAGE_KEY } from '$lib/runes/vmcps/vmcpToolFlow.svelte';
import VMcpCreationHint, {
	CONNECT_HINT_TEXT,
	PROFILES_HINT_TEXT,
	TESTER_HINT_TEXT
} from './VMcpCreationHint.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const STORAGE_KEY = VMCP_CREATION_HINT_STORAGE_KEY;
const LEGACY_STORAGE_KEY = '@obot/seen-vmcp-profiles-hint';

function profilesCaption() {
	return page.getByText(PROFILES_HINT_TEXT, { exact: false });
}

function testerCaption() {
	return page.getByText(TESTER_HINT_TEXT, { exact: false });
}

function connectCaption() {
	return page.getByText(CONNECT_HINT_TEXT, { exact: false });
}

function nextOverlay() {
	return page.getByRole('button', { name: 'Continue creation tips' });
}

function anchor(label: string, x = 100) {
	const el = document.createElement('button');
	el.textContent = label;
	el.getBoundingClientRect = () => new DOMRect(x, 40, 96, 32) as DOMRect;
	document.body.appendChild(el);
	return el;
}

function renderSeries(
	overrides: {
		show?: boolean;
		includeProfiles?: boolean;
		includeTester?: boolean;
		includeConnect?: boolean;
	} = {}
) {
	return render(VMcpCreationHint, {
		props: {
			show: true,
			includeProfiles: true,
			includeTester: true,
			includeConnect: true,
			profilesAnchorEl: anchor('Profiles', 100),
			testerAnchorEl: anchor('Tester', 220),
			connectAnchorEl: anchor('Connect', 360),
			...overrides
		}
	});
}

describe('VMcpCreationHint.svelte', () => {
	it('shows the profiles tip first when queued', async () => {
		renderSeries();

		await expect.element(profilesCaption()).toBeVisible();
		await expect.element(page.getByRole('dialog', { name: 'Profiles' })).toBeVisible();
		await expect.element(testerCaption()).not.toBeInTheDocument();
	});

	it('advances to the tester and connect tips when clicking outside', async () => {
		renderSeries();

		await nextOverlay().click();
		await expect.element(testerCaption()).toBeVisible();
		await expect.element(profilesCaption()).not.toBeInTheDocument();

		await nextOverlay().click();
		await expect.element(connectCaption()).toBeVisible();
		await expect.element(testerCaption()).not.toBeInTheDocument();

		await nextOverlay().click();
		await expect.element(connectCaption()).not.toBeInTheDocument();
		expect(localStorage.getItem(STORAGE_KEY)).not.toBeNull();
	});

	it('skips the profiles tip when there is no Profiles tab', async () => {
		renderSeries({ includeProfiles: false });

		await expect.element(testerCaption()).toBeVisible();
		await expect.element(profilesCaption()).not.toBeInTheDocument();
		await expect.element(page.getByRole('dialog', { name: 'Tester' })).toBeVisible();
	});

	it('stays hidden once it has been seen', async () => {
		localStorage.setItem(STORAGE_KEY, new Date().toISOString());
		renderSeries();

		await expect.element(profilesCaption()).not.toBeInTheDocument();
	});

	it('stays hidden when the legacy profiles hint was already dismissed', async () => {
		localStorage.setItem(LEGACY_STORAGE_KEY, new Date().toISOString());
		renderSeries();

		await expect.element(profilesCaption()).not.toBeInTheDocument();
	});

	it('stays hidden when nothing is queued', async () => {
		renderSeries({ show: false });

		await expect.element(profilesCaption()).not.toBeInTheDocument();
	});
});
