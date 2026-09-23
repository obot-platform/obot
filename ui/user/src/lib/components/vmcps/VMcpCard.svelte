<script lang="ts">
	import { toInlineHTMLFromMarkdown } from '$lib/markdown';
	import {
		vmcpItemContext,
		type OpenDiff,
		type OpenEditInstanceConfiguration,
		type OpenSelectInstance,
		type OpenUpdateConfirm
	} from '$lib/runes/vmcps/vmcpItem.svelte';
	import type { VMCP } from '$lib/services';
	import type { VMcpConnectOptions } from '$lib/services/vmcps/types';
	import { vmcpConnectURL } from '$lib/services/vmcps/utils';
	import DotDotDot from '../DotDotDot.svelte';
	import VMcpCardActions from './VMcpCardActions.svelte';
	import VMcpMenuActions from './VMcpMenuActions.svelte';
	import VMcpStatusBadge from './VMcpStatusBadge.svelte';
	import { Pencil } from '@lucide/svelte';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		vmcp: VMCP;
		owner?: string;
		connectEl?: HTMLElement;
		onSelect?: () => void;
		selected?: boolean;
		selecting?: boolean;
		onEditDetails?: () => void;
		onConnect?: (options?: VMcpConnectOptions) => void;
		hideTest?: boolean;
		onDelete?: () => void;
		icon: Snippet;
		children?: Snippet;
		class?: string;
		selectAriaLabel: string;
		onUpdate?: (vmcp: VMCP) => void;
		openSelectInstance?: OpenSelectInstance;
		openDiff?: OpenDiff;
		openUpdateConfirm?: OpenUpdateConfirm;
		openEditInstanceConfiguration?: OpenEditInstanceConfiguration;
	}

	let {
		vmcp,
		owner,
		connectEl = $bindable(),
		onSelect,
		selected = false,
		selecting = false,
		onEditDetails,
		onConnect,
		hideTest,
		onDelete,
		icon,
		children,
		class: clazz,
		selectAriaLabel,
		onUpdate,
		openSelectInstance,
		openDiff,
		openUpdateConfirm,
		openEditInstanceConfiguration
	}: Props = $props();

	let ctx = $derived(vmcpItemContext(vmcp));
	let id = $derived(vmcp.id);
	let name = $derived(ctx.name);
	let descriptionHTML = $derived(
		vmcp.description ? toInlineHTMLFromMarkdown(vmcp.description) : undefined
	);
	let connectURL = $derived(vmcpConnectURL(vmcp));
	let connectButtonId = $derived(`btn-connect-to-server-${vmcp.id}`);
	let inSelectMode = $derived(Boolean(selecting));

	function handleSelectClick(event: MouseEvent) {
		if (!inSelectMode || !onSelect) return;
		const target = event.target;
		if (!(target instanceof Element)) return;
		if (target.closest('button, a, input, textarea, select, label')) return;
		onSelect();
	}
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
	class={twMerge(
		'relative flex flex-col',
		onSelect && !inSelectMode && 'pointer-events-none',
		clazz,
		selected && 'border-primary ring-2 ring-primary/30'
	)}
	onclick={handleSelectClick}
>
	{#if onSelect && !inSelectMode}
		<button
			type="button"
			class="pointer-events-auto absolute inset-0 z-0 rounded-[inherit] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
			aria-label={selectAriaLabel}
			onclick={onSelect}
		></button>
	{/if}
	<div class="flex items-start gap-2">
		{@render icon()}
		<div class="min-w-0 grow">
			<div class="flex min-w-0 items-center gap-2">
				<p class="truncate text-sm font-semibold">{name}</p>
			</div>
			<p class="text-muted-content mt-0.5 line-clamp-2 text-xs font-light min-h-8">
				<!-- eslint-disable-next-line svelte/no-at-html-tags -- sanitized by toInlineHTMLFromMarkdown -->
				{@html descriptionHTML}
			</p>
		</div>
		{#key inSelectMode}
			{#if inSelectMode}
				<div
					class="pointer-events-auto relative z-10 flex size-9 shrink-0 items-center justify-center"
					title={ctx.canDelete ? undefined : 'You can only delete vMCPs you created.'}
				>
					<input
						type="checkbox"
						class={twMerge('checkbox checkbox-sm', selected && 'checkbox-primary')}
						checked={selected}
						disabled={!ctx.canDelete}
						aria-label={`Select ${name}`}
						onclick={(event) => event.stopPropagation()}
						onchange={() => {
							if (!ctx.canDelete) return;
							onSelect?.();
						}}
					/>
				</div>
			{:else if ctx.hasActions}
				<DotDotDot
					placement="bottom-start"
					class="pointer-events-auto relative z-10 size-9 shrink-0"
					classes={{ menu: 'min-w-48' }}
					ariaLabel={`Actions for ${name}`}
				>
					{#snippet children({ toggle })}
						{#if onEditDetails}
							<button class="menu-button" onclick={onEditDetails}>
								<Pencil class="size-4" /> Edit Details
							</button>
						{/if}
						<VMcpMenuActions
							{vmcp}
							{toggle}
							onDelete={() => onDelete?.()}
							onUpdated={onUpdate}
							{openSelectInstance}
							{openDiff}
							{openUpdateConfirm}
							{openEditInstanceConfiguration}
						/>
					{/snippet}
				</DotDotDot>
			{/if}
		{/key}
	</div>

	{#if children}
		{@render children()}
	{/if}

	<div class="pointer-events-auto relative z-10">
		<VMcpCardActions
			{id}
			{connectURL}
			{connectButtonId}
			bind:connectEl
			{onConnect}
			{hideTest}
			disabled={!ctx.canConnect}
		/>
	</div>

	<div
		class="pt-2 border-t border-base-200 dark:border-base-400 flex items-center justify-between gap-4"
	>
		<p class="text-muted-content text-xs font-light min-h-4">
			{owner}
		</p>

		<VMcpStatusBadge
			{vmcp}
			class="pointer-events-auto relative z-10"
			onUpdated={onUpdate}
			{openSelectInstance}
			{openUpdateConfirm}
			{openEditInstanceConfiguration}
		/>
	</div>
</div>
