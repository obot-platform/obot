import type { VMcpComponentView } from '$lib/services/vmcps/types';
import { createVMCP } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import VMcpList from './VMcpList.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const componentServers: VMcpComponentView[] = [
	{ key: 'github', name: 'GitHub Issues and Pull Requests' },
	{ key: 'slack', name: 'Slack Workspace Notifications' },
	{ key: 'jira', name: 'Jira Project Tracking' },
	{ key: 'pagerduty', name: 'PagerDuty Incident Response' },
	{ key: 'datadog', name: 'Datadog Observability' }
];

function overflowTooltip() {
	return page.getByCSS('.tooltip-portal-daisy-host.tooltip-surface');
}

async function renderList() {
	await preparePageData();
	return render(VMcpList, {
		items: [
			createVMCP({
				id: 'vmcp-overflow',
				displayName: 'Overflow vMCP',
				components: []
			})
		],
		components: () => componentServers
	});
}

describe('VMcpList.svelte', () => {
	it('shows overflowed component servers in a hover popover instead of a title tooltip', async () => {
		await renderList();

		const card = page.getByRole('button', { name: 'Click to edit Overflow vMCP' });
		(await card.element()).style.width = '220px';

		const more = page.getByLabelText(/more servers$/);
		await expect.element(more).toBeVisible();
		await expect.element(more).not.toHaveAttribute('title');

		(await more.element()).dispatchEvent(new MouseEvent('mouseenter', { bubbles: true }));

		await expect.element(overflowTooltip()).toHaveAttribute('aria-hidden', 'false');
		await expect
			.element(overflowTooltip().getByText('Slack Workspace Notifications'))
			.toBeVisible();
		await expect.element(overflowTooltip().getByText('Datadog Observability')).toBeVisible();
	});
});
