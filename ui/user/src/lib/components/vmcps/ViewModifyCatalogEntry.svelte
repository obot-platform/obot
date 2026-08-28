<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import DiffDialog from '$lib/components/admin/DiffDialog.svelte';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import McpDeprecatedNotice from '$lib/components/mcp/McpDeprecatedNotice.svelte';
	import McpDetachedNotice from '$lib/components/mcp/McpDetachedNotice.svelte';
	import McpServerActions from '$lib/components/mcp/McpServerActions.svelte';
	import SelectServerType from '$lib/components/mcp/SelectServerType.svelte';
	import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
	import { normalizeManifestsForDiff } from '$lib/diff';
	import { parseErrorContent } from '$lib/errors';
	import {
		AdminService,
		UserService,
		type LaunchServerType,
		type MCPCatalogEntry,
		type MCPCatalogEntryServerManifest,
		type MCPCatalogServer,
		type MCPServer
	} from '$lib/services';
	import {
		getMCPDisplayName,
		getServerTypeLabelByType,
		isDeprecatedMCPServer,
		isMultiUserCatalogEntry
	} from '$lib/services/user/mcp';
	import { errors, mcpServersAndEntries, profile, responsive } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { CircleFadingArrowUp, GitCompare, Info } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		workspaceId?: string;
		rightOffsetWidth?: number;
		onCreated?: (created: MCPCatalogEntry) => void | Promise<void>;
	}

	let { workspaceId, rightOffsetWidth, onCreated }: Props = $props();

	let selectServerTypeDialog = $state<ReturnType<typeof SelectServerType>>();
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let selectedServerType = $state<LaunchServerType>();
	let creating = $state(false);
	let closeAfterCreate = $state(false);
	let catalogEntry = $state<MCPCatalogEntry>();
	let mcpServer = $state<MCPCatalogServer>();
	let promptInitialLaunch = $state(false);
	let promptOAuthConfig = $state(false);
	let hydrateController: AbortController | undefined;

	let isAdmin = $derived(!!profile.current.isAdmin?.());
	let isAdminReadonly = $derived(!!profile.current.isAdminReadonly?.());
	let createEntity = $derived(isAdmin ? ('catalog' as const) : ('workspace' as const));
	let createScopeId = $derived(isAdmin ? DEFAULT_MCP_CATALOG_ID : (workspaceId ?? ''));

	let viewWorkspaceId = $derived(
		catalogEntry?.powerUserWorkspaceID || mcpServer?.powerUserWorkspaceID
	);
	let serverScopeEntity = $derived(viewWorkspaceId ? ('workspace' as const) : ('catalog' as const));
	let serverScopeID = $derived(viewWorkspaceId || DEFAULT_MCP_CATALOG_ID);

	let isSourcedEntry = $derived(
		catalogEntry && 'sourceURL' in catalogEntry && !!catalogEntry.sourceURL
	);
	let isComposite = $derived(catalogEntry?.manifest?.runtime === 'composite');
	let needsUpdate = $derived(catalogEntry?.needsUpdate === true);
	let showUpgradeNotification = $derived(isComposite && needsUpdate && !isAdminReadonly);
	let deprecated = $derived(
		isDeprecatedMCPServer(catalogEntry) || isDeprecatedMCPServer(mcpServer)
	);
	let catalogEntryFormType = $derived<'composite' | 'remote' | 'hosted'>(
		catalogEntry?.manifest.runtime === 'composite'
			? 'composite'
			: catalogEntry?.manifest.runtime === 'remote'
				? 'remote'
				: 'hosted'
	);

	let title = $derived(
		creating
			? `Create ${getServerTypeLabelByType(selectedServerType)} Entry`
			: catalogEntry
				? (catalogEntry.manifest.name ?? 'MCP Server')
				: (getMCPDisplayName(mcpServer) ?? 'MCP Server')
	);

	let formKey = $derived(
		creating
			? `create-${selectedServerType ?? 'unknown'}`
			: catalogEntry
				? `entry-${catalogEntry.id}`
				: mcpServer
					? `server-${mcpServer.id}`
					: 'empty'
	);

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

	export function start(options?: { closeAfterCreate?: boolean }) {
		resetView();
		creating = true;
		closeAfterCreate = options?.closeAfterCreate ?? false;
		selectedServerType = undefined;
		selectServerTypeDialog?.open();
	}

	export async function open(entity: MCPCatalogEntry | MCPCatalogServer) {
		hydrateController?.abort();
		const controller = new AbortController();
		hydrateController = controller;
		selectServerTypeDialog?.close();
		creating = false;
		selectedServerType = undefined;
		clearPrompts();
		if (isCatalogEntryEntity(entity)) {
			catalogEntry = entity;
			mcpServer = undefined;
		} else {
			mcpServer = entity;
			catalogEntry = undefined;
		}
		dialog?.open();
		await hydrate(entity, controller.signal);
		if (hydrateController === controller) hydrateController = undefined;
	}

	function handleSelectServerType(serverType: LaunchServerType) {
		selectServerTypeDialog?.close();
		resetView();
		selectedServerType = serverType;
		creating = true;
		dialog?.open();
	}

	function close() {
		dialog?.close();
		reset();
	}

	function resetView() {
		hydrateController?.abort();
		hydrateController = undefined;
		catalogEntry = undefined;
		mcpServer = undefined;
		showUpgradeConfirm = false;
		componentDiffs = [];
		selectedDiff = null;
		clearPrompts();
	}

	function clearPrompts() {
		promptInitialLaunch = false;
		promptOAuthConfig = false;
	}

	function reset() {
		creating = false;
		closeAfterCreate = false;
		selectedServerType = undefined;
		resetView();
	}

	function isCatalogEntryEntity(
		entity: MCPCatalogEntry | MCPCatalogServer
	): entity is MCPCatalogEntry {
		return 'isCatalogEntry' in entity && entity.isCatalogEntry;
	}

	async function hydrate(entity: MCPCatalogEntry | MCPCatalogServer, signal: AbortSignal) {
		try {
			if (isCatalogEntryEntity(entity)) {
				const hydratedEntry = await loadCatalogEntry(
					entity.id,
					entity.powerUserWorkspaceID,
					signal
				);
				if (signal.aborted) return;
				catalogEntry = hydratedEntry;
				mcpServer = undefined;
			} else {
				const hydratedServer = await loadCatalogServer(
					entity.id,
					entity.powerUserWorkspaceID,
					entity.mcpCatalogID,
					signal
				);
				if (signal.aborted) return;
				mcpServer = hydratedServer;
				catalogEntry = undefined;
			}
		} catch {
			// Keep the entity already shown in the dialog.
		}
	}

	async function loadCatalogEntry(id: string, entryWorkspaceId?: string, signal?: AbortSignal) {
		const opts = signal ? { signal } : undefined;
		if (entryWorkspaceId && !isAdmin) {
			return UserService.getWorkspaceMCPCatalogEntry(entryWorkspaceId, id, opts);
		}
		return AdminService.getMCPCatalogEntry(DEFAULT_MCP_CATALOG_ID, id, opts);
	}

	async function loadCatalogServer(
		id: string,
		serverWorkspaceId?: string,
		catalogId?: string,
		signal?: AbortSignal
	) {
		const opts = signal ? { signal } : undefined;
		if (serverWorkspaceId && !isAdmin) {
			return UserService.getWorkspaceMCPCatalogServer(serverWorkspaceId, id, opts);
		}
		return AdminService.getMCPCatalogServer(catalogId || DEFAULT_MCP_CATALOG_ID, id, opts);
	}

	async function handleCreated(id: string, _isMultiUserEntry: boolean, message?: string) {
		const asServer = selectedServerType === 'multi';
		try {
			if (asServer) {
				mcpServer = await loadCatalogServer(id, workspaceId);
				catalogEntry = undefined;
			} else {
				catalogEntry = await loadCatalogEntry(id, workspaceId);
				mcpServer = undefined;
			}

			// The caller owns the next step, so the entry does not stay parked behind its own dialog.
			if (closeAfterCreate && catalogEntry) {
				const created = catalogEntry;
				close();
				await mcpServersAndEntries.refreshAll();
				await onCreated?.(created);
				return;
			}

			// Stands in for the `?launch=true` / `?configure-oauth=true` params the catalog route
			// appends when it redirects to the created entry.
			promptOAuthConfig = !asServer && message === 'requires-oauth-config';
			promptInitialLaunch = !asServer && !promptOAuthConfig;
			creating = false;
			selectedServerType = undefined;
			await mcpServersAndEntries.refreshAll();
			if (catalogEntry) await onCreated?.(catalogEntry);
		} catch {
			errors.append('The entry was created, but it could not be opened.');
			close();
		}
	}

	async function handleOAuthConfigured() {
		if (!catalogEntry) return;
		catalogEntry = await loadCatalogEntry(catalogEntry.id, catalogEntry.powerUserWorkspaceID);
	}

	function handleConnect({
		entry,
		server
	}: {
		entry?: MCPCatalogEntry;
		server?: MCPCatalogServer;
	}) {
		if (isMultiUserCatalogEntry(entry) && server) {
			success.add(`${server.alias || server.manifest.name} has been created.`);
		}
	}

	async function acceptOwnership() {
		if (!catalogEntry) return;
		catalogEntry = await AdminService.acceptMCPCatalogEntryOwnership(
			DEFAULT_MCP_CATALOG_ID,
			catalogEntry.id
		);
	}

	async function handleUpgradeClick() {
		if (!catalogEntry || upgrading) return;

		try {
			const currentComponents = catalogEntry.manifest?.compositeConfig?.componentServers || [];
			const diffs: typeof componentDiffs = [];

			for (const component of currentComponents) {
				try {
					let currentManifest, componentName, componentType;

					if (component.mcpServerID) {
						const server = await AdminService.getMCPCatalogServer(
							DEFAULT_MCP_CATALOG_ID,
							component.mcpServerID,
							{ dontLogErrors: true }
						);
						currentManifest = server.manifest;
						componentName = server.manifest.name ?? component.mcpServerID ?? 'Unnamed Component';
						componentType = 'Multi-User Server';
					} else {
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

					const [snapshotManifest, normalizedCurrentManifest] = normalizeManifestsForDiff(
						component.manifest,
						currentManifest
					);
					const currentManifestStr = JSON.stringify(normalizedCurrentManifest, null, 2);
					const snapshotManifestStr = JSON.stringify(snapshotManifest, null, 2);

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
						diffs.push({
							id: component.catalogEntryID ?? component.mcpServerID ?? componentName,
							name: componentName,
							type: componentType,
							oldManifest: component.manifest,
							newManifest: undefined
						});
					} else {
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
		const prevNeedsUpdate = !!catalogEntry.needsUpdate;
		catalogEntry = { ...catalogEntry, needsUpdate: false };

		try {
			const updated = await AdminService.refreshCompositeComponents(
				DEFAULT_MCP_CATALOG_ID,
				catalogEntry.id
			);
			catalogEntry = { ...updated, needsUpdate: false };
			showUpgradeConfirm = false;
			upgradeSuccessDialog?.open();
		} catch (error) {
			catalogEntry = { ...catalogEntry, needsUpdate: prevNeedsUpdate };
			console.error('Failed to refresh composite components:', error);
		} finally {
			upgrading = false;
		}
	}
</script>

<ResponsiveDialog
	animate="fade"
	bind:this={dialog}
	class={twMerge(
		'bg-base-200 dark:bg-base-100 max-h-[calc(100dvh-2rem)] h-[calc(100dvh-2rem)]',
		'max-w-[calc(100%-2rem)] w-[calc(100%-2rem)]'
	)}
	rightPanelWidth={responsive.isMobile ? undefined : rightOffsetWidth}
	{title}
	onClose={reset}
>
	{#key formKey}
		<div class="flex h-full flex-col gap-6">
			{#if !creating}
				<McpDeprecatedNotice {deprecated} variant="notification" />
			{/if}

			{#if catalogEntry && profile.current.hasAdminAccess?.()}
				<McpDetachedNotice
					detached={catalogEntry.detached}
					sourceURL={catalogEntry.sourceURL}
					variant="notification"
					onAcceptOwnership={isAdminReadonly ? undefined : acceptOwnership}
				/>
			{/if}

			{#if showUpgradeNotification}
				<div class="border-primary bg-primary/10 flex items-center gap-3 rounded-lg border p-4">
					<Info class="text-primary size-5 shrink-0" />
					<div class="flex-1">
						<p class="text-sm font-medium">Component updates available</p>
						<p class="text-muted-foreground mt-1 text-xs">
							One or more components in this composite catalog entry have been updated.
						</p>
					</div>
					<button
						class="btn btn-primary flex items-center gap-1.5 text-sm font-normal"
						onclick={handleUpgradeClick}
						disabled={upgrading}
					>
						<CircleFadingArrowUp class="size-4" />
						{upgrading ? 'Upgrading...' : 'Upgrade'}
					</button>
				</div>
			{/if}

			{#if creating}
				<McpServerEntryForm
					entity={createEntity}
					type={selectedServerType}
					id={createScopeId}
					onCancel={close}
					onSubmit={handleCreated}
					excludeViews={['overview']}
				/>
			{:else if mcpServer}
				<McpServerEntryForm
					entry={mcpServer}
					type="multi"
					id={serverScopeID}
					entity={serverScopeEntity}
					readonly={isAdminReadonly}
					allowMultiUserServerConfigurationEdit
					limitViews={['overview', 'tools']}
				/>
			{:else if catalogEntry}
				<McpServerEntryForm
					entry={catalogEntry}
					type={catalogEntryFormType}
					readonly={isAdminReadonly || isSourcedEntry}
					id={serverScopeID}
					entity={serverScopeEntity}
					limitViews={['overview', 'tools']}
				/>
			{/if}
		</div>
	{/key}
</ResponsiveDialog>

<SelectServerType
	bind:this={selectServerTypeDialog}
	entity={createEntity}
	onSelectServerType={handleSelectServerType}
	hideComposite
/>

{#key formKey}
	{#if !creating && (catalogEntry || mcpServer)}
		<McpServerActions
			entry={catalogEntry}
			server={mcpServer}
			catalogID={viewWorkspaceId ? undefined : serverScopeID}
			workspaceID={viewWorkspaceId}
			readonly={isAdminReadonly}
			allowMultiUserServerConfigurationEdit={!!mcpServer}
			{promptInitialLaunch}
			{promptOAuthConfig}
			onOAuthConfigured={handleOAuthConfigured}
			onConnect={handleConnect}
			hideActions
		/>
	{/if}
{/key}

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
			The configuration for one or more component entries has changed. Would you like to update this
			catalog entry to match the latest configuration?
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
											class="ml-2 rounded bg-error/10 px-2 py-0.5 text-xs font-normal text-error"
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
