<script lang="ts">
	import { page } from '$app/state';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import CopyField from '$lib/components/CopyField.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import ConnectToServer from '$lib/components/mcp/ConnectToServer.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import CreateEditVMcp from '$lib/components/vmcps/CreateEditVMcp.svelte';
	import CreateVMcpButton from '$lib/components/vmcps/CreateVMcpButton.svelte';
	import McpServersSidebar from '$lib/components/vmcps/McpServersSidebar.svelte';
	import VMcpDragOverlay from '$lib/components/vmcps/VMcpDragOverlay.svelte';
	import VMcpGraph from '$lib/components/vmcps/VMcpGraph.svelte';
	import VMcpGraphRow from '$lib/components/vmcps/VMcpGraphRow.svelte';
	import VMcpSettings from '$lib/components/vmcps/VMcpSettings.svelte';
	import VMcpTable from '$lib/components/vmcps/VMcpTable.svelte';
	import VMcpToolDialogs from '$lib/components/vmcps/VMcpToolDialogs.svelte';
	import ViewModifyCatalogEntry from '$lib/components/vmcps/ViewModifyCatalogEntry.svelte';
	import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
	import Loading from '$lib/icons/Loading.svelte';
	import { createEntryDrag } from '$lib/runes/vmcps/entryDrag.svelte';
	import { createVMcpToolFlow } from '$lib/runes/vmcps/vmcpToolFlow.svelte';
	import {
		AdminService,
		Group,
		UserService,
		type CatalogComponentServer,
		type MCPCatalogEntry,
		type MCPServerInstance,
		type OrgUser
	} from '$lib/services';
	import { AiClient, COMMON_AI_CLIENTS } from '$lib/services/user/constants';
	import { isMultiUserCatalogEntry, isMultiUserServer } from '$lib/services/user/mcp';
	import { vmcpRowHeight } from '$lib/services/vmcps/camera';
	import { SHORT_DESCRIPTION_MAX_LENGTH } from '$lib/services/vmcps/constants';
	import type { VMcpSortBy } from '$lib/services/vmcps/types';
	import {
		appendComponentLabel,
		buildVMcpComponentFilterOptions,
		filterVMcps,
		isWorkspaceOwned,
		sortVMcps,
		resolveVMcpComponents,
		buildConnectAllSnippets
	} from '$lib/services/vmcps/utils';
	import { errors, mcpServersAndEntries, profile } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { setUrlParamAndUpdateUrl } from '$lib/url';
	import { ChartBarStacked, Table } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	const INITIAL_EXPANDED_VMCPS = 5;
	const EXPANDED_VMCPS_STORAGE_KEY = 'vmcps.expandedIds';
	const options = COMMON_AI_CLIENTS.slice(0, 4);

	let viewType = $state<'graph' | 'table'>('graph');
	let showRightPanel = $state(true);
	let isLoading = $derived(mcpServersAndEntries.current.loading);
	let showAllConnectors = $state(false);
	let sortBy = $state<VMcpSortBy>('name');
	let nameFilterBy = $state('');
	let ownerFilterBy = $state('');
	let componentFilterBy = $state('');
	let allComposites = $derived(
		mcpServersAndEntries.current.entries.filter(
			(entry) =>
				entry.manifest.runtime === 'composite' && (showAllConnectors || !isWorkspaceOwned(entry))
		)
	);
	function componentFilterLabel(id: string) {
		const entry = mcpServersAndEntries.current.entries.find((candidate) => candidate.id === id);
		if (entry?.manifest.name) return entry.manifest.name;
		return mcpServersAndEntries.current.servers.find((candidate) => candidate.id === id)?.manifest
			.name;
	}
	let componentFilterOptions = $derived(
		buildVMcpComponentFilterOptions(allComposites, componentFilterLabel)
	);

	let createEditVMcp = $state<ReturnType<typeof CreateEditVMcp>>();
	let catalogEntryDialog = $state<ReturnType<typeof ViewModifyCatalogEntry>>();
	let connectToServerDialog = $state<ReturnType<typeof ConnectToServer>>();

	let connectAllVMcpsDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let connectAllCopyField = $state<ReturnType<typeof CopyField>>();
	let selectedClient = $state<(typeof COMMON_AI_CLIENTS)[number]>();
	let isAdmin = $derived(!!profile.current.isAdmin?.());
	let connectAllVmcps = $derived(allComposites.filter((vmcp) => Boolean(vmcp.connectURL)));
	let connectAllSnippets = $derived(
		selectedClient ? buildConnectAllSnippets(selectedClient.id, connectAllVmcps, isAdmin) : []
	);
	let selectedConnectAllSnippetId = $state<string>();
	let selectedConnectAllSnippet = $derived(
		connectAllSnippets.find((snippet) => snippet.id === selectedConnectAllSnippetId) ??
			connectAllSnippets[0]
	);

	let users = $state<OrgUser[]>([]);

	let rightPanelEl = $state<HTMLElement>();
	let rightPanelWidth = $state(0);
	let pendingEntryDrop = $state<{ vmcp?: MCPCatalogEntry }>();
	let expandedVMcpIds = $state<string[]>([]);
	let expandedInitialized = $state(false);
	const toolFlow = createVMcpToolFlow();

	let query = $derived(page.url.searchParams.get('query') ?? '');
	let usersMap = $derived(new Map(users.map((user) => [user.id, user])));

	let composites = $derived(
		sortVMcps(
			filterVMcps(
				allComposites,
				{
					names: nameFilterBy,
					owners: ownerFilterBy,
					components: componentFilterBy
				},
				usersMap
			),
			sortBy
		)
	);

	onMount(() => {
		UserService.listUsersIncludeDeleted().then((response) => {
			users = response;
		});
	});

	function catalogEntryForComponent(component: CatalogComponentServer) {
		if (component.catalogEntryID) {
			return mcpServersAndEntries.current.entries.find(
				(entry) => entry.id === component.catalogEntryID
			);
		}

		if (!component.mcpServerID) return undefined;

		const server = mcpServersAndEntries.current.servers.find(
			(candidate) => candidate.id === component.mcpServerID
		);
		if (!server?.catalogEntryID) return undefined;

		return mcpServersAndEntries.current.entries.find((entry) => entry.id === server.catalogEntryID);
	}

	function componentManifestField(
		component: CatalogComponentServer,
		field: 'name' | 'shortDescription'
	) {
		const entry = catalogEntryForComponent(component);
		const server = component.mcpServerID
			? mcpServersAndEntries.current.servers.find(
					(candidate) => candidate.id === component.mcpServerID
				)
			: undefined;
		const manifest = component.manifest ?? entry?.manifest ?? server?.manifest;
		return manifest?.[field];
	}

	let canCreateCatalogEntry = $derived(
		profile.current.isAdmin?.() || profile.current.groups.includes(Group.POWERUSER)
	);

	const entryDrag = createEntryDrag({
		composites: () => composites,
		panelEl: () => rightPanelEl,
		openEntry: (entry) => openCatalogEntry(entry),
		createEntry: (target) => startCatalogEntryCreation(target),
		dropOnCreate: (entry) => handleDroppedOnCreate(entry),
		dropOnVMcp: (entry, vmcp) => void handleDropped(entry, vmcp)
	});

	$effect(() => {
		const el = rightPanelEl;
		if (!el) return;

		const observer = new ResizeObserver(() => {
			rightPanelWidth = el.getBoundingClientRect().width;
		});
		observer.observe(el);
		return () => observer.disconnect();
	});

	function openCatalogEntry(entry: MCPCatalogEntry) {
		void catalogEntryDialog?.open(entry);
	}

	function startCatalogEntryCreation(target?: { vmcp?: MCPCatalogEntry }) {
		pendingEntryDrop = target;
		catalogEntryDialog?.start(target ? { closeAfterCreate: true } : undefined);
	}

	async function handleCatalogEntryCreated(created: MCPCatalogEntry) {
		const pending = pendingEntryDrop;
		pendingEntryDrop = undefined;
		if (!pending) return;

		if (pending.vmcp) {
			await handleDropped(created, pending.vmcp);
			return;
		}

		handleDroppedOnCreate(created);
	}

	function toComponentServer(entry: MCPCatalogEntry): CatalogComponentServer | undefined {
		if (!isMultiUserCatalogEntry(entry)) {
			return { catalogEntryID: entry.id, manifest: entry.manifest };
		}

		const deployed = mcpServersAndEntries.current.servers.find(
			(server) => isMultiUserServer(server) && server.catalogEntryID === entry.id
		);
		return deployed ? { mcpServerID: deployed.id } : undefined;
	}

	function handleDroppedOnCreate(entry: MCPCatalogEntry) {
		const component = toComponentServer(entry);
		if (!component) {
			errors.append(
				`${entry.manifest.name} is a multi-user server and must be deployed before it can be added to a vMCP.`
			);
			return;
		}

		createEditVMcp?.openCreate([{ ...component, manifest: component.manifest ?? entry.manifest }]);
	}

	async function handleDropped(entry: MCPCatalogEntry, vmcp: MCPCatalogEntry) {
		const component = toComponentServer(entry);
		if (!component) {
			errors.append(
				`${entry.manifest.name} is a multi-user server and must be deployed before it can be added to a vMCP.`
			);
			return;
		}

		try {
			const latest = await AdminService.getMCPCatalogEntry(DEFAULT_MCP_CATALOG_ID, vmcp.id);
			const components = latest.manifest.compositeConfig?.componentServers ?? [];
			if (
				components.some(
					(existing) =>
						(component.catalogEntryID && existing.catalogEntryID === component.catalogEntryID) ||
						(component.mcpServerID && existing.mcpServerID === component.mcpServerID)
				)
			) {
				return;
			}

			const nextComponents = [...components, component];
			const updated = await AdminService.updateMCPCatalogEntry(DEFAULT_MCP_CATALOG_ID, vmcp.id, {
				...latest.manifest,
				name:
					appendComponentLabel(
						latest.manifest.name,
						components.map((existing) => componentManifestField(existing, 'name')),
						entry.manifest.name
					) ?? latest.manifest.name,
				shortDescription:
					appendComponentLabel(
						latest.manifest.shortDescription,
						components.map((existing) => componentManifestField(existing, 'shortDescription')),
						entry.manifest.shortDescription,
						SHORT_DESCRIPTION_MAX_LENGTH
					) ?? latest.manifest.shortDescription,
				compositeConfig: {
					...latest.manifest.compositeConfig,
					componentServers: nextComponents
				}
			});

			mcpServersAndEntries.current.entries = mcpServersAndEntries.current.entries.map(
				(candidate) => (candidate.id === updated.id ? updated : candidate)
			);
			success.add(`${entry.manifest.name} added to ${updated.manifest.name}.`);

			toolFlow.offerToolSelection(entry, updated);
		} catch {
			errors.append('Failed to add MCP server to vMCP.');
		}
	}

	function vmcpComponents(vmcp: MCPCatalogEntry) {
		const { entries, servers } = mcpServersAndEntries.current;
		return resolveVMcpComponents(vmcp, entries, servers);
	}

	function handleConnectVMcp(vmcp: MCPCatalogEntry) {
		connectToServerDialog?.open({ entry: vmcp });
	}

	function openConnectAllDialog(option: (typeof COMMON_AI_CLIENTS)[number]) {
		selectedClient = option;
		selectedConnectAllSnippetId = undefined;
		connectAllCopyField?.clear?.();
		connectAllVMcpsDialog?.open();
	}

	function handleConnectToServer({ instance }: { instance?: MCPServerInstance }) {
		if (instance) {
			mcpServersAndEntries.refreshUserInstances();
		} else {
			mcpServersAndEntries.refreshUserConfiguredServers();
		}
	}

	const updateSearchQuery = (value: string) => {
		setUrlParamAndUpdateUrl(page.url, 'query', value);
	};

	function readExpandedVMcps(): string[] | undefined {
		if (typeof localStorage === 'undefined') return undefined;
		const raw = localStorage.getItem(EXPANDED_VMCPS_STORAGE_KEY);
		if (raw === null) return undefined;
		try {
			const parsed = JSON.parse(raw);
			if (!Array.isArray(parsed)) return [];
			return parsed.filter((id): id is string => typeof id === 'string' && id.length > 0);
		} catch {
			return [];
		}
	}

	function persistExpandedVMcps(ids: string[]) {
		if (typeof localStorage === 'undefined') return;
		localStorage.setItem(EXPANDED_VMCPS_STORAGE_KEY, JSON.stringify(ids));
	}

	function isVMcpExpanded(id: string) {
		return expandedVMcpIds.includes(id);
	}

	function toggleExpandedVMcp(vmcp: MCPCatalogEntry) {
		expandedVMcpIds = isVMcpExpanded(vmcp.id)
			? expandedVMcpIds.filter((id) => id !== vmcp.id)
			: [...expandedVMcpIds, vmcp.id];
		persistExpandedVMcps(expandedVMcpIds);
	}

	$effect(() => {
		if (expandedInitialized) return;
		const stored = readExpandedVMcps();
		if (stored) {
			expandedVMcpIds = stored;
			expandedInitialized = true;
			return;
		}
		if (composites.length === 0) return;
		expandedVMcpIds = composites.slice(0, INITIAL_EXPANDED_VMCPS).map((vmcp) => vmcp.id);
		persistExpandedVMcps(expandedVMcpIds);
		expandedInitialized = true;
	});
