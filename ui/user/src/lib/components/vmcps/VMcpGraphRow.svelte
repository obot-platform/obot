<script lang="ts">
	import { resolve } from '$app/paths';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import { formatNumber } from '$lib/format';
	import type { EntryDrag } from '$lib/runes/vmcps/entryDrag.svelte';
	import type { MCPCatalogEntry } from '$lib/services';
	import { windowRange } from '$lib/services/vmcps/camera';
	import {
		VMCP_COMPONENT_HEIGHT,
		VMCP_COMPONENT_WINDOW_THRESHOLD
	} from '$lib/services/vmcps/constants';
	import type { RowContext, VMcpComponentView } from '$lib/services/vmcps/types';
	import McpServerIcon from './McpServerIcon.svelte';
	import './vmcpGraph.css';
	import { ChevronsRight, ExternalLink, Layers, PencilRuler, Server } from '@lucide/svelte';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	const CREATE_WIRE_DURATION_MS = 250;
	const CHAIN_STAGGER_MS = 120;
	const CHAIN_STAGGER_MAX_STEPS = 6;

	interface Props {
		vmcp: MCPCatalogEntry;
		components: VMcpComponentView[];
		expanded: boolean;
		context: RowContext;
		drag: EntryDrag;
		onToggleExpand: () => void;
		onEdit: () => void;
		onConnect: () => void;
		onModifyComponent: (component: VMcpComponentView) => void;
	}

	let {
		vmcp,
		components,
		expanded,
		context,
		drag,
		onToggleExpand,
		onEdit,
		onConnect,
		onModifyComponent
	}: Props = $props();

	const roughEstimationText =
		'This is a rough approximation of the number of tools available. The exact number may vary.';

	let componentRange = $derived.by(() => {
		if (!expanded) return { start: 0, end: 0 };
		if (components.length <= VMCP_COMPONENT_WINDOW_THRESHOLD) {
			return { start: 0, end: components.length };
		}
		return windowRange({
			viewTop: context.viewTop,
			viewBottom: context.viewBottom,
			originY: context.rowY,
			count: components.length,
			itemHeight: VMCP_COMPONENT_HEIGHT
		});
	});

	let { toolsCount, totalToolsCount, isRoughToolCountEstimate } = $derived.by(() => {
		let isRoughToolCountEstimate = false;
		let totalToolsCount = 0;
		let toolsCount = components.reduce((count, component) => {
			if (!component.toolOverrides) {
				isRoughToolCountEstimate = true;
			}
			if (component.toolOverrides) {
				const enabledCount = component.toolOverrides.filter((tool) => tool.enabled === true).length;
				totalToolsCount += component.toolOverrides.length;
				return count + enabledCount;
			}

			const previewCount = component.toolPreview?.length || 0;
			totalToolsCount += previewCount;
			return count + previewCount;
		}, 0);
		return { toolsCount, totalToolsCount, isRoughToolCountEstimate };
	});

	function chainDelay(index: number) {
		return Math.min(index, CHAIN_STAGGER_MAX_STEPS) * CHAIN_STAGGER_MS;
	}
</script>

<div
	style="--create-wire-ms: {CREATE_WIRE_DURATION_MS}ms"
	class="flex flex-col items-center md:flex-row md:items-center"
