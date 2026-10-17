<script lang="ts">
	import Skeleton from '$lib/components/Skeleton.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import GatewayStatusBanner from '$lib/components/llm-gateway/GatewayStatusBanner.svelte';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { doGet, doPut } from '$lib/services/http';
	import {
		getStatus,
		type ClientConfig,
		type GatewayStatus
	} from '$lib/services/llm-gateway/gateway';
	import { profile } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { onMount } from 'svelte';

	type FullConfig = {
		client_config: ClientConfig;
		framework_config: Record<string, unknown>;
		is_logs_connected: boolean;
	};

	let status = $state<GatewayStatus>();
	let config = $state<FullConfig>();
	let loading = $state(true);
	let saving = $state(false);

	let readonly = $derived(profile.current.isAdminReadonly?.());

	async function load() {
		loading = true;
		try {
			status = await getStatus();
			if (!status.ready) return;
			config = (await doGet('/llm-gateway/admin/config')) as FullConfig;
		} finally {
			loading = false;
		}
	}

	onMount(load);

	async function save() {
		if (!config) return;
		saving = true;
		try {
			// The framework settings are Obot's, so only the client config is sent back.
			await doPut('/llm-gateway/admin/config', { client_config: config.client_config });
			success.add(m.models_settings_saved());
			await load();
		} finally {
			saving = false;
		}
	}
</script>

<div class="flex flex-col gap-6">
	<GatewayStatusBanner {status} showNoProviders={false} />

	{#if loading}
		<Skeleton type="items" count={4} class="h-32" />
	{:else if config}
		<section class="paper">
			<div>
				<h2 class="text-lg font-semibold">{m.models_settings_traffic()}</h2>
				<p class="text-muted-content text-sm">
					{m.models_settings_traffic_description()}
				</p>
			</div>
			<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
				<div class="flex flex-col gap-1">
					<label for="s-body" class="text-sm font-light"
						>{m.models_settings_max_request_body()}</label
					>
					<input
						id="s-body"
						type="number"
						min="1"
						class="text-input-filled"
						bind:value={config.client_config.max_request_body_size_mb}
						disabled={readonly}
					/>
				</div>
				<div class="flex flex-col gap-1 md:col-span-2">
					<Toggle
						label={m.models_settings_drop_excess()}
						labelInline
						checked={config.client_config.drop_excess_requests}
						disabled={readonly}
						onChange={(v) => config && (config.client_config.drop_excess_requests = v)}
					/>
					<span class="text-muted-content text-xs">
						{m.models_settings_drop_excess_help()}
					</span>
				</div>
			</div>
		</section>

		{#if !readonly}
			<div
				class="bg-base-200 dark:bg-base-100 sticky bottom-0 left-0 z-40 flex w-full justify-end gap-2 py-4"
			>
				<button class="btn btn-secondary text-sm" disabled={saving} onclick={load}
					>{m.common_cancel()}</button
				>
				<button class="btn btn-primary text-sm" disabled={saving} onclick={save}>
					{#if saving}<Loading class="size-4" />{:else}{m.core_save()}{/if}
				</button>
			</div>
		{/if}
	{/if}
</div>
