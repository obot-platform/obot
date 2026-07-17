<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import HostedAgentForm from '$lib/components/admin/HostedAgentForm.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import Loading from '$lib/icons/Loading.svelte';
	import { type AgentSource, type HostedAgent } from '$lib/services/admin/types';
	import { AdminService } from '$lib/services/index.js';
	import { errors, profile } from '$lib/stores/index.js';
	import { clearUrlParams, goto } from '$lib/url';
	import { openUrl } from '$lib/utils.js';
	import { Bot, GitBranch, Pencil, Plus, RefreshCcw, Trash2 } from '@lucide/svelte';
	import { onDestroy, untrack } from 'svelte';
	import { SvelteMap, SvelteSet } from 'svelte/reactivity';
	import { fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	let { data } = $props();
	let hostedAgents = $state(untrack(() => data.hostedAgents));
	let agentSources = $state(untrack(() => data.agentSources));
	let agentToDelete = $state<HostedAgent>();
	let sourceToDelete = $state<AgentSource>();

	let isReadonly = $derived(profile.current.isAdminReadonly?.());
	let showCreateNew = $derived(page.url.searchParams.has('new'));
	let view = $derived<'agents' | 'sources'>(
		page.url.searchParams.get('view') === 'sources' ? 'sources' : 'agents'
	);

	// Source form
	let sourceDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let editingSource = $state<AgentSource | undefined>();
	let savingSource = $state(false);
	let sourceForm = $state({ displayName: '', repoURL: '', ref: '' });

	// Sync tracking, mirroring the Skill Sources page: refresh sets an annotation
	// and the controller does the work, so poll until isSyncing clears.
	let syncing = new SvelteSet<string>();
	let syncIntervals = new SvelteMap<string, ReturnType<typeof setInterval>>();

	function clearSyncInterval(id: string) {
		const interval = syncIntervals.get(id);
		if (interval) clearInterval(interval);
		syncIntervals.delete(id);
	}

	onDestroy(() => {
		for (const interval of syncIntervals.values()) clearInterval(interval);
	});

	function pollTillSyncComplete(id: string) {
		clearSyncInterval(id);
		syncIntervals.set(
			id,
			setInterval(async () => {
				try {
					const response = await AdminService.getAgentSource(id);
					if (response && !response.isSyncing) {
						clearSyncInterval(id);
						agentSources = await AdminService.listAgentSources();
						hostedAgents = await AdminService.listHostedAgents({ all: true });
						syncing.delete(id);
					}
				} catch (err) {
					errors.append(`Failed to sync agent source: ${err}`);
					clearSyncInterval(id);
					syncing.delete(id);
				}
			}, 3000)
		);
	}

	async function sync(id: string) {
		syncing.add(id);
		try {
			await AdminService.refreshAgentSource(id);
			pollTillSyncComplete(id);
		} catch (err) {
			errors.append(`Failed to refresh agent source: ${err}`);
			syncing.delete(id);
		}
	}

	function switchView(newView: 'agents' | 'sources') {
		goto(newView === 'sources' ? '/admin/hosted-agents?view=sources' : '/admin/hosted-agents');
	}

	async function navigateToCreated(agent: HostedAgent) {
		clearUrlParams(['new']);
		goto(`/admin/hosted-agents/${agent.id}`, { replaceState: false });
	}

	const duration = PAGE_TRANSITION_DURATION;

	let title = $derived(showCreateNew ? 'Create Agent' : 'Agents');

	let tableData = $derived(
		hostedAgents.map((agent) => ({
			id: agent.id,
			name: agent.name,
			image: agent.image,
			instancing: agent.perUser ? 'Per-user' : 'Shared',
			// Per-user agents are served by their instances, so the agent itself never
			// gets a state of its own — showing "pending" there would look stuck.
			state: agent.perUser ? '' : (agent.status?.state ?? 'pending')
		}))
	);

	let sourceTableData = $derived(
		agentSources.map((source) => ({
			id: source.id,
			displayName: source.displayName,
			repoURL: source.repoURL,
			ref: source.ref || '(default branch)',
			discoveredAgentCount: source.discoveredAgentCount ?? 0,
			syncError: source.syncError ?? '',
			isSyncing: syncing.has(source.id) || Boolean(source.isSyncing)
		}))
	);

	function openCreateSource() {
		editingSource = undefined;
		sourceForm = { displayName: '', repoURL: '', ref: '' };
		sourceDialog?.open();
	}

	function openEditSource(source: AgentSource) {
		editingSource = source;
		sourceForm = {
			displayName: source.displayName,
			repoURL: source.repoURL,
			ref: source.ref ?? ''
		};
		sourceDialog?.open();
	}

	async function saveSource() {
		savingSource = true;
		try {
			const manifest = {
				displayName: sourceForm.displayName,
				repoURL: sourceForm.repoURL,
				ref: sourceForm.ref
			};
			if (editingSource) {
				await AdminService.updateAgentSource(editingSource.id, manifest);
			} else {
				await AdminService.createAgentSource(manifest);
			}
			agentSources = await AdminService.listAgentSources();
			sourceDialog?.close();
		} catch (err) {
			errors.append(`Failed to save agent source: ${err}`);
		} finally {
			savingSource = false;
		}
	}

	let canSaveSource = $derived(Boolean(sourceForm.displayName && sourceForm.repoURL));
</script>

<Layout {title} showBackButton={showCreateNew}>
	<div
		class="h-full w-full"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if showCreateNew}
			{@render createAgentScreen()}
		{:else}
			<div
				class="flex flex-col gap-4"
				in:fly={{ x: 100, delay: duration, duration }}
				out:fly={{ x: -100, duration }}
			>
				<div class="flex w-full">
					<button
						class={twMerge('page-tab max-w-1/2', view === 'agents' && 'page-tab-active')}
						onclick={() => switchView('agents')}
					>
						Agents
					</button>
					<button
						class={twMerge('page-tab max-w-1/2', view === 'sources' && 'page-tab-active')}
						onclick={() => switchView('sources')}
					>
						Sources
					</button>
				</div>

				{#if view === 'agents'}
					{#if hostedAgents.length === 0}
						<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
							<Bot class="text-muted-content size-24 opacity-25" />
							<h4 class="text-muted-content text-lg font-semibold">No agents</h4>
							<p class="text-muted-content text-sm font-light">
								Looks like you haven't registered any agents yet. <br />
								{#if !isReadonly}
									Add one directly, or add a source to discover them from a Git repository.
								{/if}
							</p>
							{@render addAgentButton()}
						</div>
					{:else}
						{@render agentTable()}
					{/if}
				{:else if agentSources.length === 0}
					<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
						<GitBranch class="text-muted-content size-24 opacity-25" />
						<h4 class="text-muted-content text-lg font-semibold">No agent sources</h4>
						<p class="text-muted-content text-sm font-light">
							Add a Git repository to discover agents from it. <br />
							{#if !isReadonly}
								Click the button below to get started.
							{/if}
						</p>
						{@render addSourceButton()}
					</div>
				{:else}
					{@render sourceTable()}
				{/if}
			</div>
		{/if}
	</div>

	{#snippet rightNavActions()}
		{#if !showCreateNew}
			<div class="relative flex items-center gap-4">
				{#if view === 'agents'}
					{@render addAgentButton()}
				{:else}
					{@render addSourceButton()}
				{/if}
			</div>
		{/if}
	{/snippet}
</Layout>

{#snippet agentTable()}
	<Table
		data={tableData}
		fields={['name', 'image', 'instancing', 'state']}
		headers={[
			{ property: 'name', title: 'Name' },
			{ property: 'image', title: 'Image' },
			{ property: 'instancing', title: 'Instancing' },
			{ property: 'state', title: 'State' }
		]}
		onClickRow={(d, isCtrlClick) => {
			openUrl(`/admin/hosted-agents/${d.id}`, isCtrlClick);
		}}
		sortable={['name', 'instancing', 'state']}
	>
		{#snippet onRenderColumn(property, d)}
			{#if property === 'state'}
				{#if d.state}
					<span
						class="badge badge-sm {d.state === 'ready'
							? 'badge-success'
							: d.state === 'error'
								? 'badge-error'
								: 'badge-secondary'}"
					>
						{d.state}
					</span>
				{:else}
					<span class="text-muted-content">-</span>
				{/if}
			{:else}
				{d[property as keyof typeof d]}
			{/if}
		{/snippet}
		{#snippet actions(d)}
			{#if !isReadonly}
				<IconButton
					variant="danger"
					onclick={(e) => {
						e.stopPropagation();
						agentToDelete = hostedAgents.find((a) => a.id === d.id);
					}}
					tooltip={{ text: 'Delete Agent' }}
				>
					<Trash2 class="size-4" />
				</IconButton>
			{/if}
		{/snippet}
	</Table>
{/snippet}

{#snippet sourceTable()}
	<Table
		data={sourceTableData}
		fields={['displayName', 'repoURL', 'ref', 'discoveredAgentCount']}
		headers={[
			{ property: 'displayName', title: 'Name' },
			{ property: 'repoURL', title: 'Repository' },
			{ property: 'ref', title: 'Ref' },
			{ property: 'discoveredAgentCount', title: 'Agents' }
		]}
		sortable={['displayName', 'repoURL']}
		noDataMessage="No sources added."
	>
		{#snippet onRenderColumn(property, d)}
			{#if property === 'displayName'}
				<div class="flex items-center gap-2">
					<span>{d.displayName}</span>
					{#if d.isSyncing}
						<Loading class="size-3" />
					{:else if d.syncError}
						<span class="badge badge-error badge-xs" title={d.syncError}>sync error</span>
					{/if}
				</div>
			{:else}
				{d[property as keyof typeof d]}
			{/if}
		{/snippet}
		{#snippet actions(d)}
			{#if !isReadonly}
				<IconButton
					onclick={(e) => {
						e.stopPropagation();
						sync(d.id);
					}}
					disabled={d.isSyncing}
					tooltip={{ text: 'Sync Now' }}
				>
					<RefreshCcw class="size-4" />
				</IconButton>
				<IconButton
					onclick={(e) => {
						e.stopPropagation();
						const source = agentSources.find((s) => s.id === d.id);
						if (source) openEditSource(source);
					}}
					tooltip={{ text: 'Edit Source' }}
				>
					<Pencil class="size-4" />
				</IconButton>
				<IconButton
					variant="danger"
					onclick={(e) => {
						e.stopPropagation();
						sourceToDelete = agentSources.find((s) => s.id === d.id);
					}}
					tooltip={{ text: 'Delete Source' }}
				>
					<Trash2 class="size-4" />
				</IconButton>
			{/if}
		{/snippet}
	</Table>
{/snippet}

{#snippet addAgentButton()}
	{#if !isReadonly}
		<button
			class="btn btn-primary flex items-center gap-1 text-sm"
			onclick={() => goto(`/admin/hosted-agents?new=true`)}
		>
			<Plus class="size-4" /> Add Agent
		</button>
	{/if}
{/snippet}

{#snippet addSourceButton()}
	{#if !isReadonly}
		<button class="btn btn-primary flex items-center gap-1 text-sm" onclick={openCreateSource}>
			<Plus class="size-4" /> Add Source
		</button>
	{/if}
{/snippet}

{#snippet createAgentScreen()}
	<div
		class="h-full w-full"
		in:fly={{ x: 100, delay: duration, duration }}
		out:fly={{ x: -100, duration }}
	>
		<HostedAgentForm onCreate={navigateToCreated} readonly={isReadonly} />
	</div>
{/snippet}

<ResponsiveDialog
	bind:this={sourceDialog}
	title={editingSource ? 'Edit Source' : 'Add Source'}
	class="md:max-w-md"
>
	<div class="flex flex-col gap-4">
		<div class="flex flex-col gap-2">
			<label for="source-name" class="text-sm font-light">Name</label>
			<input id="source-name" bind:value={sourceForm.displayName} class="text-input-filled" />
		</div>
		<div class="flex flex-col gap-2">
			<label for="source-repo" class="text-sm font-light">Repository URL</label>
			<input
				id="source-repo"
				bind:value={sourceForm.repoURL}
				class="text-input-filled"
				placeholder="https://github.com/obot-platform/agents"
				inputmode="url"
				autocomplete="off"
			/>
		</div>
		<div class="flex flex-col gap-2">
			<label for="source-ref" class="text-sm font-light">Ref</label>
			<input
				id="source-ref"
				bind:value={sourceForm.ref}
				class="text-input-filled"
				placeholder="(default branch)"
				autocomplete="off"
			/>
			<span class="text-muted-content text-xs">Branch or tag. Leave blank for the default.</span>
		</div>
	</div>
	<div class="flex justify-end gap-2 pt-4">
		<button class="btn btn-secondary text-sm" onclick={() => sourceDialog?.close()}>Cancel</button>
		<button
			class="btn btn-primary text-sm"
			disabled={!canSaveSource || savingSource}
			onclick={saveSource}
		>
			{#if savingSource}
				<Loading class="size-4" />
			{:else}
				{editingSource ? 'Update' : 'Add'}
			{/if}
		</button>
	</div>
</ResponsiveDialog>

<Confirm
	msg={`Delete ${agentToDelete?.name || 'this agent'}?`}
	show={Boolean(agentToDelete)}
	onsuccess={async () => {
		if (!agentToDelete) return;
		await AdminService.deleteHostedAgent(agentToDelete.id);
		hostedAgents = await AdminService.listHostedAgents({ all: true });
		agentToDelete = undefined;
	}}
	oncancel={() => (agentToDelete = undefined)}
/>

<Confirm
	msg={`Delete ${sourceToDelete?.displayName || 'this source'}?`}
	note="Agents discovered from this source will be removed."
	show={Boolean(sourceToDelete)}
	onsuccess={async () => {
		if (!sourceToDelete) return;
		await AdminService.deleteAgentSource(sourceToDelete.id);
		agentSources = await AdminService.listAgentSources();
		hostedAgents = await AdminService.listHostedAgents({ all: true });
		sourceToDelete = undefined;
	}}
	oncancel={() => (sourceToDelete = undefined)}
/>

<svelte:head>
	<title>Obot | Agents</title>
</svelte:head>
