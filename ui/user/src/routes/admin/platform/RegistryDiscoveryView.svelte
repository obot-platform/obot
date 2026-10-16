<script lang="ts">
	import { m } from '$lib/i18n';
	import { AdminService, type RegistrySettings } from '$lib/services';
	import { profile } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { untrack } from 'svelte';

	let {
		settings,
		saving = false,
		dirty = $bindable(false)
	}: {
		settings?: RegistrySettings;
		saving?: boolean;
		dirty?: boolean;
	} = $props();
	let persistedEnabled = $state(untrack(() => settings?.anonymousAccessEnabled ?? false));
	let enabled = $state(untrack(() => settings?.anonymousAccessEnabled ?? false));
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	$effect(() => {
		const savedEnabled = settings?.anonymousAccessEnabled;
		// Refresh clean state without subscribing to local edits or save results.
		if (savedEnabled !== undefined && untrack(() => enabled === persistedEnabled)) {
			enabled = savedEnabled;
			persistedEnabled = savedEnabled;
		}
	});
	$effect(() => {
		dirty = settings !== undefined && enabled !== persistedEnabled;
	});
	export function reset() {
		enabled = persistedEnabled;
	}
	export async function save() {
		if (!dirty || isAdminReadonly || !settings) return false;
		try {
			const response = await AdminService.updateRegistrySettings({
				anonymousAccessEnabled: enabled
			});
			enabled = response.anonymousAccessEnabled;
			persistedEnabled = response.anonymousAccessEnabled;
			success.add(m.platform_registry_discovery_updated());
			return true;
		} catch (_err) {
			// The shared HTTP client surfaces the error; retain the unsaved change.
			return false;
		}
	}
</script>

<div class="paper flex flex-col gap-4">
	<div
		class="flex flex-col gap-2 text-sm text-muted-content"
		role="note"
		aria-labelledby="registry-discovery-risk"
	>
		<p id="registry-discovery-risk" class="font-medium">
			{m.platform_registry_discovery_risk_title()}
		</p>
		<p>{m.platform_registry_discovery_risk()}</p>
		<p>{m.platform_registry_discovery_permissions()}</p>
		<a
			class="text-link"
			href="https://docs.obot.ai/functionality/mcp-registry-api"
			target="_blank"
			rel="external noopener noreferrer">{m.platform_learn_more()}</a
		>
	</div>
	{#if settings}
		<label for="anonymous-registry-discovery" class="flex items-start justify-between gap-4">
			<span>{m.platform_registry_discovery_enable()}</span>
			<input
				id="anonymous-registry-discovery"
				type="checkbox"
				class="toggle toggle-sm"
				bind:checked={enabled}
				disabled={isAdminReadonly || saving}
			/>
		</label>
	{:else}
		<p role="alert" class="text-error">{m.platform_registry_discovery_unavailable()}</p>
	{/if}
</div>
