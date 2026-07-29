<script lang="ts">
	import { CircleMinus } from '@lucide/svelte';

	interface Props {
		detailed?: boolean;
	}

	let { detailed = false }: Props = $props();
</script>

{#snippet badge()}
	<span
		class="badge badge-error badge-soft badge-sm gap-1"
		role="status"
		aria-live="polite"
		aria-atomic="true"
	>
		<CircleMinus class="size-3" />
		Tunnel disconnected
	</span>
{/snippet}

{#if detailed}
	<section
		class="dark:bg-base-200 dark:border-error/40 bg-base-100 border-error/30 flex flex-col gap-3 rounded-lg border p-4 shadow-sm"
	>
		<div class="flex flex-wrap items-start justify-between gap-3">
			<div class="flex flex-col gap-1">
				<h2 class="text-sm font-semibold">Tunnel Status</h2>
				<p class="text-muted-content text-xs font-light">
					Live tunnel connection status refreshes automatically.
				</p>
			</div>
			{@render badge()}
		</div>
		<p class="text-muted-content text-sm font-light">
			The tunnel client used by this remote MCP server is not connected.
		</p>
	</section>
{:else}
	{@render badge()}
{/if}
