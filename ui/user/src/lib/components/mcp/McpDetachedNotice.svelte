<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
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
	const explanation =
		'This entry was removed from its Git catalog. Obot retained it to avoid disrupting deployments. It remains read-only and will resume Git synchronization if restored upstream. Accept ownership to manage it in Obot.';

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
				<p class="font-medium">Detached from Git</p>
				<p class="text-muted-content">{explanation}</p>
				{#if sourceURL}
					{#if isWebURL(sourceURL)}
						<a
							href={sourceURL}
							target="_blank"
							rel="external noopener noreferrer"
							class="text-link mt-1 inline-block"
						>
							View original Git source
						</a>
					{:else}
						<p class="text-muted-content mt-1 text-xs break-all">Original source: {sourceURL}</p>
					{/if}
				{/if}
			</div>
			{#if onAcceptOwnership}
				<button
					class="btn btn-sm btn-warning ml-6 shrink-0 sm:ml-0"
					onclick={acceptOwnership}
					disabled={acceptingOwnership}
				>
					{acceptingOwnership ? 'Accepting...' : 'Accept ownership'}
				</button>
			{/if}
		</div>
	{:else}
		<span
			class={twMerge('badge badge-xs border-warning text-warning gap-1 bg-warning/10', className)}
			use:tooltip={{ text: explanation, classes: ['w-sm'] }}
		>
			<Unplug class="size-3" />
			Detached
		</span>
	{/if}
{/if}
