<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { isWebURL } from '$lib/url';
	import { Unplug } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		detached?: boolean;
		sourceURL?: string;
		variant?: 'badge' | 'notification';
		class?: string;
	}

	let { detached, sourceURL, variant = 'badge', class: className }: Props = $props();
	const explanation =
		'This entry was removed from its Git catalog. Obot retained it to avoid disrupting deployments. It is no longer synchronized with Git and is now managed in Obot.';
</script>

{#if detached}
	{#if variant === 'notification'}
		<div
			class={twMerge(
				'border-warning bg-warning/10 flex w-full items-start gap-2 rounded-md border p-3 text-left',
				className
			)}
		>
			<Unplug class="text-warning mt-0.5 size-4 shrink-0" />
			<div class="text-sm">
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
