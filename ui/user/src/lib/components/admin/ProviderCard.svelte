<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { m } from '$lib/i18n';
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
	} from '@lucide/svelte';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		recommended?: boolean;
		experimental?: boolean;
		provider: BaseProvider;
		onConfigure: () => void;
		// Omitted when the provider cannot be deconfigured, so the menu is not offered at all.
		onDeconfigure?: () => void;
		configuredActions?: Snippet<[BaseProvider]>;
		deprecated?: boolean;
		readonly?: boolean;
		disableConfigure?: boolean;
		// Shown as a tooltip when disableConfigure hides the reason the button cannot be used.
		disableConfigureReason?: string;
		isComingSoon?: boolean;
		licenseKey?: string;
		// Settings saved as a replacement while another provider still serves logins.
		staged?: boolean;
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
		disableConfigureReason,
		isComingSoon,
		licenseKey,
		staged
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
					>{m.models_providers_recommended()}</span
				>
			{/if}
			{#if experimental}
				<span
					class="bg-warning/15 text-warning rounded-md px-2 py-1 text-[10px] font-medium flex items-center gap-1"
				>
					<FlaskConicalIcon class="size-3 text-warning" />
					{m.core_experimental()}
				</span>
			{/if}
		</div>

		<div class="flex translate-x-2 items-center gap-1">
			{#if provider.configured && !isComingSoon}
				{#if configuredActions}
					{@render configuredActions(provider)}
				{/if}
				{#if onDeconfigure}
					<DotDotDot>
						<button
							disabled={readonly}
							class="menu-button text-error"
							onclick={() => onDeconfigure()}
						>
							{m.models_providers_deconfigure_provider()}
						</button>
					</DotDotDot>
				{/if}
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
			isLicenseRequired && provider.configured && 'border-transparent bg-warning/10 text-warning',
			staged && !provider.configured && 'border-warning/40 bg-warning/10 text-warning'
		)}
	>
		<span class="flex items-center gap-1.5 text-xs font-light">
			{#if deprecated}
				<div
					class="rounded-md bg-warning px-2 py-1 text-[10px] font-medium"
					use:tooltip={{
						classes: ['w-fit'],
						text: m.models_providers_deprecated_use_bedrock()
					}}
				>
					{m.common_deprecated()}
				</div>
			{/if}
			{#if isLicenseRequired}
				{#if provider.configured}
					<TriangleAlert class="size-4 text-warning" />
					{licenseKey ? m.models_providers_license_invalid() : m.models_providers_license_missing()}
				{:else}
					<CircleAlert class="size-4 text-muted-content" />
					{m.models_providers_registration_required()}
				{/if}
			{:else if provider.configured}
				<CircleCheck class="size-4 text-success" />
				{m.core_status_configured()}
			{:else if staged}
				<TriangleAlert class="size-4 text-warning" />
				{m.models_providers_staged()}
			{:else}
				<CircleSlash class="size-4 text-error" />
				{m.core_mcp_value_not_configured()}
			{/if}
		</span>
	</div>

	<div class="mt-auto w-full">
		{#if isComingSoon}
			<div
				class="bg-base-200 dark:bg-base-400 text-muted-content flex items-center justify-center gap-1 rounded-xs px-4 py-2 text-sm"
			>
				<Construction class="size-4" />
				{m.models_providers_coming_soon()}
			</div>
		{:else}
			<div
				class="w-full"
				use:tooltip={disableConfigure && disableConfigureReason
					? { classes: ['w-fit'], text: disableConfigureReason }
					: undefined}
			>
				<button
					onclick={onConfigure}
					class={twMerge(
						'w-full border-0 text-sm btn',
						provider.configured || staged ? 'btn-secondary' : 'btn-primary'
					)}
					disabled={disableConfigure}
				>
					{#if readonly}
						{m.models_providers_view()}
					{:else if provider.configured}
						{m.models_providers_modify()}
					{:else if staged}
						{m.models_providers_resume_switch()}
					{:else}
						{m.models_providers_configure()}
					{/if}
				</button>
			</div>
		{/if}
	</div>
</div>
