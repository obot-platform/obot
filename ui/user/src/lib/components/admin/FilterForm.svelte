<script lang="ts">
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { Eye, EyeOff, LoaderCircle, Plus, Trash2, X } from 'lucide-svelte';
	import { type Snippet } from 'svelte';
	import { fly } from 'svelte/transition';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Table from '../table/Table.svelte';
	import Confirm from '../Confirm.svelte';
	import { goto } from '$app/navigation';
	import SearchMcpServers from './SearchMcpServers.svelte';
	import {
		getAdminMcpServerAndEntries,
		type AdminMcpServerAndEntriesContext
	} from '$lib/context/admin/mcpServerAndEntries.svelte';
	import {
		AdminService,
		type MCPFilter,
		type MCPFilterManifest,
		type MCPFilterResource,
		type MCPFilterWebhookSelector
	} from '$lib/services';
	import { removeSecret } from '$lib/services/admin/operations';

	interface Props {
		topContent?: Snippet;
		filter?: MCPFilter;
		onCreate?: (filter?: MCPFilter) => void;
		onUpdate?: (filter?: MCPFilter) => void;
		mcpEntriesContextFn?: () => AdminMcpServerAndEntriesContext;
		readonly?: boolean;
	}

	let {
		topContent,
		filter: initialFilter,
		onCreate,
		onUpdate,
		mcpEntriesContextFn,
		readonly
	}: Props = $props();
	const duration = PAGE_TRANSITION_DURATION;
	let filter = $state<{
		name: string;
		resources: MCPFilterResource[];
		url: string;
		secret: string;
		selectors: MCPFilterWebhookSelector[];
	}>(
		initialFilter
			? {
					name: initialFilter.name || '',
					resources: initialFilter.resources || [],
					url: initialFilter.url || '',
					secret: initialFilter.secret || '',
					selectors: initialFilter.selectors || []
				}
			: {
					name: '',
					resources: [{ id: 'default', type: 'mcpCatalog' }],
					url: '',
					secret: '',
					selectors: []
				}
	);

	let saving = $state<boolean | undefined>();
	let addMcpServerDialog = $state<ReturnType<typeof SearchMcpServers>>();
	let deletingFilter = $state(false);
	let showSecret = $state<boolean>(false);
	let removingSecret = $state(false);
	let showValidation = $state(false);

	const adminMcpServerAndEntries = getAdminMcpServerAndEntries();
	let mcpServersMap = $derived(new Map(adminMcpServerAndEntries.servers.map((i) => [i.id, i])));
	let mcpEntriesMap = $derived(new Map(adminMcpServerAndEntries.entries.map((i) => [i.id, i])));

	// Validation
	let nameError = $derived(showValidation && !filter.name.trim());
	let urlError = $derived(showValidation && !filter.url.trim());
	let mcpServersTableData = $derived.by(() => {
		if (mcpServersMap && mcpEntriesMap) {
			return convertMcpServersToTableData(filter.resources ?? []);
		}
		return [];
	});

	function convertMcpServersToTableData(resources: { id: string; type: string }[]) {
		return resources.map((resource) => {
			const entryMatch = mcpEntriesMap.get(resource.id);
			const serverMatch = mcpServersMap.get(resource.id);

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
						? 'Everything'
						: resource.id === 'default' && resource.type === 'mcpCatalog'
							? 'All Entries in Global Registry'
							: resource.id,
				type: resource.type
			};
		});
	}

	function addSelector() {
		filter.selectors = [...filter.selectors, { method: '', identifiers: [''] }];
	}

	function removeSelector(index: number) {
		filter.selectors = filter.selectors.filter((_, i) => i !== index);
	}

	function addIdentifier(selectorIndex: number) {
		filter.selectors[selectorIndex].identifiers = [
			...(filter.selectors[selectorIndex].identifiers || []),
			''
		];
	}

	function removeIdentifier(selectorIndex: number, identifierIndex: number) {
		if (filter.selectors[selectorIndex].identifiers) {
			filter.selectors[selectorIndex].identifiers = filter.selectors[
				selectorIndex
			].identifiers!.filter((_, i) => i !== identifierIndex);
		}
	}

	async function handleRemoveSecret() {
		if (!initialFilter?.id) return;

		removingSecret = true;
		try {
			await removeSecret(initialFilter.id);
			// Clear the secret field and update the filter state
			filter.secret = '';
			// Update the initial filter to reflect that it no longer has a secret
			if (initialFilter) {
				initialFilter.hasSecret = false;
			}
		} finally {
			removingSecret = false;
		}
	}
</script>

<div
	class="flex h-full w-full flex-col gap-4"
	out:fly={{ x: 100, duration }}
	in:fly={{ x: 100, delay: duration }}
