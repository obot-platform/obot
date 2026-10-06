<script lang="ts">
	import { m } from '$lib/i18n';
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
	{@const badgeText = connection
		? m.core_mcp_value_connected()
		: known
			? m.mcps_tunnels_disconnected()
			: m.core_unknown()}
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
				<h2 class="text-sm font-semibold">{m.mcps_tunnels_connection_status()}</h2>
				<p class="text-muted-content text-xs font-light">
					{m.mcps_tunnels_status_refreshes()}
				</p>
			</div>
			{@render statusBadge()}
		</div>

		<p class="text-muted-content text-sm font-light">
			{#if connection}
				{m.mcps_tunnels_client_connected()}
			{:else if known}
				{m.mcps_tunnels_no_client_connected()}
			{:else}
				{m.mcps_tunnels_status_unavailable()}
			{/if}
		</p>
	</section>
{:else}
	{@render statusBadge()}
{/if}
