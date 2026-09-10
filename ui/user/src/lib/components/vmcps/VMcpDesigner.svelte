<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import ConnectVMcp from '$lib/components/vmcps/ConnectVMcp.svelte';
	import CreateEditVMcp from '$lib/components/vmcps/CreateEditVMcp.svelte';
	import CreateVMcpButton from '$lib/components/vmcps/CreateVMcpButton.svelte';
	import McpServersSidebar from '$lib/components/vmcps/McpServersSidebar.svelte';
	import VMcpComponentConfigurationDialog from '$lib/components/vmcps/VMcpComponentConfigurationDialog.svelte';
	import VMcpDragHint from '$lib/components/vmcps/VMcpDragHint.svelte';
	import VMcpDragOverlay from '$lib/components/vmcps/VMcpDragOverlay.svelte';
	import VMcpGraph from '$lib/components/vmcps/VMcpGraph.svelte';
	import VMcpGraphRow from '$lib/components/vmcps/VMcpGraphRow.svelte';
	import VMcpProfiles from '$lib/components/vmcps/VMcpProfiles.svelte';
	import VMcpToolDialogs from '$lib/components/vmcps/VMcpToolDialogs.svelte';
	import ViewModifyCatalogEntry from '$lib/components/vmcps/ViewModifyCatalogEntry.svelte';
	import { CREATE_VMCP_DROP_ID, createEntryDrag } from '$lib/runes/vmcps/entryDrag.svelte';
	import {
		claimToolSetupForVMcp,
		createVMcpToolFlow,
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
	import { errors, mcpServersAndEntries, profile, vmcpInstances } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { goto, setUrlParamAndUpdateUrl } from '$lib/url';
	import { Trash2 } from '@lucide/svelte';
	import { onMount, untrack } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		vmcp?: VMCP;
		onBack?: () => void;
	}

	let { vmcp, onBack }: Props = $props();

	let requestedView = $derived(
		(page.url.searchParams.get('view') as 'graph' | 'profiles' | undefined) ?? 'graph'
	);
	let showRightPanel = $state(true);
	let createEditVMcp = $state<ReturnType<typeof CreateEditVMcp>>();
	let catalogEntryDialog = $state<ReturnType<typeof ViewModifyCatalogEntry>>();
	let connectVMcpDialog = $state<ReturnType<typeof ConnectVMcp>>();
	let configurationDialog = $state<ReturnType<typeof VMcpComponentConfigurationDialog>>();
	let rightPanelEl = $state<HTMLElement>();
	let graphCanvasEl = $state<HTMLElement>();
	let rightPanelWidth = $state(0);
	let pendingEntryDrop = $state<{ vmcp?: VMCP }>();
	let pendingComponentDrop = $state<{
		target: VMCP;
		entry: MCPCatalogEntry;
		component: VMCPComponent;
	}>();
	let expanded = $state(true);
	const toolFlow = createVMcpToolFlow();
	let selectedVMcp = $state<VMCP | undefined>(untrack(() => vmcp));

	let query = $derived(page.url.searchParams.get('query') ?? '');
	let composites = $derived(selectedVMcp ? [selectedVMcp] : []);
	let title = $derived(selectedVMcp?.displayName ?? 'Create vMCP');
	let canCreateCatalogEntry = $derived(
		profile.current.isAdmin?.() || profile.current.groups.includes(Group.POWERUSER)
	);
	let isOwner = $derived(profile.current.id === selectedVMcp?.userID);
	let canEdit = $derived(!selectedVMcp || profile.current.isAdmin?.() || isOwner);
	let canShare = $derived(profile.current.isAdmin?.());
	let users = $state<OrgUser[]>([]);
	let usersMap = $derived(new Map(users.map((user) => [user.id, user])));
	let viewType = $derived(canEdit ? requestedView : 'graph');

	$effect(() => {
		selectedVMcp = vmcp;
	});

	onMount(() => {
		UserService.listUsersIncludeDeleted().then((response) => {
			users = response;
		});
	});

	toolFlow.setOnVMcpChanged((updated) => {
		selectedVMcp = updated;
	});

	$effect(() => {
		const created = selectedVMcp;
		if (!created || mcpServersAndEntries.current.loading) return;

		untrack(() => {
			if (!claimToolSetupForVMcp(created.id)) return;
			toolFlow.handleVMcpCreated(created);
		});
	});

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
		dropOnVMcp: (entry, target) => void handleDropped(entry, target)
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
		if (!component.configuration?.some((field) => field.policy === 'userAllowed')) {
			toolFlow.offerToolSelection(entry, updated);
		}
		return updated;
	}

	async function handleDropped(entry: MCPCatalogEntry, target: VMCP) {
		if (!canEdit) return;
		const component = catalogEntryToVMCPComponent(entry);

		try {
			const latest = await UserService.getVMCP(target.id);
			const components = latest.components ?? [];
			if (
				components.some(
					(existing) => existing.mcpServerCatalogEntryID === component.mcpServerCatalogEntryID
				)
			) {
				return;
			}

			if (catalogConfigurationFields(entry).length === 0) {
				await addComponentToVMcp(latest, entry, component);
				return;
			}

			pendingComponentDrop = { target: latest, entry, component };
			configurationDialog?.open(entry);
		} catch {
			errors.append('Failed to add MCP server to vMCP.');
		}
	}

	async function handleConfigurationNext(configuration: VMCPConfigurationPolicy[]) {
		const pending = pendingComponentDrop;
		if (!pending) return;
		try {
			await addComponentToVMcp(pending.target, pending.entry, {
				...pending.component,
				configuration
			});
			pendingComponentDrop = undefined;
		} catch {
			errors.append('Failed to add MCP server to vMCP.');
			throw new Error('Failed to add MCP server to vMCP.');
		}
	}

	function handleVMcpCreated(created: VMCP) {
		queueToolSetupForCreatedVMcp(created.id);
		goto(`/vmcps/${created.id}`);
	}

	function vmcpComponents(target: VMCP) {
		return resolveVMcpComponents(target);
	}

	function handleConnectVMcp(vmcp: VMCP, options?: VMcpConnectOptions) {
		const vmcpInstance = vmcpInstances.current.items.find(
			(candidate) => candidate.vmcpID === vmcp.id && candidate.userID === profile.current.id
		);
		connectVMcpDialog?.open(vmcp, vmcpInstance, options);
	}

	const updateSearchQuery = (value: string) => {
		setUrlParamAndUpdateUrl(page.url, 'query', value);
	};

	function handleBack() {
		onBack?.();
		if (!onBack) goto('/vmcps');
	}
