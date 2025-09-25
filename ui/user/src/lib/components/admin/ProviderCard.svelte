<script lang="ts">
	import { CircleSlash, CircleCheck } from 'lucide-svelte';
	import DotDotDot from '../DotDotDot.svelte';
	import { darkMode } from '$lib/stores';
	import { twMerge } from 'tailwind-merge';
	import type { BaseProvider } from '$lib/services/admin/types';
	import type { Snippet } from 'svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';

	interface Props {
		recommended?: boolean;
		provider: BaseProvider;
		onConfigure: () => void;
		onDeconfigure: () => void;
		configuredActions?: Snippet<[BaseProvider]>;
		deprecated?: boolean;
		readonly?: boolean;
	}

	const {
		recommended,
		provider,
		onConfigure,
		onDeconfigure,
		configuredActions,
		deprecated,
		readonly
	}: Props = $props();
</script>

<div
	class="dark:bg-surface1 dark:border-surface3 flex w-full flex-col items-center justify-center gap-4 rounded-lg border border-transparent bg-white p-4 pt-2 shadow-sm"
>
	<div class="flex min-h-9 w-full items-center justify-between">
		<div>
			{#if recommended}
				<span class="rounded-md bg-blue-500 px-2 py-1 text-[11px] font-semibold text-white"
					>Recommended</span
				>
			{/if}
		</div>

		<div class="flex translate-x-2 items-center gap-1">
			{#if provider.configured}
				{#if configuredActions}
					{@render configuredActions(provider)}
				{/if}
				<DotDotDot>
					<div class="default-dialog flex min-w-max flex-col p-2">
						<button
							disabled={readonly}
							class="menu-button text-red-500"
							onclick={() => onDeconfigure()}
						>
							Deconfigure Provider
						</button>
					</div>
				</DotDotDot>
			{/if}
		</div>
	</div>
	{#if darkMode.isDark}
		{@const url = provider.iconDark ?? provider.icon}
		<img
			src={url}
			alt={provider.name}
			class={twMerge('size-16 rounded-md p-1', !provider.iconDark && 'bg-gray-600')}
		/>
	{:else}
		<img src={provider.icon} alt={provider.name} class="size-16 rounded-md p-1" />
	{/if}
	<h4 class="text-center text-lg font-semibold">{provider.name}</h4>
	<div class="border-surface2 rounded-md border px-2 py-1">
		<span class="flex items-center gap-2 text-xs font-light">
			{#if deprecated}
				<div
					class="rounded-md bg-yellow-500 px-2 py-1 text-[10px] font-medium"
					use:tooltip={{
						classes: ['w-fit'],
						text: 'Deprecated – use Amazon Bedrock instead.'
					}}
				>
					Deprecated
				</div>
			{/if}
			{#if provider.configured}
				<CircleCheck class="size-4 text-green-500" /> Configured
			{:else}
				<CircleSlash class="size-4 text-red-500" /> Not Configured
			{/if}
		</span>
	</div>

	<div class="mt-auto w-full">
		<button
			onclick={onConfigure}
			class={twMerge('w-full border-0 text-sm', provider.configured ? 'button' : 'button-primary ')}
		>
			{#if readonly}
				View
			{:else if provider.configured}
				Modify
			{:else}
				Configure
			{/if}
		</button>
	</div>
</div>
