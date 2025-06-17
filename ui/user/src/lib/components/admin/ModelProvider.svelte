<script lang="ts">
	import { CircleSlash, CircleCheck, PictureInPicture2 } from 'lucide-svelte';
	import DotDotDot from '../DotDotDot.svelte';
	import type { ModelProvider } from '$lib/services';
	import { darkMode } from '$lib/stores';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		recommended?: boolean;
		modelProvider: ModelProvider;
		onConfigure: () => void;
	}

	const { recommended, modelProvider, onConfigure }: Props = $props();
</script>

<div
	class="dark:bg-surface1 dark:border-surface3 flex w-full flex-col items-center justify-center gap-4 rounded-lg border border-transparent bg-white p-4 pt-2"
>
	<div class="flex w-full items-center justify-between">
		<div>
			{#if recommended}
				<span class="rounded-md bg-blue-500 px-2 py-1 text-[11px] font-semibold text-white"
					>Recommended</span
				>
			{/if}
		</div>

		<div class="flex translate-x-2 items-center gap-1">
			{#if modelProvider.configured}
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
			{/if}
		</div>
	</div>
	{#if darkMode.isDark}
		{@const url = modelProvider.iconDark ?? modelProvider.icon}
		<img
			src={url}
			alt={modelProvider.name}
			class={twMerge('size-16 rounded-md p-1', !modelProvider.iconDark && 'bg-gray-600')}
		/>
	{:else}
		<img src={modelProvider.icon} alt={modelProvider.name} class="size-16 rounded-md p-1" />
	{/if}
	<h4 class="text-center text-lg font-semibold">{modelProvider.name}</h4>
	<div class="border-surface2 rounded-md border px-2 py-1">
		<span class="flex items-center gap-2 text-xs font-light">
			{#if modelProvider.configured}
				<CircleCheck class="size-4 text-green-500" /> Configured
			{:else}
				<CircleSlash class="size-4 text-red-500" /> Not Configured
			{/if}
		</span>
	</div>

	<div class="mt-auto w-full">
		<button
			onclick={onConfigure}
			class={twMerge(
				'w-full border-0 text-sm',
				modelProvider.configured ? 'button' : 'button-primary '
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
