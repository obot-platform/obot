<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import CreateEditVMcp from '$lib/components/vmcps/CreateEditVMcp.svelte';
	import CreateVMcpButton from '$lib/components/vmcps/CreateVMcpButton.svelte';
	import McpServersSidebar from '$lib/components/vmcps/McpServersSidebar.svelte';
	import VMcpActions from '$lib/components/vmcps/VMcpActions.svelte';
	import VMcpCreationHint from '$lib/components/vmcps/VMcpCreationHint.svelte';
	import VMcpDragOverlay from '$lib/components/vmcps/VMcpDragOverlay.svelte';
	import VMcpGraph from '$lib/components/vmcps/VMcpGraph.svelte';
	import VMcpGraphRow from '$lib/components/vmcps/VMcpGraphRow.svelte';
	import VMcpIntroduction from '$lib/components/vmcps/VMcpIntroduction.svelte';
	import VMcpProfiles from '$lib/components/vmcps/VMcpProfiles.svelte';
	import VMcpTester from '$lib/components/vmcps/VMcpTester.svelte';
	import VMcpToolDialogs from '$lib/components/vmcps/VMcpToolDialogs.svelte';
	import ViewModifyCatalogEntry from '$lib/components/vmcps/ViewModifyCatalogEntry.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import { CREATE_VMCP_DROP_ID, createEntryDrag } from '$lib/runes/vmcps/entryDrag.svelte';
	import {
		claimCreationHintForVMcp,
		claimToolSetupForVMcp,
		createVMcpToolFlow,
		finishVMcpCreateHandoff,
		isVMcpCreateHandoffPending,
		markVMcpCreationHintSeen,
		peekQueuedToolSetupVMcp,
		queueCreationHintForCreatedVMcp,
		queueToolSetupForCreatedVMcp
	} from '$lib/runes/vmcps/vmcpToolFlow.svelte';
	import {
		Group,
		UserService,
		type MCPCatalogEntry,
		type OrgUser,
		type VMCP,
		type VMCPComponent,
		type VMCPConfigurationPolicy
	} from '$lib/services';
	import { vmcpRowHeight } from '$lib/services/vmcps/camera';
	import { SHORT_DESCRIPTION_MAX_LENGTH } from '$lib/services/vmcps/constants';
	import type { VMcpConnectOptions } from '$lib/services/vmcps/types';
	import {
		appendComponentLabel,
		catalogConfigurationFields,
		catalogEntryToVMCPComponent,
		resolveVMcpComponents,
		vmcpManifest
	} from '$lib/services/vmcps/utils';
	import { errors, mcpServersAndEntries, profile, responsive, vmcpInstances } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { goto, setUrlParamAndUpdateUrl } from '$lib/url';
	import { Plus, Trash2 } from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		vmcp?: VMCP;
		onBack?: () => void;
		usersMap: Map<string, OrgUser>;
		isFirstVMcp?: boolean;
		showBackButton?: boolean;
	}

	let { vmcp, onBack, usersMap, isFirstVMcp = false, showBackButton = true }: Props = $props();

	let view = $derived(
		(page.url.searchParams.get('view') as 'graph' | 'profiles' | 'inspector' | undefined) ?? 'graph'
	);
	let showRightPanel = $state(responsive.isMobile ? false : true);
	let createEditVMcp = $state<ReturnType<typeof CreateEditVMcp>>();
	let profilesPanel = $state<ReturnType<typeof VMcpProfiles>>();
	let catalogEntryDialog = $state<ReturnType<typeof ViewModifyCatalogEntry>>();
	let refreshingTester = $state(false);
	let vmcpActions = $state<ReturnType<typeof VMcpActions>>();
	let rightPanelEl = $state<HTMLElement>();
	let graphCanvasEl = $state<HTMLElement>();
	let profilesTabEl = $state<HTMLButtonElement>();
	let testerTabEl = $state<HTMLButtonElement>();
	let connectButtonEl = $state<HTMLElement>();
	let creationHintQueued = $state(false);
	let rightPanelWidth = $state(0);
	let pendingEntryDrop = $state<{ vmcp?: VMCP }>();
	let pendingComponentDrop = $state<{
		target: VMCP;
		entry: MCPCatalogEntry;
		component: VMCPComponent;
	}>();
	const toolFlow = createVMcpToolFlow();
	let selectedVMcp = $state<VMCP | undefined>(untrack(() => vmcp));

	let query = $derived(page.url.searchParams.get('query') ?? '');
	let composites = $derived(selectedVMcp ? [selectedVMcp] : []);
	let title = $derived(selectedVMcp?.displayName ?? 'Create vMCP');
	let canCreateCatalogEntry = $derived(
		profile.current.isAdmin?.() || profile.current.groups.includes(Group.POWERUSER)
	);
	let isOwner = $derived(
		profile.current.id === selectedVMcp?.userID ||
			(!selectedVMcp?.userID && profile.current.hasAdminAccess?.())
	);
	let canAccessTester = $derived(Boolean(selectedVMcp?.id && (isOwner || !selectedVMcp?.userID)));
	let canAccessProfiles = $derived(isOwner && profile.current.hasAdminAccess?.());
	let hasEntries = $derived(mcpServersAndEntries.current.entries.length > 0);
	let canEdit = $derived(
		!selectedVMcp || profile.current.isAdmin?.() || profile.current.id === selectedVMcp?.userID
	);
	let viewType = $derived(
		(view === 'profiles' && !canAccessProfiles) || (view === 'inspector' && !canAccessTester)
			? 'graph'
			: view
	);
	let componentDropPending = $state(false);
	let showDesignerLoading = $derived(
		(isVMcpCreateHandoffPending() || componentDropPending) && !toolFlow.dialog
	);

	$effect(() => {
		selectedVMcp = vmcp;
	});

	toolFlow.setOnVMcpChanged((updated) => {
		selectedVMcp = updated;
	});

	$effect(() => {
		const created = selectedVMcp;
		if (!created || mcpServersAndEntries.current.loading) return;

		untrack(() => {
			if (!claimToolSetupForVMcp(created.id)) {
				const queued = peekQueuedToolSetupVMcp();
				if (!queued || queued === created.id) {
					finishVMcpCreateHandoff();
				}
				return;
			}
			toolFlow.handleVMcpCreated(created);
			if (!toolFlow.dialog) finishVMcpCreateHandoff();
		});
	});

	$effect(() => {
		if (toolFlow.dialog) {
			finishVMcpCreateHandoff();
			componentDropPending = false;
		}
	});

	$effect(() => {
		const created = selectedVMcp;
		if (!created || !isOwner || !canEdit) return;

		untrack(() => {
			if (claimCreationHintForVMcp(created.id)) {
				creationHintQueued = true;
			}
		});
	});

	let showCreationHint = $derived(
		creationHintQueued && !toolFlow.dialog && viewType === 'graph' && isOwner && canEdit
	);

	function componentManifestField(component: VMCPComponent, field: 'name' | 'shortDescription') {
		if (field === 'name') return component.name || component.catalogEntry?.manifest?.name;
		return component.catalogEntry?.manifest?.shortDescription;
	}

	const entryDrag = createEntryDrag({
		vmcps: () => composites,
		panelEl: () => rightPanelEl,
		canvasEl: () => graphCanvasEl,
		canvasDropId: () => selectedVMcp?.id ?? CREATE_VMCP_DROP_ID,
		openEntry: (entry) => openCatalogEntry(entry),
		createEntry: (target) => startCatalogEntryCreation(target),
		dropOnCreate: (entry) => handleDroppedOnCreate(entry),
		dropOnVMcp: (entry, target) => void handleDropped(entry, target),
		disabled: () => showDesignerLoading
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

	function handleAddFromDetails(entry: MCPCatalogEntry) {
		if (responsive.isMobile) {
			showRightPanel = false;
		}
		if (selectedVMcp) {
			void handleDropped(entry, selectedVMcp);
			return;
		}
		handleDroppedOnCreate(entry);
	}

	function isAddedToSelectedVMcp(entry: MCPCatalogEntry) {
		return Boolean(
			selectedVMcp?.components?.some((component) => component.mcpServerCatalogEntryID === entry.id)
		);
	}

	function startCatalogEntryCreation(target?: { vmcp?: VMCP }) {
		if (!canEdit) return;
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

	function handleDroppedOnCreate(entry: MCPCatalogEntry) {
		if (!canEdit) return;
		createEditVMcp?.openCreate([catalogEntryToVMCPComponent(entry)]);
	}

	async function addComponentToVMcp(
		target: VMCP,
		entry: MCPCatalogEntry,
		component: VMCPComponent
	) {
		const latest = await UserService.getVMCP(target.id);
		const components = latest.components ?? [];
		if (
			components.some(
				(existing) => existing.mcpServerCatalogEntryID === component.mcpServerCatalogEntryID
			)
		) {
			return latest;
		}

		const updated = await UserService.updateVMCP(latest.id, {
			...vmcpManifest(latest),
			displayName:
				appendComponentLabel(
					latest.displayName,
					components.map((existing) => componentManifestField(existing, 'name')),
					entry.manifest.name
				) ?? latest.displayName,
			description:
				appendComponentLabel(
					latest.description,
					components.map((existing) => componentManifestField(existing, 'shortDescription')),
					entry.manifest.shortDescription,
					SHORT_DESCRIPTION_MAX_LENGTH
				) ?? latest.description,
			components: [...components, component]
		});

		selectedVMcp = updated;
		success.add(`${entry.manifest.name} added to ${updated.displayName}.`);
		toolFlow.offerToolSelection(entry, updated);
		return updated;
	}

	async function handleDropped(entry: MCPCatalogEntry, target: VMCP) {
		if (!canEdit) return;
		const component = catalogEntryToVMCPComponent(entry);
		componentDropPending = true;

		try {
			const latest = await UserService.getVMCP(target.id);
			const components = latest.components ?? [];
			if (
				components.some(
					(existing) => existing.mcpServerCatalogEntryID === component.mcpServerCatalogEntryID
				)
			) {
				componentDropPending = false;
				return;
			}

			if (catalogConfigurationFields(entry).length === 0) {
				await addComponentToVMcp(latest, entry, component);
				if (!toolFlow.dialog) componentDropPending = false;
				return;
			}

			pendingComponentDrop = { target: latest, entry, component };
			componentDropPending = false;
			vmcpActions?.openConfiguration(entry);
		} catch {
			componentDropPending = false;
			errors.append('Failed to add MCP server to vMCP.');
		}
	}

	async function handleConfigurationNext(
		configuration: VMCPConfigurationPolicy[],
		forceSingleUser: boolean
	) {
		const pending = pendingComponentDrop;
		if (!pending) return;
		componentDropPending = true;
		try {
			await addComponentToVMcp(pending.target, pending.entry, {
				...pending.component,
				configuration,
				forceSingleUser
			});
			pendingComponentDrop = undefined;
			if (!toolFlow.dialog) componentDropPending = false;
		} catch {
			componentDropPending = false;
			errors.append('Failed to add MCP server to vMCP.');
			throw new Error('Failed to add MCP server to vMCP.');
		}
	}

	function handleVMcpCreated(created: VMCP) {
		queueToolSetupForCreatedVMcp(created.id);
		if (isFirstVMcp) queueCreationHintForCreatedVMcp(created.id);
		goto(`/vmcps/${created.id}`);
	}

	function dismissCreationHint() {
		creationHintQueued = false;
		markVMcpCreationHintSeen();
	}

	function vmcpComponents(target: VMCP) {
		return resolveVMcpComponents(target);
	}

	function handleConnectVMcp(vmcp: VMCP, options?: VMcpConnectOptions) {
		vmcpActions?.openConnect(vmcp, undefined, options);
	}

	async function refreshTester(vmcpID: string) {
		refreshingTester = true;
		try {
			// Readiness belongs to the vMCP, not its user instance. Refresh both after
			// launch/OAuth before the tester checks whether it can connect.
			const [updated] = await Promise.all([UserService.getVMCP(vmcpID), vmcpInstances.refresh()]);
			if (selectedVMcp?.id === vmcpID) selectedVMcp = updated;
		} catch {
			errors.append('Failed to refresh vMCP status.');
		} finally {
			refreshingTester = false;
		}
	}

	const updateSearchQuery = (value: string) => {
		setUrlParamAndUpdateUrl(page.url, 'query', value);
	};

	function leaveDesigner() {
		onBack?.();
		if (!onBack) goto('/vmcps');
	}

	function handleBack() {
		if (profilesPanel?.leaveEditor()) return;
		leaveDesigner();
	}

	$effect(() => {
		if (responsive.isMobile) {
			showRightPanel = true;
		}
	});
</script>

<Layout
	classes={{
		container: 'p-0 md:px-0 min-h-0',
		childrenContainer: 'max-w-full',
		collapsedSidebarHeaderContent: 'p-4 pb-0'
	}}
	{title}
	{showBackButton}
	onBackButtonClick={handleBack}
>
	<div
		class="@container dark:from-base-300 to-base-200 relative h-full min-h-0 w-full overflow-y-auto default-scrollbar-thin bg-radial-[at_50%_50%] from-gray-50 dark:to-black"
		aria-busy={showDesignerLoading}
	>
		{#if showDesignerLoading}
			<div
				class="bg-base-200/70 dark:bg-black/60 absolute inset-0 z-60 flex items-center justify-center"
				role="status"
				aria-label="Setting up tools"
			>
				<Loading class="size-8" />
			</div>
		{/if}

		{#if (isOwner || canAccessTester) && !responsive.isMobile}
			{@render toggleSubview()}
			<VMcpCreationHint
				show={showCreationHint}
				profilesAnchorEl={profilesTabEl}
				testerAnchorEl={testerTabEl}
				connectAnchorEl={connectButtonEl}
				includeProfiles={Boolean(profile.current.hasAdminAccess?.())}
				includeTester={Boolean(selectedVMcp?.id)}
				includeConnect={Boolean(selectedVMcp)}
				onDismiss={dismissCreationHint}
			/>
		{/if}
		{#if viewType === 'profiles'}
			<VMcpProfiles
				bind:this={profilesPanel}
				vmcp={selectedVMcp}
				{toolFlow}
				onUpdated={(updated) => {
					selectedVMcp = updated;
				}}
				readonly={!canEdit}
			/>
		{:else if viewType === 'inspector'}
			{#if selectedVMcp}
				<div class="flex h-full min-h-0 flex-col p-3 pt-14">
					<VMcpTester
						vmcp={selectedVMcp}
						loading={refreshingTester}
						onLaunch={() => {
							if (!selectedVMcp) return;
							const vmcpID = selectedVMcp.id;
							refreshingTester = true;
							handleConnectVMcp(selectedVMcp, {
								onConnected: () => {
									void refreshTester(vmcpID);
								},
								onDismissed: () => {
									refreshingTester = false;
								}
							});
						}}
					/>
				</div>
			{/if}
		{:else}
			<VMcpGraph
				bind:viewportEl={graphCanvasEl}
				item={selectedVMcp}
				dragActive={entryDrag.active}
				estimateHeight={(item) => vmcpRowHeight(vmcpComponents(item).length)}
			>
				{#snippet row(item, ctx)}
					<VMcpGraphRow
						vmcp={item}
						components={vmcpComponents(item)}
						{canEdit}
						context={ctx}
						drag={entryDrag}
						onEdit={canEdit ? () => createEditVMcp?.openEdit(item) : undefined}
						onConnect={(options) => handleConnectVMcp(item, options)}
						onDelete={canEdit ? () => createEditVMcp?.openDelete(item) : undefined}
						openSelectInstance={vmcpActions?.openSelectInstance}
						openDiff={vmcpActions?.openDiff}
						openUpdateConfirm={vmcpActions?.openUpdateConfirm}
						openEditInstanceConfiguration={vmcpActions?.openEditInstanceConfiguration}
						onUpdate={(updated) => {
							selectedVMcp = updated;
						}}
						onModifyComponent={canEdit
							? (component) => toolFlow.openComponent(component, item)
							: undefined}
						bind:connectEl={connectButtonEl}
						{usersMap}
					/>
				{/snippet}
				{#snippet empty()}
					<CreateVMcpButton drag={entryDrag} />
				{/snippet}
				{#snippet actions()}
					{#if selectedVMcp && canEdit}
						<div
							class="bg-base-100/80 dark:bg-base-300/80 flex gap-1 rounded-md border border-transparent p-1 shadow-sm"
							data-vmcp-ui
							role="toolbar"
							tabindex="-1"
							aria-label="vMCP actions"
							onpointerdown={(event) => event.stopPropagation()}
						>
							<IconButton
								class="btn-sm"
								variant="danger"
								tooltip={{ text: 'Delete vMCP', placement: 'bottom' }}
								onclick={() => {
									if (!selectedVMcp) return;
									createEditVMcp?.openDelete(selectedVMcp);
								}}
							>
								<Trash2 class="size-4" />
							</IconButton>
						</div>
					{/if}
				{/snippet}
			</VMcpGraph>
		{/if}
	</div>
	{#if responsive.isMobile && !showRightPanel && viewType === 'graph'}
		<div class="fixed bottom-0 left-0 z-50 flex justify-center w-full py-4">
			<button
				class="btn btn-primary"
				onclick={() => {
					showRightPanel = true;
				}}
			>
				<Plus class="size-4" /> Add MCP Servers
			</button>
		</div>
	{/if}
	{#snippet rightSidebar()}
		{#if canEdit && viewType === 'graph' && (!responsive.isMobile || (responsive.isMobile && showRightPanel && (canCreateCatalogEntry || hasEntries)))}
			<McpServersSidebar
				bind:panelEl={rightPanelEl}
				bind:open={showRightPanel}
				drag={entryDrag}
				{query}
				onSearch={updateSearchQuery}
				canCreateEntry={canCreateCatalogEntry}
			/>
		{/if}
	{/snippet}
</Layout>

{#snippet toggleSubview()}
	<div class={twMerge('p-2 w-fit', viewType !== 'profiles' && 'absolute z-40 top-0 left-0')}>
		<div class="tabs tabs-box bg-base-100 shadow-sm dark:bg-base-300">
			<button
				class={twMerge(
					'tab text-xs min-w-24',
					viewType === 'graph' && 'tab-active bg-base-300 dark:bg-base-100'
				)}
				onclick={() => {
					setUrlParamAndUpdateUrl(page.url, 'view', 'graph');
				}}>Designer</button
			>
			{#if canAccessProfiles}
				<button
					bind:this={profilesTabEl}
					class={twMerge(
						'tab text-xs min-w-24',
						viewType === 'profiles' && 'tab-active bg-base-300 dark:bg-base-100'
					)}
					onclick={() => {
						setUrlParamAndUpdateUrl(page.url, 'view', 'profiles');
					}}>Profiles</button
				>
			{/if}
			{#if canAccessTester}
				<button
					bind:this={testerTabEl}
					class={twMerge(
						'tab text-xs min-w-24',
						viewType === 'inspector' && 'tab-active bg-base-300 dark:bg-base-100'
					)}
					onclick={() => {
						setUrlParamAndUpdateUrl(page.url, 'view', 'inspector');
					}}>Inspector</button
				>
			{/if}
		</div>
	</div>
{/snippet}

<VMcpDragOverlay drag={entryDrag} />

<VMcpToolDialogs flow={toolFlow} />

<VMcpActions bind:this={vmcpActions} onConfigurationNext={handleConfigurationNext} />

<CreateEditVMcp
	bind:this={createEditVMcp}
	onCreated={handleVMcpCreated}
	onDeleted={leaveDesigner}
	onUpdated={(updated) => {
		selectedVMcp = updated;
	}}
/>

<ViewModifyCatalogEntry
	bind:this={catalogEntryDialog}
	rightOffsetWidth={rightPanelWidth}
	onCreated={handleCatalogEntryCreated}
	onAddToVMcp={canEdit ? handleAddFromDetails : undefined}
	addToVMcpLabel={selectedVMcp ? 'Add to vMCP' : 'Create vMCP'}
	isAddedToVMcp={isAddedToSelectedVMcp}
/>

<VMcpIntroduction show={isFirstVMcp && !selectedVMcp && canEdit && viewType === 'graph'} />

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
