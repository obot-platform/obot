<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Select from '$lib/components/Select.svelte';
	import ApiKeyRevealNotice from '$lib/components/agent-auth-scope/ApiKeyRevealNotice.svelte';
	import { gatewayModelProvider } from '$lib/components/llm-gateway/GatewayModelsDialog.svelte';
	import GenerateApiKeyButton, {
		isUsableLlmApiKey
	} from '$lib/components/llm-gateway/GenerateApiKeyButton.svelte';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService, ApiKeysService, type APIKey, type Model } from '$lib/services';
	import { ModelUsage } from '$lib/services/admin/types';
	import { testProvider, type ProviderTestEvent } from '$lib/services/llm-gateway/gateway';
	import { providerLabel } from '$lib/services/llm-gateway/providers';
	import errors from '$lib/stores/errors.svelte';
	import { Send, Square } from '@lucide/svelte';
	import { tick } from 'svelte';
	import { slide } from 'svelte/transition';

	const MAX_MESSAGE_BYTES = 4096;
	const encoder = new TextEncoder();

	interface Run {
		id: number;
		message: string;
		state: 'streaming' | 'done' | 'failed' | 'stopped';
		reply: string;
		status?: number;
		error?: string;
		durationMs?: number;
		servedBy?: string;
	}

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let provider = $state('');
	let models = $state<Model[]>([]);
	let loadingModels = $state(false);
	let selectedModel = $state('');
	let apiKeyId = $state<number | undefined>();
	let existingApiKeys = $state<APIKey[]>([]);
	let keysReady = $state(false);
	let generatingKey = $state(false);
	let generatedKey = $state<string | undefined>();
	let keyController: AbortController | undefined;
	let keysRequest = 0;
	let message = $state(m.models_provider_test_default_message());
	let runs = $state<Run[]>([]);
	let selectedRunID = $state<number | undefined>();
	let controller: AbortController | undefined;
	let nextRunID = 0;
	let transcript = $state<HTMLElement>();
	let runTabs = $state<HTMLElement>();

	let running = $derived(runs.some((run) => run.state === 'streaming'));
	let selectedRun = $derived(runs.find((run) => run.id === selectedRunID) ?? runs.at(-1));
	let modelOptions = $derived(
		models
			.filter(
				(model) =>
					model.active &&
					model.usage === ModelUsage.LLM &&
					gatewayModelProvider(model.targetModel ?? '') === provider
			)
			.map((model) => ({
				id: model.id,
				label: model.targetModel ?? model.name,
				searchText: `${model.targetModel ?? ''} ${model.displayName ?? ''} ${model.name}`
			}))
			.sort((a, b) => a.label.localeCompare(b.label))
	);
	let apiKeyOptions = $derived(
		existingApiKeys.filter(isUsableLlmApiKey).map((key) => ({
			id: key.id,
			label: `${key.name} (ok1-${key.userId}-${key.id}-*****)`,
			searchText: `${key.name} ${key.description ?? ''} ${key.id}`
		}))
	);
	let messageTooLong = $derived(encoder.encode(message.trim()).length > MAX_MESSAGE_BYTES);
	let canSend = $derived(
		!running &&
			!!selectedModel &&
			apiKeyId !== undefined &&
			message.trim() !== '' &&
			!messageTooLong
	);

	export async function open(providerName: string) {
		reset();
		provider = providerName;
		dialog?.open();
		void loadExistingApiKeys();
		loadingModels = true;
		try {
			models = await AdminService.listModels({ all: true });
		} catch {
			// The HTTP client records the error.
			models = [];
		} finally {
			loadingModels = false;
		}
		if (!selectedModel && modelOptions.length > 0) selectedModel = String(modelOptions[0].id);
	}

	function reset() {
		controller?.abort();
		controller = undefined;
		keyController?.abort();
		keyController = undefined;
		generatingKey = false;
		generatedKey = undefined;
		models = [];
		selectedModel = '';
		apiKeyId = undefined;
		existingApiKeys = [];
		keysReady = false;
		keysRequest++;
		message = m.models_provider_test_default_message();
		runs = [];
		selectedRunID = undefined;
	}

	async function loadExistingApiKeys() {
		const request = keysRequest;
		try {
			const keys = await ApiKeysService.listApiKeys();
			if (request !== keysRequest) return;
			existingApiKeys = keys;
			const usable = keys.filter(isUsableLlmApiKey);
			if (apiKeyId === undefined && usable.length === 1) apiKeyId = usable[0].id;
		} catch {
			if (request !== keysRequest) return;
			existingApiKeys = [];
		} finally {
			if (request === keysRequest) keysReady = true;
		}
	}

	async function generateKey() {
		if (generatingKey) return;

		const abort = new AbortController();
		keyController = abort;
		generatingKey = true;
		try {
			const response = await ApiKeysService.createApiKey(
				{
					name: m.models_provider_test_key_name(),
					mcpServerIds: [],
					canAccessLLMProxy: true
				},
				{
					signal: abort.signal,
					errorHandler: (error) => {
						if (!abort.signal.aborted) errors.append(error);
					}
				}
			);
			if (abort.signal.aborted) return;
			generatedKey = response.key;
			const created: APIKey = { ...response };
			delete (created as Partial<typeof response>).key;
			existingApiKeys = [created, ...existingApiKeys];
			apiKeyId = created.id;
		} catch {
			// The error handler records failures; aborts are expected on close.
		} finally {
			if (keyController === abort) {
				keyController = undefined;
				generatingKey = false;
			}
		}
	}

	function apply(run: Run, event: ProviderTestEvent) {
		switch (event.type) {
			case 'text_delta':
				run.reply += event.delta;
				break;
			case 'reply':
				run.reply = event.reply;
				run.state = 'done';
				run.durationMs = event.durationMs;
				run.servedBy = servedBy(event.provider, event.servedModel);
				break;
			case 'error':
				run.state = 'failed';
				run.status = event.status;
				run.error = event.error;
				run.durationMs = event.durationMs;
				run.servedBy = servedBy(event.provider, event.servedModel);
				break;
		}
	}

	function servedBy(servedProvider?: string, servedModel?: string) {
		if (!servedProvider && !servedModel) return undefined;
		return [servedProvider, servedModel].filter(Boolean).join('/');
	}

	function formatDuration(ms: number) {
		if (ms < 1000) return m.models_provider_test_duration_ms({ duration: Math.round(ms) });
		const seconds = Math.round(ms / 1000);
		if (seconds < 60) return m.models_provider_test_duration_s({ duration: seconds });
		return m.models_provider_test_duration_m({ duration: Math.max(1, Math.round(ms / 60_000)) });
	}

	async function send() {
		if (!canSend) return;

		const text = message.trim();
		const id = nextRunID++;
		runs.push({ id, message: text, state: 'streaming', reply: '' });
		selectedRunID = id;
		const run = runs[runs.length - 1];

		const abort = new AbortController();
		controller = abort;
		await scrollToEnd();
		try {
			await testProvider(
				provider,
				{ model: selectedModel, apiKeyId: apiKeyId!, message: text },
				(event) => {
					apply(run, event);
					if (selectedRunID === run.id) scrollToEnd();
				},
				{ signal: abort.signal }
			);
			if (run.state === 'streaming') run.state = 'stopped';
		} catch (error) {
			if (abort.signal.aborted) {
				run.state = 'stopped';
			} else {
				run.state = 'failed';
				run.error = error instanceof Error ? error.message : String(error);
			}
		} finally {
			if (controller === abort) controller = undefined;
		}
	}

	function stop() {
		controller?.abort();
	}

	async function selectRun(id: number) {
		selectedRunID = id;
		await tick();
		const run = runs.find((item) => item.id === id);
		transcript?.scrollTo({ top: run?.state === 'streaming' ? transcript.scrollHeight : 0 });
		revealSelectedTab();
	}

	function revealSelectedTab() {
		runTabs
			?.querySelector<HTMLElement>('[aria-selected="true"]')
			?.scrollIntoView({ block: 'nearest', inline: 'nearest' });
	}

	async function scrollToEnd() {
		await tick();
		transcript?.scrollTo({ top: transcript.scrollHeight });
		revealSelectedTab();
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	title={m.models_provider_test_title({ provider: provider ? providerLabel(provider) : '' })}
	class="max-w-2xl"
	onClose={reset}
>
	<div class="flex flex-col gap-4 text-sm p-4 md:p-0 @container">
		<p class="text-sm font-light">{m.models_provider_test_description()}</p>

		<div class="flex flex-col gap-1">
			<span id="provider-test-model-label" class="text-xs font-medium"
				>{m.models_provider_test_model_label()}</span
			>
			{#if loadingModels}
				<Loading class="size-4" />
			{:else if modelOptions.length === 0}
				<p class="text-muted-content text-xs">{m.models_provider_test_no_models()}</p>
			{:else}
				<Select
					id="provider-test-model"
					options={modelOptions}
					selected={selectedModel}
					searchInDropdown
					placeholder={m.models_provider_test_model_placeholder()}
					searchPlaceholder={m.models_provider_test_search_models()}
					ariaLabelledby="provider-test-model-label"
					class="bg-base-200 dark:bg-base-100 min-h-9 py-1.5 text-sm shadow-none"
					onSelect={(option) => (selectedModel = String(option.id))}
				/>
			{/if}
		</div>

		<div class="divider my-0"></div>

		<div class="flex items-start justify-between gap-4 flex-col @lg:flex-row">
			<div>
				<h4 id="provider-test-api-key-label" class="text-sm font-semibold">
					{m.models_gateway_api_key_title()}
				</h4>
				<p class="text-xs font-light">
					{#if keysReady && apiKeyOptions.length === 0}
						{m.models_provider_test_api_key_none()}
					{:else if apiKeyOptions.length > 0}
						{m.models_provider_test_api_key_help()}
					{/if}
				</p>
			</div>
			{#if keysReady}
				<GenerateApiKeyButton
					apiKeys={existingApiKeys}
					generating={generatingKey}
					onGenerate={generateKey}
					disablePortal
				/>
			{/if}
		</div>

		{#if generatedKey}
			<div in:slide={{ axis: 'y' }}>
				<ApiKeyRevealNotice
					keyValue={generatedKey}
					id="provider-test-generated-key"
					class="gap-3"
				/>
			</div>
		{/if}

		{#if apiKeyOptions.length > 0}
			<div class="w-full">
				<Select
					id="provider-test-api-key"
					options={apiKeyOptions}
					selected={apiKeyId}
					searchInDropdown
					placeholder={m.models_provider_test_api_key_placeholder()}
					ariaLabelledby="provider-test-api-key-label"
					classes={{
						root: 'w-full'
					}}
					class="bg-base-200 dark:bg-base-100 min-h-9 py-1.5 text-sm shadow-none"
					onSelect={(option) => (apiKeyId = Number(option.id))}
				/>
			</div>
		{/if}

		<div class="divider my-0"></div>

		<div class="flex flex-col bg-base-100 dark:bg-base-200 rounded-md border border-base-400">
			<div
				bind:this={runTabs}
				role="tablist"
				aria-label={m.models_provider_test_runs()}
				class="tabs tabs-box tabs-xs flex w-full overflow-x-auto bg-base-200 dark:bg-base-400 rounded-b-none rounded-t-sm"
			>
				{#if selectedRun}
					{#each runs as run, index (run.id)}
						<button
							type="button"
							role="tab"
							class="tab shrink-0"
							class:tab-active={selectedRun.id === run.id}
							aria-selected={selectedRun.id === run.id}
							aria-controls="provider-test-run"
							title={run.message}
							onclick={() => selectRun(run.id)}
						>
							{m.models_provider_test_run({ n: index + 1 })}
						</button>
					{/each}
				{:else}
					<div class="tab shrink-0 tab-disabled">
						{m.models_provider_test_run({ n: 1 })}
					</div>
				{/if}
			</div>
			<div
				id="provider-test-run"
				role="tabpanel"
				bind:this={transcript}
				class="flex h-48 min-h-0 max-h-48 flex-col gap-3 overflow-y-auto p-3"
				aria-live="polite"
			>
				{#if selectedRun}
					<div class="flex justify-end">
						<div class="ml-12 w-fit max-w-full bg-base-200 dark dark:bg-base-300 rounded-md p-3">
							<p class="whitespace-pre-wrap">{selectedRun.message}</p>
						</div>
					</div>
					<div class="flex flex-col gap-1">
						{#if selectedRun.state === 'failed'}
							<div class="notification-error flex flex-col gap-1">
								<span class="text-xs font-medium">
									{selectedRun.status
										? m.models_provider_test_failed_status({ status: selectedRun.status })
										: m.models_provider_test_failed()}
								</span>
								{#if selectedRun.error}
									<pre
										class="max-h-48 overflow-auto font-mono text-xs wrap-break-word whitespace-pre-wrap">{selectedRun.error}</pre>
								{/if}
							</div>
						{:else}
							{#if selectedRun.reply}
								<p class="whitespace-pre-wrap">{selectedRun.reply}</p>
							{:else if selectedRun.state === 'streaming'}
								<p class="text-muted-content flex items-center gap-2 text-xs">
									<Loading class="size-3" />
									{m.models_provider_test_waiting()}
								</p>
							{/if}
							{#if selectedRun.state === 'stopped'}
								<p class="text-muted-content text-xs">{m.models_provider_test_stopped()}</p>
							{/if}
						{/if}
						{#if selectedRun.durationMs !== undefined || selectedRun.servedBy}
							<p class="text-muted-content flex gap-3 text-xs">
								{#if selectedRun.durationMs !== undefined}
									<span>{formatDuration(selectedRun.durationMs)}</span>
								{/if}
								{#if selectedRun.servedBy}
									<span>{m.models_provider_test_served_by({ target: selectedRun.servedBy })}</span>
								{/if}
							</p>
						{/if}
					</div>
				{/if}
			</div>
			<form
				class="flex items-end gap-2 p-3 mt-3 border-t border-base-400"
				onsubmit={(event) => {
					event.preventDefault();
					send();
				}}
			>
				<div class="flex flex-1 flex-col gap-1">
					<label class="flex flex-col gap-1">
						<span class="text-xs font-medium sr-only">{m.models_provider_test_message_label()}</span
						>
						<input
							class="text-input-filled"
							bind:value={message}
							maxlength={MAX_MESSAGE_BYTES}
							aria-invalid={messageTooLong}
							aria-describedby={messageTooLong ? 'provider-test-message-too-long' : undefined}
						/>
					</label>
					{#if messageTooLong}
						<span id="provider-test-message-too-long" class="text-error text-xs"
							>{m.models_provider_test_message_too_long()}</span
						>
					{/if}
				</div>
				{#if running}
					<button
						type="button"
						class="btn btn-secondary btn-circle"
						onclick={stop}
						aria-label={m.models_provider_test_stop()}
					>
						<Square class="size-4" />
					</button>
				{:else}
					<button
						type="submit"
						class="btn btn-primary btn-circle"
						disabled={!canSend}
						aria-label={m.models_provider_test_send()}
					>
						<Send class="size-4" />
					</button>
				{/if}
			</form>
		</div>
	</div>
</ResponsiveDialog>
