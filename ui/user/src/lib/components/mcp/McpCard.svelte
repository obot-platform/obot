<script lang="ts">
	import { twMerge } from 'tailwind-merge';
	import { CircleCheckBig, Star } from 'lucide-svelte';
	import { formatNumber } from '$lib/format';
	import type { TransformedMcp } from './McpCatalog.svelte';
	interface Props {
		data: TransformedMcp;
		onSelect: (data: TransformedMcp) => void;
		selected?: boolean;
		disabled?: boolean;
		tags?: string[];
	}
	let { data, selected, disabled, tags, onSelect }: Props = $props();
</script>

<div class="relative h-full w-full">
	{#if selected && !disabled}
		<CircleCheckBig class="absolute top-3 right-3 z-25 size-5 text-blue-500" />
	{/if}
	<button
		onclick={() => {
			if (!disabled) {
				onSelect(data);
			}
		}}
		class={twMerge(
			'card group from-surface2 to-surface1 relative z-20 h-full w-full flex-col overflow-hidden border border-transparent bg-radial-[at_25%_25%] to-75% shadow-sm select-none',
			selected && !disabled && 'transform-none border border-blue-500',
			disabled && 'cursor-not-allowed opacity-50'
		)}
	>
		{#if data}
			<div class="flex h-fit w-full flex-col gap-2 p-3 md:h-auto md:grow">
				<div class="flex w-full items-center gap-2">
					<div class="flex-shrink-0 rounded-md bg-gray-50 p-1 dark:bg-gray-600">
						<img alt="obot logo" src={data.icon} class="size-6" />
					</div>
					<div class="flex flex-col text-left">
						<h4 class="line-clamp-1 text-sm font-semibold">
							{data.name}
						</h4>
						<p class="line-clamp-1 grow text-left text-xs font-light text-gray-500">
							{data.description}
						</p>
					</div>
				</div>
				<div class="flex w-full grow justify-between gap-2 text-xs">
					<div class="flex h-fit flex-wrap gap-1">
						{#if tags}
							{#each tags as tag}
								<span
									class="border-surface3 dark:border-surface3 flex h-fit items-center gap-1 rounded-md border px-1 text-[11px] text-gray-500"
								>
									{tag}
								</span>
							{/each}
						{/if}
					</div>
					{#if data.githubStars > 0}
						<span
							class="dark:bg-surface2 mt-auto flex h-fit items-center gap-1 rounded-md bg-gray-50 px-1 text-xs text-gray-500"
						>
							<Star class="size-3" />
							{formatNumber(data.githubStars)}
						</span>
					{/if}
				</div>
			</div>
		{/if}
	</button>
</div>