>
	{@render vmcpCard()}
	{@render chainWire(!expanded || components.length !== 1)}
	<div class="relative flex flex-col items-center md:items-stretch">
		<div class={expanded ? 'md:absolute md:-top-9 md:left-0 md:z-20' : ''}>
			{@render serversChip()}
		</div>
		{#if expanded && components.length === 0}
			{@render emptyComponentBlock()}
		{:else if expanded}
			{#if componentRange.start > 0}
				<div
					class="shrink-0"
					style="height: {componentRange.start * VMCP_COMPONENT_HEIGHT}px"
					aria-hidden="true"
				></div>
			{/if}
			{#each components.slice(componentRange.start, componentRange.end) as component, sliceIndex (component.key)}
				{@const index = componentRange.start + sliceIndex}
				<div class="flex flex-col items-center md:flex-row md:items-stretch">
					{@render componentBranch(index, components.length)}
					<div class="pb-3">
						{@render componentBlock(component, index)}
					</div>
				</div>
			{/each}
			{#if componentRange.end < components.length}
				<div
					class="shrink-0"
					style="height: {(components.length - componentRange.end) * VMCP_COMPONENT_HEIGHT}px"
					aria-hidden="true"
				></div>
			{/if}
		{/if}
	</div>
</div>

{#snippet chainWire(showEndNode = true)}
	<div class="text-primary flex flex-col items-center md:hidden" aria-hidden="true">
		<span class="bg-current size-1.5 rounded-full opacity-70"></span>
		<div class="vmcp-wire-y"></div>
		{#if showEndNode}
			<span
				class="bg-current size-1.5 rounded-full translate-x-1"
				in:fade={{ delay: CREATE_WIRE_DURATION_MS, duration: 180 }}
			></span>
		{/if}
	</div>
	<div class="text-primary hidden items-center md:flex" aria-hidden="true">
		<span class="bg-current size-1.5 rounded-full opacity-70"></span>
		<div class="vmcp-wire-x"></div>
		{#if showEndNode}
			<span
				class="bg-current size-1.5 rounded-full translate-x-1"
				in:fade={{ delay: CREATE_WIRE_DURATION_MS, duration: 180 }}
			></span>
		{/if}
	</div>
{/snippet}

{#snippet componentBranch(index: number, total: number)}
	{@const delay = chainDelay(index)}
	{#if index > 0}
		<div
			class="text-primary flex flex-col items-center md:hidden"
			style="--wire-delay: {delay}ms"
			aria-hidden="true"
		>
			<div class="vmcp-wire-y"></div>
			<span
				class="bg-current size-1.5 rounded-full"
				in:fade={{ delay: delay + CREATE_WIRE_DURATION_MS, duration: 180 }}
			></span>
		</div>
	{/if}
	<div
		class="text-primary relative hidden w-14 shrink-0 self-stretch md:block lg:w-20"
		style="--wire-delay: {delay}ms"
		aria-hidden="true"
	>
		{#if total > 1}
			<div
				class={twMerge(
					'vmcp-trunk',
					index === 0 && 'vmcp-trunk-first',
					index === total - 1 && 'vmcp-trunk-last'
				)}
			></div>
		{/if}
		<div class="vmcp-branch"></div>
		<span
			class="bg-current absolute top-1/2 right-0 size-1.5 -translate-y-1/2 rounded-full"
			in:fade={{ delay: delay + CREATE_WIRE_DURATION_MS, duration: 180 }}
		></span>
	</div>
{/snippet}

{#snippet serversChip()}
	{@const label = components.length === 1 ? 'server' : 'servers'}
	<button
		type="button"
		class="bg-base-100 dark:bg-base-300 dark:border-base-400 text-base-content relative z-10 flex items-center gap-2 rounded-lg border border-transparent px-3 py-2 text-left shadow-md"
		aria-expanded={expanded}
		aria-label={expanded
			? `Hide servers in ${vmcp.manifest.name ?? 'vMCP'}`
			: `Show ${components.length} ${label} in ${vmcp.manifest.name ?? 'vMCP'}`}
		onclick={onToggleExpand}
	>
		<Server class="text-primary size-4 shrink-0" />
		<span class="font-mono text-xs uppercase">{components.length} {label}</span>
		<ChevronsRight class={twMerge('size-3.5 opacity-70', expanded && 'rotate-90')} />
	</button>
{/snippet}

{#snippet vmcpCard()}
	{@const linked = drag.isLinked(vmcp.id)}
	<div
		use:drag.vmcpTarget={vmcp.id}
		class={twMerge(
			'max-w-full md:w-sm shrink-0 rounded-lg translate-y-0 transition-transform',
			linked
				? 'vmcp-drop-target border-primary text-primary'
				: 'p-0.5 hover:aura text-transparent hover:text-primary hover:-translate-y-0.5'
		)}
		in:fade={{ duration: 150 }}
	>
		<div
			class="bg-base-100 dark:bg-base-300 dark:border-base-400 text-base-content relative flex rounded-lg border border-transparent p-2 text-left transition-all duration-200 shadow-sm"
		>
			<div class="flex size-full flex-col">
				<div class="flex items-center justify-between gap-2 mb-2">
					<button
						type="button"
						class="flex min-w-0 grow cursor-pointer items-center gap-2 rounded-md text-left after:absolute after:inset-0 after:rounded-lg after:content-[''] focus-visible:outline-none focus-visible:after:ring-2 focus-visible:after:ring-primary"
						onclick={onEdit}
						aria-label={`Edit ${vmcp.manifest.name ?? 'vMCP'}`}
					>
						<div class="bg-primary/10 text-primary shrink-0 rounded-md p-2">
							<Layers class="size-5" />
						</div>
						<div class="flex min-w-0 grow flex-col">
							<p class="truncate text-sm font-semibold">{vmcp.manifest.name}</p>
						</div>
					</button>
					<DotDotDot
						placement="bottom-start"
						class="relative z-10 size-9 shrink-0"
						classes={{ menu: 'min-w-48' }}
					>
						{#snippet children({ toggle })}
							<!-- The menu closes itself on any click that reaches it, and cancels that click's
							     default action along with it, so these links have to close it themselves. -->
							<a
								class="menu-button justify-between"
								href={resolve(`/audit-logs?mcp_id=${encodeURIComponent(vmcp.id)}`)}
								target="_blank"
								rel="noopener"
								onclick={(e) => {
									e.stopPropagation();
									toggle(false);
								}}
							>
								View Audit Logs <ExternalLink class="size-4" />
							</a>
							<a
								class="menu-button justify-between"
								href={resolve(`/usage?mcp_id=${encodeURIComponent(vmcp.id)}`)}
								target="_blank"
								rel="noopener"
								onclick={(e) => {
									e.stopPropagation();
									toggle(false);
								}}
							>
								View Usage <ExternalLink class="size-4" />
							</a>
						{/snippet}
					</DotDotDot>
				</div>

				<div class="flex items-center gap-2">
					<div
						class="relative z-10 flex grow items-center border border-base-300 dark:border-base-400 rounded-lg"
					>
						<button
							class="btn flex grow font-mono text-xs uppercase bg-primary/10 hover:bg-primary hover:text-primary-content border-transparent rounded-r-none"
							onclick={onConnect}
						>
							Connect
						</button>
						<CopyButton
							tooltipText="Copy Connect URL"
							text={vmcp.connectURL}
							noButtonText
							classes={{
								button:
									'size-10 p-2 hover:bg-primary hover:text-primary-content justify-center rounded-r-md border-l border-l-base-300 dark:border-l-base-400'
							}}
						/>
					</div>
					<div
						class="relative z-10 badge badge-primary badge-soft py-4 w-26"
						title={roughEstimationText}
					>
						<PencilRuler class="size-4 shrink-0" aria-label="Tools" />
						<div class="flex grow justify-center">
							{#if totalToolsCount > 0}
								<span class="font-mono text-xs">
									{isRoughToolCountEstimate ? '≈' : ''}{formatNumber(toolsCount)}/{formatNumber(
										totalToolsCount
									)}
								</span>
							{:else if components.length > 0}
								<span class="font-mono text-xs">All</span>
							{/if}
						</div>
					</div>
				</div>
			</div>
		</div>
	</div>
{/snippet}

{#snippet emptyComponentBlock()}
	{@const linked = drag.isComponentLinked(vmcp.id, 'empty')}
	<div
		use:drag.componentTarget={{ vmcpId: vmcp.id, key: 'empty' }}
		class={twMerge(
			'bg-base-100 dark:bg-base-300 dark:border-base-400 relative z-10 flex w-[min(20rem,calc(100vw-3rem))] flex-col rounded-lg border border-transparent p-5 shadow-md md:w-81',
			linked && 'vmcp-drop-target border-primary'
		)}
		role="region"
		aria-label={`MCP Servers in ${vmcp.manifest.name ?? 'vMCP'}`}
		in:fade={{ delay: CREATE_WIRE_DURATION_MS, duration: 200 }}
	>
		<p class="text-muted-content text-xs italic">
			No servers yet. Drag one in from the Add Tools panel.
		</p>
	</div>
{/snippet}

{#snippet componentBlock(component: VMcpComponentView, index: number)}
	<div class="aura text-transparent hover:text-primary hover:-translate-y-0.5">
		<button
			use:drag.componentTarget={{ vmcpId: vmcp.id, key: component.key }}
			class={twMerge(
				'text-base-content bg-base-100 dark:bg-base-300 dark:border-base-400 relative z-10 flex w-[min(20rem,calc(100vw-3rem))] flex-col rounded-lg border border-transparent p-2 shadow-md md:w-81 text-left items-start'
			)}
			aria-label={component.name}
			in:fade={{ delay: chainDelay(index) + CREATE_WIRE_DURATION_MS, duration: 200 }}
			onclick={() => onModifyComponent(component)}
		>
			<div class="mb-3 flex items-start gap-2">
				<div class="flex items-center gap-2">
					<McpServerIcon icon={component.icon} />
					<div class="min-w-0 grow">
						<p class="truncate text-sm font-semibold">{component.name}</p>
						<p class="text-muted-content line-clamp-2 text-xs">
							{component.description || 'No description'}
						</p>
					</div>
				</div>
			</div>
			{@render tools(component)}
		</button>
	</div>
{/snippet}

{#snippet tools(component: VMcpComponentView)}
	{@const withToolOverrides = component.toolOverrides}
	<div class="divider my-0 text-xs font-medium text-muted-content mb-2">Tools</div>
	{#if withToolOverrides && withToolOverrides.length > 0}
		{@const total = withToolOverrides.length}
		{@const selectedCount = withToolOverrides.filter((tool) => tool.enabled === true).length}
		<p class="text-muted-content font-mono text-xs text-center w-full">
			{selectedCount} / {total} selected
		</p>
	{:else}
		<p class="text-muted-content font-mono text-xs text-center w-full">
			All tools enabled by default
		</p>
	{/if}
{/snippet}
