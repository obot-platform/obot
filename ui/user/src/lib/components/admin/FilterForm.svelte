<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { PAGE_TRANSITION_DURATION, PII_REDACT_TYPES, PII_BLOCK_TYPES } from '$lib/constants';
	import { MCP_FILTERS_FIELD_IDS } from '$lib/constants';
	import { HttpError } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		type MCPFilter,
		type MCPFilterInput,
		type MCPFilterManifest,
		type Runtime,
		type RuntimeFormData
	} from '$lib/services';
	import { EventStreamService } from '$lib/services/admin/eventstream.svelte';
	import { getManifestConfiguration } from '$lib/services/user/mcp';
	import {
		convertServerRuntimeFormDataToManifest,
		validateRuntimeForm
	} from '$lib/services/user/mcp';
	import PageLoading from '../PageLoading.svelte';
	import Select from '../Select.svelte';
	import Toggle from '../Toggle.svelte';
	import ContainerizedRuntimeForm from '../mcp/ContainerizedRuntimeForm.svelte';
	import CustomConfigurationForm from '../mcp/CustomConfigurationForm.svelte';
	import NpxRuntimeForm from '../mcp/NpxRuntimeForm.svelte';
	import RemoteRuntimeForm from '../mcp/RemoteRuntimeForm.svelte';
	import UvxRuntimeForm from '../mcp/UvxRuntimeForm.svelte';
	import CredentialFilterConfiguration from './CredentialFilterConfiguration.svelte';
	import FilterActions from './FilterActions.svelte';
	import FilterFormTypeSelection from './FilterFormTypeSelection.svelte';
	import SelectorsAndResourcesFormSegment from './SelectorsAndResourcesFormSegment.svelte';
	import {
		credentialEnvironment,
		credentialKeys,
		credentialMutationRequired,
		readCredentialPolicy,
		validateCredentialPolicy
	} from './credentialPolicy';
	import { reconcileFilterDraft } from './filterDraft';
	import { filterUnsavedChanges } from './filterUnsavedChanges.svelte';
	import { Eye, EyeOff } from '@lucide/svelte';
	import { onMount, untrack, type Snippet } from 'svelte';
	import { fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		topContent?: Snippet;
		filter?: MCPFilterInput;
		onCreate?: (filter?: MCPFilter) => void;
		onUpdate?: (filter?: MCPFilter) => void;
		readonly?: boolean;
		mcpSystemCatalogEntryId?: string;
	}

	let {
		topContent,
		filter: initialFilter,
		onCreate,
		onUpdate,
		readonly,
		mcpSystemCatalogEntryId
	}: Props = $props();

	let initialFilterId = $state(untrack(() => initialFilter?.id));
	let savedFilter = $state<MCPFilterInput | undefined>(
		untrack(() => (initialFilter ? structuredClone($state.snapshot(initialFilter)) : undefined))
	);
	let activationFailed = $state(false);
	let saveStage = $state<'save' | 'configure' | 'activate'>('save');
	const duration = PAGE_TRANSITION_DURATION;

	function toFilterFields(input?: MCPFilterInput, secret = input?.secret ?? '') {
		return {
			name: input?.name ?? '',
			resources: structuredClone(
				$state.snapshot(
					input?.resources ?? (input ? [] : [{ id: 'default', type: 'mcpCatalog' as const }])
				)
			),
			url: input?.url ?? '',
			secret,
			selectors: structuredClone($state.snapshot(input?.selectors ?? [])),
			toolName: input?.toolName ?? '',
			allowedToMutate: input?.allowedToMutate ?? false
		};
	}
	type FilterFields = ReturnType<typeof toFilterFields>;
	let filter = $state(untrack(() => toFilterFields(initialFilter)));

	type FilterRuntimeFormData = Omit<RuntimeFormData, 'serverUserType'>;
	let runtimeFormData = $state<FilterRuntimeFormData | undefined>(
		untrack(() => convertToRuntimeFormData(structuredClone($state.snapshot(initialFilter))))
	);
	let credentialConfigurationLoaded = $state(untrack(() => !initialFilter?.id));
	let credentialLoadError = $state('');
	let disableError = $state('');
	let isCredentialFilter = $derived(
		filter.toolName === 'filter_credentials' && !!mcpSystemCatalogEntryId
	);
	let credentialPolicy = $derived(readCredentialPolicy(runtimeFormData?.env ?? []));
	let credentialError = $derived(
		isCredentialFilter ? validateCredentialPolicy(credentialPolicy) : undefined
	);
	let runtimeTypeSelect = $derived(runtimeFormData ? runtimeFormData.runtime : 'webhook-url');
	let showRuntimeRequired = $state<Record<string, boolean>>({});
	const runtimeOptions = [
		{ id: 'webhook-url', label: m.mcps_filters_webhook_url() },
		{ id: 'remote', label: m.mcps_catalog_runtime_remote() },
		{ id: 'npx', label: 'NPX' },
		{ id: 'uvx', label: 'UVX' },
		{ id: 'containerized', label: m.mcps_catalog_runtime_containerized() }
	];

	let saving = $state<boolean | undefined>();
	let showSecret = $state<boolean>(false);
	let removingSecret = $state(false);
	let showValidation = $state(false);

	let launchError = $state<string>();
	let launchProgress = $state(0);
	let launchLogsEventStream = $state<EventStreamService<string>>();
	let launchLogs = $state<string[]>([]);
	let savedFields = $state(untrack(() => JSON.stringify(filter)));
	let savedRuntime = $state(untrack(() => JSON.stringify(runtimeFormData)));
	let dirty = $derived(
		JSON.stringify(filter) !== savedFields || JSON.stringify(runtimeFormData) !== savedRuntime
	);
	const navigation = filterUnsavedChanges(() => !readonly && dirty);
	export function allowNavigation() {
		navigation.allowNavigation();
	}

	function syncSavedFilter(
		result: MCPFilter,
		submittedFields: FilterFields,
		submittedRuntime: FilterRuntimeFormData | undefined
	) {
		savedFilter = structuredClone(result);
		const fields = toFilterFields(result, submittedFields.secret);
		const runtime = convertToRuntimeFormData(structuredClone(result));
		if (runtime) {
			// Configuration credentials are stored separately and are not returned in the manifest.
			const values = new Map(
				[
					...(submittedRuntime?.env ?? []),
					...(submittedRuntime?.remoteServerConfig?.headers ?? [])
				].map((field) => [field.key, field.value])
			);
			runtime.env = runtime.env.map((field) => ({
				...field,
				value: values.get(field.key) ?? field.value
			}));
			if (runtime.remoteServerConfig?.headers) {
				runtime.remoteServerConfig.headers = runtime.remoteServerConfig.headers.map((field) => ({
					...field,
					value: values.get(field.key) ?? field.value
				}));
			}
			if (fields.toolName === 'filter_credentials' && mcpSystemCatalogEntryId) {
				for (const field of submittedRuntime?.env ?? []) {
					if (
						credentialKeys.includes(field.key) &&
						!runtime.env.some((saved) => saved.key === field.key)
					)
						runtime.env.push(structuredClone(field));
				}
			}
		}
		filter = reconcileFilterDraft($state.snapshot(filter), submittedFields, fields);
		runtimeFormData = reconcileFilterDraft(
			$state.snapshot(runtimeFormData),
			submittedRuntime,
			runtime
		);
		savedFields = JSON.stringify(fields);
		savedRuntime = JSON.stringify(runtime);
	}

	// Validation
	let nameError = $derived(showValidation && !filter.name.trim());
	let urlError = $derived(showValidation && !filter.url.trim());
	let toolNameError = $derived(showValidation && !filter.toolName?.trim());

	onMount(() => {
		if (initialFilterId) void revealServerValues();
		return () => launchLogsEventStream?.disconnect();
	});

	async function revealServerValues() {
		if (!initialFilterId || readonly) return;
		try {
			const response = await AdminService.revealMCPFilter(initialFilterId);

			// Update environment variables with revealed values
			if (runtimeFormData?.env) {
				runtimeFormData.env = runtimeFormData.env.map((env) => ({
					...env,
					value: response[env.key] ?? ''
				}));
			}

			if (isCredentialFilter && runtimeFormData) {
				// Saved values can outlive fields in the instance's old manifest.
				// Preserve them, including unsupported IDs, for validation and repair.
				for (const key of credentialKeys) {
					if (!runtimeFormData.env.some((field) => field.key === key)) {
						runtimeFormData.env.push({
							key,
							name: key,
							description: '',
							required: false,
							sensitive: false,
							value: response[key] ?? ''
						});
					}
				}
			}
			credentialConfigurationLoaded = true;

			// Update headers in the appropriate runtime config based on runtime type
			if (runtimeFormData?.runtime === 'remote') {
				if (runtimeFormData.remoteConfig?.headers) {
					runtimeFormData.remoteConfig.headers = runtimeFormData.remoteConfig.headers.map(
						(header) => ({
							...header,
							value: response[header.key] ?? ''
						})
					);
				}
				if (runtimeFormData.remoteServerConfig?.headers) {
					runtimeFormData.remoteServerConfig.headers =
						runtimeFormData.remoteServerConfig.headers.map((header) => ({
							...header,
							value: response[header.key] ?? ''
						}));
				}
			}
		} catch (error) {
			if (error instanceof HttpError && error.statusCode === 404) {
				// 404 means no credentials were set.
				credentialConfigurationLoaded = true;
				return;
			}
			if (isCredentialFilter) {
				credentialLoadError =
					'Unable to load the saved credential policy. Reload this page before editing.';
				return;
			}
			credentialLoadError = error instanceof Error ? error.message : m.mcps_unknown_error();
		} finally {
			savedRuntime = JSON.stringify(runtimeFormData);
		}
	}

	function convertToRuntimeFormData(filter?: MCPFilterInput): FilterRuntimeFormData | undefined {
		if (!filter || !filter.mcpServerManifest) {
			return undefined;
		} else {
			const manifest = filter.mcpServerManifest;
			const formData: FilterRuntimeFormData = {
				categories: manifest.metadata?.categories?.split(',').filter((c) => c.trim()) ?? [''],
				icon: manifest.icon ?? '',
				name: manifest.name ?? '',
				description: manifest.description ?? '',
				env: getManifestConfiguration(manifest).env?.map((env) => ({ ...env, value: '' })) ?? [],
				runtime: manifest.runtime ?? 'npx',
				npxConfig: undefined,
				uvxConfig: undefined,
				containerizedConfig: undefined,
				remoteConfig: undefined,
				remoteServerConfig: undefined
			};

			// Initialize the appropriate runtime config based on the runtime type
			switch (manifest.runtime) {
				case 'npx':
					formData.npxConfig = manifest.npxConfig || { package: '', args: [] };
					break;
				case 'uvx':
					formData.uvxConfig = manifest.uvxConfig || { package: '', command: '', args: [] };
					break;
				case 'containerized':
					formData.containerizedConfig = manifest.containerizedConfig || {
						image: '',
						port: 0,
						path: '',
						command: '',
						args: []
					};
					break;
				case 'remote':
					formData.remoteServerConfig = manifest.remoteConfig
						? {
								url: manifest.remoteConfig.url ?? '',
								localhostCallbackEnabled: manifest.remoteConfig.localhostCallbackEnabled,
								localhostCallbackPath: manifest.remoteConfig.localhostCallbackPath,
								headers:
									getManifestConfiguration(manifest).headers.map((h) => ({ ...h, value: '' })) ?? []
							}
						: { url: '', headers: [] };
					break;
			}

			return formData;
		}
	}

	async function handleRemoveSecret() {
		if (!initialFilterId) return;

		removingSecret = true;
		const submittedSecret = filter.secret;
		try {
			await AdminService.deconfigureMCPFilter(initialFilterId);
			if (filter.secret === submittedSecret) filter.secret = '';
			const fields: FilterFields = JSON.parse(savedFields);
			fields.secret = '';
			savedFields = JSON.stringify(fields);
			if (savedFilter) {
				const result = {
					...$state.snapshot(savedFilter),
					id: initialFilterId,
					hasSecret: false,
					secret: undefined
				};
				savedFilter = result;
				onUpdate?.(result);
			}
		} finally {
			removingSecret = false;
		}
	}

	function handleRuntimeChange(option: { id: string; label: string }) {
		if (option.id === 'webhook-url') {
			runtimeFormData = undefined;
			return;
		}

		// reset webhook url fields
		filter.url = '';
		filter.secret = '';

		const newRuntime = option.id as Runtime;
		if (!runtimeFormData) {
			runtimeFormData = {
				categories: [''],
				name: '',
				description: '',
				env: [],
				icon: '',
				runtime: 'npx' as Runtime,
				npxConfig: { package: '', args: [] },
				uvxConfig: undefined,
				containerizedConfig: undefined,
				remoteConfig: undefined,
				remoteServerConfig: undefined
			};
		}
		runtimeFormData.runtime = newRuntime;

		// Clear all runtime configs first
		runtimeFormData.npxConfig = undefined;
		runtimeFormData.uvxConfig = undefined;
		runtimeFormData.containerizedConfig = undefined;
		runtimeFormData.remoteConfig = undefined;
		runtimeFormData.remoteServerConfig = undefined;

		// Initialize the appropriate config based on the new runtime
		switch (newRuntime) {
			case 'npx':
				runtimeFormData.npxConfig = { package: '', args: [] };
				break;
			case 'uvx':
				runtimeFormData.uvxConfig = { package: '', command: '', args: [] };
				break;
			case 'containerized':
				runtimeFormData.containerizedConfig = {
					image: '',
					port: 0,
					path: '',
					command: '',
					args: []
				};
				break;
			case 'remote':
				runtimeFormData.remoteServerConfig = { url: '', headers: [] };
				break;
		}
	}

	function handleUpdateRequired(field: string) {
		delete showRuntimeRequired[field];
	}

	function listLaunchLogs(filterId: string) {
		launchLogsEventStream = new EventStreamService<string>();

		launchLogsEventStream.connect(`/api/mcp-webhook-validations/${filterId}/logs`, {
			onMessage: (data) => {
				launchLogs = [...launchLogs, data];
			}
		});
	}

	function handleCloseLaunch() {
		launchLogsEventStream?.disconnect();
		launchError = undefined;
		saving = false;
	}

	async function validateLaunch(
		filterData: MCPFilter,
		mcpServerManifest: ReturnType<typeof convertServerRuntimeFormDataToManifest> | undefined,
		submittedFields: FilterFields,
		submittedRuntime: FilterRuntimeFormData | undefined
	) {
		saveStage = 'configure';
		if (mcpServerManifest) {
			const configValues =
				submittedFields.toolName === 'filter_credentials' && mcpSystemCatalogEntryId
					? {
							...Object.fromEntries(
								(submittedRuntime?.env ?? []).map((field) => [field.key, field.value ?? ''])
							),
							...credentialEnvironment(readCredentialPolicy(submittedRuntime?.env ?? []))
						}
					: Object.fromEntries(
							(mcpServerManifest.manifest.config ?? [])
								.filter((field) => !field.userAllowed && field.key && field.value)
								.map((field) => [field.key, field.value])
						);

			// Configure the server with the collected values if any exist
			if (Object.keys(configValues).length > 0) {
				filterData = await AdminService.configureMCPFilter(filterData.id, configValues);
			}
		}

		syncSavedFilter(filterData, submittedFields, submittedRuntime);
		saveStage = 'activate';
		const launchResponse = filterData.disabled
			? { success: true }
			: await AdminService.launchMCPFilter(filterData.id);

		if (!launchResponse.success) {
			launchError = launchResponse.message;
			activationFailed = true;
			listLaunchLogs(filterData.id);
			return undefined;
		}

		launchProgress = 100;
		await new Promise((resolve) => setTimeout(resolve, 500));
		return filterData;
	}

	async function disableFilter() {
		if (!initialFilterId || !savedFilter || readonly || saving) return;
		saving = true;
		disableError = '';
		launchError = undefined;
		saveStage = 'save';
		try {
			// Only persist the saved manifest. Disabling must never apply draft edits.
			const {
				name,
				resources,
				url,
				selectors,
				toolName,
				allowedToMutate,
				mcpServerManifest,
				systemMCPServerCatalogEntryID
			} = savedFilter;
			const manifest: MCPFilterManifest = {
				name,
				resources,
				url,
				selectors,
				toolName,
				allowedToMutate,
				...(mcpSystemCatalogEntryId
					? { systemMCPServerCatalogEntryID: mcpSystemCatalogEntryId }
					: { mcpServerManifest, systemMCPServerCatalogEntryID }),
				disabled: true
			};
			const result = await AdminService.updateMCPFilter(initialFilterId, manifest);
			syncSavedFilter(
				result,
				JSON.parse(savedFields),
				savedRuntime ? JSON.parse(savedRuntime) : undefined
			);
			activationFailed = false;
			onUpdate?.(result);
		} catch (err) {
			disableError = err instanceof Error ? err.message : m.mcps_unknown_error();
		} finally {
			saving = false;
		}
	}

	async function handleSave(disabled: boolean) {
		if (saving || readonly || (runtimeFormData && !credentialConfigurationLoaded)) return;
		if (
			isCredentialFilter &&
			(!credentialConfigurationLoaded || credentialLoadError || credentialError)
		)
			return;
		// Show validation errors if required fields are missing
		if (
			!filter.name.trim() ||
			(!runtimeFormData && !filter.url.trim()) ||
			(runtimeFormData && !filter.toolName?.trim())
		) {
			showValidation = true;
			return;
		}

		if (runtimeFormData) {
			showRuntimeRequired = {}; // reset
			const { required } = validateRuntimeForm(runtimeFormData as RuntimeFormData, 'filter', true);
			if (Object.keys(required).length > 0) {
				showRuntimeRequired = required;
				return;
			}
		}

		saving = true;
		saveStage = 'save';
		const submittedFields = $state.snapshot(filter);
		const submittedRuntime = $state.snapshot(runtimeFormData);

		if (launchLogsEventStream) {
			// reset launch logs
			launchLogsEventStream.disconnect();
			launchLogsEventStream = undefined;
			launchLogs = [];
		}

		launchError = undefined;

		launchProgress = 0;

		let timeout1 = setTimeout(() => {
			launchProgress = 10;
		}, 100);

		let timeout2 = setTimeout(() => {
			launchProgress = 30;
		}, 3000);

		let timeout3 = setTimeout(() => {
			launchProgress = 80;
		}, 10000);

		try {
			const mcpServerManifest = runtimeFormData
				? convertServerRuntimeFormDataToManifest(submittedRuntime as RuntimeFormData)
				: undefined;
			const selectors =
				filter.selectors.length > 0
					? filter.selectors
							.map((s) => ({
								...s,
								identifiers: s.identifiers?.filter((id) => id.trim()) || []
							}))
							.filter((s) => s.method || (s.identifiers && s.identifiers.length > 0))
					: undefined;

			const manifest: MCPFilterManifest = {
				name: filter.name,
				resources: filter.resources,
				url: filter.url,
				secret: filter.secret || undefined,
				selectors,
				toolName: filter.toolName,
				allowedToMutate:
					!!filter.allowedToMutate ||
					(isCredentialFilter && credentialMutationRequired(credentialPolicy)),
				disabled,
				...(isPrebuiltEntry
					? {
							systemMCPServerCatalogEntryID: mcpSystemCatalogEntryId
						}
					: {
							mcpServerManifest: mcpServerManifest?.manifest
						})
			};

			const updating = !!initialFilter?.id;
			const result = initialFilterId
				? await AdminService.updateMCPFilter(initialFilterId, manifest)
				: await AdminService.createMCPFilter(manifest);
			initialFilterId = result.id;
			savedFilter = structuredClone(result);
			const launchSuccess = await validateLaunch(
				result,
				mcpServerManifest,
				submittedFields,
				submittedRuntime
			);
			if (launchSuccess) {
				activationFailed = false;
				saving = false;
				if (updating) onUpdate?.(launchSuccess);
				else onCreate?.(launchSuccess);
			}
		} catch (err) {
			launchError = err instanceof Error ? err.message : m.mcps_unknown_error();
			if (saveStage !== 'save') activationFailed = true;
		} finally {
			clearTimeout(timeout1);
			clearTimeout(timeout2);
			clearTimeout(timeout3);
		}
	}

	const isPrebuiltEntry = $derived(!!mcpSystemCatalogEntryId);
