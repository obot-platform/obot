<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import HostedAgentForm from '$lib/components/admin/HostedAgentForm.svelte';
	import HostedAgentPools from '$lib/components/admin/HostedAgentPools.svelte';
	import AgentIcon from '$lib/components/hosted-agents/AgentIcon.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import Loading from '$lib/icons/Loading.svelte';
	import { type AgentCatalog, type Harness, type HostedAgent } from '$lib/services/admin/types';
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
	let agentCatalogs = $state(untrack(() => data.agentCatalogs));
	let harnesses = $state(untrack(() => data.harnesses));
	let pools = $state(untrack(() => data.pools));
	let assignments = $state(untrack(() => data.assignments));
	let poolDefaults = $state(untrack(() => data.poolDefaults));
	let agentToDelete = $state<HostedAgent>();
	let catalogToDelete = $state<AgentCatalog>();
	let harnessToDelete = $state<Harness>();

	let isReadonly = $derived(profile.current.isAdminReadonly?.());
	let showCreateNew = $derived(page.url.searchParams.has('new'));
	let view = $derived<'agents' | 'catalogs' | 'harnesses' | 'pools'>(
		page.url.searchParams.get('view') === 'catalogs'
			? 'catalogs'
			: page.url.searchParams.get('view') === 'harnesses'
				? 'harnesses'
				: page.url.searchParams.get('view') === 'pools'
					? 'pools'
					: 'agents'
	);

	// Source form
	let catalogDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let editingCatalog = $state<AgentCatalog | undefined>();
	let savingCatalog = $state(false);
	let catalogForm = $state({ displayName: '', repoURL: '', ref: '' });

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
					const response = await AdminService.getAgentCatalog(id);
					if (response && !response.isSyncing) {
						clearSyncInterval(id);
						agentCatalogs = await AdminService.listAgentCatalogs();
						hostedAgents = await AdminService.listHostedAgents({ all: true });
						harnesses = await AdminService.listHarnesses();
						syncing.delete(id);
					}
				} catch (err) {
					errors.append(`Failed to sync config source: ${err}`);
					clearSyncInterval(id);
					syncing.delete(id);
				}
			}, 3000)
		);
	}

	async function sync(id: string) {
		syncing.add(id);
		try {
			await AdminService.refreshAgentCatalog(id);
			pollTillSyncComplete(id);
		} catch (err) {
			errors.append(`Failed to refresh config source: ${err}`);
			syncing.delete(id);
		}
	}

	function switchView(newView: 'agents' | 'catalogs' | 'harnesses' | 'pools') {
		goto(newView === 'agents' ? '/admin/hosted-agents' : `/admin/hosted-agents?view=${newView}`);
	}

	async function navigateToCreated(agent: HostedAgent) {
		clearUrlParams(['new']);
		goto(`/admin/hosted-agents/${agent.id}`, { replaceState: false });
	}

	const duration = PAGE_TRANSITION_DURATION;

	let title = $derived(showCreateNew ? 'Create Agent Template' : 'Hosted Agents');

	const viewDescriptions = {
		agents:
			'A template describes an agent someone can launch: the harness it runs on, the MCP servers, skills and models it may use, and anything the user is asked when they create one.',
		harnesses:
			'A harness is the container image an agent runs inside — its runtime and preinstalled tooling, such as Claude Code. Templates pick a harness; the harness decides the image and whether the agent gets an interactive shell.',
		pools:
			'A pool is a shared bucket of CPU and memory. Every agent placed in it draws from the same budget and can borrow whatever its neighbours are not using, so agents have no fixed size of their own.',
		catalogs:
			'A config source is a Git repository Obot syncs templates and harnesses from, so they are defined in version control rather than added by hand here.'
	};
	let viewDescription = $derived(viewDescriptions[view]);

	let harnessesById = $derived(new Map(harnesses.map((h) => [h.id, h])));

	// A template's own icon, falling back to its harness's: a config source
	// often gives the harness the brand mark and leaves the template plain.
	let tableData = $derived(
		hostedAgents.map((agent) => ({
			id: agent.id,
			name: agent.name,
			harness: harnessesById.get(agent.harnessID)?.name ?? agent.harnessID,
			icon: agent.icon || harnessesById.get(agent.harnessID)?.icon || '',
			iconDark: agent.iconDark || harnessesById.get(agent.harnessID)?.iconDark || ''
		}))
	);

	let harnessTableData = $derived(
		harnesses.map((harness) => ({
			id: harness.id,
			name: harness.name,
			description: harness.description ?? '',
			image: harness.image,
			icon: harness.icon ?? '',
			iconDark: harness.iconDark ?? ''
		}))
	);

	let catalogTableData = $derived(
		agentCatalogs.map((source) => ({
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

	function openCreateCatalog() {
		editingCatalog = undefined;
		catalogForm = { displayName: '', repoURL: '', ref: '' };
		catalogDialog?.open();
	}

	function openEditCatalog(source: AgentCatalog) {
		editingCatalog = source;
		catalogForm = {
			displayName: source.displayName,
			repoURL: source.repoURL,
			ref: source.ref ?? ''
		};
		catalogDialog?.open();
	}

	async function saveCatalog() {
		savingCatalog = true;
		try {
			const manifest = {
				displayName: catalogForm.displayName,
				repoURL: catalogForm.repoURL,
				ref: catalogForm.ref
			};
			if (editingCatalog) {
				await AdminService.updateAgentCatalog(editingCatalog.id, manifest);
			} else {
				await AdminService.createAgentCatalog(manifest);
			}
			agentCatalogs = await AdminService.listAgentCatalogs();
			catalogDialog?.close();
		} catch (err) {
			errors.append(`Failed to save config source: ${err}`);
		} finally {
			savingCatalog = false;
		}
	}

	let canSaveCatalog = $derived(Boolean(catalogForm.displayName && catalogForm.repoURL));

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
						Templates
					</button>
					<button
						class={twMerge('page-tab max-w-1/2', view === 'harnesses' && 'page-tab-active')}
						onclick={() => switchView('harnesses')}
					>
						Harnesses
					</button>
					<button
						class={twMerge('page-tab max-w-1/2', view === 'pools' && 'page-tab-active')}
						onclick={() => switchView('pools')}
					>
						Pools
					</button>
					<button
						class={twMerge('page-tab max-w-1/2', view === 'catalogs' && 'page-tab-active')}
						onclick={() => switchView('catalogs')}
					>
						Config Sources
					</button>
				</div>

				<!-- These four words mean nothing on first contact, and the empty-state
				     copy that explains them disappears as soon as there is one row --
				     which for a prepopulated install is immediately. Keeping the
				     definition beside the tab costs one line and is always there. -->
				<p class="text-muted-content -mt-1 text-sm font-light">{viewDescription}</p>

				{#if view === 'agents'}
					{#if hostedAgents.length === 0}
						<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
							<Bot class="text-muted-content size-24 opacity-25" />
							<h4 class="text-muted-content text-lg font-semibold">No agent templates</h4>
							{#if !isReadonly}
								<p class="text-muted-content text-sm font-light">
									Add one directly, or sync them from a config source.
								</p>
							{/if}
							{@render addAgentButton()}
						</div>
					{:else}
						{@render agentTable()}
					{/if}
				{:else if view === 'catalogs'}
					{#if agentCatalogs.length === 0}
						<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
							<GitBranch class="text-muted-content size-24 opacity-25" />
							<h4 class="text-muted-content text-lg font-semibold">No config sources</h4>
							{#if !isReadonly}
								<p class="text-muted-content text-sm font-light">
									Add a Git repository to sync templates and harnesses from.
								</p>
							{/if}
							{@render addSourceButton()}
						</div>
					{:else}
						{@render sourceTable()}
					{/if}
				{:else if view === 'pools'}
					<HostedAgentPools
						bind:pools
						bind:assignments
						bind:defaults={poolDefaults}
						readonly={isReadonly}
					/>
				{:else if harnesses.length === 0}
					<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
						<Cpu class="text-muted-content size-24 opacity-25" />
						<h4 class="text-muted-content text-lg font-semibold">No harnesses</h4>
						{#if !isReadonly}
							<p class="text-muted-content text-sm font-light">
								Add one before registering a template, which has to pick a harness.
							</p>
						{/if}
						{@render addHarnessButton()}
					</div>
				{:else if view === 'harnesses'}
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
				{:else if view === 'catalogs'}
					{@render addSourceButton()}
				{:else if view === 'harnesses'}
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
		{#snippet onRenderColumn(property, d)}
			{#if property === 'name'}
				<div class="flex items-center gap-2">
					<AgentIcon icon={d.icon} iconDark={d.iconDark} alt="" />
					<span class="truncate">{d.name}</span>
				</div>
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
		data={catalogTableData}
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
						const source = agentCatalogs.find((s) => s.id === d.id);
						if (source) openEditCatalog(source);
					}}
					tooltip={{ text: 'Edit Config Source' }}
				>
					<Pencil class="size-4" />
				</IconButton>
				<IconButton
					variant="danger"
					onclick={(e) => {
						e.stopPropagation();
						catalogToDelete = agentCatalogs.find((s) => s.id === d.id);
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
		{#snippet onRenderColumn(property, d)}
			{#if property === 'name'}
				<div class="flex items-center gap-2">
					<AgentIcon icon={d.icon} iconDark={d.iconDark} alt="" />
					<span class="truncate">{d.name}</span>
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
			<Plus class="size-4" /> Add Template
		</button>
	{/if}
{/snippet}

{#snippet addSourceButton()}
	{#if !isReadonly}
		<button class="btn btn-primary flex items-center gap-1 text-sm" onclick={openCreateCatalog}>
			<Plus class="size-4" /> Add Config Source
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
	bind:this={catalogDialog}
	title={editingCatalog ? 'Edit Config Source' : 'Add Config Source'}
	class="md:max-w-md"
>
	<div class="flex flex-col gap-4">
		<div class="flex flex-col gap-2">
			<label for="source-name" class="text-sm font-light">Name</label>
			<input id="source-name" bind:value={catalogForm.displayName} class="text-input-filled" />
		</div>
		<div class="flex flex-col gap-2">
			<label for="source-repo" class="text-sm font-light">Repository URL</label>
			<input
				id="source-repo"
				bind:value={catalogForm.repoURL}
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
				bind:value={catalogForm.ref}
				class="text-input-filled"
				placeholder="(default branch)"
				autocomplete="off"
			/>
			<span class="text-muted-content text-xs">Branch or tag. Leave blank for the default.</span>
		</div>
	</div>
	<div class="flex justify-end gap-2 pt-4">
		<button class="btn btn-secondary text-sm" onclick={() => catalogDialog?.close()}>Cancel</button>
		<button
			class="btn btn-primary text-sm"
			disabled={!canSaveCatalog || savingCatalog}
			onclick={saveCatalog}
		>
			{#if savingCatalog}
				<Loading class="size-4" />
			{:else}
				{editingCatalog ? 'Update' : 'Add'}
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
	msg={`Delete ${catalogToDelete?.displayName || 'this source'}?`}
	note="Agents and harnesses discovered from this source will be removed."
	show={Boolean(catalogToDelete)}
	onsuccess={async () => {
		if (!catalogToDelete) return;
		await AdminService.deleteAgentCatalog(catalogToDelete.id);
		agentCatalogs = await AdminService.listAgentCatalogs();
		hostedAgents = await AdminService.listHostedAgents({ all: true });
		harnesses = await AdminService.listHarnesses();
		catalogToDelete = undefined;
	}}
	oncancel={() => (catalogToDelete = undefined)}
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
	<title>Obot | Hosted Agents</title>
</svelte:head>
