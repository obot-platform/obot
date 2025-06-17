<script lang="ts">
	import { CircleSlash, CircleCheck, PictureInPicture2 } from 'lucide-svelte';
	import DotDotDot from '../DotDotDot.svelte';
	import type { ModelProvider } from '$lib/services';
	import { darkMode } from '$lib/stores';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		recommended?: boolean;
		modelProvider: ModelProvider;
	}

	const { recommended, modelProvider }: Props = $props();
</script>

<div
	class="dark:bg-surface1 dark:border-surface3 flex w-full flex-col items-center justify-center gap-4 rounded-lg bg-white p-4 pt-2 dark:border"
>
	<div class="flex w-full items-center justify-between">
		<div>
			{#if recommended}
				<span class="rounded-md bg-blue-500 px-2 py-1 text-xs font-semibold text-white"
					>Recommended</span
				>
			{/if}
		</div>

		<div class="flex items-center">
			<button class="icon-button">
				<PictureInPicture2 class="size-5" />
			</button>
			<DotDotDot>
				<div class="default-dialog flex min-w-max flex-col p-2">
					<button class="menu-button text-red-500" onclick={() => {}}>
						Deconfigure Provider
					</button>
				</div>
			</DotDotDot>
		</div>
	</div>
	{#if darkMode.isDark}
		{@const url = modelProvider.iconDark ?? modelProvider.icon}
		<img src={url} alt={modelProvider.name} class="h-16 w-16" />
	{:else}
		<img src={modelProvider.icon} alt={modelProvider.name} class="h-16 w-16" />
	{/if}
	<h4 class="text-center text-lg font-semibold">{modelProvider.name}</h4>
	<div class="border-surface2 rounded-md border px-2 py-1">
		<span class="flex items-center gap-1 text-xs font-light">
			{#if modelProvider.configured}
				<CircleCheck class="size-4 text-green-500" /> Configure
			{:else}
				<CircleSlash class="size-4 text-red-500" /> Not Configured
			{/if}
		</span>
	</div>

	<div class="mt-auto w-full">
		<button
			onclick={() => {}}
			class={twMerge(
				'w-full border-0 text-sm',
				modelProvider.configured ? 'button' : 'button-primary bg-blue-300'
			)}
		>
			{#if modelProvider.configured}
				Modify
			{:else}
				Configure
			{/if}
		</button>
	</div>
</div>