>
	<div class="flex grow flex-col gap-4" out:fly={{ x: -100, duration }} in:fly={{ x: -100 }}>
		{#if topContent}
			{@render topContent()}
		{/if}
		{#if initialFilter}
			<div class="flex w-full items-center justify-between gap-4">
				<h1 class="flex items-center gap-4 text-2xl font-semibold">
					{initialFilter.name || 'Filter'}
				</h1>
				{#if !readonly}
					<button
						class="button-destructive flex items-center gap-1 text-xs font-normal"
						use:tooltip={'Delete Filter'}
						disabled={saving}
						onclick={() => (deletingFilter = true)}
					>
						<Trash2 class="size-4" />
					</button>
				{/if}
			</div>
		{:else}
			<h1 class="text-2xl font-semibold">Create Filter</h1>
		{/if}

		<div
			class="dark:bg-surface2 dark:border-surface3 rounded-lg border border-transparent bg-white p-4"
		>
			<div class="flex flex-col gap-6">
				<div class="flex flex-col gap-2">
					<label for="filter-name" class="flex-1 text-sm font-light capitalize"> Name </label>
					<input
						id="filter-name"
						bind:value={filter.name}
						class="text-input-filled mt-0.5 {nameError
							? 'border-red-500 focus:border-red-500 focus:ring-red-500'
							: ''}"
						disabled={readonly}
					/>
					{#if nameError}
						<p class="text-xs text-red-600 dark:text-red-400">Name is required</p>
					{/if}
				</div>

				<div class="flex flex-col gap-2">
					<label for="webhook-url" class="flex-1 text-sm font-light capitalize">
						Webhook URL
					</label>
					<input
						id="webhook-url"
						bind:value={filter.url}
						class="text-input-filled mt-0.5 {urlError
							? 'border-red-500 focus:border-red-500 focus:ring-red-500'
							: ''}"
						required
						disabled={readonly}
					/>
					{#if urlError}
						<p class="text-xs text-red-600 dark:text-red-400">Webhook URL is required</p>
					{/if}
				</div>

				<div class="flex flex-col gap-2">
					<label for="webhook-secret" class="flex-1 text-sm font-light capitalize">
						Secret (Optional)
					</label>
					<div class="relative">
						<input
							id="webhook-secret"
							bind:value={filter.secret}
							class="text-input-filled pr-10"
							type={showSecret ? 'text' : 'password'}
							placeholder={initialFilter?.hasSecret && !filter.secret ? '*****' : ''}
							disabled={readonly}
						/>
						{#if filter.secret || (initialFilter?.hasSecret && !filter.secret)}
							<button
								type="button"
								class="absolute top-1/2 right-2 -translate-y-1/2 p-1 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
								onclick={() => (showSecret = !showSecret)}
								use:tooltip={{
									text: showSecret ? 'Hide secret' : 'Show secret',
									placement: 'top-end'
								}}
							>
								{#if filter.secret}
									{#if showSecret}
										<EyeOff class="size-4" />
									{:else}
										<Eye class="size-4" />
									{/if}
								{/if}
							</button>
						{/if}
					</div>
					{#if initialFilter?.hasSecret}
						<div class="flex items-start justify-between gap-4">
							<p class="flex-1 text-xs text-amber-600 dark:text-amber-400">
								There is currently a secret configured for this webhook. If you've lost or forgotten
								this secret, you can change it, but be aware that any integrations using this secret
								will need to be updated. If you want to keep the secret, you can leave this field
								unchanged.
							</p>
							{#if !readonly}
								<button
									type="button"
									class="button-destructive flex-shrink-0 text-xs"
									disabled={removingSecret || saving}
									onclick={handleRemoveSecret}
								>
									{#if removingSecret}
										<LoaderCircle class="size-3 animate-spin" />
										Removing...
									{:else}
										Remove Secret
									{/if}
								</button>
							{/if}
						</div>
					{:else}
						<p class="text-xs text-gray-500 dark:text-gray-400">
							A shared secret used to sign the payload for webhook verification.
						</p>
					{/if}
				</div>
			</div>
		</div>

		<div class="flex flex-col gap-2">
			<div class="mb-2 flex items-center justify-between">
				<div class="flex flex-col gap-1">
					<h2 class="text-lg font-semibold">Selectors</h2>
					<p class="text-sm text-gray-500 dark:text-gray-400">
						Specify which requests should be matched by this filter.
					</p>
				</div>
				{#if !readonly}
					<div class="relative flex items-center gap-4">
						<button class="button-primary flex items-center gap-1 text-sm" onclick={addSelector}>
							<Plus class="size-4" /> Add Selector
						</button>
					</div>
				{/if}
			</div>

			{#if filter.selectors.length === 0}
				<div class="p-4 text-center text-gray-500">
					No selectors added. This filter will match all MCP requests.<br />Click "Add Selector" to
					specify filter criteria.
				</div>
			{:else}
				{#each filter.selectors as selector, selectorIndex (selectorIndex)}
					<div
						class="dark:bg-surface2 dark:border-surface3 rounded-lg border border-transparent bg-white p-4"
					>
						<div class="mb-4 flex items-center justify-between">
							<h3 class="text-sm font-medium text-gray-700 dark:text-gray-300">
								Selector {selectorIndex + 1}
							</h3>
							{#if !readonly}
								<button
									class="icon-button text-red-500 hover:text-red-600"
									onclick={() => removeSelector(selectorIndex)}
									use:tooltip={'Remove Selector'}
								>
									<Trash2 class="size-4" />
								</button>
							{/if}
						</div>

						<div class="flex flex-col gap-4">
							<div class="flex flex-col gap-2">
								<label for="method-{selectorIndex}" class="text-sm font-light"
									>Method (Optional)</label
								>
								<input
									id="method-{selectorIndex}"
									bind:value={selector.method}
									class="text-input-filled"
									placeholder="e.g., tools/call, resources/read"
									disabled={readonly}
								/>
							</div>

							<div class="flex flex-col gap-2">
								<div class="flex items-center justify-between">
									<label for="identifier-btn" class="text-sm font-light">
										Identifiers (Optional)
									</label>
									{#if !readonly}
										<button
											id="identifier-btn"
											type="button"
											class="button-text flex items-center gap-1 text-xs"
											onclick={() => addIdentifier(selectorIndex)}
										>
											<Plus class="size-3" /> Add Identifier
										</button>
									{/if}
								</div>

								{#if !selector.identifiers || selector.identifiers.length === 0}
									<div class="p-3 text-center text-sm text-gray-500">
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
												placeholder="e.g., tool name, resource URI"
												disabled
											/>
											{#if !readonly}
												<button
													type="button"
													class="icon-button text-red-500 hover:text-red-600"
													onclick={() => removeIdentifier(selectorIndex, identifierIndex)}
													use:tooltip={'Remove Identifier'}
												>
													<X class="size-4" />
												</button>
											{/if}
										</div>
									{/each}
								{/if}
							</div>
						</div>
					</div>
				{/each}
			{/if}
		</div>

		<div class="flex flex-col gap-2">
			<div class="mb-2 flex items-center justify-between">
				<div class="flex flex-col gap-1">
					<h2 class="text-lg font-semibold">MCP Servers</h2>
					<p class="text-sm text-gray-500 dark:text-gray-400">
						Specify which MCP servers this filter should be applied to.
					</p>
				</div>
				{#if !readonly}
					<div class="relative flex items-center gap-4">
						<button
							class="button-primary flex items-center gap-1 text-sm"
							onclick={() => {
								addMcpServerDialog?.open();
							}}
						>
							<Plus class="size-4" /> Add MCP Server
						</button>
					</div>
				{/if}
			</div>
			<Table data={mcpServersTableData} fields={['name']} noDataMessage="No MCP servers added.">
				{#snippet actions(d)}
					{#if !readonly}
						<button
							class="icon-button hover:text-red-500"
							onclick={() => {
								filter.resources = filter.resources.filter((resource) => resource.id !== d.id);
							}}
							use:tooltip={'Remove MCP Server'}
						>
							<Trash2 class="size-4" />
						</button>
					{/if}
				{/snippet}
			</Table>
		</div>
	</div>
	{#if !readonly}
		<div
			class="bg-surface1 sticky bottom-0 left-0 flex w-full justify-end gap-2 py-4 text-gray-400 dark:bg-black dark:text-gray-600"
			out:fly={{ x: -100, duration }}
			in:fly={{ x: -100 }}
		>
			<div class="flex w-full justify-end gap-2">
				<button
					class="button text-sm"
					onclick={() => {
						if (initialFilter) {
							onUpdate?.(undefined);
						} else {
							onCreate?.(undefined);
						}
					}}
				>
					Cancel
				</button>
				<button
					class="button-primary text-sm disabled:opacity-75"
					disabled={saving}
					onclick={async () => {
						// Show validation errors if required fields are missing
						if (!filter.name.trim() || !filter.url.trim()) {
							showValidation = true;
							return;
						}

						saving = true;
						try {
							const manifest: MCPFilterManifest = {
								name: filter.name,
								resources: filter.resources,
								url: filter.url,
								secret: filter.secret || undefined,
								selectors:
									filter.selectors.length > 0
										? filter.selectors
												.map((s) => ({
													...s,
													identifiers: s.identifiers?.filter((id) => id.trim()) || []
												}))
												.filter((s) => s.method || (s.identifiers && s.identifiers.length > 0))
										: undefined
							};

							let result: MCPFilter;
							if (initialFilter) {
								result = await AdminService.updateMCPFilter(initialFilter.id, manifest);
								onUpdate?.(result);
							} else {
								result = await AdminService.createMCPFilter(manifest);
								onCreate?.(result);
							}
						} finally {
							saving = false;
						}
					}}
				>
					{#if saving}
						<LoaderCircle class="size-4 animate-spin" />
					{:else}
						Save
					{/if}
				</button>
			</div>
		</div>
	{/if}
</div>

<SearchMcpServers
	bind:this={addMcpServerDialog}
	exclude={filter.resources.map((r) => r.id)}
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
		filter.resources = [
			...filter.resources,
			...catalogEntryResources,
			...serverResources,
			...selectorResources
		];
	}}
	{mcpEntriesContextFn}
/>

<Confirm
	msg="Are you sure you want to delete this filter?"
	show={deletingFilter}
	onsuccess={async () => {
		if (!initialFilter) return;
		await AdminService.deleteMCPFilter(initialFilter.id);
		goto('/admin/filters');
	}}
	oncancel={() => (deletingFilter = false)}
/>
