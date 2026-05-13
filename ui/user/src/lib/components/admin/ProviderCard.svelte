<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import type { BaseProvider } from '$lib/services/admin/types';
	import { darkMode } from '$lib/stores';
	import DotDotDot from '../DotDotDot.svelte';
	import {
		CircleSlash,
		CircleCheck,
		Construction,
		FlaskConicalIcon,
		TriangleAlert,
		CircleAlert
	} from 'lucide-svelte';
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
		licenseKey?: string;
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
		isComingSoon,
		licenseKey
	}: Props = $props();

	const isLicenseRequired = $derived(
		provider.missingEntitlements && provider.missingEntitlements.length > 0
	);
</script>

<div
	class={twMerge(
		'dark:bg-base-200 dark:border-base-400 bg-base-100 flex w-full flex-col items-center justify-center gap-4 rounded-lg border border-transparent p-4 pt-2 shadow-sm',
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
					class="bg-warning/15 text-warning rounded-md px-2 py-1 text-[10px] font-medium flex items-center gap-1"
				>
					<FlaskConicalIcon class="size-3 text-warning" /> Experimental
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
						class="menu-button text-error"
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
			class={twMerge('size-16 rounded-md p-1', !provider.iconDark && 'bg-base-400')}
		/>
	{:else}
		<img src={provider.icon} alt={provider.name} class="size-16 rounded-md p-1" />
	{/if}
	<h4 class="text-center text-lg font-semibold">{provider.name}</h4>
	<div
		class={twMerge(
			'border-base-400 rounded-md border px-2 py-1',
			isLicenseRequired &&
				!provider.configured &&
				'border-transparent bg-base-200 dark:bg-base-300 text-muted-content',
			isLicenseRequired && provider.configured && 'border-transparent bg-warning/10 text-warning'
		)}
	>
		<span class="flex items-center gap-1.5 text-xs font-light">
			{#if deprecated}
				<div
					class="rounded-md bg-warning px-2 py-1 text-[10px] font-medium"
					use:tooltip={{
						classes: ['w-fit'],
						text: 'Deprecated – use Amazon Bedrock instead.'
					}}
				>
					Deprecated
				</div>
			{/if}
			{#if isLicenseRequired}
				{#if provider.configured}
					<TriangleAlert class="size-4 text-warning" /> License {licenseKey ? 'Invalid' : 'Missing'}
				{:else}
					<CircleAlert class="size-4 text-muted-content" /> License Required
				{/if}
			{:else if provider.configured}
				<CircleCheck class="size-4 text-success" /> Configured
			{:else}
				<CircleSlash class="size-4 text-error" /> Not Configured
			{/if}
		</span>
	</div>

	<div class="mt-auto w-full">
		{#if isComingSoon}
			<div
				class="bg-base-200 dark:bg-base-400 text-muted-content flex items-center justify-center gap-1 rounded-xs px-4 py-2 text-sm"
			>
				<Construction class="size-4" /> Coming Soon
			</div>
		{:else}
			<button
				onclick={onConfigure}
				class={twMerge(
					'w-full border-0 text-sm btn',
					provider.configured ? 'btn-secondary' : 'btn-primary'
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
