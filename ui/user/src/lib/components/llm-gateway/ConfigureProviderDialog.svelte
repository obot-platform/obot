<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
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
	import { resolve } from '$lib/url';
	import { Pencil, Plus, RefreshCw, Trash2 } from '@lucide/svelte';
	import { onDestroy, tick } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		keyNamesByProvider?: Record<string, string[]>;
		onClosed?: () => void;
	}

	let { keyNamesByProvider = {}, onClosed }: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let name = $state('');
	let provider = $state<Provider>();
	let keys = $state<ProviderKey[]>([]);
	let allKeyNames = $derived([
		...new Set([
			...Object.entries(keyNamesByProvider).flatMap(([p, names]) => (p === name ? [] : names)),
			...keys.map((k) => k.name)
		])
	]);
	let models = $state<string[]>([]);
	let saving = $state(false);
	let refreshing = $state(false);
	let deleting = $state(false);
	let keyToDelete = $state<ProviderKey>();
	let keyDialog = $state<ReturnType<typeof ProviderKeyDialog>>();
	// Bumps whenever a load is superseded, so a closed or replaced provider cannot land late.
	let requestId = 0;
	// Stops the background model reload once the dialog is closed.
	let left = false;
	// Add Key borrows the screen, so this close should not dismiss the provider.
	let pausedForKey = false;
	// Saving a new base URL removes the provider's keys, so they aren't sent to the new
	// address; the admin confirms first.
	let confirmingBaseURLChange = $state(false);

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
	let title = $derived(base ? name : providerLabel(name));

	// A provider serves the Responses API unless its allowed requests leave it out.
	function servesResponses(p: Provider) {
		const allowed = p.custom_provider_config?.allowed_requests;
		return !allowed || !!allowed.responses;
	}

	// Loads the provider, then opens. Callers show their own progress until this resolves.
	export async function open(providerName: string): Promise<boolean> {
		const id = ++requestId;
		left = false;
		name = providerName;
		provider = undefined;
		keys = [];
		models = [];
		confirmingBaseURLChange = false;
		deleting = false;
		keyToDelete = undefined;
		await load(id);
		if (id !== requestId || !provider) return false;
		await tick();
		dialog?.open();
		return true;
	}

	export function close() {
		dialog?.close();
	}

	function addKey() {
		pausedForKey = true;
		dialog?.close();
		keyDialog?.open();
	}

	function resumeAfterKey() {
		if (!pausedForKey) return;
		pausedForKey = false;
		dialog?.open();
	}

	function handleClose() {
		if (pausedForKey) return;
		requestId += 1;
		left = true;
		confirmingBaseURLChange = false;
		deleting = false;
		keyToDelete = undefined;
		onClosed?.();
	}

	async function load(id: number) {
		const providerName = name;
		const next = await getProvider(providerName);
		if (id !== requestId) return;
		provider = next;
		const nc = next.network_config ?? {};
		baseURL = nc.base_url ?? '';
		timeout = nc.default_request_timeout_in_seconds ?? 300;
		retries = nc.max_retries ?? 0;
		backoffInitial = nc.retry_backoff_initial ?? 0;
		backoffMax = nc.retry_backoff_max ?? 0;
		streamIdle = nc.stream_idle_timeout_in_seconds ?? 0;
		allowPrivate = !!nc.allow_private_network;
		headers = Object.entries(nc.extra_headers ?? {}).map(([key, value]) => ({ key, value }));
		concurrency = next.concurrency_and_buffer_size?.concurrency ?? 1000;
		bufferSize = next.concurrency_and_buffer_size?.buffer_size ?? 5000;
		responsesAPI = servesResponses(next);
		await Promise.all([loadKeys(), loadModels(), loadSubscriptionSetting()]);
	}

	async function loadSubscriptionSetting() {
		if (!isAnthropic) return;
		const id = requestId;
		const allowed = (await getSettings()).subscriptionPassthrough;
		if (id !== requestId) return;
		subscriptionPassthrough = allowed;
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
		const id = requestId;
		const providerName = name;
		const nextKeys = await listProviderKeys(providerName);
		if (id !== requestId || providerName !== name) return;
		keys = nextKeys;
	}

	// The model sync runs in the background and reports no completion, so reload the provider's
	// models until they change, for up to 15 seconds or until the dialog is closed. Callers don't
	// wait for it: a sync that finds nothing new changes nothing.
	onDestroy(() => (left = true));
	async function loadModelsAfterSync() {
		const id = requestId;
		const before = models.join('\n');
		for (let i = 0; i < 15 && !left && id === requestId; i++) {
			await new Promise((r) => setTimeout(r, 1000));
			if (left || id !== requestId) return;
			await loadModels().catch(() => {});
			if (models.join('\n') !== before) return;
		}
	}

	// Obot's models for this provider, which profiles grant: what the provider lists, and for an
	// Anthropic provider used only for Claude subscriptions, its models from the model catalog.
	async function loadModels() {
		const id = requestId;
		const providerName = name;
		const prefix = `${providerName}/`;
		const next = (await AdminService.listModels({ all: true }))
			.map((model) => model.targetModel ?? '')
			.filter((target) => target.startsWith(prefix))
			.map((target) => target.slice(prefix.length))
			.sort();
		if (id !== requestId || providerName !== name) return;
		models = next;
	}

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
		let saved = false;
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
			saved = true;
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
			if (saved) dialog?.close();
		}
	}

	async function refresh() {
		refreshing = true;
		try {
			const result = await refreshProviderModels(name);
			if (result?.status !== 'pending') await syncModels();
			success.add(
				result?.status === 'pending'
					? m.models_providers_models_refresh_requested()
					: m.models_providers_models_refreshed()
			);
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

<ResponsiveDialog
	bind:this={dialog}
	{title}
	class="max-w-6xl"
	classes={{
		content: 'p-0',
		header: 'p-4 pb-0'
	}}
	onClose={handleClose}
	disableCloseAction={saving || deleting}
>
	{#if provider}
		<div class="flex max-h-[70vh] flex-col gap-6 overflow-y-auto p-4">
			<div class="flex flex-row items-center gap-3">
				{#if providerIcon(base ?? name)}
					<img
						src={providerIcon(base ?? name)}
						alt=""
						class="size-10 rounded object-contain {providerIconClass(base ?? name)}"
					/>
				{/if}
				<div class="text-muted-content flex min-w-0 flex-1 flex-col text-sm">
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
				{#if !readonly}
					<button class="btn btn-error btn-sm btn-soft" onclick={() => (deleting = true)}>
						{m.models_providers_deconfigure_provider()}
					</button>
				{/if}
			</div>

			<section class="paper-inset">
				<div class="paper dark:bg-base-300 p-0">
					<div class="flex flex-wrap items-center justify-between gap-2 p-4 pb-0">
						<div>
							<h2 class="text-lg font-semibold">{m.models_providers_keys_heading()}</h2>
							<p class="text-muted-content text-xs">
								{m.models_providers_keys_description()}
							</p>
						</div>
						{#if !readonly && !provider.custom_provider_config?.is_key_less}
							<button class="btn btn-primary flex items-center gap-1 btn-sm" onclick={addKey}>
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
								<span
									class={twMerge(
										'badge badge-xs badge-soft',
										row.status === 'success' ? 'badge-success' : 'badge-warning'
									)}>{row.statusLabel}</span
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
				</div>
			</section>

			{#if isAnthropic}
				<section class="flex flex-col gap-4">
					<div>
						<h2 class="text-lg font-semibold">{m.models_providers_subscriptions_heading()}</h2>
						<p class="text-muted-content text-xs">
							{m.models_providers_subscriptions_description_prefix()}
							<code>X-Obot-Api-Key</code>
							{m.models_providers_subscriptions_description_suffix()}
						</p>
						<p class="text-muted-content text-xs mt-2">
							{m.models_providers_subscriptions_forwarded_prefix()}
							<code>https://api.anthropic.com</code>
							{m.models_providers_subscriptions_forwarded_suffix()}
						</p>
					</div>
					<Toggle
						classes={{
							label:
								'rounded-md border border-base-400 flex justify-between p-3 text-base-content text-sm'
						}}
						label={m.models_providers_allow_subscriptions()}
						labelInline
						checked={subscriptionPassthrough}
						disabled={readonly || savingSubscriptions}
						onChange={setSubscriptionPassthrough}
					/>
				</section>
			{/if}

			<section class="flex flex-col gap-4">
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
					<label
						for="p-allow-private"
						class={twMerge(
							'border-base-400 flex flex-col gap-1 rounded-md border p-3 md:col-span-2',
							readonly ? 'cursor-default' : 'cursor-pointer'
						)}
					>
						<Toggle
							label={m.models_providers_allow_private_network()}
							labelInline
							id="p-allow-private"
							checked={allowPrivate}
							disabled={readonly}
							onChange={(v) => (allowPrivate = v)}
							classes={{
								label: 'flex justify-between items-center text-base-content text-sm'
							}}
						/>
						<span class="text-muted-content text-xs pr-12">
							{m.models_providers_allow_private_network_help()}
						</span>
					</label>
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
							<div class="flex justify-end">
								<button
									class="btn btn-secondary flex w-fit items-center gap-1 btn-sm"
									onclick={() => headers.push({ key: '', value: '' })}
								>
									<Plus class="size-3" />
									{m.models_providers_add_header()}
								</button>
							</div>
						{/if}
					</div>
				</div>
			</section>

			<div class="divider my-0"></div>

			<section class="flex flex-col gap-4">
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
							class="btn btn-secondary flex items-center gap-1 btn-sm"
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
				class="bg-base-100 dark:bg-base-300 sticky bottom-0 left-0 z-40 flex w-full justify-end gap-2 p-4"
			>
				<button
					class="btn btn-secondary text-sm"
					disabled={saving || deleting}
					onclick={() => dialog?.close()}>{m.common_cancel()}</button
				>
				<button
					class="btn btn-primary text-sm"
					onclick={save}
					disabled={saving || !!concurrencyError || !!queueSizeError || deleting}
				>
					{#if saving}<Loading class="size-4" />{:else}{m.core_save()}{/if}
				</button>
			</div>
		{/if}
	{/if}
</ResponsiveDialog>

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
	onClosed={resumeAfterKey}
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
		const label = providerLabel(name);
		try {
			await deleteProvider(name);
		} catch {
			// The request reports its own error; stay on the provider.
			deleting = false;
			return;
		}
		deleting = false;
		success.add(m.models_providers_removed({ name: label }));
		dialog?.close();
	}}
	oncancel={() => (deleting = false)}
/>

<style lang="postcss">
	.paper-inset {
		background-color: var(--color-base-300);
		border-radius: var(--radius-lg);
		padding: 0.5rem;
		:global(.dark) & {
			background-color: var(--color-base-200);
		}
	}
</style>
