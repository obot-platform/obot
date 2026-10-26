<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import ProviderKeyDialog from '$lib/components/llm-gateway/ProviderKeyDialog.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { parseErrorContent } from '$lib/errors';
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
		CHAT_COMPLETIONS_ONLY_REQUESTS,
		providerIcon,
		providerIconClass,
		providerLabel
	} from '$lib/services/llm-gateway/providers';
	import { errors, profile } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { goto, resolve } from '$lib/url';
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
	// Whether a custom OpenAI-compatible provider serves the Responses API; without it, the
	// gateway serves Responses and Messages requests to it through Chat Completions.
	let responsesAPI = $state(false);

	// The gateway needs at least one worker, and a queue that holds as many requests as the
	// workers, since every request queues before a worker takes it.
	let concurrencyError = $derived(
		Number(concurrency) < 1 ? m.models_providers_concurrency_too_small() : ''
	);
	let queueSizeError = $derived(
		Number(bufferSize) < Number(concurrency) ? m.models_providers_queue_size_too_small() : ''
	);

	// Whether Claude Code users may use their own Claude subscription through this provider.
	// Only the built-in Anthropic provider can forward a subscription.
	let subscriptionPassthrough = $state(false);
	let savingSubscriptions = $state(false);

	let readonly = $derived(profile.current.isAdminReadonly?.());
	let base = $derived(provider?.custom_provider_config?.base_provider_type);
	let isAnthropic = $derived(name === 'anthropic' && !base);

	// A provider serves the Responses API unless its allowed requests leave it out.
	function servesResponses(p: Provider) {
		const allowed = p.custom_provider_config?.allowed_requests;
		return !allowed || !!allowed.responses;
	}

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
			responsesAPI = servesResponses(provider);
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

	// Saving a new base URL removes the provider's keys, so they aren't sent to the new
	// address; the admin confirms first.
	let confirmingBaseURLChange = $state(false);

	function sameBaseURL(a: string, b: string) {
		return a.trim().replace(/\/+$/, '') === b.trim().replace(/\/+$/, '');
	}

	function save() {
		if (!provider || concurrencyError || queueSizeError) return;
		if (keys.length > 0 && !sameBaseURL(baseURL, provider.network_config?.base_url ?? '')) {
			confirmingBaseURLChange = true;
			return;
		}
		void saveNetwork();
	}

	async function saveNetwork(removeKeys = false) {
		if (!provider) return;
		saving = true;
		try {
			const extra = Object.fromEntries(
				headers.filter((h) => h.key.trim()).map((h) => [h.key.trim(), h.value])
			);
			// Only a change is sent, so allowed requests set some other way are kept.
			const custom = provider.custom_provider_config;
			const customChange =
				custom && base === 'openai' && responsesAPI !== servesResponses(provider)
					? {
							custom_provider_config: {
								...custom,
								allowed_requests: responsesAPI ? undefined : CHAT_COMPLETIONS_ONLY_REQUESTS
							}
						}
					: {};
			provider = await updateProvider(
				name,
				{
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
					},
					...customChange
				},
				{ removeKeys, dontLogErrors: true }
			);
			responsesAPI = servesResponses(provider);
			success.add(m.models_providers_settings_saved());
			if (removeKeys) await loadKeys();
		} catch (e) {
			// The gateway calls the queue size the buffer size; use the field's name instead.
			const { message } = parseErrorContent(e);
			errors.append(
				/concurrency must be less than or equal to buffer size/i.test(message)
					? new Error(m.models_providers_queue_size_too_small())
					: e
			);
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
						m.models_providers_concurrency_help(),
						concurrencyError
					)}
					{@render num(
						'p-buffer',
						m.models_providers_queue_size(),
						() => bufferSize,
						(v) => (bufferSize = v),
						m.models_providers_queue_size_help(),
						queueSizeError
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
					{#if base === 'openai'}
						<div class="flex flex-col gap-1 md:col-span-2">
							<label class="flex items-center gap-2 text-sm">
								<input
									type="checkbox"
									class="checkbox checkbox-sm"
									bind:checked={responsesAPI}
									disabled={readonly}
									aria-describedby="p-responses-api-help"
								/>
								{m.models_providers_responses_api()}
							</label>
							<span id="p-responses-api-help" class="text-muted-content text-xs">
								{m.models_providers_responses_api_help()}
							</span>
						</div>
					{/if}
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
				<button
					class="btn btn-primary text-sm"
					onclick={save}
					disabled={saving || !!concurrencyError || !!queueSizeError}
				>
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
	help?: string,
	error?: string
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
			aria-invalid={error ? true : undefined}
			aria-describedby={error ? `${id}-error` : undefined}
		/>
		{#if error}<span id="{id}-error" class="text-error text-xs">{error}</span>{/if}
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
	show={confirmingBaseURLChange}
	msg={m.models_providers_base_url_change_confirm({ count: keys.length })}
	onsuccess={async () => {
		confirmingBaseURLChange = false;
		await saveNetwork(true);
	}}
	oncancel={() => (confirmingBaseURLChange = false)}
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
