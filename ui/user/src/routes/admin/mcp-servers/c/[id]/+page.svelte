<script lang="ts">
	import { untrack, type Component } from 'svelte';
	import { fly } from 'svelte/transition';
	import { goto } from '$lib/url';
	import { VirtualPageViewport } from '$lib/components/ui/virtual-page';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import Layout from '$lib/components/Layout.svelte';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import { mcpServersAndEntries, profile } from '$lib/stores/index.js';
	import { CircleFadingArrowUp, Info, GitCompare } from 'lucide-svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import DiffDialog from '$lib/components/admin/DiffDialog.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import type { MCPCatalogEntryServerManifest } from '$lib/services/admin/types';
	import type { MCPServer, MCPCatalogServer } from '$lib/services/chat/types';
	import { AdminService } from '$lib/services/index.js';
	import { parseErrorContent } from '$lib/errors';
	import McpServerActions from '$lib/components/mcp/McpServerActions.svelte';
	import { page } from '$app/state';
	import { success } from '$lib/stores/success';

	const duration = PAGE_TRANSITION_DURATION;

	let { data } = $props();
	let catalogEntry = $state(untrack(() => data.catalogEntry));

	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	let isSourcedEntry = $derived(
		catalogEntry && 'sourceURL' in catalogEntry && !!catalogEntry.sourceURL
	);
	let isComposite = $derived(catalogEntry?.manifest?.runtime === 'composite');
	let needsUpdate = $derived(catalogEntry?.needsUpdate === true);
	let showUpgradeNotification = $derived(isComposite && needsUpdate && !isAdminReadonly);

	let upgrading = $state(false);
	let showUpgradeConfirm = $state(false);
	let componentDiffs = $state<
		Array<{
			id: string;
			name: string;
			type: string;
			oldManifest: MCPCatalogEntryServerManifest | undefined;
			newManifest: MCPServer | MCPCatalogEntryServerManifest | undefined;
		}>
	>([]);
	let diffDialog: DiffDialog | undefined = $state();
	let selectedDiff: {
		id: string;
		name: string;
		oldManifest: MCPCatalogEntryServerManifest | undefined;
		newManifest: MCPServer | MCPCatalogEntryServerManifest | undefined;
	} | null = $state(null);

	let upgradeSuccessDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	const hasExistingConfigured = $derived(
		Boolean(
			catalogEntry &&
				mcpServersAndEntries.current.userConfiguredServers.some(
					(server) => server.catalogEntryID === catalogEntry?.id
				)
		)
	);

	async function handleUpgradeClick() {
		if (!catalogEntry || upgrading) return;

		// Calculate component changes by fetching current component entries
		try {
			const currentComponents = catalogEntry.manifest?.compositeConfig?.componentServers || [];
			const diffs: typeof componentDiffs = [];

			// Check for modified components by fetching their current manifests
			for (const component of currentComponents) {
				try {
					let currentManifest, componentName, componentType;

					if (component.mcpServerID) {
						// Multi-user component
						const server = await AdminService.getMCPCatalogServer(
							DEFAULT_MCP_CATALOG_ID,
							component.mcpServerID,
							{ dontLogErrors: true }
						);
						currentManifest = server.manifest;
						componentName = server.manifest.name ?? component.mcpServerID ?? 'Unnamed Component';
						componentType = 'Multi-User Server';
					} else {
						// Catalog entry component
						const currentEntry = await AdminService.getMCPCatalogEntry(
							DEFAULT_MCP_CATALOG_ID,
							component.catalogEntryID!,
							{ dontLogErrors: true }
						);
						currentManifest = currentEntry.manifest;
						componentName =
							currentEntry.manifest.name ?? component.catalogEntryID ?? 'Unnamed Component';
						componentType = 'Catalog Entry';
					}

					const currentManifestStr = JSON.stringify(currentManifest, null, 2);
					const snapshotManifestStr = JSON.stringify(component.manifest, null, 2);

					if (currentManifestStr !== snapshotManifestStr) {
						diffs.push({
							id: component.catalogEntryID ?? component.mcpServerID ?? componentName,
							name: componentName,
							type: componentType,
							oldManifest: component.manifest,
							newManifest: currentManifest
						});
					}
				} catch (error) {
					const { status } = parseErrorContent(error);
					if (status === 404) {
						const componentName =
							component.manifest?.name ??
							component.catalogEntryID ??
							component.mcpServerID ??
							'Unnamed Component';
						const componentType = component.mcpServerID ? 'Multi-User Server' : 'Catalog Entry';
						// Treat missing parent as empty manifest for diffing (indicates removal)
						diffs.push({
							id: component.catalogEntryID ?? component.mcpServerID ?? componentName,
							name: componentName,
							type: componentType,
							oldManifest: component.manifest,
							newManifest: undefined
						});
					} else {
						// If we can't fetch the entry, it might have been deleted or another error occurred
						console.warn(`Could not fetch component:`, error);
					}
				}
			}

			componentDiffs = diffs;
			showUpgradeConfirm = true;
		} catch (error) {
			console.error('Failed to calculate component changes:', error);
		}
	}

	async function confirmUpgrade() {
		if (!catalogEntry || upgrading) return;

		upgrading = true;
		// Optimistically clear the update flag to avoid waiting on backend reconcile
		const prevNeedsUpdate = !!catalogEntry.needsUpdate;
		catalogEntry = { ...catalogEntry, needsUpdate: false };

		try {
			const updated = await AdminService.refreshCompositeComponents(
				DEFAULT_MCP_CATALOG_ID,
				catalogEntry.id
			);
			// Keep the flag cleared even if backend status lags
			catalogEntry = { ...updated, needsUpdate: false };
			showUpgradeConfirm = false;
			upgradeSuccessDialog?.open();
		} catch (error) {
			// Restore on error
			catalogEntry = { ...catalogEntry, needsUpdate: prevNeedsUpdate };
			console.error('Failed to refresh composite components:', error);
		} finally {
			upgrading = false;
		}
	}

	let title = $derived(catalogEntry?.manifest?.name ?? 'MCP Server');
	let promptInitialLaunch = $derived(page.url.searchParams.get('launch') === 'true');
	let promptOAuthConfig = $derived(page.url.searchParams.get('configure-oauth') === 'true');