</script>

<Layout
	classes={{
		container: 'p-0 md:px-0 min-h-0',
		childrenContainer: 'max-w-full',
		collapsedSidebarHeaderContent: 'p-4 pb-0'
	}}
	{title}
	showBackButton
	onBackButtonClick={handleBack}
>
	<div
		class="@container dark:from-base-300 to-base-200 relative h-full min-h-0 w-full overflow-y-auto default-scrollbar-thin bg-radial-[at_50%_50%] from-gray-50 dark:to-black"
	>
		{#if canShare}
			{@render toggleSubview()}
		{/if}
		{#if viewType === 'profiles'}
			<VMcpProfiles
				vmcp={selectedVMcp}
				{toolFlow}
				onUpdated={(updated) => {
					selectedVMcp = updated;
				}}
			/>
		{:else}
			<VMcpGraph
				bind:viewportEl={graphCanvasEl}
				item={selectedVMcp}
				{expanded}
				dragActive={entryDrag.active}
				estimateHeight={(item, expanded) => vmcpRowHeight(vmcpComponents(item).length, expanded)}
			>
				{#snippet row(item, ctx)}
					<VMcpGraphRow
						vmcp={item}
						components={vmcpComponents(item)}
						{expanded}
						{canEdit}
						{isOwner}
						context={ctx}
						drag={entryDrag}
						onToggleExpand={() => (expanded = !expanded)}
						onEdit={canEdit ? () => createEditVMcp?.openEdit(item) : undefined}
						onConnect={(options) => handleConnectVMcp(item, options)}
						onDelete={canEdit ? () => createEditVMcp?.openDelete(item) : undefined}
						onModifyComponent={canEdit
							? (component) => toolFlow.openComponent(component, item)
							: undefined}
						{usersMap}
					/>
				{/snippet}
				{#snippet empty()}
					<CreateVMcpButton drag={entryDrag} onCreate={() => createEditVMcp?.openCreate()} />
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
			{#if showRightPanel && canEdit}
				<VMcpDragHint
					dragActive={entryDrag.active}
					class="absolute top-1/2 right-4 z-20 hidden -translate-y-1/2 @2xl:block"
				/>
			{/if}
		{/if}
	</div>
	{#snippet rightSidebar()}
		{#if canEdit && viewType === 'graph'}
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
	<div class={twMerge('p-2 w-fit', viewType === 'graph' && 'absolute top-0 left-0')}>
		<div class="tabs tabs-box bg-base-300 shadow-inner dark:bg-base-200">
			<button
				class={twMerge(
					'tab text-xs min-w-24',
					viewType === 'graph' && 'tab-active dark:bg-base-300'
				)}
				onclick={() => {
					setUrlParamAndUpdateUrl(page.url, 'view', 'graph');
				}}>Designer</button
			>
			<button
				class={twMerge(
					'tab text-xs min-w-24',
					viewType === 'profiles' && 'tab-active dark:bg-base-300'
				)}
				onclick={() => {
					setUrlParamAndUpdateUrl(page.url, 'view', 'profiles');
				}}>Profiles</button
			>
		</div>
	</div>
{/snippet}

<VMcpDragOverlay drag={entryDrag} />

<VMcpToolDialogs flow={toolFlow} />

<ConnectVMcp bind:this={connectVMcpDialog} />

<VMcpComponentConfigurationDialog
	bind:this={configurationDialog}
	onNext={handleConfigurationNext}
/>

<CreateEditVMcp
	bind:this={createEditVMcp}
	onCreated={handleVMcpCreated}
	onDeleted={handleBack}
	onUpdated={(updated) => {
		selectedVMcp = updated;
	}}
/>

<ViewModifyCatalogEntry
	bind:this={catalogEntryDialog}
	rightOffsetWidth={rightPanelWidth}
	onCreated={handleCatalogEntryCreated}
/>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
