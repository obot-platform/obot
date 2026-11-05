import { preparePageData } from '../../tests/helpers/pageData';
import AuditLogs from './+page.svelte';
import { expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

it.each([
	{ engine: 'docker' as const, enabled: false, visible: false },
	{ engine: 'kubernetes' as const, enabled: false, visible: false },
	{ engine: 'kubernetes' as const, enabled: true, visible: true }
])(
	'shows Claude audit logs for $engine with enabled=$enabled',
	async ({ engine, enabled, visible }) => {
		await preparePageData({ version: { engine, claudeEnvironmentsEnabled: enabled } });
		await render(AuditLogs);
		const tab = page.getByRole('button', { name: 'Environments', exact: true });
		if (visible) await expect.element(tab).toBeVisible();
		else await expect.element(tab).not.toBeInTheDocument();
	}
);
