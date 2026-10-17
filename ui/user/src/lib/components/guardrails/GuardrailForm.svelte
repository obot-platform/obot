<script lang="ts">
	import Select from '$lib/components/Select.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		guardrailFieldDescription,
		guardrailFieldLabel,
		guardrailFindingMessage,
		guardrailTypeDescription,
		guardrailTypeLabel
	} from '$lib/services/llm-gateway/guardrail-text';
	import {
		createGuardrail,
		getGuardrail,
		listGuardrailProviders,
		loadGuardrailFieldOptions,
		testGuardrail,
		updateGuardrail,
		type GuardrailField,
		type GuardrailProvider,
		type GuardrailStage,
		type GuardrailTarget,
		type Guardrail,
		type GuardrailTestResult
	} from '$lib/services/llm-gateway/guardrails';
	import { profile } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { onMount } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		/** The saved guardrail to edit; omit it to create one of `type`. */
		id?: string;
		/** The provider type of a new guardrail. */
		type?: string;
		/**
		 * Shows the form in a dialog: without the test section, which needs a saved guardrail,
		 * and with its buttons inline rather than pinned to the bottom of the page.
		 */
		embedded?: boolean;
		/** Receives the form's title, for the page or dialog around it. */
		onTitle?: (title: string) => void;
		onSaved?: (guardrail: Guardrail, created: boolean) => void;
		onCancel: () => void;
	}

	let { id, type, embedded = false, onTitle, onSaved, onCancel }: Props = $props();

	// A new guardrail's ID once it is saved; the form stays open so it can be tested.
	let createdID = $state<string>();
	let guardrailID = $derived(createdID ?? id);
	let isNew = $derived(!guardrailID);

	let loading = $state(true);
	let saving = $state(false);
	let error = $state('');
	let loadError = $state('');

	let provider = $state<GuardrailProvider>();
	let name = $state('');
	let description = $state('');
	let values = $state<Record<string, unknown>>({});
	// List fields are edited as one entry per line.
	let listText = $state<Record<string, string>>({});
	let secrets = $state<Record<string, string>>({});
	let configuredSecrets = $state<string[]>([]);
	// Choices loaded from the configured service, by field.
	let serviceOptions = $state<Record<string, string[]>>({});
	let loadingOptions = $state<string>();
	let optionsError = $state<Record<string, string>>({});
	let guardrailError = $state('');

	let testTarget = $state<GuardrailTarget>('llm');
	let testStage = $state<GuardrailStage>('input');
	let testText = $state('');
	let testing = $state(false);
	let testResult = $state<GuardrailTestResult>();
	let testError = $state('');

	let readonly = $derived(profile.current.isAdminReadonly?.());
	// Testing needs a saved guardrail, and auditors can't run tests.
	let testDisabled = $derived(isNew || !!readonly);

	// List fields without known choices are edited as text, one entry per line.
	function isTextList(f: GuardrailField) {
		return f.type === 'list' && !f.options?.length && !f.optionsFromService;
	}

	// The choices shown for a list field: its fixed options, its defaults, what is selected,
	// and what was loaded from the service.
	function listChoices(f: GuardrailField) {
		const defaults = Array.isArray(f.default) ? (f.default as string[]) : [];
		return [
			...new Set([
				...(f.options ?? []),
				...defaults,
				...((values[f.name] as string[]) ?? []),
				...(serviceOptions[f.name] ?? [])
			])
		];
	}

	async function loadOptions(f: GuardrailField) {
		if (!provider) return;
		loadingOptions = f.name;
		optionsError[f.name] = '';
		try {
			serviceOptions[f.name] = await loadGuardrailFieldOptions(
				provider.type,
				f.name,
				{ guardrailID, config: buildConfig() },
				{ dontLogErrors: true }
			);
		} catch (e) {
			optionsError[f.name] = parseErrorContent(e).message;
		} finally {
			loadingOptions = undefined;
		}
	}

	function applyConfig(fields: GuardrailField[], config: Record<string, unknown>) {
		const next: Record<string, unknown> = {};
		const nextLists: Record<string, string> = {};
		for (const f of fields) {
			if (f.sensitive) continue;
			const v = config[f.name] ?? f.default;
			if (isTextList(f)) {
				nextLists[f.name] = Array.isArray(v) ? v.join('\n') : typeof v === 'string' ? v : '';
			} else if (f.type === 'list') {
				next[f.name] = Array.isArray(v) ? v : [];
			} else if (f.type === 'bool') {
				next[f.name] = v === true || v === 'true';
			} else {
				next[f.name] = v ?? '';
			}
		}
		values = next;
		listText = nextLists;
	}

	onMount(async () => {
		try {
			const providers = await listGuardrailProviders();
			if (isNew) {
				provider = providers.find((p) => p.type === type);
				if (!provider) {
					loadError = type ? m.guardrails_unknown_type({ type }) : m.guardrails_no_type_selected();
					return;
				}
				name = guardrailTypeLabel(provider);
				applyConfig(provider.fields, {});
				return;
			}
			const p = await getGuardrail(guardrailID!);
			provider = providers.find((x) => x.type === p.type);
			name = p.name;
			description = p.description ?? '';
			configuredSecrets = p.configuredSecrets ?? [];
			guardrailError = p.error ?? '';
			if (!provider) {
				loadError = m.guardrails_unknown_type({ type: p.type });
				return;
			}
			applyConfig(provider.fields, p.config ?? {});
		} finally {
			loading = false;
		}
	});

	let missingRequired = $derived(
		(provider?.fields ?? []).some((f) => {
			if (!f.required) return false;
			if (f.sensitive) return !secrets[f.name]?.trim() && !configuredSecrets.includes(f.name);
			if (f.type === 'list') {
				return isTextList(f) ? !listText[f.name]?.trim() : !(values[f.name] as string[])?.length;
			}
			const v = values[f.name];
			return v === undefined || v === null || v === '';
		})
	);

	let valid = $derived(!!provider && name.trim().length > 0 && !missingRequired);

	function buildConfig() {
		const config: Record<string, unknown> = {};
		for (const f of provider?.fields ?? []) {
			if (f.sensitive) {
				const v = secrets[f.name]?.trim();
				if (v) config[f.name] = v;
				continue;
			}
			if (isTextList(f)) {
				const items = (listText[f.name] ?? '')
					.split('\n')
					.map((s) => s.trim())
					.filter(Boolean);
				if (items.length) config[f.name] = items;
				continue;
			}
			const v = values[f.name];
			if (v === undefined || v === null || v === '') continue;
			config[f.name] = f.type === 'number' ? Number(v) : v;
		}
		return config;
	}

	async function save() {
		if (!provider) return;
		saving = true;
		error = '';
		try {
			const body = {
				name: name.trim(),
				description,
				type: provider.type,
				config: buildConfig()
			};
			const saved = guardrailID
				? await updateGuardrail(guardrailID, body, { dontLogErrors: true })
				: await createGuardrail(body, { dontLogErrors: true });
			configuredSecrets = saved.configuredSecrets ?? [];
			guardrailError = saved.error ?? '';
			secrets = {};
			const created = !guardrailID;
			success.add(created ? m.guardrails_created() : m.guardrails_saved());
			if (created && saved.id) {
				createdID = saved.id;
			}
			onSaved?.(saved, created);
		} catch (e) {
			error = parseErrorContent(e).message;
		} finally {
			saving = false;
		}
	}

	async function runTest() {
		if (!guardrailID) return;
		testing = true;
		testError = '';
		testResult = undefined;
		try {
			testResult = await testGuardrail(
				guardrailID,
				{ target: testTarget, stage: testStage, text: testText },
				{ dontLogErrors: true }
			);
		} catch (e) {
			testError = parseErrorContent(e).message;
		} finally {
			testing = false;
		}
	}

	function toggleOption(field: string, option: string, checked: boolean) {
		const current = (values[field] as string[]) ?? [];
		values[field] = checked ? [...current, option] : current.filter((o) => o !== option);
	}

	let title = $derived(
		isNew
			? provider
				? m.guardrails_new_title({ type: guardrailTypeLabel(provider) })
				: m.guardrails_new_title_generic()
			: name || m.guardrails_fallback_title()
	);
	$effect(() => onTitle?.(title));

	const DEFAULT_OPTION = '__default';
	const testTargets = [
		{ id: 'llm', label: 'LLM' },
		{ id: 'mcp', label: 'MCP' }
	];
	const testStages = [
		{ id: 'input', label: m.core_col_input() },
		{ id: 'output', label: m.core_col_output() }
	];
