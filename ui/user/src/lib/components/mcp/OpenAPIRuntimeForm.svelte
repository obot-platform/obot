<script lang="ts">
	import { m } from '$lib/i18n';
	import type { OpenAPIRuntimeConfig } from '$lib/services';
	import {
		importOpenAPI,
		type OpenAPIImportResult,
		type OpenAPIDestinationValidation
	} from '$lib/services/openapi';
	import Toggle from '../Toggle.svelte';
	import { MultiValueInput } from '../ui/multi-value-input';
	import { FileUp } from '@lucide/svelte';
	import { onDestroy, untrack } from 'svelte';

	interface Props {
		config: OpenAPIRuntimeConfig;
		readonly?: boolean;
		importOnly?: boolean;
		imported?: boolean;
		showEgressDomains?: boolean;
		defaultDenyAllEgress?: boolean;
		importedBaseURL?: string;
		destinationValidation?: OpenAPIDestinationValidation;
		overrideValidation?: OpenAPIDestinationValidation;
		showBaseURLError?: boolean;
		onImported?: (result: OpenAPIImportResult) => void;
		onReplace?: () => void;
		onBaseURLChange?: () => void;
	}
	let {
		config = $bindable(),
		readonly = false,
		importOnly = false,
		imported = false,
		showEgressDomains = false,
		defaultDenyAllEgress = false,
		importedBaseURL = '',
		destinationValidation,
		overrideValidation,
		showBaseURLError = false,
		onImported,
		onReplace,
		onBaseURLChange
	}: Props = $props();
	let sourceType = $state(untrack(() => (config.source.content !== undefined ? 'file' : 'url')));
	let busy = $state(false);
	let error = $state('');
	let resolvedBaseURL = $state('');
	let fileName = $state('');
	let fileInput = $state<HTMLInputElement>();
	let disposed = false;
	config.egressDomains ??= [];
	const hasEgressDomains = $derived(config.egressDomains?.some((domain) => domain.trim()) ?? false);
	const explicitAllowAll = $derived(
		defaultDenyAllEgress && config.denyAllEgress === false && !hasEgressDomains
	);
	const explicitDenyAll = $derived(!defaultDenyAllEgress && config.denyAllEgress === true);
	const toggleChecked = $derived(defaultDenyAllEgress ? explicitAllowAll : explicitDenyAll);
	const toggleLabel = $derived(
		defaultDenyAllEgress ? m.mcps_egress_allow_all() : m.mcps_egress_deny_all()
	);
	const egressHelpText = $derived(
		defaultDenyAllEgress ? m.mcps_egress_help_default_deny() : m.mcps_egress_help_default_allow()
	);
	let baseURLRequired = $derived(imported && destinationValidation?.valid === false);
	let baseURLMissing = $derived(baseURLRequired && !config.baseURL?.trim());
	let baseURLInvalid = $derived(
		showBaseURLError && (baseURLMissing || overrideValidation?.valid === false)
	);
	let showBaseURLMessage = $derived(baseURLMissing || overrideValidation?.valid === false);
	onDestroy(() => {
		disposed = true;
	});

	function invalidate() {
		resolvedBaseURL = '';
		error = '';
	}

	function handleEgressToggle(checked: boolean) {
		config.denyAllEgress = checked ? !defaultDenyAllEgress : undefined;
		if (checked) config.egressDomains = [];
	}

	async function upload(event: Event) {
		const file = (event.currentTarget as HTMLInputElement).files?.[0];
		if (!file) return;
		invalidate();
		config.source = {};
		fileName = file.name;
		if (file.size > 1024 * 1024) {
			error = m.mcps_openapi_file_too_large();
			return;
		}
		busy = true;
		try {
			const content = await file.text();
			if (!disposed) config.source = { content };
		} catch {
			error = m.mcps_openapi_file_read_error();
		} finally {
			busy = false;
		}
	}

	async function importSchema() {
		busy = true;
		invalidate();
		try {
			const result = await importOpenAPI(config);
			if (disposed) return;
			resolvedBaseURL = result.baseURL;
			onImported?.(result);
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		} finally {
			busy = false;
		}
	}
</script>

