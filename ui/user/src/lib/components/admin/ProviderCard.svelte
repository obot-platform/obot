<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import type { BaseProvider } from '$lib/services/admin/types';
	import { darkMode } from '$lib/stores';
	import DotDotDot from '../DotDotDot.svelte';
	import { CircleSlash, CircleCheck, Construction, FlaskConicalIcon } from 'lucide-svelte';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		recommended?: boolean;
		experimental?: boolean;
		provider: BaseProvider;
		onConfigure: () => void;
		onDeconfigure: () => void;
		configuredActions?: Snippet<[BaseProvider]>;
		deprecated?: boolean;
		readonly?: boolean;
		disableConfigure?: boolean;
		isComingSoon?: boolean;
	}

	const {
		recommended,
		experimental,
		provider,
		onConfigure,
		onDeconfigure,
		configuredActions,
		deprecated,
		readonly,
		disableConfigure,
		isComingSoon
	}: Props = $props();
</script>

<div
	class={twMerge(
		'dark:bg-surface1 dark:border-surface3 bg-background flex w-full flex-col items-center justify-center gap-4 rounded-lg border border-transparent p-4 pt-2 shadow-sm',
		isComingSoon && 'opacity-50'
	)}
>
	<div class="flex min-h-9 w-full items-center justify-between">
		<div>
			{#if recommended && !isComingSoon}
				<span class="bg-primary rounded-md px-2 py-1 text-[11px] font-semibold text-white"
					>Recommended</span
				>
			{/if}
			{#if experimental}
				<span
					class="bg-yellow-500/15 text-yellow-500 rounded-md px-2 py-1 text-[10px] font-medium flex items-center gap-1"
				>
					<FlaskConicalIcon class="size-3 text-yellow-500" /> Experimental
				</span>
			{/if}
		</div>

		<div class="flex translate-x-2 items-center gap-1">
			{#if provider.configured && !isComingSoon}
				{#if configuredActions}
					{@render configuredActions(provider)}
				{/if}
				<DotDotDot>
					<button
						disabled={readonly}
						class="menu-button text-red-500"
						onclick={() => onDeconfigure()}
					>
						Deconfigure Provider
					</button>
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
		{#if isComingSoon}
			<div
				class="bg-surface1 dark:bg-surface2/50 text-on-surface1 flex items-center justify-center gap-1 rounded-xs px-4 py-2 text-sm"
			>
				<Construction class="size-4" /> Coming Soon
			</div>
		{:else}
			<button
				onclick={onConfigure}
				class={twMerge(
					'w-full border-0 text-sm',
					provider.configured ? 'button' : 'button-primary'
				)}
				disabled={disableConfigure}
			>
				{#if readonly}
					View
				{:else if provider.configured}
					Modify
				{:else}
					Configure
				{/if}
			</button>
		{/if}
	</div>
</div>
