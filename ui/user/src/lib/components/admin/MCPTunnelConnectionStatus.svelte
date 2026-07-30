<script lang="ts">
	import type { TunnelConnection } from '$lib/services';
	import { CircleCheck, CircleQuestionMark, CircleMinus } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		connection?: TunnelConnection;
		detailed?: boolean;
		known?: boolean;
	}

	let { connection, detailed = false, known = true }: Props = $props();
</script>

{#snippet statusBadge()}
	{@const badgeClass = connection ? 'badge-success' : known ? 'badge-neutral' : 'badge-secondary'}
	{@const badgeText = connection ? 'Connected' : known ? 'Disconnected' : 'Unknown'}
	{@const BadgeIcon = connection ? CircleCheck : known ? CircleMinus : CircleQuestionMark}
	<span
		class={twMerge('badge badge-soft badge-sm gap-1', badgeClass)}
		role="status"
		aria-live="polite"
		aria-atomic="true"
	>
		<BadgeIcon class="size-3" />
		{badgeText}
	</span>
{/snippet}

{#if detailed}
	<section
		class="dark:bg-base-200 dark:border-base-400 bg-base-100 flex flex-col gap-4 rounded-lg border border-transparent p-4 shadow-sm"
	>
		<div class="flex flex-wrap items-start justify-between gap-3">
			<div class="flex flex-col gap-1">
				<h2 class="text-sm font-semibold">Connection Status</h2>
				<p class="text-muted-content text-xs font-light">
					Live tunnel connection status refreshes automatically.
				</p>
			</div>
			{@render statusBadge()}
		</div>

		<p class="text-muted-content text-sm font-light">
			{#if connection}
				A tunnel client is currently connected.
			{:else if known}
				No tunnel client is currently connected.
			{:else}
				Connection status is temporarily unavailable.
			{/if}
		</p>
	</section>
{:else}
	{@render statusBadge()}
{/if}
