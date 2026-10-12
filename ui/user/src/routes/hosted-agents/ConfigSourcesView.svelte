<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { type AgentCatalog } from '$lib/services/admin/types';
	import { AdminService } from '$lib/services/index.js';
	import { errors, profile } from '$lib/stores/index.js';
	import { GitBranch, Pencil, Plus, RefreshCcw, Trash2 } from '@lucide/svelte';
	import { onDestroy, untrack } from 'svelte';
	import { SvelteMap, SvelteSet } from 'svelte/reactivity';
	import { fade } from 'svelte/transition';

	interface Props {
		agentCatalogs: AgentCatalog[];
	}

	let { agentCatalogs: initialCatalogs }: Props = $props();
	let agentCatalogs = $state(untrack(() => initialCatalogs));
	let catalogToDelete = $state<AgentCatalog>();
	let catalogDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let editingCatalog = $state<AgentCatalog | undefined>();
	let savingCatalog = $state(false);
	let catalogForm = $state({ displayName: '', repoURL: '', ref: '' });
	let isReadonly = $derived(profile.current.isAdminReadonly?.());
	const duration = PAGE_TRANSITION_DURATION;

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
						syncing.delete(id);
					}
				} catch (err) {
					errors.append(m.hosted_agents_config_sources_sync_source_failed({ error: String(err) }));
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
			errors.append(m.hosted_agents_config_sources_refresh_source_failed({ error: String(err) }));
			syncing.delete(id);
		}
	}

	export function openCreate() {
		openCreateCatalog();
	}

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
			errors.append(m.hosted_agents_config_sources_save_source_failed({ error: String(err) }));
		} finally {
			savingCatalog = false;
		}
	}

	let canSaveCatalog = $derived(Boolean(catalogForm.displayName && catalogForm.repoURL));
	let catalogTableData = $derived(
		agentCatalogs.map((source) => ({
			id: source.id,
			displayName: source.displayName,
			repoURL: source.repoURL,
			ref: source.ref || m.hosted_agents_config_sources_default_branch(),
			discoveredAgentCount: source.discoveredAgentCount ?? 0,
			discoveredHarnessCount: source.discoveredHarnessCount ?? 0,
			syncError: source.syncError ?? '',
			isSyncing: syncing.has(source.id) || Boolean(source.isSyncing)
		}))
	);
</script>

<div class="flex flex-col gap-4" in:fade={{ duration }}>
	<p class="text-muted-content text-sm font-light">
		{m.hosted_agents_config_sources_desc()}
	</p>

	{#if agentCatalogs.length === 0}
		<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
			<GitBranch class="text-muted-content size-24 opacity-25" />
			<h4 class="text-muted-content text-lg font-semibold">
				{m.hosted_agents_config_sources_no_config_sources()}
			</h4>
			{#if !isReadonly}
				<p class="text-muted-content text-sm font-light">
					{m.hosted_agents_config_sources_no_config_sources_desc()}
				</p>
				<button class="btn btn-primary flex items-center gap-1 text-sm" onclick={openCreateCatalog}>
					<Plus class="size-4" />
					{m.hosted_agents_config_sources_add_config_source()}
				</button>
			{/if}
		</div>
	{:else}
		<Table
			data={catalogTableData}
			fields={['displayName', 'repoURL', 'ref', 'discoveredAgentCount', 'discoveredHarnessCount']}
			headers={[
				{ property: 'displayName', title: m.core_name() },
				{ property: 'repoURL', title: m.core_repository() },
				{ property: 'ref', title: m.chat_config_sources_col_ref() },
				{ property: 'discoveredAgentCount', title: m.chat_config_sources_col_agents() },
				{ property: 'discoveredHarnessCount', title: m.chat_config_sources_col_harnesses() }
			]}
			sortable={['displayName', 'repoURL']}
			noDataMessage={m.hosted_agents_config_sources_no_sources_added()}
		>
			{#snippet onRenderColumn(property, d)}
				{#if property === 'displayName'}
					<div class="flex items-center gap-2">
						<span>{d.displayName}</span>
						{#if d.isSyncing}
							<Loading class="size-3" />
						{:else if d.syncError}
							<span class="badge badge-error badge-xs" title={d.syncError}
								>{m.hosted_agents_config_sources_sync_error()}</span
							>
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
						tooltip={{ text: m.hosted_agents_config_sources_sync_now() }}
					>
						<RefreshCcw class="size-4" />
					</IconButton>
					<IconButton
						onclick={(e) => {
							e.stopPropagation();
							const source = agentCatalogs.find((s) => s.id === d.id);
							if (source) openEditCatalog(source);
						}}
						tooltip={{ text: m.hosted_agents_config_sources_edit_config_source() }}
					>
						<Pencil class="size-4" />
					</IconButton>
					<IconButton
						variant="danger"
						onclick={(e) => {
							e.stopPropagation();
							catalogToDelete = agentCatalogs.find((s) => s.id === d.id);
						}}
						tooltip={{ text: m.hosted_agents_config_sources_delete_source() }}
					>
						<Trash2 class="size-4" />
					</IconButton>
				{/if}
			{/snippet}
		</Table>
	{/if}
</div>

<ResponsiveDialog
	bind:this={catalogDialog}
	title={editingCatalog
		? m.hosted_agents_config_sources_edit_config_source()
		: m.hosted_agents_config_sources_add_config_source()}
	class="md:max-w-md"
>
	<div class="flex flex-col gap-4">
		<div class="flex flex-col gap-2">
			<label for="source-name" class="text-sm font-light">{m.core_name()}</label>
			<input id="source-name" bind:value={catalogForm.displayName} class="text-input-filled" />
		</div>
		<div class="flex flex-col gap-2">
			<label for="source-repo" class="text-sm font-light"
				>{m.hosted_agents_config_sources_repository_url()}</label
			>
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
			<label for="source-ref" class="text-sm font-light"
				>{m.hosted_agents_config_sources_ref()}</label
			>
			<input
				id="source-ref"
				bind:value={catalogForm.ref}
				class="text-input-filled"
				placeholder={m.hosted_agents_config_sources_default_branch()}
				autocomplete="off"
			/>
			<span class="text-muted-content text-xs">{m.hosted_agents_config_sources_ref_hint()}</span>
		</div>
	</div>
	<div class="flex justify-end gap-2 pt-4">
		<button class="btn btn-secondary text-sm" onclick={() => catalogDialog?.close()}
			>{m.common_cancel()}</button
		>
		<button
			class="btn btn-primary text-sm"
			disabled={!canSaveCatalog || savingCatalog}
			onclick={saveCatalog}
		>
			{#if savingCatalog}
				<Loading class="size-4" />
			{:else}
				{editingCatalog ? m.core_update() : m.chat_add()}
			{/if}
		</button>
	</div>
</ResponsiveDialog>

<Confirm
	msg={m.chat_delete_named({
		name: catalogToDelete?.displayName || m.chat_config_sources_this_source()
	})}
	note={m.hosted_agents_config_sources_delete_source_note()}
	show={Boolean(catalogToDelete)}
	onsuccess={async () => {
		if (!catalogToDelete) return;
		await AdminService.deleteAgentCatalog(catalogToDelete.id);
		agentCatalogs = await AdminService.listAgentCatalogs();
		catalogToDelete = undefined;
	}}
	oncancel={() => (catalogToDelete = undefined)}
/>
