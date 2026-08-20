<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import McpDetachedNotice from '$lib/components/mcp/McpDetachedNotice.svelte';
	import { VirtualPageViewport } from '$lib/components/ui/virtual-page';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { AdminService } from '$lib/services';
	import { getMCPDisplayName } from '$lib/services/user/mcp.js';
	import { mcpServersAndEntries, profile } from '$lib/stores';
	import { type Component, untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	const duration = PAGE_TRANSITION_DURATION;

	let { data } = $props();
	let { catalogEntry, mcpServer } = $derived(data);

	let sourceEntity = $derived(
		catalogEntry?.powerUserWorkspaceID ? ('workspace' as const) : ('catalog' as const)
	);
	let sourceID = $derived(catalogEntry?.powerUserWorkspaceID || DEFAULT_MCP_CATALOG_ID);

	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	let isSourcedEntry = $derived(
		catalogEntry && 'sourceURL' in catalogEntry && !!catalogEntry.sourceURL
	);

	async function acceptOwnership() {
		if (!catalogEntry) return;
		catalogEntry = await AdminService.acceptMCPCatalogEntryOwnership(
			DEFAULT_MCP_CATALOG_ID,
			catalogEntry.id
		);
	}

	$effect(() => {
		if (catalogEntry?.manifest.runtime === 'composite') {
			untrack(() => mcpServersAndEntries.refreshAll());
		}
	});

	let title = $derived(
		getMCPDisplayName(mcpServer) || getMCPDisplayName(catalogEntry) || 'MCP Server'
	);
</script>

<Layout
	main={{
		component: VirtualPageViewport as unknown as Component,
		props: { class: '', as: 'main', itemHeight: 56, overscan: 5, disabled: true }
	}}
	{title}
	showBackButton
>
	<div class="flex h-full flex-col gap-6" in:fly={{ x: 100, delay: duration, duration }}>
		{#if profile.current.hasAdminAccess?.()}
			<McpDetachedNotice
				detached={catalogEntry?.detached}
				sourceURL={catalogEntry?.sourceURL}
				variant="notification"
				onAcceptOwnership={isAdminReadonly ? undefined : acceptOwnership}
			/>
		{/if}

		<McpServerEntryForm
			entry={catalogEntry}
			server={mcpServer}
			type={catalogEntry?.manifest.runtime === 'composite'
				? 'composite'
				: catalogEntry?.manifest.runtime === 'remote'
					? 'remote'
					: 'hosted'}
			readonly={isAdminReadonly || isSourcedEntry}
			id={sourceID}
			entity={sourceEntity}
			excludeViews={['overview']}
		/>
	</div>
</Layout>

<svelte:head>
	<title>Obot | {catalogEntry?.manifest?.name ?? 'MCP Server'}</title>
</svelte:head>