</script>

<div
	class="flex h-full w-full flex-col gap-4"
	out:fly={{ x: 100, duration }}
	in:fly={{ x: 100, delay: duration }}
>
	<div class="flex grow flex-col gap-4" out:fly={{ x: -100, duration }} in:fly={{ x: -100 }}>
		{#if topContent}
			{@render topContent()}
		{/if}
		{#if !initialFilterId}
			<h1 class="text-2xl font-semibold">{m.mcps_filters_create_filter()}</h1>
		{/if}

		<div
			class="dark:bg-base-200 dark:border-base-400 bg-base-100 rounded-lg border border-transparent p-4"
			id={MCP_FILTERS_FIELD_IDS.basicDetails}
		>
			<div class="flex flex-col gap-6">
				<div class="flex flex-col gap-2">
					<label for="filter-name" class="flex-1 text-sm font-light capitalize">
						{m.core_name()}
					</label>
					<div class="flex grow flex-col gap-0.5">
						<input
							id="filter-name"
							bind:value={filter.name}
							class="text-input-filled dark:bg-base-100 mt-0.5 {nameError
								? 'border-error focus:border-error focus:ring-error'
								: ''}"
							disabled={readonly}
						/>
						{#if nameError}
							<p class="text-xs text-error">{m.mcps_name_required()}</p>
						{/if}
					</div>
				</div>

				{#if !mcpSystemCatalogEntryId}
					<div class="flex flex-col gap-2">
						<label for={MCP_FILTERS_FIELD_IDS.runtimeSelector} class="text-sm font-light"
							>{m.core_type()}</label
						>
						<div class="w-full">
							<Select
								id={MCP_FILTERS_FIELD_IDS.runtimeSelector}
								options={runtimeOptions}
								bind:selected={runtimeTypeSelect}
								onSelect={handleRuntimeChange}
								disabled={readonly || isPrebuiltEntry}
							/>
						</div>
					</div>
				{/if}
			</div>
		</div>

		{#if !runtimeFormData}
			<div
				class="dark:bg-base-200 dark:border-base-400 bg-base-100 flex flex-col gap-8 rounded-lg border border-transparent p-4 shadow-sm"
				id={MCP_FILTERS_FIELD_IDS.runtimeFormDetails}
			>
				<div class="flex flex-col gap-2">
					<label for="webhook-url" class="flex-1 text-sm font-light capitalize">
						{m.mcps_filters_webhook_url()}
					</label>
					<input
						id="webhook-url"
						bind:value={filter.url}
						class="text-input-filled dark:bg-base-100 mt-0.5 {urlError
							? 'border-error focus:border-error focus:ring-error'
							: ''}"
						required
						disabled={readonly || isPrebuiltEntry}
					/>
					{#if urlError}
						<p class="text-xs text-error">{m.mcps_filters_webhook_url_required()}</p>
					{/if}
				</div>

				<div class="flex flex-col gap-2">
					<label for="webhook-secret" class="flex-1 text-sm font-light capitalize">
						{m.mcps_filters_secret_optional()}
					</label>
					<div class="relative">
						<input
							id="webhook-secret"
							bind:value={filter.secret}
							class="text-input-filled pr-10 dark:bg-base-100"
							type={showSecret ? 'text' : 'password'}
							placeholder={savedFilter?.hasSecret && !filter.secret ? '*****' : ''}
							disabled={readonly || isPrebuiltEntry}
						/>
						{#if filter.secret || (savedFilter?.hasSecret && !filter.secret)}
							<button
								type="button"
								class="absolute top-1/2 right-2 -translate-y-1/2 p-1 text-muted-content hover:text-base-content dark:text-muted-content dark:hover:text-base-content"
								onclick={() => (showSecret = !showSecret)}
								use:tooltip={{
									text: showSecret ? m.mcps_filters_hide_secret() : m.mcps_filters_show_secret(),
									placement: 'top-end'
								}}
							>
								{#if filter.secret}
									{#if showSecret}
										<EyeOff class="size-4" />
									{:else}
										<Eye class="size-4" />
									{/if}
								{/if}
							</button>
						{/if}
					</div>
					{#if savedFilter?.hasSecret}
						<div class="flex items-start justify-between gap-4">
							<p class="flex-1 text-xs text-amber-600 dark:text-amber-400">
								{m.mcps_filters_secret_configured()}
							</p>
							{#if !readonly}
								<button
									type="button"
									class="btn btn-error btn-sm shrink-0"
									disabled={removingSecret || saving || isPrebuiltEntry}
									onclick={handleRemoveSecret}
								>
									{#if removingSecret}
										<Loading class="size-3 inline-block" />
										{m.mcps_filters_removing()}
									{:else}
										{m.mcps_filters_remove_secret()}
									{/if}
								</button>
							{/if}
						</div>
					{:else}
						<p class="text-muted-content text-xs">
							{m.mcps_filters_secret_hint()}
						</p>
					{/if}
				</div>
			</div>
		{:else if runtimeFormData}
			<fieldset
				disabled={!credentialConfigurationLoaded}
				class="flex flex-col gap-4 min-w-0"
				id={MCP_FILTERS_FIELD_IDS.runtimeFormDetails}
			>
				{#if !isPrebuiltEntry}
					{#if runtimeFormData.runtime === 'npx' && runtimeFormData.npxConfig}
						<NpxRuntimeForm
							bind:config={runtimeFormData.npxConfig}
							readonly={readonly || isPrebuiltEntry}
							showRequired={showRuntimeRequired}
							onFieldChange={handleUpdateRequired}
						>
							{@render toolNameForm()}
						</NpxRuntimeForm>
					{:else if runtimeFormData.runtime === 'uvx' && runtimeFormData.uvxConfig}
						<UvxRuntimeForm
							bind:config={runtimeFormData.uvxConfig}
							readonly={readonly || isPrebuiltEntry}
							showRequired={showRuntimeRequired}
							onFieldChange={handleUpdateRequired}
						>
							{@render toolNameForm()}
						</UvxRuntimeForm>
					{:else if runtimeFormData.runtime === 'containerized' && runtimeFormData.containerizedConfig}
						<ContainerizedRuntimeForm
							bind:config={runtimeFormData.containerizedConfig}
							readonly={readonly || isPrebuiltEntry}
							showRequired={showRuntimeRequired}
							onFieldChange={handleUpdateRequired}
						>
							{@render toolNameForm()}
						</ContainerizedRuntimeForm>
					{:else if runtimeFormData.runtime === 'remote' && runtimeFormData.remoteServerConfig}
						<RemoteRuntimeForm
							bind:config={runtimeFormData.remoteServerConfig}
							variant="server"
							readonly={readonly || isPrebuiltEntry}
							showRequired={showRuntimeRequired}
							onFieldChange={handleUpdateRequired}
							isNewEntry={!initialFilterId}
							disableStaticOAuth
							disableHostnameOption
						>
							{@render toolNameForm()}
						</RemoteRuntimeForm>
					{/if}
				{/if}

				{#if isCredentialFilter}
					{#if readonly && initialFilterId}<p class="text-sm text-muted-content">
							Saved credential policy is available only to administrators.
						</p>{:else if credentialLoadError}<p role="alert" class="text-error">
							{credentialLoadError}
						</p>{:else if !credentialConfigurationLoaded}<p>
							Loading saved credential policy...
						</p>{:else}
						<CredentialFilterConfiguration bind:config={runtimeFormData.env} {readonly} />
					{/if}
				{:else if runtimeFormData.runtime !== 'remote'}
					<CustomConfigurationForm
						bind:config={runtimeFormData.env}
						{readonly}
						serverUserType="multiUser"
						{isPrebuiltEntry}
						overrideEnvField={[PII_REDACT_TYPES, PII_BLOCK_TYPES]}
						showRequired={showRuntimeRequired.env}
					>
						{#snippet overrideEnvTemplate({ config })}
							{#if config.key === PII_BLOCK_TYPES && runtimeFormData}
								<FilterFormTypeSelection bind:config={runtimeFormData.env} />
							{/if}
						{/snippet}
					</CustomConfigurationForm>
				{/if}
			</fieldset>
		{/if}

		<div class="h-px bg-base-400 w-full my-4"></div>

		<SelectorsAndResourcesFormSegment bind:form={filter} {readonly} />
	</div>
	<FilterActions
		exists={!!initialFilterId}
		disabled={savedFilter?.disabled ?? !initialFilterId}
		{dirty}
		busy={!!saving || removingSecret}
		{readonly}
		blocked={(!!runtimeFormData && !credentialConfigurationLoaded) ||
			!!credentialLoadError ||
			(isCredentialFilter && (!credentialConfigurationLoaded || !!credentialError))}
		{activationFailed}
		canReapply={isPrebuiltEntry}
		error={disableError || (!isCredentialFilter ? credentialLoadError : '')}
		onSave={handleSave}
		onDisable={disableFilter}
		onCancel={() => {
			if (navigation.discard()) {
				if (onUpdate) onUpdate(undefined);
				else onCreate?.(undefined);
			}
		}}
	/>
</div>

<PageLoading
	isProgressBar
	show={!!saving}
	text={m.mcps_filters_configuring()}
	progress={launchProgress}
	error={launchError}
	errorClasses={{
		root: 'md:w-[95vw]'
	}}
	onClose={handleCloseLaunch}
>
	{#snippet errorPreContent()}
		<h4 class="text-xl font-semibold">
			{saveStage === 'save'
				? m.mcps_filters_save_failed()
				: m.mcps_filters_launch_failed_form_filter()}
		</h4>
		<p>
			{saveStage === 'save'
				? m.mcps_filters_changes_not_saved()
				: saveStage === 'configure'
					? m.mcps_filters_configuration_failed()
					: m.mcps_filters_saved_activation_failed()}
		</p>
	{/snippet}

	{#snippet errorPostContent()}
		{#if launchLogs.length > 0}
			<div
				class="default-scrollbar-thin bg-base-200 max-h-[50vh] w-full overflow-y-auto rounded-lg p-4 shadow-inner"
			>
				{#each launchLogs as log, i (i)}
					<div class="font-mono text-sm">
						<span class="text-muted-content">{log}</span>
					</div>
				{/each}
			</div>
		{:else}
			<p class="text-md self-start">{m.mcps_filters_launch_issue_form_filter()}</p>
		{/if}

		<div class="flex w-full flex-col items-center gap-2 md:flex-row">
			<button class="btn btn-secondary w-full md:w-1/2 md:flex-1" onclick={handleCloseLaunch}
				>{m.core_close()}</button
			>
		</div>
	{/snippet}
</PageLoading>

{#snippet toolNameForm()}
	<div
		class={twMerge(
			'flex',
			runtimeFormData?.runtime === 'remote' ? 'flex-col gap-2' : 'items-center gap-3'
		)}
	>
		<label
			for="tool-name"
			class={twMerge(
				'shrink-0 text-sm font-light capitalize',
				runtimeFormData?.runtime === 'containerized' ? 'w-20' : ''
			)}
		>
			{m.mcps_filters_tool_name()}
		</label>
		<div class="flex grow flex-col gap-0.5">
			<input
				id="tool-name"
				bind:value={filter.toolName}
				class="text-input-filled dark:bg-base-100 mt-0.5 {toolNameError
					? 'border-error focus:border-error focus:ring-error'
					: ''}"
				required
				disabled={readonly || isPrebuiltEntry}
			/>
			{#if toolNameError}
				<p class="text-xs text-error">{m.mcps_filters_tool_name_required()}</p>
			{/if}
		</div>
	</div>
	<div class="flex flex-col gap-1">
		<div class="flex items-center gap-4">
			<Toggle
				classes={{
					label: 'text-sm gap-2 font-light text-base-content'
				}}
				checked={filter.allowedToMutate ?? false}
				onChange={(checked) => {
					filter.allowedToMutate = checked;
				}}
				disabled={readonly || isPrebuiltEntry}
				label={m.mcps_filters_enable_mutable()}
				labelInline
			/>
		</div>

		<p class="text-muted-content text-xs font-light">
			{m.mcps_filters_mutable_hint()}
		</p>
	</div>
{/snippet}