</script>

<Layout
	classes={{
		container: 'p-0 md:px-0 min-h-0',
		childrenContainer: 'max-w-full',
		collapsedSidebarHeaderContent: 'p-4 pb-0'
	}}
	title="vMCPs"
>
	{#snippet rightNavActions()}
		<div class="flex items-center gap-2">
			<p class="text-xs font-light">Connect all vMCPs:</p>
			{#each options as option (option.id)}
				<IconButton
					class="btn-sm bg-base-200 hover:bg-base-400 dark:hover:bg-base-300"
					tooltip={{ text: option.alt, placement: 'bottom' }}
					onclick={() => openConnectAllDialog(option)}
				>
					<img src={option.icon} alt={option.alt} class="size-4 block dark:hidden" />
					<img
						src={option.iconDark ?? option.icon}
						alt={option.alt}
						class="size-4 hidden dark:block"
					/>
				</IconButton>
			{/each}
		</div>
	{/snippet}
	<div
		class="@container dark:from-base-300 to-base-200 relative h-full min-h-0 w-full overflow-hidden bg-radial-[at_50%_50%] from-gray-50 p-4 dark:to-black"
	>
		{#if isLoading}
			<Loading class="text-primary" />
		{:else if allComposites.length > 0}
			<div class="absolute top-3 left-3 z-10">
				<VMcpSettings
					bind:showAllConnectors
					bind:sortBy
					bind:ownerFilterBy
					bind:componentFilterBy
					{componentFilterOptions}
				/>
			</div>
			{#if viewType === 'graph'}
				{@render graphView()}
			{:else}
				{@render tableView()}
			{/if}
		{:else}
			<div class="flex h-full items-center justify-center">
				<CreateVMcpButton drag={entryDrag} onCreate={() => createEditVMcp?.openCreate()} />
			</div>
		{/if}
	</div>
	{#snippet rightSidebar()}
		<McpServersSidebar
			bind:panelEl={rightPanelEl}
			bind:open={showRightPanel}
			drag={entryDrag}
			{query}
			onSearch={updateSearchQuery}
			{showAllConnectors}
			canCreateEntry={canCreateCatalogEntry}
		/>
	{/snippet}
</Layout>

{#snippet graphView()}
	<VMcpGraph
		items={composites}
		expandedIds={expandedVMcpIds}
		dragActive={entryDrag.active}
		estimateHeight={(vmcp, expanded) => vmcpRowHeight(vmcpComponents(vmcp).length, expanded)}
	>
		{#snippet actions()}
			{@render viewActions()}
		{/snippet}
		{#snippet row(vmcp, ctx)}
			<VMcpGraphRow
				{vmcp}
				components={vmcpComponents(vmcp)}
				expanded={isVMcpExpanded(vmcp.id)}
				context={ctx}
				drag={entryDrag}
				onToggleExpand={() => toggleExpandedVMcp(vmcp)}
				onEdit={() => createEditVMcp?.openEdit(vmcp)}
				onConnect={() => handleConnectVMcp(vmcp)}
				onModifyComponent={(component) => toolFlow.openComponent(component, vmcp)}
			/>
		{/snippet}
		{#snippet footer()}
			<CreateVMcpButton drag={entryDrag} embedded onCreate={() => createEditVMcp?.openCreate()} />
		{/snippet}
	</VMcpGraph>
{/snippet}

{#snippet tableView()}
	<VMcpTable
		items={composites}
		drag={entryDrag}
		components={vmcpComponents}
		{rightPanelWidth}
		onEdit={(component, vmcp) => toolFlow.editComponent(component, vmcp)}
		onDelete={(component, vmcp) => toolFlow.promptRemoveComponent(component, vmcp)}
	>
		{#snippet actions()}
			{@render viewActions()}
		{/snippet}
	</VMcpTable>
{/snippet}

{#snippet viewActions()}
	<div
		class="bg-base-100/80 dark:bg-base-300/80 flex gap-1 rounded-md border border-transparent p-1 shadow-sm"
		data-vmcp-ui
		role="toolbar"
		tabindex="-1"
		aria-label="View type"
		onpointerdown={(event) => event.stopPropagation()}
	>
		<IconButton
			class={twMerge('btn-sm', viewType === 'graph' && 'bg-base-400 dark:bg-base-100')}
			tooltip={{ text: 'Graph View', placement: 'bottom' }}
			onclick={() => (viewType = 'graph')}
		>
			<ChartBarStacked class="size-4" />
		</IconButton>
		<IconButton
			class={twMerge('btn-sm', viewType === 'table' && 'bg-base-400 dark:bg-base-100')}
			tooltip={{ text: 'Table View', placement: 'bottom' }}
			onclick={() => (viewType = 'table')}
		>
			<Table class="size-4" />
		</IconButton>
	</div>
{/snippet}

<VMcpDragOverlay drag={entryDrag} />

<VMcpToolDialogs flow={toolFlow} />

<ConnectToServer
	bind:this={connectToServerDialog}
	catalogID={DEFAULT_MCP_CATALOG_ID}
	onConnect={handleConnectToServer}
/>

<CreateEditVMcp bind:this={createEditVMcp} onCreated={toolFlow.handleVMcpCreated} />

<ViewModifyCatalogEntry
	bind:this={catalogEntryDialog}
	rightOffsetWidth={rightPanelWidth}
	onCreated={handleCatalogEntryCreated}
/>

<ResponsiveDialog bind:this={connectAllVMcpsDialog} id="connect-all-vmcps-dialog">
	{#snippet titleContent()}
		{#if selectedClient}
			<img src={selectedClient.icon} alt="" class="mt-0.5 size-4 block dark:hidden" />
			<img
				src={selectedClient.iconDark ?? selectedClient.icon}
				alt=""
				class="mt-0.5 size-4 hidden dark:block"
			/>
			Connect All vMCPs
		{/if}
	{/snippet}
	<div class="flex flex-col gap-3 md:p-0 p-4">
		{#if connectAllVmcps.length === 0}
			<p class="text-sm text-muted-content font-light">
				No vMCPs currently have a connection URL to copy.
			</p>
		{:else if selectedConnectAllSnippet}
			{#if connectAllSnippets.length > 1}
				<div role="tablist" class="tabs tabs-box" aria-label="Configuration files">
					{#each connectAllSnippets as snippet (snippet.id)}
						<button
							type="button"
							role="tab"
							aria-selected={selectedConnectAllSnippet.id === snippet.id}
							aria-controls="connect-all-snippet-panel"
							class={twMerge('tab', selectedConnectAllSnippet.id === snippet.id && 'tab-active')}
							onclick={() => (selectedConnectAllSnippetId = snippet.id)}
						>
							{snippet.label}
						</button>
					{/each}
				</div>
			{/if}
			{#if selectedClient}
				<div class="flex items-start gap-2 text-sm">
					<div class="flex flex-col gap-2 text-muted-content font-light">
						{#if selectedClient.id === AiClient.Claude}
							{#if isAdmin && selectedConnectAllSnippet.id === 'claude-settings-json'}
								<p>
									Go to <code class="text-base-content"
										>Admin Settings > Claude Code > Managed settings</code
									> and add the following configuration JSON:
								</p>
							{:else}
								<p>
									Copy the configuration below into your project's <code class="text-base-content"
										>.mcp.json</code
									>
									or your user-level
									<code class="text-base-content">~/.claude.json</code>.
								</p>
							{/if}
						{:else if selectedClient.id === AiClient.Codex}
							<p>
								Copy these tables into
								<code class="text-base-content">~/.codex/config.toml</code>
								or a project-scoped
								<code class="text-base-content">.codex/config.toml</code>.
							</p>
						{:else if selectedClient.id === AiClient.Cursor}
							<p>
								Copy the configuration below into
								<code class="text-base-content">~/.cursor/mcp.json</code>
								or your project's
								<code class="text-base-content">.cursor/mcp.json</code>.
							</p>
						{:else if selectedClient.id === AiClient.VSCode}
							<p>
								Copy this configuration into your workspace
								<code class="text-base-content">.vscode/mcp.json</code>.
							</p>
						{/if}
					</div>
				</div>
			{/if}
			<div class="relative" id="connect-all-snippet-panel" role="tabpanel">
				<pre
					class="pl-4 pr-22 py-2 m-0 max-h-96 overflow-y-auto dark:bg-base-200"
					id={`connect-all-mcp-json-${selectedConnectAllSnippet.id}`}><code
						class="font-mono text-xs">{selectedConnectAllSnippet.value}</code
					></pre>
				<div class="absolute top-4 right-4">
					<CopyButton
						text={selectedConnectAllSnippet.value}
						id={`connect-all-mcp-json-copy-button-${selectedConnectAllSnippet.id}`}
						classes={{ button: 'flex shrink-0 gap-2 text-xs' }}
						showTextLeft
					/>
				</div>
			</div>
		{/if}
	</div>
</ResponsiveDialog>

<svelte:head>
	<title>Obot | vMCPs</title>
</svelte:head>
