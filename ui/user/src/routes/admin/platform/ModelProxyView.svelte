<script lang="ts">
	import { invalidate } from '$app/navigation';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { m } from '$lib/i18n';
	import {
		AdminService,
		type ModelProxySettings,
		type ModelProxyTokenUsage,
		type ModelProxyUsage
	} from '$lib/services';
	import { profile } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { formatTimeUntil } from '$lib/time';
	import { TriangleAlert } from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { fade } from 'svelte/transition';

	type Props = {
		settings: ModelProxySettings;
		usage?: ModelProxyUsage;
	};

	const duration = PAGE_TRANSITION_DURATION;
	let {
		settings,
		usage,
		saving = false,
		dirty = $bindable(false)
	}: Props & { saving?: boolean; dirty?: boolean } = $props();
	let persistedEnabled = $state(untrack(() => settings.enabled));
	let enabled = $state(untrack(() => settings.enabled));

	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	let isModelProxyConfigured = $derived(!!settings.url);
	let isDirty = $derived(enabled !== persistedEnabled);
	let canSave = $derived(isDirty && (isModelProxyConfigured || (settings.enabled && !enabled)));

	$effect(() => {
		if (dirty !== isDirty) dirty = isDirty;
	});

	export function reset() {
		enabled = persistedEnabled;
	}

	export async function save() {
		if (!canSave || isAdminReadonly) return false;

		try {
			const response = await AdminService.updateModelProxySettings({ enabled });
			enabled = response.enabled;
			persistedEnabled = response.enabled;
			await invalidate('model-proxy:usage');
			success.add(m.platform_settings_model_proxy_updated());
			return true;
		} catch (_err) {
			// errors are surfaced via the global HTTP error handling
			return false;
		}
	}
</script>

<div class="flex w-full flex-col gap-2 @container">
	{#if !isModelProxyConfigured}
		<div class="notification-alert text-sm">
			<div class="flex grow flex-col gap-1">
				<div class="flex items-center gap-2">
					<TriangleAlert class="size-5 shrink-0 text-warning" />
					<p class="font-semibold">{m.platform_settings_model_proxy_missing_url()}</p>
				</div>
				<span class="font-light break-all">
					{m.platform_settings_model_proxy_missing_url_description()}</span
				>
			</div>
		</div>
	{/if}
	<div class="flex flex-col gap-2 @container">
		<div class="paper gap-5">
			<div class="flex flex-col gap-2">
				<p class="text-sm font-medium">{m.platform_settings_model_proxy_usage()}</p>
				<div class="grid grid-cols-1 gap-5 @xl:grid-cols-2">
					{@render usageCard(m.platform_settings_model_proxy_input_tokens(), usage?.input)}
					{@render usageCard(m.platform_settings_model_proxy_output_tokens(), usage?.output)}
				</div>
				{#if usage}
					<p class="font-light text-muted-content text-xs">
						{m.platform_settings_model_proxy_resets({
							relative: formatTimeUntil(usage?.resetAt).relativeTime,
							date: formatTimeUntil(usage?.resetAt).fullDate
						})}
					</p>
				{/if}
			</div>
			<div class="divider my-0"></div>
			<label for="enable-model-proxy" class="flex items-start justify-between gap-4">
				<div class="text-sm">
					<div class="font-medium">{m.platform_settings_model_proxy_enable()}</div>
					<p class="mt-0.5 text-xs font-light text-muted-content">
						{m.platform_settings_model_proxy_enable_description()}
					</p>
				</div>
				<input
					id="enable-model-proxy"
					type="checkbox"
					class="toggle toggle-sm"
					bind:checked={enabled}
					disabled={isAdminReadonly || saving || (!isModelProxyConfigured && !enabled)}
				/>
			</label>
		</div>
	</div>
</div>

{#snippet usageCard(title: string, usage?: ModelProxyTokenUsage)}
	<div class="flex flex-col gap-2 border border-base-300 dark:border-base-400 rounded-md p-4">
		<p class="text-xs font-medium">{title}</p>

		{#if usage && usage.max > 0}
			{@const remaining = usage.max - usage.used}
			{@const percentageRemaining = (remaining / usage.max) * 100}
			<div in:fade={{ duration }} class="w-full">
				<p class="font-semibold">
					{m.platform_settings_model_proxy_remaining({
						percent: percentageRemaining < 0 ? 0 : percentageRemaining.toFixed(1)
					})}
				</p>
				<progress
					class="progress progress-primary"
					value={remaining < 0 ? 0 : remaining}
					max={usage.max}
				></progress>
			</div>
		{:else}
			<p in:fade={{ duration }} class="text-sm text-muted-content">{m.platform_na()}</p>
		{/if}
	</div>
{/snippet}