<section class="paper flex flex-col gap-4 p-4" aria-label={m.mcps_openapi_configuration()}>
	<h4 class="text-sm font-semibold">OpenAPI</h4>
	<p class="text-muted-content text-xs">
		{m.mcps_openapi_intro()}
	</p>
	<fieldset disabled={readonly || busy} class="flex flex-col gap-4">
		{#if importOnly}
			<div class="flex flex-col gap-1">
				<label for="openapi-source-type">{m.mcps_openapi_schema_source()}</label>
				<select
					id="openapi-source-type"
					class="select w-full"
					bind:value={sourceType}
					onchange={() => {
						config.source = {};
						fileName = '';
						invalidate();
					}}
				>
					<option value="url">{m.mcps_openapi_source_url_option()}</option>
					<option value="file">{m.mcps_openapi_source_file_option()}</option>
				</select>
			</div>
			{#if sourceType === 'url'}
				<div class="flex flex-col gap-1">
					<label for="openapi-source-url">{m.mcps_openapi_schema_url()}</label>
					<input
						id="openapi-source-url"
						class="text-input-filled"
						type="url"
						placeholder="https://api.example.com/openapi.json"
						bind:value={config.source.url}
						oninput={invalidate}
						onkeydown={(event) => {
							// Enter would otherwise submit the surrounding catalog form.
							if (event.key !== 'Enter') return;
							event.preventDefault();
							if (!busy && config.source.url?.trim()) importSchema();
						}}
					/>
				</div>
			{:else if !readonly}
				<div class="flex flex-col gap-2">
					<p id="openapi-source-file-label" class="text-sm font-light">
						{m.mcps_openapi_schema_file()}
					</p>
					<input
						bind:this={fileInput}
						id="openapi-source-file"
						type="file"
						class="hidden"
						accept=".json,.yaml,.yml"
						aria-labelledby="openapi-source-file-label"
						aria-describedby="openapi-file-hint"
						onchange={upload}
					/>
					<div
						class="border-base-300 bg-base-200 flex flex-wrap items-center gap-3 rounded-lg border p-4"
					>
						<FileUp class="text-muted-content size-5 shrink-0" aria-hidden="true" />
						<div class="min-w-0 flex-1 basis-40">
							<p class="text-sm font-medium break-all">
								{fileName ||
									(config.source.content
										? m.mcps_openapi_saved_schema()
										: m.mcps_openapi_choose_schema())}
							</p>
							<p id="openapi-file-hint" class="text-muted-content mt-1 text-xs">
								{#if !fileName && config.source.content}
									{m.mcps_openapi_existing_schema_help()}
								{:else}
									{m.mcps_openapi_file_hint()}
								{/if}
							</p>
						</div>
						<button
							type="button"
							class="btn btn-secondary btn-sm shrink-0"
							onclick={() => {
								if (!fileInput) return;
								fileInput.value = '';
								fileInput.click();
							}}
						>
							{fileName || config.source.content
								? m.mcps_openapi_replace_file()
								: m.mcps_openapi_choose_file()}
						</button>
					</div>
				</div>
			{/if}
		{:else}
			<div class="flex items-center justify-between gap-4">
				<div class="min-w-0">
					<p class="text-sm font-medium">{m.mcps_openapi_schema_source()}</p>
					<p class="text-muted-content text-xs break-all">
						{config.source.url || m.mcps_openapi_uploaded_schema()}
					</p>
				</div>
				{#if !readonly}<button
						type="button"
						class="btn btn-secondary btn-sm shrink-0"
						onclick={onReplace}>{m.mcps_openapi_replace_schema()}</button
					>{/if}
			</div>
		{/if}
		{#if imported && !importOnly}
			<div class="border-base-300 mt-2 flex flex-col gap-2 border-t pt-4">
				<div class="flex items-center gap-2">
					<label for="openapi-base-url" class="text-sm font-medium">
						{m.mcps_openapi_base_url_label()}
						{#if baseURLRequired}
							<span class:text-error={baseURLInvalid} aria-hidden="true">*</span>
						{/if}
					</label>
					<span class="text-muted-content bg-base-200 rounded px-2 py-0.5 text-xs"
						>{baseURLRequired ? m.mcps_catalog_field_required() : m.mcps_openapi_optional()}</span
					>
				</div>
				<p id="openapi-base-url-hint" class="text-muted-content text-xs">
					{m.mcps_openapi_base_url_help()}
					{#if !baseURLRequired}{m.mcps_openapi_base_url_optional_help()}{/if}
				</p>
				<input
					id="openapi-base-url"
					class="text-input-filled"
					class:error={baseURLInvalid}
					type="url"
					bind:value={config.baseURL}
					oninput={onBaseURLChange}
					required={baseURLRequired}
					aria-invalid={baseURLInvalid ? 'true' : undefined}
					placeholder={baseURLRequired
						? 'https://api.example.com/v1'
						: importedBaseURL || resolvedBaseURL || 'https://api.example.com/v1'}
					aria-describedby={showBaseURLMessage
						? 'openapi-base-url-hint openapi-base-url-error'
						: 'openapi-base-url-hint'}
				/>
				{#if showBaseURLMessage}
					<p
						id="openapi-base-url-error"
						class="text-xs"
						class:text-error={baseURLInvalid}
						class:text-muted-content={!baseURLInvalid}
					>
						{#if overrideValidation?.valid === false}
							{overrideValidation.error || m.mcps_openapi_destination_override_required()}
						{:else if destinationValidation?.valid === false && destinationValidation.error}
							{destinationValidation.error}
							{m.mcps_openapi_destination_override_required()}
						{:else}
							{m.mcps_openapi_base_url_required_error()}
						{/if}
					</p>
				{/if}
			</div>
		{/if}
		{#if importOnly && !readonly}
			<button
				type="button"
				class="btn btn-secondary self-start"
				onclick={importSchema}
				disabled={!config.source.url?.trim() && !config.source.content}
			>
				{busy ? m.mcps_openapi_importing() : m.mcps_openapi_import_schema()}
			</button>
		{/if}
	</fieldset>
	{#if error}<p role="alert" class="text-sm text-error">{error}</p>{/if}
	{#if !importOnly}
		{#if imported}
			<p role="status" class="text-sm">
				{m.mcps_openapi_imported_status()}
			</p>
			{#if resolvedBaseURL}<p class="text-muted-content text-xs">
					{m.mcps_openapi_imported_base_url({ url: resolvedBaseURL })}
				</p>{/if}
		{:else}
			<p class="text-muted-content text-xs">{m.mcps_openapi_import_before_save()}</p>
		{/if}
		{#if showEgressDomains}
			<div class="flex gap-4">
				<span class="pt-2.5 text-sm font-light">{m.mcps_egress_domains()}</span>
				<div class="flex min-h-10 grow flex-col gap-2">
					<Toggle
						label={toggleLabel}
						labelInline
						checked={toggleChecked}
						disabled={readonly}
						onChange={handleEgressToggle}
					/>
					<MultiValueInput
						bind:value={config.egressDomains}
						class="text-input-filled dark:bg-base-100"
						disabled={readonly || toggleChecked}
						readonly={readonly || toggleChecked}
						placeholder={m.mcps_hit_enter_to_insert()}
					/>
					<p class="text-muted-content text-xs">{egressHelpText}</p>
				</div>
			</div>
		{/if}
	{/if}
</section>