</script>

<Layout
	main={{
		component: VirtualPageViewport as unknown as Component,
		props: { class: '', as: 'main', itemHeight: 56, overscan: 5, disabled: true }
	}}
	{title}
	showBackButton
>
	{#snippet rightNavActions()}
		<McpServerActions entry={catalogEntry} {promptInitialLaunch} {promptOAuthConfig} />
	{/snippet}
	<div class="flex h-full flex-col gap-6" in:fly={{ x: 100, delay: duration, duration }}>
		{#if showUpgradeNotification}
			<div class="border-primary bg-primary/10 flex items-center gap-3 rounded-lg border p-4">
				<Info class="text-primary size-5 flex-shrink-0" />
				<div class="flex-1">
					<p class="text-sm font-medium">Component updates available</p>
					<p class="text-muted-foreground mt-1 text-xs">
						One or more components in this composite catalog entry have been updated.
					</p>
				</div>
				<button
					class="button-primary flex items-center gap-1.5 text-sm font-normal"
					onclick={handleUpgradeClick}
					disabled={upgrading}
				>
					<CircleFadingArrowUp class="size-4" />
					{upgrading ? 'Upgrading...' : 'Upgrade'}
				</button>
			</div>
		{/if}

		<McpServerEntryForm
			entry={catalogEntry}
			type={catalogEntry?.manifest.runtime === 'composite'
				? 'composite'
				: catalogEntry?.manifest.runtime === 'remote'
					? 'remote'
					: 'single'}
			readonly={isAdminReadonly || isSourcedEntry}
			id={DEFAULT_MCP_CATALOG_ID}
			onCancel={() => {
				goto('/admin/mcp-servers');
			}}
			onSubmit={async (_id, _type, message) => {
				if (message) {
					success.add(message);
				}
			}}
			{hasExistingConfigured}
		/>
	</div>
</Layout>

<Confirm
	title="Confirm Upgrade"
	show={showUpgradeConfirm}
	onsuccess={confirmUpgrade}
	oncancel={() => (showUpgradeConfirm = false)}
	loading={upgrading}
	classes={{
		confirm: 'bg-primary hover:bg-primary/50 transition-colors duration-200'
	}}
	type="info"
	msg="Upgrade Composite Catalog Entry?"
>
	{#snippet note()}
		<p class="text-sm font-light">
			The configuration for one or more component servers has changed. Would you like to update this
			server to match the latest configuration?
		</p>
		{#if componentDiffs.length > 0}
			<div class="max-h-96 space-y-4 overflow-y-auto text-sm">
				<p class="mb-2 font-medium">Components with updates ({componentDiffs.length}):</p>
				{#each componentDiffs as diff (diff.id)}
					<div class="border-border/50 bg-secondary/20 rounded border p-3">
						<div class="flex items-start justify-between">
							<div class="flex-1">
								<p class="mb-2 font-medium">
									{diff.name}
									{#if !diff.newManifest}
										<span
											class="ml-2 rounded bg-red-500/10 px-2 py-0.5 text-xs font-normal text-red-500"
										>
											Removed
										</span>
									{/if}
								</p>
							</div>
							{#if diff.newManifest}
								<button
									type="button"
									class="text-primary hover:bg-primary/10 flex items-center gap-1.5 rounded px-3 py-1.5 text-xs"
									onclick={() => {
										selectedDiff = {
											id: diff.id,
											name: diff.name,
											oldManifest: diff.oldManifest,
											newManifest: diff.newManifest
										};
										diffDialog?.open();
									}}
								>
									<GitCompare class="size-3.5" />
									View Diff
								</button>
							{/if}
						</div>
					</div>
				{/each}
			</div>
		{/if}
	{/snippet}
</Confirm>

<ResponsiveDialog bind:this={upgradeSuccessDialog} title="Update Applied" class="md:w-sm">
	<div class="p-4">
		<p class="text-sm">You can update tool selections from the Configuration tab</p>
	</div>
</ResponsiveDialog>

<DiffDialog
	bind:this={diffDialog}
	fromServer={selectedDiff
		? ({
				id: selectedDiff.id,
				manifest: selectedDiff.oldManifest as unknown as MCPServer
			} as unknown as MCPCatalogServer)
		: undefined}
	toServer={selectedDiff
		? ({
				id: selectedDiff.id,
				manifest: selectedDiff.newManifest as unknown as MCPServer
			} as unknown as MCPCatalogServer)
		: undefined}
/>

<svelte:head>
	<title>Obot | {catalogEntry?.manifest?.name ?? 'MCP Server'}</title>
</svelte:head>
