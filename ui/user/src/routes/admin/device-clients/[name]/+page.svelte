<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { formatDeviceCommand } from '$lib/format.js';
	import type { DeviceClientFleetSummary } from '$lib/services';
	import { goto } from '$lib/url';
	import { openUrl } from '$lib/utils.js';
	import { CheckIcon, PencilRuler, Server, Users, XIcon } from 'lucide-svelte';
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
			goto(resolve('/admin/device-clients'));
		}
	}}
>
	<div
		class="flex flex-col gap-6"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if !client}
			<p class="text-on-surface1 text-sm font-light">Client not found.</p>
		{:else}
			<div class="dark:bg-surface2 bg-background flex flex-col gap-4 rounded-md p-4 shadow-sm">
				<div class="flex flex-col gap-2">
					<h2 class="flex items-center gap-2 text-xl font-semibold">
						{detail.name}
					</h2>
					<div class="text-on-surface1 flex flex-wrap items-center gap-3 text-xs">
						<span>{detail.users.length} user{detail.users.length === 1 ? '' : 's'}</span>
						<span>·</span>
						{#if detail.mcpServers}
							<span
								>{detail.mcpServers.length} mcp server{detail.mcpServers.length === 1
									? ''
									: 's'}</span
							>
						{/if}
						{#if detail.skills}
							<span>·</span>
							<span>{detail.skills.length} skill{detail.skills.length === 1 ? '' : 's'}</span>
						{/if}
					</div>
				</div>
			</div>

			<div class="flex flex-col gap-2">
				<div class="border-surface2 dark:border-surface2 flex gap-2 border-b">
					{@render tabButton('users', Users, 'Users', detail.users.length)}
					{@render tabButton('mcp', Server, 'MCP Servers', detail.mcpServers?.length ?? 0)}
					{@render tabButton('skills', PencilRuler, 'Skills', detail.skills?.length ?? 0)}
				</div>

				{#if activeTab === 'users'}
					<Table
						data={detail.users}
						fields={['email']}
						headers={[{ title: 'User', property: 'email' }]}
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
						{@render emptyTab('No MCP servers found for this client.')}
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
							onClickRow={(d, isCtrlClick) => {
								if (!d.configHash) {
									console.error('No config hash found for MCP server', d);
									return;
								}
								openUrl(
									resolve(`/admin/device-mcp-servers/${encodeURIComponent(d.configHash)}`),
									isCtrlClick
								);
							}}
						>
							{#snippet onRenderColumn(property, d)}
								{#if property === 'name'}
									<span class="font-mono text-xs">{d.name}</span>
								{:else if property === 'endpoint'}
									<span class="font-mono text-xs">{d.endpoint}</span>
								{:else}
									{d[property as keyof (typeof rows)[number]] ?? '—'}
								{/if}
							{/snippet}
						</Table>
					{/if}
				{:else if activeTab === 'skills'}
					{#if !hasSkills}
						{@render emptyTab('No skills found for this client.')}
					{:else}
						{@const rows = detail.skills!.map((s, i) => ({
							...s,
							id: `${client.name}-${s.name}-${i}`
						}))}

						<Table
							data={rows}
							fields={['name', 'description', 'hasScripts', 'files']}
							headers={[
								{ title: 'Name', property: 'name' },
								{ title: 'Description', property: 'description' },
								{ title: 'Has Scripts', property: 'hasScripts' }
							]}
							onClickRow={(d, isCtrlClick) => {
								openUrl(resolve(`/admin/device-skills/${encodeURIComponent(d.name)}`), isCtrlClick);
							}}
						>
							{#snippet onRenderColumn(property, d)}
								{#if property === 'name'}
									{d.name}
								{:else if property === 'description'}
									<span class="text-on-surface1 text-xs">{d.description ?? '—'}</span>
								{:else if property === 'hasScripts'}
									{#if d.hasScripts}
										<CheckIcon class="text-primary size-3 shrink-0" />
									{:else}
										<XIcon class="text-on-surface1 size-3 shrink-0" />
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
		<span class="text-on-surface1">({count})</span>
	</button>
{/snippet}

{#snippet emptyTab(msg: string)}
	<div class="text-on-surface1 flex items-center gap-2 p-4 text-sm font-light">
		{msg}
	</div>
{/snippet}
