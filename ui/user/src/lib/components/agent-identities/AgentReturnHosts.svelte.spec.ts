import { HttpError } from '$lib/errors';
import AgentReturnHosts from './AgentReturnHosts.svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const warning = /No return hosts are set, so this agent can't ask users for approval/;

describe('AgentReturnHosts', () => {
	it('warns that the agent can not ask for approval when there are no hosts', async () => {
		render(AgentReturnHosts, { hosts: [], onSave: vi.fn() });

		await expect.element(page.getByText(warning)).toBeVisible();
	});

	it('lists the hosts, without the warning', async () => {
		render(AgentReturnHosts, { hosts: ['agent.example.com', 'localhost:3000'], onSave: vi.fn() });

		await expect.element(page.getByText('agent.example.com')).toBeVisible();
		await expect.element(page.getByText('localhost:3000')).toBeVisible();
		await expect.element(page.getByText(warning)).not.toBeInTheDocument();
	});

	it('adds and removes hosts, and saves the whole list', async () => {
		const onSave = vi.fn().mockResolvedValue(undefined);
		render(AgentReturnHosts, { hosts: ['agent.example.com'], onSave });
		const save = page.getByRole('button', { name: 'Save Hosts' });
		await expect.element(save).toBeDisabled();

		await page.getByLabelText('Host').fill('localhost:3000');
		await page.getByRole('button', { name: 'Add Host' }).click();
		await page.getByRole('button', { name: 'Remove agent.example.com' }).click();
		await save.click();

		await vi.waitFor(() => expect(onSave).toHaveBeenCalledWith(['localhost:3000']));
	});

	it('saves a host that was typed but not added', async () => {
		const onSave = vi.fn().mockResolvedValue(undefined);
		render(AgentReturnHosts, { hosts: [], onSave });

		await page.getByLabelText('Host').fill('agent.example.com');
		await page.getByRole('button', { name: 'Save Hosts' }).click();

		await vi.waitFor(() => expect(onSave).toHaveBeenCalledWith(['agent.example.com']));
	});

	it('shows the server error next to the hosts', async () => {
		const onSave = vi
			.fn()
			.mockRejectedValue(
				new HttpError(
					400,
					'400 /agents/7: invalid allowedReturnHosts: invalid allowed return host "a b": it must be a host name or IP address, with an optional port, and nothing else'
				)
			);
		render(AgentReturnHosts, { hosts: [], onSave });

		await page.getByLabelText('Host').fill('a b');
		await page.getByRole('button', { name: 'Add Host' }).click();
		await page.getByRole('button', { name: 'Save Hosts' }).click();

		await expect
			.element(page.getByRole('alert'))
			.toHaveTextContent(
				'invalid allowed return host "a b": it must be a host name or IP address, with an optional port, and nothing else'
			);
	});

	it('can not be edited by read-only admins and auditors', async () => {
		render(AgentReturnHosts, { hosts: ['agent.example.com'], readonly: true, onSave: vi.fn() });

		await expect.element(page.getByText('agent.example.com')).toBeVisible();
		await expect.element(page.getByLabelText('Host')).not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Save Hosts' })).not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Remove agent.example.com' }))
			.not.toBeInTheDocument();
	});
});
