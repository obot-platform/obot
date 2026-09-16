<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import McpDeprecatedNotice from '$lib/components/mcp/McpDeprecatedNotice.svelte';
	import McpDetachedNotice from '$lib/components/mcp/McpDetachedNotice.svelte';
	import McpServerActions from '$lib/components/mcp/McpServerActions.svelte';
	import SelectServerType from '$lib/components/mcp/SelectServerType.svelte';
	import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
	import { AdminService, UserService, type LaunchType, type MCPCatalogEntry } from '$lib/services';
	import { getServerTypeLabelByType, isDeprecatedMCPServer } from '$lib/services/user/mcp';
	import { errors, mcpServersAndEntries, profile, responsive } from '$lib/stores';
	import { Plus } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		workspaceId?: string;
		rightOffsetWidth?: number;
		onCreated?: (created: MCPCatalogEntry) => void | Promise<void>;
		onAddToVMcp?: (entry: MCPCatalogEntry) => void | Promise<void>;
		addToVMcpLabel?: string;
		isAddedToVMcp?: (entry: MCPCatalogEntry) => boolean;
	}

	let {
		workspaceId,
		rightOffsetWidth,
		onCreated,
		onAddToVMcp,
		addToVMcpLabel = 'Add to vMCP',
		isAddedToVMcp
	}: Props = $props();
	let selectServerTypeDialog = $state<ReturnType<typeof SelectServerType>>();
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let selectedServerType = $state<LaunchType>();
	let creating = $state(false);
	let closeAfterCreate = $state(false);
	let catalogEntry = $state<MCPCatalogEntry>();
	let promptOAuthConfig = $state(false);
	let hydrateController: AbortController | undefined;

	let isAdmin = $derived(!!profile.current.isAdmin?.());
	let isAdminReadonly = $derived(!!profile.current.isAdminReadonly?.());
	let createEntity = $derived(isAdmin ? ('catalog' as const) : ('workspace' as const));
	let createScopeId = $derived(isAdmin ? DEFAULT_MCP_CATALOG_ID : (workspaceId ?? ''));
	let viewWorkspaceId = $derived(catalogEntry?.powerUserWorkspaceID);
	let serverScopeEntity = $derived(viewWorkspaceId ? ('workspace' as const) : ('catalog' as const));
	let serverScopeID = $derived(viewWorkspaceId || DEFAULT_MCP_CATALOG_ID);
	let isSourcedEntry = $derived(
		catalogEntry && 'sourceURL' in catalogEntry && !!catalogEntry.sourceURL
	);
	let deprecated = $derived(isDeprecatedMCPServer(catalogEntry));
	let catalogEntryFormType = $derived<LaunchType>(
		catalogEntry?.manifest.runtime === 'remote' ? 'remote' : 'hosted'
	);
	let title = $derived(
		creating
			? `Create ${getServerTypeLabelByType(selectedServerType)} Entry`
			: (catalogEntry?.manifest.name ?? 'MCP Server')
	);
	let formKey = $derived(
		creating
			? `create-${selectedServerType ?? 'unknown'}`
			: catalogEntry
				? `entry-${catalogEntry.id}`
				: 'empty'
	);

	export function start(options?: { closeAfterCreate?: boolean }) {
		resetView();
		creating = true;
		closeAfterCreate = options?.closeAfterCreate ?? false;
		selectedServerType = undefined;
		selectServerTypeDialog?.open();
	}

	export async function open(entity: MCPCatalogEntry) {
		hydrateController?.abort();
		const controller = new AbortController();
		hydrateController = controller;
		selectServerTypeDialog?.close();
		creating = false;
		selectedServerType = undefined;
		clearPrompts();
		catalogEntry = entity;
		dialog?.open();
		await hydrate(entity, controller.signal);
		if (hydrateController === controller) hydrateController = undefined;
	}

	function handleSelectServerType(serverType: LaunchType) {
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

	let alreadyAdded = $derived(Boolean(catalogEntry && isAddedToVMcp?.(catalogEntry)));
	let showAddToVMcp = $derived(
		Boolean(onAddToVMcp && catalogEntry && !creating && responsive.isMobile)
	);

	function handleAddToVMcp() {
		if (!catalogEntry || alreadyAdded) return;
		const entry = catalogEntry;
		close();
		void onAddToVMcp?.(entry);
	}

	function resetView() {
		hydrateController?.abort();
		hydrateController = undefined;
		catalogEntry = undefined;
		clearPrompts();
	}

	function clearPrompts() {
		promptOAuthConfig = false;
	}

	function reset() {
		creating = false;
		closeAfterCreate = false;
		selectedServerType = undefined;
		resetView();
	}

	async function hydrate(entity: MCPCatalogEntry, signal: AbortSignal) {
		try {
			const hydratedEntry = await loadCatalogEntry(entity.id, entity.powerUserWorkspaceID, signal);
			if (signal.aborted) return;
			catalogEntry = hydratedEntry;
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

	async function handleCreated(id: string, _isMultiUserEntry: boolean, message?: string) {
		try {
			const createdEntry = await loadCatalogEntry(id, workspaceId);

			if (closeAfterCreate) {
				close();
				await mcpServersAndEntries.refreshAll();
				await onCreated?.(createdEntry);
				return;
			}

			catalogEntry = createdEntry;
			promptOAuthConfig = message === 'requires-oauth-config';
			creating = false;
			selectedServerType = undefined;
			await mcpServersAndEntries.refreshAll();
			await onCreated?.(createdEntry);
		} catch {
			errors.append('The entry was created, but it could not be opened.');
			close();
		}
	}

	async function handleOAuthConfigured() {
		if (!catalogEntry) return;
		catalogEntry = await loadCatalogEntry(catalogEntry.id, catalogEntry.powerUserWorkspaceID);
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
		'bg-base-200 dark:bg-base-100 md:max-h-[calc(100dvh-2rem)] md:h-[calc(100dvh-2rem)]',
		'md:max-w-[calc(100%-2rem)] md:w-[calc(100%-2rem)]'
	)}
	rightPanelWidth={responsive.isMobile ? undefined : rightOffsetWidth}
	{title}
	onClose={reset}
>
	{#key formKey}
		<div class="flex h-full flex-col gap-6 p-4 md:p-0">
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
					hideTitleBarAction
					entity={createEntity}
					type={selectedServerType}
					id={createScopeId}
					onCancel={close}
					onSubmit={handleCreated}
					excludeViews={['overview']}
					isDialogView
				/>
			{:else if catalogEntry}
				<McpServerEntryForm
					hideTitleBarAction
					entry={catalogEntry}
					type={catalogEntryFormType}
					readonly={isAdminReadonly || isSourcedEntry}
					id={serverScopeID}
					entity={serverScopeEntity}
					limitViews={['overview', 'tools']}
					isDialogView
				/>
			{/if}
			{#if showAddToVMcp}
				<div class="p-2 fixed bottom-0 left-0 w-full">
					<button
						type="button"
						class="btn btn-primary w-full"
						disabled={alreadyAdded}
						onclick={handleAddToVMcp}
					>
						<Plus class="size-4" />
						{alreadyAdded ? 'Already added to vMCP' : addToVMcpLabel}
					</button>
				</div>
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
	{#if !creating && catalogEntry}
		<McpServerActions
			entry={catalogEntry}
			catalogID={viewWorkspaceId ? undefined : serverScopeID}
			workspaceID={viewWorkspaceId}
			readonly={isAdminReadonly}
			{promptOAuthConfig}
			onOAuthConfigured={handleOAuthConfigured}
			hideActions
			skipConnectDialog
		/>
	{/if}
{/key}
