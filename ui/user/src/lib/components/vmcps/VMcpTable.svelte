<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import type { EntryDrag } from '$lib/runes/vmcps/entryDrag.svelte';
	import type { MCPCatalogEntry } from '$lib/services';
	import type { VMcpComponentView } from '$lib/services/vmcps/types';
	import { responsive } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import McpServerIcon from './McpServerIcon.svelte';
	import './vmcpGraph.css';
	import { PencilLine, Trash2 } from '@lucide/svelte';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		items: MCPCatalogEntry[];
		drag: EntryDrag;
		components: (vmcp: MCPCatalogEntry) => VMcpComponentView[];
		rightPanelWidth?: number;
		actions?: Snippet;
		onEdit?: (component: VMcpComponentView, vmcp: MCPCatalogEntry) => void;
		onDelete?: (component: VMcpComponentView, vmcp: MCPCatalogEntry) => void;
	}

	let { items, drag, components, rightPanelWidth, actions, onEdit, onDelete }: Props = $props();

	let tableData = $derived(
		items.map((item) => ({
			id: item.id,
			name: item.manifest.name,
			created: item.created,
			componentServersCount: item.manifest?.compositeConfig?.componentServers?.length ?? 0,
			powerUserID: item.powerUserID,
			powerUserWorkspaceID: item.powerUserWorkspaceID,
			data: item
		}))
	);

	let vMcpDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let selectedId = $state<string>();
	let dialogOpen = $state(false);

	let selectedVMcp = $derived(items.find((item) => item.id === selectedId));
	let selectedComponents = $derived(selectedVMcp ? components(selectedVMcp) : []);

	function openVMcp(vmcp: MCPCatalogEntry) {
		selectedId = vmcp.id;
		vMcpDialog?.open();
	}
</script>

<div class="absolute top-4 right-4 z-50 flex items-center gap-4">
	{#if actions}
		{@render actions()}
	{/if}
</div>
<div class="h-full w-full pt-14 overflow-y-auto">
	<Table
		data={tableData}
		fields={['name', 'created', 'componentServersCount', 'powerUserID']}
		headers={[
			{
				property: 'componentServersCount',
				title: '# MCP Servers'
			},
			{
				property: 'powerUserID',
				title: 'Owner'
			}
		]}
		onClickRow={(row) => openVMcp(row.data)}
		setRowClasses={(row) =>
			drag.isLinked(row.id) ? 'bg-primary/10 outline-primary outline-2 -outline-offset-2' : ''}
		rowAttachment={(row) => (dialogOpen ? undefined : drag.vmcpTargetAttachment(row.id))}
	>
		{#snippet onRenderColumn(field, d)}
			{#if field === 'created'}
				<span>{formatTimeAgo(d.created).relativeTime}</span>
			{:else}
				{d[field as keyof typeof d]}
			{/if}
		{/snippet}
	</Table>
</div>

<ResponsiveDialog
	bind:this={vMcpDialog}
	title={selectedVMcp?.manifest.name}
	rightPanelWidth={responsive.isMobile ? undefined : rightPanelWidth}
	onOpen={() => (dialogOpen = true)}
	onClose={() => {
		dialogOpen = false;
		selectedId = undefined;
	}}
	class="md:max-w-lg"
>
	{#if selectedVMcp}
		{@const linked = drag.isLinked(selectedVMcp.id)}
		<div class="divide-y divide-base-200 dark:divide-base-400">
			{#each selectedComponents as component (component.key)}
				<div
					class="flex items-center gap-4 justify-between px-2 py-3 text-left"
					aria-label={component.name}
				>
					<div class="flex gap-2 items-center">
						<McpServerIcon icon={component.icon} />
						<div class="min-w-0 grow">
							<p class="truncate text-sm font-semibold">{component.name}</p>
							<p class="text-muted-content line-clamp-2 text-xs">
								{component.description || 'No description'}
							</p>
						</div>
					</div>
					<div class="flex items-center gap-1">
						<IconButton
							tooltip={{ text: 'Edit tools', disablePortal: true }}
							onclick={() => {
								vMcpDialog?.close();
								onEdit?.(component, selectedVMcp);
							}}
						>
							<PencilLine class="size-4" />
						</IconButton>
						<IconButton
							tooltip={{ text: 'Remove', disablePortal: true }}
							variant="danger"
							onclick={() => {
								onDelete?.(component, selectedVMcp);
								vMcpDialog?.close();
							}}
						>
							<Trash2 class="size-4" />
						</IconButton>
					</div>
				</div>
			{/each}
		</div>
		<div
			use:drag.vmcpTarget={selectedVMcp.id}
			class={twMerge(
				'border-base-300 dark:border-base-400 mt-4 flex flex-col gap-2 rounded-lg border border-dashed p-3',
				linked && 'vmcp-drop-target border-primary border-solid'
			)}
			role="region"
			aria-label={`MCP Servers in ${selectedVMcp.manifest.name ?? 'vMCP'}`}
		>
			<p class="text-muted-content text-xs italic text-center">
				{selectedComponents.length === 0
					? 'No servers yet. Drag and drop MCP servers here to add them.'
					: 'Drag and drop MCP servers here.'}
			</p>
		</div>
	{/if}
</ResponsiveDialog>
