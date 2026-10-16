<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import ProviderKeyDialog from '$lib/components/llm-gateway/ProviderKeyDialog.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService } from '$lib/services';
	import {
		deleteProvider,
		deleteProviderKey,
		getProvider,
		getSettings,
		listProviderKeys,
		listProviders,
		refreshProviderModels,
		secretDisplay,
		syncModels,
		updateProvider,
		updateProviderKey,
		updateSettings,
		type Provider,
		type ProviderKey
	} from '$lib/services/llm-gateway/gateway';
	import {
		providerIcon,
		providerIconClass,
		providerLabel
	} from '$lib/services/llm-gateway/providers';
	import { profile } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { goto } from '$lib/url';
	import { Pencil, Plus, RefreshCw, Trash2 } from '@lucide/svelte';
	import { onDestroy, onMount } from 'svelte';

	const PROVIDERS_URL = '/models?view=providers';

	let name = $derived(decodeURIComponent(page.params.name ?? ''));
	let provider = $state<Provider>();
	let keys = $state<ProviderKey[]>([]);
	let allKeyNames = $state<string[]>([]);
	let models = $state<string[]>([]);
	let loading = $state(true);
	let saving = $state(false);
	let refreshing = $state(false);
	let deleting = $state(false);
	let keyToDelete = $state<ProviderKey>();
	let keyDialog = $state<ReturnType<typeof ProviderKeyDialog>>();

	// Editable network settings.
	let baseURL = $state('');
	let timeout = $state(300);
	let retries = $state(0);
	let backoffInitial = $state(0);
	let backoffMax = $state(0);
	let streamIdle = $state(0);
	let concurrency = $state(1000);
	let bufferSize = $state(5000);
	let allowPrivate = $state(false);
	let headers = $state<{ key: string; value: string }[]>([]);

	// Whether Claude Code users may use their own Claude subscription through this provider.
	// Only the built-in Anthropic provider can forward a subscription.
	let subscriptionPassthrough = $state(false);
	let savingSubscriptions = $state(false);

	let readonly = $derived(profile.current.isAdminReadonly?.());
	let base = $derived(provider?.custom_provider_config?.base_provider_type);
	let isAnthropic = $derived(name === 'anthropic' && !base);

	async function load() {
		loading = true;
		try {
			provider = await getProvider(name);
			const nc = provider.network_config ?? {};
			baseURL = nc.base_url ?? '';
			timeout = nc.default_request_timeout_in_seconds ?? 300;
			retries = nc.max_retries ?? 0;
			backoffInitial = nc.retry_backoff_initial ?? 0;
			backoffMax = nc.retry_backoff_max ?? 0;
			streamIdle = nc.stream_idle_timeout_in_seconds ?? 0;
			allowPrivate = !!nc.allow_private_network;
			headers = Object.entries(nc.extra_headers ?? {}).map(([key, value]) => ({ key, value }));
			concurrency = provider.concurrency_and_buffer_size?.concurrency ?? 1000;
			bufferSize = provider.concurrency_and_buffer_size?.buffer_size ?? 5000;
			await Promise.all([loadKeys(), loadModels(), loadSubscriptionSetting()]);
		} finally {
			loading = false;
		}
	}

	async function loadSubscriptionSetting() {
		if (!isAnthropic) return;
		subscriptionPassthrough = (await getSettings()).subscriptionPassthrough;
	}

	async function setSubscriptionPassthrough(allowed: boolean) {
		savingSubscriptions = true;
		try {
			await updateSettings({ ...(await getSettings()), subscriptionPassthrough: allowed });
			subscriptionPassthrough = allowed;
			success.add(
				allowed
					? m.models_providers_subscriptions_allowed()
					: m.models_providers_subscriptions_turned_off()
			);
		} finally {
			savingSubscriptions = false;
		}
		// Saving the setting syncs models: an Anthropic provider without keys lists its models
		// only while subscriptions are allowed.
		void loadModelsAfterSync();
	}

	async function loadKeys() {
		keys = await listProviderKeys(name);
		const all = await listProviders();
		const lists = await Promise.all(all.map((p) => listProviderKeys(p.name).catch(() => [])));
		allKeyNames = lists.flat().map((k) => k.name);
	}

	// The model sync runs in the background and reports no completion, so reload the provider's
	// models until they change, for up to 15 seconds or until the page is left. Callers don't
	// wait for it: a sync that finds nothing new changes nothing.
	let left = false;
	onDestroy(() => (left = true));
	async function loadModelsAfterSync() {
		const before = models.join('\n');
		for (let i = 0; i < 15 && !left; i++) {
			await new Promise((r) => setTimeout(r, 1000));
			await loadModels().catch(() => {});
			if (models.join('\n') !== before) return;
		}
	}

	// Obot's models for this provider, which profiles grant: what the provider lists, and for an
	// Anthropic provider used only for Claude subscriptions, its models from the model catalog.
	async function loadModels() {
		const prefix = `${name}/`;
		models = (await AdminService.listModels({ all: true }))
			.map((model) => model.targetModel ?? '')
			.filter((target) => target.startsWith(prefix))
			.map((target) => target.slice(prefix.length))
			.sort();
	}

	async function saveNetwork() {
		if (!provider) return;
		saving = true;
		try {
			const extra = Object.fromEntries(
				headers.filter((h) => h.key.trim()).map((h) => [h.key.trim(), h.value])
			);
			provider = await updateProvider(name, {
				network_config: {
					...provider.network_config,
					base_url: baseURL.trim() || undefined,
					default_request_timeout_in_seconds: Number(timeout),
					max_retries: Number(retries),
					retry_backoff_initial: Number(backoffInitial),
					retry_backoff_max: Number(backoffMax),
					stream_idle_timeout_in_seconds: Number(streamIdle) || undefined,
					allow_private_network: allowPrivate,
					extra_headers: Object.keys(extra).length ? extra : undefined
				},
				concurrency_and_buffer_size: {
					concurrency: Number(concurrency),
					buffer_size: Number(bufferSize)
				}
			});
			success.add(m.models_providers_settings_saved());
		} finally {
			saving = false;
		}
	}

	async function refresh() {
		refreshing = true;
		try {
			await refreshProviderModels(name);
			await syncModels();
			success.add(m.models_providers_models_refreshed());
			void loadModelsAfterSync();
		} finally {
			refreshing = false;
		}
	}

	async function toggleKey(key: ProviderKey, enabled: boolean) {
		if (!key.id) return;
		await updateProviderKey(name, key.id, { ...key, enabled });
		await loadKeys();
	}

	onMount(load);

	let keyRows = $derived(
		keys.map((k) => ({
			...k,
			id: k.id ?? k.name,
			maskedValue: secretDisplay(k.value) || '—',
			modelsLabel: (k.models ?? []).join(', ') || m.models_providers_key_models_none(),
			statusLabel:
				k.status === 'success'
					? m.models_providers_key_status_healthy()
					: (k.status ?? m.models_providers_status_unknown())
		}))
	);
