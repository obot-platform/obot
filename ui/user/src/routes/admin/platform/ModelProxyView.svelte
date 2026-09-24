<script lang="ts">
	import { invalidate } from '$app/navigation';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
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
	let { settings, usage }: Props = $props();
	let enabled = $state(untrack(() => settings.enabled));
	let saving = $state(false);

	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	let isModelProxyConfigured = $derived(!!settings.url);
	let canSave = $derived(
		!isAdminReadonly && !saving && (isModelProxyConfigured || (settings.enabled && !enabled))
	);

	async function handleSave(event: SubmitEvent) {
		event.preventDefault();
		if (!canSave) return;

		saving = true;
		try {
			const response = await AdminService.updateModelProxySettings({ enabled });
			enabled = response.enabled;
			await invalidate('model-proxy:usage');
			success.add('Model proxy settings updated successfully.');
		} catch (_err) {
			// errors are surfaced via the global HTTP error handling
		} finally {
			saving = false;
		}
	}
</script>

<form
	class="flex h-full w-full flex-col gap-4 @container"
	in:fade={{ duration }}
	onsubmit={handleSave}
>
	{#if !isModelProxyConfigured}
		<div class="notification-alert text-sm">
			<div class="flex grow flex-col gap-1">
				<div class="flex items-center gap-2">
					<TriangleAlert class="size-5 shrink-0 text-warning" />
					<p class="font-semibold">Missing URL Configuration</p>
				</div>
				<span class="font-light break-all">
					The service base URL is missing. This configuration is required to use the model proxy.</span
				>
			</div>
		</div>
	{/if}
	<div class="flex flex-col gap-2 @container">
		<div class="paper gap-5">
			<div class="flex flex-col gap-2">
				<p class="text-sm font-medium">Usage</p>
				<div class="grid grid-cols-1 gap-5 @xl:grid-cols-2">
					{@render usageCard('Input tokens', usage?.input)}
					{@render usageCard('Output tokens', usage?.output)}
				</div>
				{#if usage}
					<p class="font-light text-muted-content text-xs">
						Resets {formatTimeUntil(usage?.resetAt).relativeTime} ({formatTimeUntil(usage?.resetAt)
							.fullDate})
					</p>
				{/if}
			</div>
			<div class="divider my-0"></div>
			<label for="enable-model-proxy" class="flex items-start justify-between gap-4">
				<div class="text-sm">
					<div class="font-medium">Enable Model Proxy</div>
					<p class="mt-0.5 text-xs font-light text-muted-content">
						When no model provider is configured, utilize the model proxy as fallback for chat
						generation in the vMCP Inspector.
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

		{#if !isAdminReadonly}
			<div class="paper flex-row justify-end py-2">
				<button type="submit" class="btn btn-primary" disabled={!canSave}>Save</button>
			</div>
		{/if}
	</div>
</form>

{#snippet usageCard(title: string, usage?: ModelProxyTokenUsage)}
	<div class="flex flex-col gap-2 border border-base-300 dark:border-base-400 rounded-md p-4">
		<p class="text-xs font-medium">{title}</p>

		{#if usage && usage.max > 0}
			{@const remaining = usage.max - usage.used}
			{@const percentageRemaining = (remaining / usage.max) * 100}
			<div in:fade={{ duration }} class="w-full">
				<p class="font-semibold">
					{percentageRemaining < 0 ? 0 : percentageRemaining.toFixed(1)}% remaining
				</p>
				<progress
					class="progress progress-primary"
					value={remaining < 0 ? 0 : remaining}
					max={usage.max}
				></progress>
			</div>
		{:else}
			<p in:fade={{ duration }} class="text-sm text-muted-content">N/A</p>
		{/if}
	</div>
{/snippet}
