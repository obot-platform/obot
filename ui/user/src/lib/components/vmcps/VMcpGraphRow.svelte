<script lang="ts">
	import { toInlineHTMLFromMarkdown } from '$lib/markdown';
	import type { EntryDrag } from '$lib/runes/vmcps/entryDrag.svelte';
	import type { OrgUser, VMCP } from '$lib/services';
	import { windowRange } from '$lib/services/vmcps/camera';
	import {
		VMCP_COMPONENT_HEIGHT,
		VMCP_COMPONENT_WINDOW_THRESHOLD
	} from '$lib/services/vmcps/constants';
	import type {
		RowContext,
		VMcpComponentView,
		VMcpConnectOptions
	} from '$lib/services/vmcps/types';
	import { getToolCounts, vmcpConnectURL } from '$lib/services/vmcps/utils';
	import { profile } from '$lib/stores';
	import { getUserDisplayName } from '$lib/utils';
	import InfoTooltip from '../InfoTooltip.svelte';
	import McpServerIcon from './McpServerIcon.svelte';
	import VMcpCard from './VMcpCard.svelte';
	import VMcpIcon from './VMcpIcon.svelte';
	import './vmcpGraph.css';
	import { Layers } from '@lucide/svelte';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	const CREATE_WIRE_DURATION_MS = 250;
	const CHAIN_STAGGER_MS = 120;
	const CHAIN_STAGGER_MAX_STEPS = 6;

	interface Props {
		vmcp: VMCP;
		components: VMcpComponentView[];
		canEdit?: boolean;
		isOwner?: boolean;
		context: RowContext;
		drag: EntryDrag;
		onEdit?: () => void;
		onConnect: (options?: VMcpConnectOptions) => void;
		onDelete?: () => void;
		onModifyComponent?: (component: VMcpComponentView) => void;
		usersMap: Map<string, OrgUser>;
	}

	let {
		vmcp,
		components,
		canEdit = true,
		isOwner = false,
		context,
		drag,
		onEdit,
		onConnect,
		onDelete,
		onModifyComponent,
		usersMap
	}: Props = $props();

	let tools = $derived(getToolCounts(components));
	const roughEstimationText =
		'This is a rough approximation of the number of tools available. The exact number may vary.';

	let componentRange = $derived.by(() => {
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

	let owner = $derived.by(() => {
		if (vmcp.userID) {
			return `Created by ${vmcp.userID === profile.current.id ? 'me' : getUserDisplayName(usersMap, vmcp.userID)}`;
		}
		return 'Created by admin';
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
	{@render chainWire(components.length !== 1)}
	<div class="relative flex flex-col items-center md:items-stretch">
		{#if components.length === 0}
			{@render emptyComponentBlock()}
		{:else}
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

{#snippet vmcpCard()}
	{@const linked = drag.isLinked(vmcp.id)}
	{@const name = vmcp.displayName || 'vMCP'}
	<div class="translate-y-1">
		<div
			use:drag.vmcpTarget={vmcp.id}
			class={twMerge(
				'max-w-full md:w-xs shrink-0 rounded-lg translate-y-0 transition-transform',
				linked
					? 'vmcp-drop-target border-primary text-primary'
					: canEdit
						? 'p-0.5 aura text-primary hover:-translate-y-0.5'
						: 'p-0.5'
			)}
			in:fade={{ duration: 150 }}
		>
			<VMcpCard
				id={vmcp.id}
				{name}
				descriptionHTML={vmcp.description ? toInlineHTMLFromMarkdown(vmcp.description) : undefined}
				connectURL={vmcpConnectURL(vmcp)}
				selectAriaLabel={canEdit ? `Edit ${name}` : name}
				onSelect={canEdit ? onEdit : undefined}
				{onConnect}
				hideTest
				onDelete={canEdit ? onDelete : undefined}
				{isOwner}
				class={twMerge(
					'bg-base-100 dark:bg-base-300 dark:border-base-400 text-base-content relative gap-2 rounded-lg border border-transparent p-2 text-left shadow-sm transition-all duration-200',
					canEdit && 'cursor-pointer'
				)}
				note={owner}
			>
				{#snippet icon()}
					{#if (vmcp.components ?? []).length > 0}
						<VMcpIcon
							components={vmcp.components.map((component) => ({
								name: component.name,
								icon: component.catalogEntry.manifest.icon
							}))}
						/>
					{:else}
						<div class="bg-primary/10 text-primary shrink-0 rounded-md p-2">
							<Layers class="size-5" />
						</div>
					{/if}
				{/snippet}
			</VMcpCard>
		</div>
		<div class="flex justify-between gap-4 pt-1 px-0.5">
			<p class="text-muted-content text-xs font-light">{vmcp.components.length} Servers</p>

			{#if tools}
				<p class="text-muted-content text-xs font-light items-center flex gap-1">
					{#if tools.total === 0}
						All tools enabled
					{:else}
						{tools.approximate ? '~' : ''}{tools.enabled} tools enabled
						{#if tools.approximate}
							<InfoTooltip
								class="pointer-events-auto relative z-10"
								text={roughEstimationText}
								placement="bottom-end"
							/>
						{/if}
					{/if}
				</p>
			{/if}
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
		aria-label={`MCP Servers in ${vmcp.displayName || 'vMCP'}`}
		in:fade={{ delay: CREATE_WIRE_DURATION_MS, duration: 200 }}
	>
		<p class="text-muted-content text-xs italic">
			{canEdit ? 'No servers yet. Drag one in from the MCP Servers panel.' : 'No servers yet.'}
		</p>
	</div>
{/snippet}

{#snippet componentBlock(component: VMcpComponentView, index: number)}
	<div
		class={twMerge(
			canEdit &&
				'hover:aura hover:aura-glow p-0.5 text-transparent hover:text-primary hover:-translate-y-0.5'
		)}
	>
		{#if canEdit}
			<button
				use:drag.componentTarget={{ vmcpId: vmcp.id, key: component.key }}
				class={twMerge(
					'text-base-content bg-base-100 dark:bg-base-300 dark:border-base-400 relative z-10 flex w-[min(20rem,calc(100vw-3rem))] flex-col rounded-lg border border-transparent p-2 shadow-md md:w-81 text-left items-start'
				)}
				aria-label={component.name}
				in:fade={{ delay: chainDelay(index) + CREATE_WIRE_DURATION_MS, duration: 200 }}
				onclick={() => onModifyComponent?.(component)}
			>
				{@render componentContent(component)}
			</button>
		{:else}
			<div
				class="text-base-content bg-base-100 dark:bg-base-300 dark:border-base-400 relative z-10 flex w-[min(20rem,calc(100vw-3rem))] flex-col rounded-lg border border-transparent p-2 shadow-md md:w-81 text-left items-start"
				in:fade={{ delay: chainDelay(index) + CREATE_WIRE_DURATION_MS, duration: 200 }}
			>
				{@render componentContent(component)}
			</div>
		{/if}
	</div>
{/snippet}

{#snippet componentContent(component: VMcpComponentView)}
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
	{@render componentTools(component)}
{/snippet}

{#snippet componentTools(component: VMcpComponentView)}
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
