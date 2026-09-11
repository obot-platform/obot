<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import McpDeprecatedNotice from '$lib/components/mcp/McpDeprecatedNotice.svelte';
	import McpDetachedNotice from '$lib/components/mcp/McpDetachedNotice.svelte';
	import McpServerActions from '$lib/components/mcp/McpServerActions.svelte';
	import SelectServerType from '$lib/components/mcp/SelectServerType.svelte';
	import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
	import {
		AdminService,
		UserService,
		type LaunchServerType,
		type MCPCatalogEntry,
		type MCPCatalogServer
	} from '$lib/services';
	import {
		getMCPDisplayName,
		getServerTypeLabelByType,
		isDeprecatedMCPServer,
		isMultiUserCatalogEntry
	} from '$lib/services/user/mcp';
	import { errors, mcpServersAndEntries, profile, responsive } from '$lib/stores';
	import { success } from '$lib/stores/success';
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
	let deprecated = $derived(
		isDeprecatedMCPServer(catalogEntry) || isDeprecatedMCPServer(mcpServer)
	);
	let catalogEntryFormType = $derived<'remote' | 'hosted'>(
		catalogEntry?.manifest.runtime === 'remote' ? 'remote' : 'hosted'
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
		if (!profile.current.hasAdminAccess?.()) {
			return UserService.getMCP(id, opts);
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
			if (closeAfterCreate && catalogEntry) {
				const created = catalogEntry;
				close();
				await mcpServersAndEntries.refreshAll();
				await onCreated?.(created);
				return;
			}
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
