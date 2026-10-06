<script lang="ts">
	import { MCP_FILTERS_FIELD_IDS } from '$lib/constants';
	import { m } from '$lib/i18n';
	import {
		UserService,
		type MCPFilterResource,
		type MCPFilterWebhookSelector,
		type VMCP
	} from '$lib/services';
	import { errors, mcpServersAndEntries } from '$lib/stores';
	import IconButton from '../primitives/IconButton.svelte';
	import Table from '../table/Table.svelte';
	import SearchMcpServers from './SearchMcpServers.svelte';
	import { Plus, Trash2, X } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		form: {
			selectors: MCPFilterWebhookSelector[];
			resources: MCPFilterResource[];
		};
		readonly?: boolean;
		inDialog?: boolean;
	}

	let { form = $bindable(), readonly, inDialog }: Props = $props();

	let addMcpServerDialog = $state<ReturnType<typeof SearchMcpServers>>();
	let vmcps = $state<VMCP[]>([]);
	let vmcpsMap = $derived(new Map(vmcps.map((vmcp) => [vmcp.id, vmcp])));

	onMount(async () => {
		try {
			vmcps = await UserService.listVMCPs({ all: true });
		} catch {
			errors.append(m.mcps_filters_failed_load_vmcps());
		}
	});
	let mcpServersMap = $derived(new Map(mcpServersAndEntries.current.servers.map((i) => [i.id, i])));
	let mcpEntriesMap = $derived(new Map(mcpServersAndEntries.current.entries.map((i) => [i.id, i])));

	let mcpServersTableData = $derived.by(() => {
		if (mcpServersMap && mcpEntriesMap) {
			return convertMcpServersToTableData(form.resources ?? []);
		}
		return [];
	});

	function convertMcpServersToTableData(resources: { id: string; type: string }[]) {
		return resources.map((resource) => {
			const entryMatch = mcpEntriesMap.get(resource.id);
			const serverMatch = mcpServersMap.get(resource.id);
			const vmcpMatch = vmcpsMap.get(resource.id);

			if (vmcpMatch) {
				return { id: resource.id, name: vmcpMatch.displayName || '-', type: 'mcpserver' };
			}

			if (entryMatch) {
				return {
					id: resource.id,
					name: entryMatch.manifest.name || '-',
					type: 'mcpentry'
				};
			}

			if (serverMatch) {
				return {
					id: resource.id,
					name: serverMatch.manifest.name || '-',
					type: 'mcpserver'
				};
			}

			return {
				id: resource.id,
				name:
					resource.id === '*' && resource.type === 'selector'
						? m.mcps_filters_everything()
						: resource.id === 'default' && resource.type === 'mcpCatalog'
							? m.mcps_filters_all_entries_global_registry()
							: resource.id,
				type: resource.type
			};
		});
	}

	function addSelector() {
		form.selectors = [...form.selectors, { method: '', identifiers: [''] }];
	}

	function removeSelector(index: number) {
		form.selectors = form.selectors.filter((_, i) => i !== index);
	}

	function addIdentifier(selectorIndex: number) {
		form.selectors[selectorIndex].identifiers = [
			...(form.selectors[selectorIndex].identifiers || []),
			''
		];
	}

	function removeIdentifier(selectorIndex: number, identifierIndex: number) {
		if (form.selectors[selectorIndex].identifiers) {
			form.selectors[selectorIndex].identifiers = form.selectors[selectorIndex].identifiers!.filter(
				(_, i) => i !== identifierIndex
			);
		}
	}
</script>