</script>

{#if loading && !loadError}
	<Skeleton type="card" count={2} />
{:else if loadError || !provider}
	<div class="notification-error rounded-md p-3 text-sm">{loadError}</div>
{:else}
	<div class={twMerge('flex flex-col gap-6', !embedded && 'pb-4')}>
		<section class="paper">
			<div>
				<h2 class="text-lg font-semibold">{guardrailTypeLabel(provider)}</h2>
				<p class="text-muted-content text-sm">{guardrailTypeDescription(provider)}</p>
			</div>
			<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
				<div class="flex flex-col gap-1">
					<label for="guardrail-name" class="text-sm font-light">{m.core_name()}</label>
					<input
						id="guardrail-name"
						class="text-input-filled"
						bind:value={name}
						disabled={readonly}
					/>
				</div>
				<div class="flex flex-col gap-1">
					<label for="guardrail-description" class="text-sm font-light"
						>{m.core_description()}</label
					>
					<input
						id="guardrail-description"
						class="text-input-filled"
						bind:value={description}
						disabled={readonly}
					/>
				</div>
			</div>
			{#if guardrailError}
				<div class="notification-alert rounded-md p-3 text-sm">{guardrailError}</div>
			{/if}
		</section>

		<section class="paper">
			<div>
				<h2 class="text-lg font-semibold">{m.guardrails_settings_heading()}</h2>
				<p class="text-muted-content text-sm">{m.guardrails_settings_description()}</p>
			</div>
			{#each provider.fields as field (field.name)}
				{@const inputID = `field-${field.name}`}
				{@const fieldLabel = guardrailFieldLabel(provider.type, field)}
				{@const fieldDescription = guardrailFieldDescription(provider.type, field)}
				<div class="flex flex-col gap-1">
					{#if field.type === 'bool'}
						<Toggle
							label={fieldLabel}
							labelInline
							checked={!!values[field.name]}
							disabled={readonly}
							onChange={(v) => (values[field.name] = v)}
						/>
					{:else}
						<label for={inputID} id={`${inputID}-label`} class="text-sm font-light">
							{fieldLabel}
							{#if field.required}<span class="text-error">*</span>{/if}
						</label>
						{#if field.sensitive}
							<SensitiveInput
								name={inputID}
								textarea={field.type === 'textarea'}
								bind:value={() => secrets[field.name] ?? '', (v) => (secrets[field.name] = v)}
								placeholder={configuredSecrets.includes(field.name)
									? m.guardrails_secret_configured_placeholder()
									: undefined}
								disabled={readonly}
							/>
						{:else if field.type === 'textarea'}
							<textarea
								id={inputID}
								class="text-input-filled min-h-24 text-sm"
								bind:value={() => String(values[field.name] ?? ''), (v) => (values[field.name] = v)}
								disabled={readonly}></textarea>
						{:else if field.type === 'number'}
							<input
								id={inputID}
								type="number"
								step="any"
								class="text-input-filled w-40"
								bind:value={values[field.name]}
								disabled={readonly}
							/>
						{:else if field.type === 'select'}
							<Select
								id={inputID}
								ariaLabelledby={`${inputID}-label`}
								classes={{ root: 'w-full md:w-72' }}
								class="bg-base-200 dark:bg-base-200 dark:border-base-400 border border-transparent shadow-inner"
								options={[
									...(field.required ? [] : [{ id: DEFAULT_OPTION, label: m.core_default() }]),
									...(field.options ?? []).map((o) => ({ id: o, label: o }))
								]}
								selected={(values[field.name] as string) || DEFAULT_OPTION}
								onSelect={(option) =>
									(values[field.name] = option.id === DEFAULT_OPTION ? '' : option.id)}
								disabled={readonly}
							/>
						{:else if field.type === 'list' && !isTextList(field)}
							<div id={inputID} class="flex flex-wrap gap-x-4 gap-y-1">
								{#each listChoices(field) as option (option)}
									<label class="flex items-center gap-2 text-sm">
										<input
											type="checkbox"
											class="checkbox checkbox-sm"
											checked={((values[field.name] as string[]) ?? []).includes(option)}
											disabled={readonly}
											onchange={(e) =>
												toggleOption(field.name, option, (e.target as HTMLInputElement).checked)}
										/>
										<span class="font-mono text-xs">{option}</span>
									</label>
								{/each}
							</div>
							{#if field.optionsFromService && !readonly}
								<button
									class="btn btn-secondary btn-sm mt-1 w-fit text-xs"
									disabled={loadingOptions === field.name}
									onclick={() => loadOptions(field)}
								>
									{#if loadingOptions === field.name}<Loading
											class="size-4"
										/>{:else}{m.guardrails_load_service_options()}{/if}
								</button>
							{/if}
							{#if optionsError[field.name]}
								<div class="notification-error rounded-md p-2 text-xs">
									{optionsError[field.name]}
								</div>
							{/if}
						{:else if field.type === 'list'}
							<textarea
								id={inputID}
								class="text-input-filled min-h-20 font-mono text-sm"
								placeholder={m.guardrails_one_per_line()}
								bind:value={listText[field.name]}
								disabled={readonly}></textarea>
						{:else}
							<input
								id={inputID}
								class="text-input-filled"
								bind:value={() => String(values[field.name] ?? ''), (v) => (values[field.name] = v)}
								placeholder={typeof field.default === 'string' ? field.default : undefined}
								disabled={readonly}
							/>
						{/if}
					{/if}
					{#if fieldDescription}
						<span class="text-muted-content text-xs">{fieldDescription}</span>
					{/if}
				</div>
			{:else}
				<span class="text-muted-content text-sm">{m.guardrails_no_settings()}</span>
			{/each}
		</section>

		{#if !embedded}
			<section class="paper">
				<div>
					<h2 class="text-lg font-semibold">{m.guardrails_test_heading()}</h2>
					<p class="text-muted-content text-sm">
						{isNew ? m.guardrails_test_save_first() : m.guardrails_test_description()}
					</p>
				</div>
				<div class="flex flex-wrap gap-3">
					<div class="flex w-40 flex-col gap-1">
						<span id="test-target-label" class="text-sm font-light"
							>{m.guardrails_test_target()}</span
						>
						<Select
							id="test-target"
							ariaLabelledby="test-target-label"
							class="bg-base-200 dark:bg-base-200 dark:border-base-400 border border-transparent shadow-inner"
							options={testTargets}
							selected={testTarget}
							onSelect={(option) => (testTarget = option.id as GuardrailTarget)}
							disabled={testDisabled}
						/>
					</div>
					<div class="flex w-40 flex-col gap-1">
						<span id="test-stage-label" class="text-sm font-light">{m.guardrails_test_stage()}</span
						>
						<Select
							id="test-stage"
							ariaLabelledby="test-stage-label"
							class="bg-base-200 dark:bg-base-200 dark:border-base-400 border border-transparent shadow-inner"
							options={testStages}
							selected={testStage}
							onSelect={(option) => (testStage = option.id as GuardrailStage)}
							disabled={testDisabled}
						/>
					</div>
				</div>
				<div class="flex flex-col gap-1">
					<label for="test-text" class="text-sm font-light">{m.guardrails_test_sample_text()}</label
					>
					<textarea
						id="test-text"
						class="text-input-filled min-h-24 text-sm"
						bind:value={testText}
						disabled={testDisabled}></textarea>
				</div>
				<button
					class="btn btn-secondary w-fit text-sm"
					disabled={testDisabled || testing || !testText.trim()}
					onclick={runTest}
				>
					{#if testing}<Loading class="size-4" />{:else}{m.guardrails_run_test()}{/if}
				</button>
				{#if testError || testResult?.error}
					<div class="notification-error rounded-md p-3 text-sm">
						{testError || testResult?.error}
					</div>
				{:else if testResult}
					{#if testResult.findings.length === 0}
						<span class="text-success text-sm">{m.guardrails_no_findings()}</span>
					{:else}
						<div class="flex flex-col gap-1">
							<span class="text-sm font-medium">
								{testResult.findings.length === 1
									? m.guardrails_findings_one({ count: testResult.findings.length })
									: m.guardrails_findings_other({ count: testResult.findings.length })}
							</span>
							<ul class="flex flex-col gap-1 text-sm">
								{#each testResult.findings as f, i (i)}
									<li>
										<span class="bg-error/15 text-error rounded-full px-2 py-0.5 text-xs"
											>{f.category}</span
										>
										{#if f.count && f.count > 1}<span class="text-xs">×{f.count}</span>{/if}
										{#if f.message}<span class="text-muted-content text-xs"
												>{guardrailFindingMessage(f.message)}</span
											>{/if}
									</li>
								{/each}
							</ul>
						</div>
						{#if testResult.redactedText}
							<div class="flex flex-col gap-1">
								<span class="text-sm font-medium">{m.guardrails_redacted_text()}</span>
								<pre
									class="bg-base-200 dark:bg-base-100 rounded-md p-3 text-xs whitespace-pre-wrap">{testResult.redactedText}</pre>
							</div>
						{:else if !testResult.canRedact}
							<span class="text-muted-content text-xs">
								{m.guardrails_cannot_redact()}
							</span>
						{/if}
					{/if}
				{/if}
			</section>
		{/if}

		{#if error}
			<div class="notification-error rounded-md p-3 text-sm">{error}</div>
		{/if}
	</div>

	{#if !readonly}
		<div
			class={twMerge(
				'flex w-full justify-end gap-2',
				embedded ? 'pt-4' : 'bg-base-200 dark:bg-base-100 sticky bottom-0 left-0 z-40 py-4'
			)}
		>
			<button class="btn btn-secondary text-sm" onclick={onCancel}>{m.common_cancel()}</button>
			<button class="btn btn-primary text-sm" disabled={!valid || saving} onclick={save}>
				{#if saving}<Loading class="size-4" />{:else}{m.core_save()}{/if}
			</button>
		</div>
	{/if}
{/if}
