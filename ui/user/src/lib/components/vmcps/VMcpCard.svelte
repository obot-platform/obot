<script lang="ts">
	import { resolve } from '$app/paths';
	import type { VMcpConnectOptions } from '$lib/services/vmcps/types';
	import { isInteractiveChildEvent } from '$lib/utils';
	import DotDotDot from '../DotDotDot.svelte';
	import VMcpCardActions from './VMcpCardActions.svelte';
	import { ExternalLink, Trash2 } from '@lucide/svelte';
	import type { Snippet } from 'svelte';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		id: string;
		name: string;
		descriptionHTML?: string;
		connectURL?: string;
		connectButtonId?: string;
		connected?: boolean;
		onSelect?: () => void;
		onConnect?: (options?: VMcpConnectOptions) => void;
		onDelete?: () => void;
		icon: Snippet;
		children?: Snippet;
		class?: string;
		selectAriaLabel: string;
		enterDelay?: number;
	}

	let {
		id,
		name,
		descriptionHTML,
		connectURL,
		connectButtonId,
		connected,
		onSelect,
		onConnect,
		onDelete,
		icon,
		children,
		class: clazz,
		selectAriaLabel,
		enterDelay
	}: Props = $props();
</script>

<div
	class={twMerge('flex flex-col', clazz)}
	role="button"
	tabindex="0"
	aria-label={selectAriaLabel}
	in:fade={{ delay: enterDelay ?? 0, duration: enterDelay === undefined ? 0 : 150 }}
	onkeydown={(e) => {
		if (!onSelect || isInteractiveChildEvent(e)) {
			return;
		}
		if (e.key === 'Enter' || e.key === ' ') {
			e.preventDefault();
			e.stopPropagation();
			onSelect();
		}
	}}
	onclick={(e) => {
		if (onSelect && !isInteractiveChildEvent(e)) {
			onSelect();
		}
	}}
>
	<div class="flex items-start gap-2">
		{@render icon()}
		<div class="min-w-0 grow">
			<div class="flex min-w-0 items-center gap-2">
				<p class="truncate text-sm font-semibold">{name}</p>
				{#if connected}
					<div class="badge badge-xs badge-secondary shrink-0 gap-1">
						<span class="status status-primary"></span>
						Connected
					</div>
				{/if}
			</div>
			<p class="text-muted-content mt-0.5 line-clamp-2 text-xs font-light min-h-8">
				<!-- eslint-disable-next-line svelte/no-at-html-tags -- sanitized by toInlineHTMLFromMarkdown -->
				{@html descriptionHTML}
			</p>
		</div>
		<DotDotDot
			placement="bottom-start"
			class="relative z-10 size-9 shrink-0"
			classes={{ menu: 'min-w-48' }}
			ariaLabel={`Actions for ${name}`}
		>
			{#snippet children({ toggle })}
				<a
					class="menu-button justify-between"
					href={resolve(`/audit-logs?mcp_id=${encodeURIComponent(id)}`)}
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
					href={resolve(`/usage?mcp_id=${encodeURIComponent(id)}`)}
					target="_blank"
					rel="noopener"
					onclick={(e) => {
						e.stopPropagation();
						toggle(false);
					}}
				>
					View Usage <ExternalLink class="size-4" />
				</a>
				<button
					class="menu-button-destructive"
					onclick={(e) => {
						e.stopPropagation();
						onDelete?.();
						toggle(false);
					}}
				>
					<Trash2 class="size-4" />
					Delete
				</button>
			{/snippet}
		</DotDotDot>
	</div>

	{#if children}
		{@render children()}
	{/if}

	<VMcpCardActions {id} {connectURL} {connectButtonId} {onConnect} />
</div>