<div class="flex flex-col gap-2" id={MCP_FILTERS_FIELD_IDS.filterSelectors}>
	<div class="mb-2 flex md:flex-row flex-col md:items-center items-start gap-4 justify-between">
		<div class="flex flex-col gap-1">
			<h2 class="text-lg font-semibold">{m.mcps_filters_selectors()}</h2>
			<p class="text-muted-content text-sm">
				{m.mcps_filters_selectors_description()}
			</p>
		</div>
		{#if !readonly}
			<div class="relative flex items-center gap-4">
				<button class="btn btn-primary flex items-center gap-1 text-sm" onclick={addSelector}>
					<Plus class="size-4" />
					{m.mcps_filters_add_selector()}
				</button>
			</div>
		{/if}
	</div>

	{#if form.selectors.length === 0}
		<div class="text-muted-content p-4 text-center font-light text-sm">
			{m.mcps_filters_no_selectors_added()}<br />{m.mcps_filters_click_add_selector()}
		</div>
	{:else}
		{#each form.selectors as selector, selectorIndex (selectorIndex)}
			{#if inDialog}
				<div class="bg-base-200 dark:bg-base-100 rounded-lg p-2 shadow-inner">
					{@render selectorView(selector, selectorIndex)}
				</div>
			{:else}
				{@render selectorView(selector, selectorIndex)}
			{/if}
		{/each}
	{/if}
</div>

<div class="flex flex-col gap-2" id={MCP_FILTERS_FIELD_IDS.filterMcpServers}>
	<div class="mb-2 flex md:flex-row flex-col md:items-center items-start gap-4 justify-between">
		<div class="flex flex-col gap-1">
			<h2 class="text-lg font-semibold">{m.mcps_filters_mcp_servers()}</h2>
			<p class="text-muted-content text-sm">
				{m.mcps_filters_mcp_servers_description()}
			</p>
		</div>
		{#if !readonly}
			<div class="relative flex items-center gap-4">
				<button
					class="btn btn-primary flex items-center gap-1 text-sm"
					onclick={() => {
						addMcpServerDialog?.open();
					}}
				>
					<Plus class="size-4" />
					{m.mcps_add_mcp_server()}
				</button>
			</div>
		{/if}
	</div>
	{#if inDialog}
		<div class="bg-base-200 dark:bg-base-100 rounded-lg p-2 shadow-inner">
			{@render mcpServersTable()}
		</div>
	{:else}
		{@render mcpServersTable()}
	{/if}
</div>

<SearchMcpServers
	bind:this={addMcpServerDialog}
	{vmcps}
	exclude={form.resources.map((r) => r.id)}
	type="filter"
	onAdd={async (mcpCatalogEntryIds, mcpServerIds, otherSelectors) => {
		const catalogEntryResources = mcpCatalogEntryIds.map((id) => ({
			id,
			name: id,
			type: 'mcpServerCatalogEntry' as const
		}));
		const serverResources = mcpServerIds.map((id) => ({
			name: id,
			id,
			type: 'mcpServer' as const
		}));
		const selectorResources = otherSelectors.map((id) => ({
			name: id === '*' ? 'Everything' : id === 'default' ? 'All Entries in Global Registry' : id,
			id,
			type: id === '*' ? ('selector' as const) : ('mcpCatalog' as const)
		}));
		form.resources = [
			...form.resources,
			...catalogEntryResources,
			...serverResources,
			...selectorResources
		];
	}}
	mcpEntriesContextFn={() => mcpServersAndEntries.current}
/>

{#snippet selectorView(selector: MCPFilterWebhookSelector, selectorIndex: number)}
	<div
		class={twMerge(
			'dark:border-base-400 bg-base-100 rounded-lg border border-transparent p-4',
			inDialog ? 'dark:bg-base-400' : 'dark:bg-base-100 '
		)}
		in:slide|global={{ axis: 'y', duration: 150 }}
	>
		<div class="mb-1 flex items-center justify-between">
			<h3 class="text-sm font-medium text-muted-content dark:text-muted-content">
				{m.mcps_filters_selector_n({ index: selectorIndex + 1 })}
			</h3>
			{#if !readonly}
				<IconButton
					variant="danger"
					onclick={() => removeSelector(selectorIndex)}
					tooltip={{ text: m.mcps_filters_remove_selector() }}
				>
					<Trash2 class="size-4" />
				</IconButton>
			{/if}
		</div>

		<div class="flex flex-col gap-4">
			<div class="flex flex-col gap-2">
				<label for="method-{selectorIndex}" class="text-sm font-light"
					>{m.mcps_filters_method_optional()}</label
				>
				<input
					id="method-{selectorIndex}"
					bind:value={selector.method}
					class="text-input-filled"
					placeholder={m.mcps_filters_method_placeholder()}
					disabled={readonly}
				/>
			</div>

			<div class="flex flex-col gap-2">
				<div class="flex items-center justify-between">
					<label for="identifier-btn" class="text-sm font-light">
						{m.mcps_filters_identifiers_optional()}
					</label>
					{#if !readonly}
						<button
							id="identifier-btn"
							type="button"
							class="btn btn-secondary btn-sm flex items-center gap-1"
							onclick={() => addIdentifier(selectorIndex)}
						>
							<Plus class="size-3" />
							{m.mcps_filters_add_identifier()}
						</button>
					{/if}
				</div>

				{#if !selector.identifiers || selector.identifiers.length === 0}
					<div class="text-muted-content p-3 text-center text-sm">
						{#if !readonly}
							{m.mcps_filters_no_identifiers_click_add()}
						{:else}
							{m.mcps_filters_no_identifiers_added()}
						{/if}
					</div>
				{:else}
					{#each selector.identifiers as _, identifierIndex (identifierIndex)}
						<div class="flex items-center gap-2">
							<input
								id="identifier-{selectorIndex}-{identifierIndex}"
								bind:value={selector.identifiers[identifierIndex]}
								class="text-input-filled flex-1"
								placeholder={m.mcps_filters_identifier_placeholder()}
								disabled={readonly}
							/>
							{#if !readonly}
								<IconButton
									variant="danger"
									onclick={() => removeIdentifier(selectorIndex, identifierIndex)}
									tooltip={{ text: m.mcps_filters_remove_identifier() }}
								>
									<X class="size-4" />
								</IconButton>
							{/if}
						</div>
					{/each}
				{/if}
			</div>
		</div>
	</div>
{/snippet}

{#snippet mcpServersTable()}
	<Table
		data={mcpServersTableData}
		fields={['name']}
		headers={[{ property: 'name', title: m.core_name() }]}
		noDataMessage={m.mcps_filters_no_mcp_servers_added()}
	>
		{#snippet actions(d)}
			{#if !readonly}
				<IconButton
					variant="danger"
					onclick={() => {
						form.resources = form.resources.filter((resource) => resource.id !== d.id);
					}}
					tooltip={{ text: m.mcps_filters_remove_mcp_server() }}
				>
					<Trash2 class="size-4" />
				</IconButton>
			{/if}
		{/snippet}
	</Table>
{/snippet}