</script>

<svelte:head>
	<title>Obot | {providerLabel(name)}</title>
</svelte:head>

<Layout
	title={base ? name : providerLabel(name)}
	showBackButton
	onBackButtonClick={() => goto(PROVIDERS_URL)}
>
	{#snippet rightNavActions()}
		{#if !readonly && provider}
			<IconButton
				variant="danger2"
				tooltip={{ text: m.models_providers_delete_provider() }}
				onclick={() => (deleting = true)}
			>
				<Trash2 class="size-4" />
			</IconButton>
		{/if}
	{/snippet}

	{#if loading}
		<Skeleton
			type="table"
			count={4}
			classes={{ header: 'h-14 rounded-none', body: 'rounded-none' }}
		/>
	{:else if provider}
		<div class="flex flex-col gap-6">
			<div class="paper flex-row items-center gap-3">
				{#if providerIcon(base ?? name)}
					<img
						src={providerIcon(base ?? name)}
						alt=""
						class="size-10 rounded object-contain {providerIconClass(base ?? name)}"
					/>
				{/if}
				<div class="text-muted-content flex flex-col text-sm">
					<span>
						{m.models_providers_addressed_as_prefix()}
						<code class="font-mono">{name}/&lt;model&gt;</code>
						{#if base}· {m.models_providers_compatible_api({ name: providerLabel(base) })}{/if}
					</span>
					<span
						>{m.models_providers_status_line({
							status: provider.provider_status ?? m.models_providers_status_unknown()
						})}</span
					>
				</div>
			</div>

			<section class="paper">
				<div class="flex flex-wrap items-center justify-between gap-2">
					<div>
						<h2 class="text-lg font-semibold">{m.models_providers_keys_heading()}</h2>
						<p class="text-muted-content text-sm">
							{m.models_providers_keys_description()}
						</p>
					</div>
					{#if !readonly && !provider.custom_provider_config?.is_key_less}
						<button
							class="btn btn-primary flex items-center gap-1 text-sm"
							onclick={() => keyDialog?.open()}
						>
							<Plus class="size-4" />
							{m.models_providers_add_key()}
						</button>
					{/if}
				</div>
				<Table
					data={keyRows}
					fields={['name', 'maskedValue', 'modelsLabel', 'weight', 'statusLabel', 'enabled']}
					headers={[
						{ property: 'name', title: m.core_name() },
						{ property: 'maskedValue', title: m.core_col_value() },
						{ property: 'modelsLabel', title: m.models_title() },
						{ property: 'weight', title: m.models_providers_key_weight_col() },
						{ property: 'statusLabel', title: m.core_status() },
						{ property: 'enabled', title: m.core_status_enabled() }
					]}
					noDataMessage={provider.custom_provider_config?.is_key_less
						? m.models_providers_keyless_no_keys()
						: m.models_providers_no_keys_yet()}
				>
					{#snippet onRenderColumn(property, row)}
						{#if property === 'enabled'}
							<Toggle
								label={m.core_status_enabled()}
								checked={row.enabled !== false}
								disabled={readonly}
								onChange={(v) => toggleKey(row, v)}
							/>
						{:else if property === 'maskedValue'}
							<span class="font-mono text-xs">{row.maskedValue}</span>
						{:else if property === 'statusLabel'}
							<span class={row.status === 'success' ? 'text-success' : 'text-warning'}
								>{row.statusLabel}</span
							>
						{:else}
							{row[property as keyof typeof row]}
						{/if}
					{/snippet}
					{#snippet actions(row)}
						{#if !readonly}
							<IconButton
								tooltip={{ text: m.models_providers_edit_key() }}
								onclick={() => keyDialog?.open(row)}
							>
								<Pencil class="size-4" />
							</IconButton>
							<IconButton
								variant="danger"
								tooltip={{ text: m.models_providers_delete_key() }}
								onclick={() => (keyToDelete = row)}
							>
								<Trash2 class="size-4" />
							</IconButton>
						{/if}
					{/snippet}
				</Table>
			</section>

			{#if isAnthropic}
				<section class="paper">
					<div>
						<h2 class="text-lg font-semibold">{m.models_providers_subscriptions_heading()}</h2>
						<p class="text-muted-content text-sm">
							{m.models_providers_subscriptions_description_prefix()}
							<code>X-Obot-Api-Key</code>
							{m.models_providers_subscriptions_description_suffix()}
						</p>
						<p class="text-muted-content text-sm">
							{m.models_providers_subscriptions_forwarded_prefix()}
							<code>https://api.anthropic.com</code>
							{m.models_providers_subscriptions_forwarded_suffix()}
						</p>
					</div>
					<Toggle
						label={m.models_providers_allow_subscriptions()}
						labelInline
						checked={subscriptionPassthrough}
						disabled={readonly || savingSubscriptions}
						onChange={setSubscriptionPassthrough}
					/>
				</section>
			{/if}

			<section class="paper">
				<div>
					<h2 class="text-lg font-semibold">{m.models_providers_network_heading()}</h2>
					<p class="text-muted-content text-sm">
						{m.models_providers_network_description()}
					</p>
				</div>
				<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
					<div class="flex flex-col gap-1 md:col-span-2">
						<label for="p-base-url" class="text-sm font-light"
							>{m.models_providers_base_url()}</label
						>
						<input
							id="p-base-url"
							class="text-input-filled"
							bind:value={baseURL}
							placeholder={m.models_providers_provider_default()}
							disabled={readonly}
						/>
					</div>
					{@render num(
						'p-timeout',
						m.models_providers_request_timeout(),
						() => timeout,
						(v) => (timeout = v)
					)}
					{@render num(
						'p-retries',
						m.models_providers_max_retries(),
						() => retries,
						(v) => (retries = v),
						m.models_providers_max_retries_help()
					)}
					{@render num(
						'p-backoff-initial',
						m.models_providers_backoff_initial(),
						() => backoffInitial,
						(v) => (backoffInitial = v)
					)}
					{@render num(
						'p-backoff-max',
						m.models_providers_backoff_max(),
						() => backoffMax,
						(v) => (backoffMax = v)
					)}
					{@render num(
						'p-stream-idle',
						m.models_providers_stream_idle_timeout(),
						() => streamIdle,
						(v) => (streamIdle = v),
						m.models_providers_stream_idle_timeout_help()
					)}
					{@render num(
						'p-concurrency',
						m.models_providers_concurrency(),
						() => concurrency,
						(v) => (concurrency = v),
						m.models_providers_concurrency_help()
					)}
					{@render num(
						'p-buffer',
						m.models_providers_queue_size(),
						() => bufferSize,
						(v) => (bufferSize = v),
						m.models_providers_queue_size_help()
					)}
					<div class="flex flex-col gap-1 pt-6">
						<Toggle
							label={m.models_providers_allow_private_network()}
							labelInline
							checked={allowPrivate}
							disabled={readonly}
							onChange={(v) => (allowPrivate = v)}
						/>
						<span class="text-muted-content text-xs">
							{m.models_providers_allow_private_network_help()}
						</span>
					</div>
					<div class="flex flex-col gap-2 md:col-span-2">
						<span class="text-sm font-light">{m.models_providers_extra_headers()}</span>
						{#each headers as h, i (i)}
							<div class="flex gap-2">
								<input
									class="text-input-filled flex-1"
									placeholder={m.models_providers_header_name_placeholder()}
									bind:value={h.key}
									disabled={readonly}
								/>
								<input
									class="text-input-filled flex-1"
									placeholder={m.core_col_value()}
									bind:value={h.value}
									disabled={readonly}
								/>
								{#if !readonly}
									<IconButton
										variant="danger"
										tooltip={{ text: m.core_remove() }}
										onclick={() => headers.splice(i, 1)}
									>
										<Trash2 class="size-4" />
									</IconButton>
								{/if}
							</div>
						{/each}
						{#if !readonly}
							<button
								class="btn btn-secondary flex w-fit items-center gap-1 text-xs"
								onclick={() => headers.push({ key: '', value: '' })}
							>
								<Plus class="size-3" />
								{m.models_providers_add_header()}
							</button>
						{/if}
					</div>
				</div>
			</section>

			<section class="paper">
				<div class="flex flex-wrap items-center justify-between gap-2">
					<div>
						<h2 class="text-lg font-semibold">
							{m.models_providers_models_heading({ count: models.length })}
						</h2>
						<p class="text-muted-content text-sm">
							{m.models_providers_models_appear_prefix()}
							<a href={resolve('/models?view=connect')} class="text-link">{m.models_page_title()}</a
							>
							{m.models_providers_models_appear_suffix()}
						</p>
					</div>
					{#if !readonly}
						<button
							class="btn btn-secondary flex items-center gap-1 text-sm"
							onclick={refresh}
							disabled={refreshing}
						>
							{#if refreshing}<Loading class="size-4" />{:else}<RefreshCw class="size-4" />{/if}
							{m.models_providers_refresh_models()}
						</button>
					{/if}
				</div>
				<div class="flex flex-row flex-wrap gap-1.5">
					{#each models as model (model)}
						<span class="bg-base-200 dark:bg-base-300 rounded px-2 py-0.5 font-mono text-xs"
							>{model}</span
						>
					{:else}
						<span class="text-muted-content text-sm">{m.models_providers_no_models_yet()}</span>
					{/each}
				</div>
			</section>
		</div>

		{#if !readonly}
			<div
				class="bg-base-200 dark:bg-base-100 sticky bottom-0 left-0 z-40 flex w-full justify-end gap-2 py-4"
			>
				<button class="btn btn-secondary text-sm" onclick={() => goto(PROVIDERS_URL)}
					>{m.common_cancel()}</button
				>
				<button class="btn btn-primary text-sm" onclick={saveNetwork} disabled={saving}>
					{#if saving}<Loading class="size-4" />{:else}{m.core_save()}{/if}
				</button>
			</div>
		{/if}
	{/if}
</Layout>

{#snippet num(
	id: string,
	label: string,
	get: () => number,
	set: (v: number) => void,
	help?: string
)}
	<div class="flex flex-col gap-1">
		<label for={id} class="text-sm font-light">{label}</label>
		<input
			{id}
			type="number"
			min="0"
			class="text-input-filled"
			value={get()}
			oninput={(e) => set(Number((e.target as HTMLInputElement).value))}
			disabled={readonly}
		/>
		{#if help}<span class="text-muted-content text-xs">{help}</span>{/if}
	</div>
{/snippet}

<ProviderKeyDialog
	bind:this={keyDialog}
	provider={name}
	baseProvider={base}
	existingKeyNames={allKeyNames}
	onSaved={async () => {
		success.add(m.models_providers_key_saved());
		await loadKeys();
		await syncModels().catch(() => {});
		void loadModelsAfterSync();
	}}
/>

<Confirm
	show={!!keyToDelete}
	msg={m.models_providers_delete_key_confirm({ name: keyToDelete?.name ?? '' })}
	onsuccess={async () => {
		if (keyToDelete?.id) await deleteProviderKey(name, keyToDelete.id);
		keyToDelete = undefined;
		await loadKeys();
	}}
	oncancel={() => (keyToDelete = undefined)}
/>

<Confirm
	show={deleting}
	msg={m.models_providers_delete_provider_confirm({ name: providerLabel(name) })}
	onsuccess={async () => {
		await deleteProvider(name);
		deleting = false;
		goto(PROVIDERS_URL);
	}}
	oncancel={() => (deleting = false)}
/>
