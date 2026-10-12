<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { m } from '$lib/i18n';
	import { isWebURL } from '$lib/url';
	import { Unplug } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		detached?: boolean;
		sourceURL?: string;
		variant?: 'badge' | 'notification';
		onAcceptOwnership?: () => Promise<void>;
		class?: string;
	}

	let {
		detached,
		sourceURL,
		variant = 'badge',
		onAcceptOwnership,
		class: className
	}: Props = $props();
	let acceptingOwnership = $state(false);
	const explanation = m.mcps_servers_detached_explanation();

	async function acceptOwnership() {
		if (!onAcceptOwnership || acceptingOwnership) return;
		acceptingOwnership = true;
		try {
			await onAcceptOwnership();
		} finally {
			acceptingOwnership = false;
		}
	}
</script>

{#if detached}
	{#if variant === 'notification'}
		<div
			class={twMerge(
				'border-warning bg-warning/10 flex w-full flex-wrap items-start gap-2 rounded-md border p-3 text-left sm:flex-nowrap',
				className
			)}
		>
			<Unplug class="text-warning mt-0.5 size-4 shrink-0" />
			<div class="min-w-0 flex-1 text-sm">
				<p class="font-medium">{m.mcps_servers_detached_from_git()}</p>
				<p class="text-muted-content">{explanation}</p>
				{#if sourceURL}
					{#if isWebURL(sourceURL)}
						<a
							href={sourceURL}
							target="_blank"
							rel="external noopener noreferrer"
							class="text-link mt-1 inline-block"
						>
							{m.mcps_servers_detached_view_source()}
						</a>
					{:else}
						<p class="text-muted-content mt-1 text-xs break-all">
							{m.mcps_servers_detached_original_source({ url: sourceURL })}
						</p>
					{/if}
				{/if}
			</div>
			{#if onAcceptOwnership}
				<button
					class="btn btn-sm btn-warning ml-6 shrink-0 sm:ml-0"
					onclick={acceptOwnership}
					disabled={acceptingOwnership}
				>
					{acceptingOwnership
						? m.mcps_servers_detached_accepting()
						: m.mcps_servers_detached_accept_ownership()}
				</button>
			{/if}
		</div>
	{:else}
		<span
			class={twMerge('badge badge-xs border-warning text-warning gap-1 bg-warning/10', className)}
			use:tooltip={{ text: explanation, classes: ['w-sm'] }}
		>
			<Unplug class="size-3" />
			{m.mcps_servers_detached_badge()}
		</span>
	{/if}
{/if}
