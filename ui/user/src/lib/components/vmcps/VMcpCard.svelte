<script lang="ts">
	import { resolve } from '$app/paths';
	import type { VMcpConnectOptions } from '$lib/services/vmcps/types';
	import { profile } from '$lib/stores';
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
		hideTest?: boolean;
		onDelete?: () => void;
		icon: Snippet;
		children?: Snippet;
		class?: string;
		selectAriaLabel: string;
		enterDelay?: number;
		isOwner?: boolean;
		note?: string;
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
		hideTest,
		onDelete,
		icon,
		children,
		class: clazz,
		selectAriaLabel,
		enterDelay,
		isOwner,
		note
	}: Props = $props();
</script>

<div
	class={twMerge('relative flex flex-col', onSelect && 'pointer-events-none', clazz)}
	in:fade={{ delay: enterDelay ?? 0, duration: enterDelay === undefined ? 0 : 150 }}
>
	{#if onSelect}
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
			class="pointer-events-auto relative z-10 size-9 shrink-0"
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
				{#if profile.current.isAdmin?.() || isOwner}
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
				{/if}
			{/snippet}
		</DotDotDot>
	</div>

	{#if children}
		{@render children()}
	{/if}

	<div class="pointer-events-auto relative z-10">
		<VMcpCardActions {id} {connectURL} {connectButtonId} {onConnect} {hideTest} />
	</div>

	{#if note}
		<div class="pt-2 border-t border-base-200 dark:border-base-400 flex justify-between gap-4">
			<p class="text-muted-content text-xs font-light min-h-4">
				{note}
			</p>
		</div>
	{/if}
</div>
