<script lang="ts">
	import { MCP_FILTERS_FIELD_IDS } from '$lib/constants';
	import type {
		MCPFilterLocalAgentEvent,
		MCPFilterResource,
		MCPFilterWebhookSelector
	} from '$lib/services';
	import { mcpServersAndEntries } from '$lib/stores';
	import IconButton from '../primitives/IconButton.svelte';
	import Table from '../table/Table.svelte';
	import SearchMcpServers from './SearchMcpServers.svelte';
	import { Plus, Trash2, X } from '@lucide/svelte';
	import { slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		form: {
			localAgentEvents: MCPFilterLocalAgentEvent[];
			selectors: MCPFilterWebhookSelector[];
			resources: MCPFilterResource[];
		};
		readonly?: boolean;
		inDialog?: boolean;
		showLocalAgentEventsError?: boolean;
	}

	let {
		form = $bindable(),
		readonly,
		inDialog,
		showLocalAgentEventsError = false
	}: Props = $props();

	const localAgentEventOptions: {
		id: Exclude<MCPFilterLocalAgentEvent, '*'>;
		label: string;
	}[] = [
		{ id: 'userPrompt', label: 'User prompts' },
		{ id: 'toolCallArguments', label: 'Tool call arguments' },
		{ id: 'toolResponse', label: 'Tool responses' }
	];

	let addMcpServerDialog = $state<ReturnType<typeof SearchMcpServers>>();
	let mcpServersMap = $derived(new Map(mcpServersAndEntries.current.servers.map((i) => [i.id, i])));
	let mcpEntriesMap = $derived(new Map(mcpServersAndEntries.current.entries.map((i) => [i.id, i])));
	let mcpResources = $derived(
		form.resources.filter((resource) => resource.type !== 'deviceSelector')
	);
	let allDevicesSelected = $derived(
		form.resources.some((resource) => resource.type === 'deviceSelector' && resource.id === '*')
	);

	let mcpServersTableData = $derived.by(() => {
		if (mcpServersMap && mcpEntriesMap) {
			return convertMcpServersToTableData(mcpResources);
		}
		return [];
	});

	function convertMcpServersToTableData(resources: MCPFilterResource[]) {
		return resources.map((resource) => {
			if (resource.type === 'mcpServerCatalogEntry') {
				return {
					id: resource.id,
					name: mcpEntriesMap.get(resource.id)?.manifest.name || '-',
					type: resource.type
				};
			}

			if (resource.type === 'mcpServer') {
				return {
					id: resource.id,
					name: mcpServersMap.get(resource.id)?.manifest.name || '-',
					type: resource.type
				};
			}

			return {
				id: resource.id,
				name:
					resource.id === '*' && resource.type === 'selector'
						? 'Everything'
						: resource.id === 'default' && resource.type === 'mcpCatalog'
							? 'All Entries in Global Registry'
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

	function setAllDevices(selected: boolean) {
		if (selected) {
			if (!allDevicesSelected) {
				form.resources = [...form.resources, { type: 'deviceSelector', id: '*' }];
			}
			if (form.localAgentEvents.length === 0) {
				form.localAgentEvents = ['*'];
			}
			form = { ...form };
			return;
		}

		form.resources = form.resources.filter((resource) => resource.type !== 'deviceSelector');
		form.localAgentEvents = [];
		form = { ...form };
	}

	function eventSelected(event: Exclude<MCPFilterLocalAgentEvent, '*'>) {
		return form.localAgentEvents.includes('*') || form.localAgentEvents.includes(event);
	}

	function setLocalAgentEvent(event: Exclude<MCPFilterLocalAgentEvent, '*'>, selected: boolean) {
		const selectedEvents = localAgentEventOptions
			.map((option) => option.id)
			.filter((candidate) => eventSelected(candidate) && candidate !== event);
		if (selected) {
			selectedEvents.push(event);
		}

		form.localAgentEvents =
			selectedEvents.length === localAgentEventOptions.length ? ['*'] : selectedEvents;
		form = { ...form };
	}
</script>

<div class="flex flex-col gap-2" id={MCP_FILTERS_FIELD_IDS.filterSelectors}>
	<div class="mb-2 flex md:flex-row flex-col md:items-center items-start gap-4 justify-between">
		<div class="flex flex-col gap-1">
			<h2 class="text-lg font-semibold">MCP Request Selectors</h2>
			<p class="text-muted-content text-sm">
				Specify which requests should be matched by this filter.
			</p>
		</div>
		{#if !readonly}
			<div class="relative flex items-center gap-4">
				<button class="btn btn-primary flex items-center gap-1 text-sm" onclick={addSelector}>
					<Plus class="size-4" /> Add Selector
				</button>
			</div>
		{/if}
	</div>

	{#if form.selectors.length === 0}
		<div class="text-muted-content p-4 text-center font-light text-sm">
			No selectors added. This filter will match all MCP requests.<br />Click "Add Selector" to
			specify filter criteria.
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
			<h2 class="text-lg font-semibold">MCP Targets</h2>
			<p class="text-muted-content text-sm">
				Specify which MCP servers, registry entries, or catalogs this filter should apply to.
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
					<Plus class="size-4" /> Add MCP Target
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
	exclude={mcpResources.map((resource) => resource.id)}
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

<div class="flex flex-col gap-4">
	<div class="flex flex-col gap-1">
		<h2 class="text-lg font-semibold">Devices</h2>
		<p class="text-muted-content text-sm">
			Apply this filter to supported local-agent events on all enrolled devices.
		</p>
	</div>

	<div
		class={twMerge(
			'dark:border-base-400 bg-base-100 rounded-lg border border-transparent p-4',
			inDialog ? 'dark:bg-base-400 bg-base-200 shadow-inner' : 'dark:bg-base-100'
		)}
	>
		<label class="flex items-center gap-3 font-medium">
			<input
				type="checkbox"
				class="checkbox checkbox-sm"
				checked={allDevicesSelected}
				disabled={readonly}
				onchange={(event) => setAllDevices(event.currentTarget.checked)}
			/>
			All Devices
		</label>

		{#if allDevicesSelected}
			<fieldset class="mt-4 flex flex-col gap-3 border-t border-base-300 pt-4">
				<legend class="mb-2 text-sm font-medium">Local agent events</legend>
				{#each localAgentEventOptions as option (option.id)}
					<label class="flex items-center gap-3 text-sm">
						<input
							type="checkbox"
							class="checkbox checkbox-sm"
							checked={eventSelected(option.id)}
							disabled={readonly}
							onchange={(event) => setLocalAgentEvent(option.id, event.currentTarget.checked)}
						/>
						{option.label}
					</label>
				{/each}
				{#if showLocalAgentEventsError || form.localAgentEvents.length === 0}
					<p class="text-xs text-error">Select at least one local agent event.</p>
				{/if}
			</fieldset>
		{/if}
	</div>
</div>

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
				Selector {selectorIndex + 1}
			</h3>
			{#if !readonly}
				<IconButton
					variant="danger"
					onclick={() => removeSelector(selectorIndex)}
					tooltip={{ text: 'Remove Selector' }}
				>
					<Trash2 class="size-4" />
				</IconButton>
			{/if}
		</div>

		<div class="flex flex-col gap-4">
			<div class="flex flex-col gap-2">
				<label for="method-{selectorIndex}" class="text-sm font-light">Method (Optional)</label>
				<input
					id="method-{selectorIndex}"
					bind:value={selector.method}
					class="text-input-filled"
					placeholder="e.g.: 'tools/call' or 'resources/read'"
					disabled={readonly}
				/>
			</div>

			<div class="flex flex-col gap-2">
				<div class="flex items-center justify-between">
					<label for="identifier-btn" class="text-sm font-light"> Identifiers (Optional) </label>
					{#if !readonly}
						<button
							id="identifier-btn"
							type="button"
							class="btn btn-secondary btn-sm flex items-center gap-1"
							onclick={() => addIdentifier(selectorIndex)}
						>
							<Plus class="size-3" /> Add Identifier
						</button>
					{/if}
				</div>

				{#if !selector.identifiers || selector.identifiers.length === 0}
					<div class="text-muted-content p-3 text-center text-sm">
						{#if !readonly}
							No identifiers added. Click "Add Identifier" to specify filter criteria.
						{:else}
							No identifiers added.
						{/if}
					</div>
				{:else}
					{#each selector.identifiers as _, identifierIndex (identifierIndex)}
						<div class="flex items-center gap-2">
							<input
								id="identifier-{selectorIndex}-{identifierIndex}"
								bind:value={selector.identifiers[identifierIndex]}
								class="text-input-filled flex-1"
								placeholder="e.g.: tool name or resource URI"
								disabled={readonly}
							/>
							{#if !readonly}
								<IconButton
									variant="danger"
									onclick={() => removeIdentifier(selectorIndex, identifierIndex)}
									tooltip={{ text: 'Remove Identifier' }}
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
	<Table data={mcpServersTableData} fields={['name']} noDataMessage="No MCP targets added.">
		{#snippet actions(d)}
			{#if !readonly}
				<IconButton
					variant="danger"
					onclick={() => {
						form.resources = form.resources.filter(
							(resource) => resource.id !== d.id || resource.type !== d.type
						);
					}}
					tooltip={{ text: 'Remove MCP Target' }}
				>
					<Trash2 class="size-4" />
				</IconButton>
			{/if}
		{/snippet}
	</Table>
{/snippet}
