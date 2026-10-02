<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { formatDeviceCommand } from '$lib/format.js';
	import { m } from '$lib/i18n';
	import type { DeviceClientFleetSummary } from '$lib/services';
	import { goto } from '$lib/url';
	import { openUrl } from '$lib/utils.js';
	import { CheckIcon, PencilRuler, Server, Users, XIcon } from '@lucide/svelte';
	import { fly } from 'svelte/transition';

	type TabIcon = typeof Users | typeof Server | typeof PencilRuler;

	let { data } = $props();

	type Tab = 'mcp' | 'skills' | 'users';

	let client = $derived<DeviceClientFleetSummary | null | undefined>(data.client);
	let userMap = $derived(new Map(data?.users?.map((u) => [u.id, u]) ?? []));
	let detail = $derived({
		...(client ?? {}),
		users:
			client?.users?.map(
				(u) => userMap.get(u) ?? { id: u, displayName: u, email: u, username: u }
			) ?? []
	});
	let hasMcpServers = $derived((client?.mcpServers?.length ?? 0) > 0);
	let hasSkills = $derived((client?.skills?.length ?? 0) > 0);
	let clientName = $derived(page.params.name ?? '');

	let activeTab = $state<Tab>('users');

	const duration = PAGE_TRANSITION_DURATION;
</script>

<svelte:head>
	<title>Obot | {clientName}</title>
</svelte:head>

<Layout
	title={clientName}
	showBackButton
	onBackButtonClick={() => {
		if (typeof window !== 'undefined' && window.history.length > 1) {
			window.history.back();
		} else {
			goto(resolve('/inventory?view=device-clients'));
		}
	}}
>
	<div
		class="flex flex-col gap-6"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if !client}
			<p class="text-muted-content text-sm font-light">{m.routes_invd_client_not_found()}</p>
		{:else}
			<div class="dark:bg-base-300 bg-base-100 flex flex-col gap-4 rounded-md p-4 shadow-sm">
				<div class="flex flex-col gap-2">
					<h2 class="flex items-center gap-2 text-xl font-semibold">
						{detail.name}
					</h2>
					<div class="text-muted-content flex flex-wrap items-center gap-3 text-xs">
						<span
							>{detail.users.length === 1
								? m.routes_invd_users_count_one({ count: detail.users.length })
								: m.routes_invd_users_count_other({ count: detail.users.length })}</span
						>
						<span>·</span>
						{#if detail.mcpServers}
							<span
								>{detail.mcpServers.length === 1
									? m.routes_invd_mcp_servers_count_one({ count: detail.mcpServers.length })
									: m.routes_invd_mcp_servers_count_other({
											count: detail.mcpServers.length
										})}</span
							>
						{/if}
						{#if detail.skills}
							<span>·</span>
							<span
								>{detail.skills.length === 1
									? m.routes_invd_skills_count_one({ count: detail.skills.length })
									: m.routes_invd_skills_count_other({ count: detail.skills.length })}</span
							>
						{/if}
					</div>
				</div>
			</div>

			<div class="flex flex-col gap-2">
				<div class="border-base-300 dark:border-base-400 flex gap-2 border-b">
					{@render tabButton('users', Users, m.routes_invd_tab_users(), detail.users.length)}
					{@render tabButton(
						'mcp',
						Server,
						m.routes_invd_tab_mcp_servers(),
						detail.mcpServers?.length ?? 0
					)}
					{@render tabButton(
						'skills',
						PencilRuler,
						m.routes_invd_tab_skills(),
						detail.skills?.length ?? 0
					)}
				</div>

				{#if activeTab === 'users'}
					<Table
						data={detail.users}
						fields={['email']}
						headers={[{ title: m.routes_invd_col_user(), property: 'email' }]}
					>
						{#snippet onRenderColumn(property, d)}
							{#if property === 'email'}
								{d.displayName || d.email || '-'}
							{:else}
								{d[property as keyof (typeof detail.users)[number]]}
							{/if}
						{/snippet}
					</Table>
				{:else if activeTab === 'mcp'}
					{#if !hasMcpServers}
						{@render emptyTab(m.routes_invd_no_mcp_for_client())}
					{:else}
						{@const rows = detail.mcpServers!.map((s, i) => ({
							...s,
							id: `${client.name}-${s.name}-${i}`,
							endpoint:
								s.transport === 'stdio' ? formatDeviceCommand(s.command, s.args) : s.url || '—'
						}))}
						<Table
							data={rows}
							fields={['name', 'transport', 'endpoint']}
							headers={[
								{ title: m.routes_invd_col_name(), property: 'name' },
								{ title: m.routes_invd_col_transport(), property: 'transport' },
								{ title: m.routes_invd_col_endpoint(), property: 'endpoint' }
							]}
							onClickRow={(d, isCtrlClick) => {
								if (!d.configHash) {
									console.error('No config hash found for MCP server', d);
									return;
								}
								openUrl(
									resolve(`/inventory/mcp-servers/${encodeURIComponent(d.configHash)}`),
									isCtrlClick
								);
							}}
						>
							{#snippet onRenderColumn(property, d)}
								{d[property as keyof (typeof rows)[number]] ?? '—'}
							{/snippet}
						</Table>
					{/if}
				{:else if activeTab === 'skills'}
					{#if !hasSkills}
						{@render emptyTab(m.routes_invd_no_skills_for_client())}
					{:else}
						{@const rows = detail.skills!.map((s, i) => ({
							...s,
							id: `${client.name}-${s.name}-${i}`
						}))}

						<Table
							data={rows}
							fields={['name', 'description', 'hasScripts', 'files']}
							headers={[
								{ title: m.routes_invd_col_name(), property: 'name' },
								{ title: m.routes_invd_col_description(), property: 'description' },
								{ title: m.routes_invd_col_has_scripts(), property: 'hasScripts' },
								{ title: m.routes_invd_col_files(), property: 'files' }
							]}
							onClickRow={(d, isCtrlClick) => {
								openUrl(resolve(`/inventory/skills/${encodeURIComponent(d.name)}`), isCtrlClick);
							}}
						>
							{#snippet onRenderColumn(property, d)}
								{#if property === 'name'}
									{d.name}
								{:else if property === 'description'}
									<span class="text-muted-content text-xs">{d.description ?? '—'}</span>
								{:else if property === 'hasScripts'}
									{#if d.hasScripts}
										<CheckIcon class="text-primary size-3 shrink-0" />
									{:else}
										<XIcon class="text-muted-content size-3 shrink-0" />
									{/if}
								{:else if property === 'files'}
									{d.files ?? '-'}
								{/if}
							{/snippet}
						</Table>
					{/if}
				{/if}
			</div>
		{/if}
	</div>
</Layout>

{#snippet tabButton(tab: Tab, Icon: TabIcon, label: string, count: number)}
	<button class="tab-button" class:tab-active={activeTab === tab} onclick={() => (activeTab = tab)}>
		<Icon class="size-4" />
		{label}
		<span class="text-muted-content">({count})</span>
	</button>
{/snippet}

{#snippet emptyTab(msg: string)}
	<div class="text-muted-content flex items-center gap-2 p-4 text-sm font-light">
		{msg}
	</div>
{/snippet}
