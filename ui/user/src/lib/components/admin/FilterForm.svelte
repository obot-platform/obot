<script lang="ts">
	import { beforeNavigate } from '$app/navigation';
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
		type MCPFilterResource,
		type MCPFilterWebhookSelector,
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
	import FilterFormTypeSelection from './FilterFormTypeSelection.svelte';
	import SelectorsAndResourcesFormSegment from './SelectorsAndResourcesFormSegment.svelte';
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

	let initialFilterId = $derived(initialFilter?.id);
	const duration = PAGE_TRANSITION_DURATION;

	let filter = $state<{
		name: string;
		resources: MCPFilterResource[];
		url: string;
		secret: string;
		selectors: MCPFilterWebhookSelector[];
		toolName?: string;
		allowedToMutate?: boolean;
		disabled?: boolean;
	}>(
		untrack(() =>
			initialFilter
				? {
						name: initialFilter.name || '',
						resources: initialFilter.resources || [],
						url: initialFilter.url || '',
						secret: initialFilter.secret || '',
						selectors: initialFilter.selectors || [],
						toolName: initialFilter.toolName || '',
						allowedToMutate: initialFilter.allowedToMutate || false,
						disabled: initialFilter.disabled || false
					}
				: {
						name: '',
						resources: [{ id: 'default', type: 'mcpCatalog' }],
						url: '',
						secret: '',
						selectors: [],
						toolName: '',
						allowedToMutate: false,
						disabled: false
					}
		)
	);

	type FilterRuntimeFormData = Omit<RuntimeFormData, 'serverUserType'>;
	let runtimeFormData = $state<FilterRuntimeFormData | undefined>(
		untrack(() => convertToRuntimeFormData(initialFilter))
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

	let launchFilterData = $state<MCPFilter>();
	let launchError = $state<string>();
	let launchProgress = $state<number>(0);
	let launchLogsEventStream = $state<EventStreamService<string>>();
	let launchLogs = $state<string[]>([]);

	const UNSAVED_LAUNCH_EXIT_MESSAGE = m.mcps_filters_unsaved_exit();

	beforeNavigate(({ cancel }) => {
		if (!launchFilterData) return;
		if (!confirm(UNSAVED_LAUNCH_EXIT_MESSAGE)) {
			cancel();
			return;
		}
		if (!initialFilterId && launchFilterData.id) {
			void AdminService.deleteMCPFilter(launchFilterData.id).catch(() => {});
		}
	});

	// Validation
	let nameError = $derived(showValidation && !filter.name.trim());
	let urlError = $derived(showValidation && !filter.url.trim());
	let toolNameError = $derived(showValidation && !filter.toolName?.trim());

	onMount(() => {
		if (initialFilterId) {
			revealServerValues();
		}

		function handleBeforeUnload(e: BeforeUnloadEvent) {
			if (!launchFilterData) return;
			e.preventDefault();
			e.returnValue = UNSAVED_LAUNCH_EXIT_MESSAGE;
		}

		function handlePageHide(e: PageTransitionEvent) {
			if (e.persisted) return;
			if (initialFilterId) return;
			const id = launchFilterData?.id;
			if (!id) return;
			void AdminService.deleteMCPFilter(id, { keepalive: true }).catch(() => {});
		}

		window.addEventListener('beforeunload', handleBeforeUnload);
		window.addEventListener('pagehide', handlePageHide);

		return () => {
			window.removeEventListener('beforeunload', handleBeforeUnload);
			window.removeEventListener('pagehide', handlePageHide);
		};
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
				// ignore, 404 means no credentials were set
				return;
			}
			// Re-throw other errors
			throw error;
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
		try {
			await AdminService.deconfigureMCPFilter(initialFilterId);
			// Clear the secret field and update the filter state
			filter.secret = '';
			// Update the initial filter to reflect that it no longer has a secret
			if (initialFilter) {
				initialFilter.hasSecret = false;
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

	async function handleCloseLaunch() {
		if (launchLogsEventStream) {
			launchLogsEventStream.disconnect();
		}

		if (!initialFilter && launchFilterData) {
			// delete the filter
			await AdminService.deleteMCPFilter(launchFilterData.id);
		}

		launchError = undefined;
		launchFilterData = undefined;
		saving = false;
	}

	async function validateLaunch(
		filterData: MCPFilter,
		mcpServerManifest: ReturnType<typeof convertServerRuntimeFormDataToManifest> | undefined
	) {
		if (mcpServerManifest) {
			const configValues = Object.fromEntries(
				(mcpServerManifest.manifest.config ?? [])
					.filter((field) => !field.userAllowed && field.key && field.value)
					.map((field) => [field.key, field.value])
			);

			// Configure the server with the collected values if any exist
			if (Object.keys(configValues).length > 0) {
				await AdminService.configureMCPFilter(filterData.id, configValues);
			}
		}

		const launchResponse = filterData.disabled
			? { success: true }
			: await AdminService.launchMCPFilter(filterData.id);

		if (!launchResponse.success) {
			launchError = launchResponse.message;
			launchFilterData = filterData;
			listLaunchLogs(filterData.id);
			return false;
		}

		launchProgress = 100;
		await new Promise((resolve) => setTimeout(resolve, 500));
		return true;
	}

	async function handleSave() {
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

		if (launchLogsEventStream) {
			// reset launch logs
			launchLogsEventStream.disconnect();
			launchLogsEventStream = undefined;
			launchLogs = [];
		}

		launchError = undefined;
		launchFilterData = undefined;
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
				? convertServerRuntimeFormDataToManifest(runtimeFormData as RuntimeFormData)
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
				allowedToMutate: filter.allowedToMutate ?? false,
				disabled: filter.disabled ?? false,
				...(isPrebuiltEntry
					? {
							systemMCPServerCatalogEntryID: mcpSystemCatalogEntryId
						}
					: {
							mcpServerManifest: mcpServerManifest?.manifest
						})
			};

			if (initialFilterId) {
				const result = await AdminService.updateMCPFilter(initialFilterId, manifest);
				// skip launch for disabled mcp filter
				const launchSuccess = await validateLaunch(result, mcpServerManifest);

				if (launchSuccess) {
					saving = false;
					onUpdate?.(result);
				}
			} else {
				const result = await AdminService.createMCPFilter(manifest);
				const launchSuccess = await validateLaunch(result, mcpServerManifest);
				if (launchSuccess) {
					saving = false;
					onCreate?.(result);
				}
			}
		} catch (err) {
			launchError = err instanceof Error ? err.message : m.mcps_unknown_error();
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
								class="bg-base-200 dark:bg-base-200 dark:border-base-400 flex-1 border border-transparent shadow-inner"
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
							placeholder={initialFilter?.hasSecret && !filter.secret ? '*****' : ''}
							disabled={readonly || isPrebuiltEntry}
						/>
						{#if filter.secret || (initialFilter?.hasSecret && !filter.secret)}
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
					{#if initialFilter?.hasSecret}
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
			<div class="flex flex-col gap-4" id={MCP_FILTERS_FIELD_IDS.runtimeFormDetails}>
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

				{#if runtimeFormData.runtime !== 'remote'}
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
			</div>
		{/if}

		<div class="h-px bg-base-400 w-full my-4"></div>

		<SelectorsAndResourcesFormSegment bind:form={filter} {readonly} />
	</div>
	{#if !readonly}
		<div
			class="bg-base-200 dark:bg-base-100 dark:text-muted-content sticky bottom-0 left-0 flex w-full justify-end gap-2 py-4 text-gray-400 z-50"
			out:fly={{ x: -100, duration }}
			in:fly={{ x: -100 }}
		>
			<div class="flex w-full justify-between gap-2">
				<div>
					{#if initialFilter?.id}
						<button
							class={twMerge('text-sm btn btn-soft', filter.disabled ? 'btn-primary' : 'btn-error')}
							disabled={saving}
							onclick={() => {
								filter.disabled = !filter.disabled;
								handleSave();
							}}
						>
							{#if saving}
								<Loading class="size-4" />
							{:else}
								{filter.disabled ? m.mcps_filters_enable() : m.mcps_filters_disable()}
							{/if}
						</button>
					{/if}
				</div>
				<div class="flex gap-2">
					<button
						class="btn btn-secondary"
						onclick={() => {
							if (initialFilterId) {
								onUpdate?.(undefined);
							} else {
								onCreate?.(undefined);
							}
						}}
					>
						{m.common_cancel()}
					</button>
					<button
						id={MCP_FILTERS_FIELD_IDS.saveBtn}
						class="btn btn-primary text-sm"
						disabled={saving}
						onclick={handleSave}
					>
						{#if saving}
							<Loading class="size-4" />
						{:else}
							{m.core_save()}
						{/if}
					</button>
				</div>
			</div>
		</div>
	{/if}
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
		<h4 class="text-xl font-semibold">{m.mcps_filters_launch_failed_form_filter()}</h4>
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
