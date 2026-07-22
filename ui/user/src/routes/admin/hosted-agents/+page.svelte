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
	import { type AgentSource, type Harness, type HostedAgent } from '$lib/services/admin/types';
	import { AdminService } from '$lib/services/index.js';
	import { errors, profile } from '$lib/stores/index.js';
	import { clearUrlParams, goto } from '$lib/url';
	import { openUrl } from '$lib/utils.js';
	import { Bot, Cpu, GitBranch, Pencil, Plus, RefreshCcw, Trash2 } from '@lucide/svelte';
	import { onDestroy, untrack } from 'svelte';
	import { SvelteMap, SvelteSet } from 'svelte/reactivity';
	import { fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	let { data } = $props();
	let hostedAgents = $state(untrack(() => data.hostedAgents));
	let agentSources = $state(untrack(() => data.agentSources));
	let harnesses = $state(untrack(() => data.harnesses));
	let agentToDelete = $state<HostedAgent>();
	let sourceToDelete = $state<AgentSource>();
	let harnessToDelete = $state<Harness>();

	let isReadonly = $derived(profile.current.isAdminReadonly?.());
	let showCreateNew = $derived(page.url.searchParams.has('new'));
	let view = $derived<'agents' | 'sources' | 'harnesses'>(
		page.url.searchParams.get('view') === 'sources'
			? 'sources'
			: page.url.searchParams.get('view') === 'harnesses'
				? 'harnesses'
				: 'agents'
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
						harnesses = await AdminService.listHarnesses();
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

	function switchView(newView: 'agents' | 'sources' | 'harnesses') {
		goto(newView === 'agents' ? '/admin/hosted-agents' : `/admin/hosted-agents?view=${newView}`);
	}

	async function navigateToCreated(agent: HostedAgent) {
		clearUrlParams(['new']);
		goto(`/admin/hosted-agents/${agent.id}`, { replaceState: false });
	}

	const duration = PAGE_TRANSITION_DURATION;

	let title = $derived(showCreateNew ? 'Create Agent' : 'Agents');

	let harnessesById = $derived(new Map(harnesses.map((h) => [h.id, h])));

	let tableData = $derived(
		hostedAgents.map((agent) => ({
			id: agent.id,
			name: agent.name,
			harness: harnessesById.get(agent.harnessID)?.name ?? agent.harnessID
		}))
	);

	let harnessTableData = $derived(
		harnesses.map((harness) => ({
			id: harness.id,
			name: harness.name,
			description: harness.description ?? '',
			image: harness.image
		}))
	);

	let sourceTableData = $derived(
		agentSources.map((source) => ({
			id: source.id,
			displayName: source.displayName,
			repoURL: source.repoURL,
			ref: source.ref || '(default branch)',
			discoveredAgentCount: source.discoveredAgentCount ?? 0,
			discoveredHarnessCount: source.discoveredHarnessCount ?? 0,
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

	// Harness form
	let harnessDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let editingHarness = $state<Harness | undefined>();
	let savingHarness = $state(false);
	let harnessForm = $state({ name: '', description: '', icon: '', iconDark: '', image: '' });

	function openCreateHarness() {
		editingHarness = undefined;
		harnessForm = { name: '', description: '', icon: '', iconDark: '', image: '' };
		harnessDialog?.open();
	}

	function openEditHarness(harness: Harness) {
		editingHarness = harness;
		harnessForm = {
			name: harness.name,
			description: harness.description ?? '',
			icon: harness.icon ?? '',
			iconDark: harness.iconDark ?? '',
			image: harness.image
		};
		harnessDialog?.open();
	}

	async function saveHarness() {
		savingHarness = true;
		try {
			const manifest = {
				name: harnessForm.name,
				description: harnessForm.description,
				icon: harnessForm.icon,
				iconDark: harnessForm.iconDark,
				image: harnessForm.image
			};
			if (editingHarness) {
				await AdminService.updateHarness(editingHarness.id, manifest);
			} else {
				await AdminService.createHarness(manifest);
			}
			harnesses = await AdminService.listHarnesses();
			harnessDialog?.close();
		} catch (err) {
			errors.append(`Failed to save harness: ${err}`);
		} finally {
			savingHarness = false;
		}
	}

	let canSaveHarness = $derived(Boolean(harnessForm.name && harnessForm.image));
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
					<button
						class={twMerge('page-tab max-w-1/2', view === 'harnesses' && 'page-tab-active')}
						onclick={() => switchView('harnesses')}
					>
						Harnesses
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
				{:else if view === 'sources'}
					{#if agentSources.length === 0}
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
				{:else if harnesses.length === 0}
					<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
						<Cpu class="text-muted-content size-24 opacity-25" />
						<h4 class="text-muted-content text-lg font-semibold">No harnesses</h4>
						<p class="text-muted-content text-sm font-light">
							Harnesses are the runtimes agents are built on, such as Claude Code or Codex. <br />
							{#if !isReadonly}
								Add one before registering agents.
							{/if}
						</p>
						{@render addHarnessButton()}
					</div>
				{:else}
					{@render harnessTable()}
				{/if}
			</div>
		{/if}
	</div>

	{#snippet rightNavActions()}
		{#if !showCreateNew}
			<div class="relative flex items-center gap-4">
				{#if view === 'agents'}
					{@render addAgentButton()}
				{:else if view === 'sources'}
					{@render addSourceButton()}
				{:else}
					{@render addHarnessButton()}
				{/if}
			</div>
		{/if}
	{/snippet}
</Layout>

{#snippet agentTable()}
	<Table
		data={tableData}
		fields={['name', 'harness']}
		headers={[
			{ property: 'name', title: 'Name' },
			{ property: 'harness', title: 'Harness' }
		]}
		onClickRow={(d, isCtrlClick) => {
			openUrl(`/admin/hosted-agents/${d.id}`, isCtrlClick);
		}}
		sortable={['name', 'harness']}
	>
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
		fields={['displayName', 'repoURL', 'ref', 'discoveredAgentCount', 'discoveredHarnessCount']}
		headers={[
			{ property: 'displayName', title: 'Name' },
			{ property: 'repoURL', title: 'Repository' },
			{ property: 'ref', title: 'Ref' },
			{ property: 'discoveredAgentCount', title: 'Agents' },
			{ property: 'discoveredHarnessCount', title: 'Harnesses' }
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

{#snippet harnessTable()}
	<Table
		data={harnessTableData}
		fields={['name', 'description', 'image']}
		headers={[
			{ property: 'name', title: 'Name' },
			{ property: 'description', title: 'Description' },
			{ property: 'image', title: 'Image' }
		]}
		sortable={['name', 'image']}
		noDataMessage="No harnesses added."
	>
		{#snippet actions(d)}
			{#if !isReadonly}
				<IconButton
					onclick={(e) => {
						e.stopPropagation();
						const harness = harnesses.find((h) => h.id === d.id);
						if (harness) openEditHarness(harness);
					}}
					tooltip={{ text: 'Edit Harness' }}
				>
					<Pencil class="size-4" />
				</IconButton>
				<IconButton
					variant="danger"
					onclick={(e) => {
						e.stopPropagation();
						harnessToDelete = harnesses.find((h) => h.id === d.id);
					}}
					tooltip={{ text: 'Delete Harness' }}
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

{#snippet addHarnessButton()}
	{#if !isReadonly}
		<button class="btn btn-primary flex items-center gap-1 text-sm" onclick={openCreateHarness}>
			<Plus class="size-4" /> Add Harness
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

<ResponsiveDialog
	bind:this={harnessDialog}
	title={editingHarness ? 'Edit Harness' : 'Add Harness'}
	class="md:max-w-md"
>
	<div class="flex flex-col gap-4">
		<div class="flex flex-col gap-2">
			<label for="harness-name" class="text-sm font-light">Name</label>
			<input
				id="harness-name"
				bind:value={harnessForm.name}
				class="text-input-filled"
				placeholder="Claude Code"
			/>
		</div>
		<div class="flex flex-col gap-2">
			<label for="harness-description" class="text-sm font-light">Description</label>
			<textarea
				id="harness-description"
				bind:value={harnessForm.description}
				class="text-input-filled"
				rows="2"
			></textarea>
		</div>
		<div class="flex flex-col gap-2">
			<label for="harness-image" class="text-sm font-light">Docker Image</label>
			<input
				id="harness-image"
				bind:value={harnessForm.image}
				class="text-input-filled"
				placeholder="ghcr.io/example/claude-code:latest"
				autocomplete="off"
			/>
		</div>
		<div class="flex flex-col gap-2">
			<label for="harness-icon" class="text-sm font-light">Icon URL</label>
			<div class="flex items-center gap-3">
				{#if harnessForm.icon}
					<img src={harnessForm.icon} alt="" class="size-10 shrink-0 rounded-md object-contain" />
				{/if}
				<input
					type="text"
					id="harness-icon"
					bind:value={harnessForm.icon}
					class="text-input-filled grow"
					inputmode="url"
					autocomplete="off"
				/>
			</div>
		</div>
		<div class="flex flex-col gap-2">
			<label for="harness-icon-dark" class="text-sm font-light">Icon URL (Dark)</label>
			<div class="flex items-center gap-3">
				{#if harnessForm.iconDark}
					<img
						src={harnessForm.iconDark}
						alt=""
						class="bg-base-300 size-10 shrink-0 rounded-md object-contain"
					/>
				{/if}
				<input
					type="text"
					id="harness-icon-dark"
					bind:value={harnessForm.iconDark}
					class="text-input-filled grow"
					inputmode="url"
					autocomplete="off"
				/>
			</div>
		</div>
	</div>
	<div class="flex justify-end gap-2 pt-4">
		<button class="btn btn-secondary text-sm" onclick={() => harnessDialog?.close()}>Cancel</button>
		<button
			class="btn btn-primary text-sm"
			disabled={!canSaveHarness || savingHarness}
			onclick={saveHarness}
		>
			{#if savingHarness}
				<Loading class="size-4" />
			{:else}
				{editingHarness ? 'Update' : 'Add'}
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
	note="Agents and harnesses discovered from this source will be removed."
	show={Boolean(sourceToDelete)}
	onsuccess={async () => {
		if (!sourceToDelete) return;
		await AdminService.deleteAgentSource(sourceToDelete.id);
		agentSources = await AdminService.listAgentSources();
		hostedAgents = await AdminService.listHostedAgents({ all: true });
		harnesses = await AdminService.listHarnesses();
		sourceToDelete = undefined;
	}}
	oncancel={() => (sourceToDelete = undefined)}
/>

<Confirm
	msg={`Delete ${harnessToDelete?.name || 'this harness'}?`}
	note="A harness that agents still run on cannot be deleted."
	show={Boolean(harnessToDelete)}
	onsuccess={async () => {
		if (!harnessToDelete) return;
		try {
			await AdminService.deleteHarness(harnessToDelete.id);
			harnesses = await AdminService.listHarnesses();
		} catch (err) {
			errors.append(`Failed to delete harness: ${err}`);
		}
		harnessToDelete = undefined;
	}}
	oncancel={() => (harnessToDelete = undefined)}
/>

<svelte:head>
	<title>Obot | Agents</title>
</svelte:head>
