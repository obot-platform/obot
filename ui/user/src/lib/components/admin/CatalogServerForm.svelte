<script lang="ts">
	import { HttpError } from '$lib/errors';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		UserService,
		type MCPCatalogServer,
		type LaunchServerType,
		type MCPCatalogEntry,
		type MCPResourceRequirements,
		type RuntimeFormData,
		type MCPCatalogEntryServerManifest,
		type Runtime,
		Group
	} from '$lib/services';
	import {
		convertCategoriesToMetadata,
		convertServerRuntimeFormDataToManifest,
		hasSecretBinding,
		sanitizeEgressDomains,
		sanitizeResourceRuntimeConfig,
		validateRuntimeForm
	} from '$lib/services/user/mcp';
	import { profile, version } from '$lib/stores';
	import MarkdownInput from '../MarkdownInput.svelte';
	import Select from '../Select.svelte';
	import CompositeRuntimeForm from '../mcp/CompositeRuntimeForm.svelte';
	import ContainerizedRuntimeForm from '../mcp/ContainerizedRuntimeForm.svelte';
	import CustomConfigurationForm from '../mcp/CustomConfigurationForm.svelte';
	import MultiUserHeadersForm from '../mcp/MultiUserHeadersForm.svelte';
	import NpxRuntimeForm from '../mcp/NpxRuntimeForm.svelte';
	import RemoteRuntimeForm from '../mcp/RemoteRuntimeForm.svelte';
	import ResourceRuntimeForm from '../mcp/ResourceRuntimeForm.svelte';
	import RuntimeSelector from '../mcp/RuntimeSelector.svelte';
	import UvxRuntimeForm from '../mcp/UvxRuntimeForm.svelte';
	import SelectMcpAccessControlRules from './SelectMcpAccessControlRules.svelte';
	import { Info } from '@lucide/svelte';
	import { onMount, untrack, type Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		id?: string;
		entity?: 'workspace' | 'catalog';
		entry?: MCPCatalogEntry | MCPCatalogServer;
		type?: LaunchServerType;
		readonly?: boolean;
		onCancel?: () => void;
		onSubmit?: (data: MCPCatalogEntry | MCPCatalogServer, message?: string) => void;
		hideTitle?: boolean;
		readonlyMessage?: Snippet;
		onConfigureOAuth?: () => void;
	}

	function getType(entry?: MCPCatalogEntry | MCPCatalogServer) {
		if (!entry) return undefined;
		if (entry.type === 'mcpserver') {
			return 'multi';
		} else {
			// For catalog entries, determine type based on runtime
			const catalogEntry = entry as MCPCatalogEntry;
			return catalogEntry.manifest.runtime === 'composite'
				? 'composite'
				: catalogEntry.manifest.runtime === 'remote'
					? 'remote'
					: 'hosted';
		}
	}

	let {
		id,
		entity = 'catalog',
		entry,
		readonly,
		type: newType = 'hosted',
		onCancel,
		onSubmit,
		readonlyMessage,
		onConfigureOAuth
	}: Props = $props();
	let type = $derived(getType(entry) ?? newType);

	let savedEntry = $state<MCPCatalogEntry | MCPCatalogServer>();
	let selectRulesDialog = $state<ReturnType<typeof SelectMcpAccessControlRules>>();
	let showRequired = $state<Record<string, boolean>>({});
	let loading = $state(false);
	let compositeHasToolNameErrors = $state(false);
	let mcpResourceDefaults = $state<MCPResourceRequirements>();

	let formData = $state<RuntimeFormData>(untrack(() => convertToFormData(entry)));

	const isAtLeastPowerUserPlus = $derived(profile.current?.groups.includes(Group.POWERUSER_PLUS));
	const showEgressDomains = $derived(!!version.current.mcpNetworkPolicyEnabled);
	const secretBoundHeaders = $derived(
		(formData.remoteConfig?.headers ?? []).filter((h) => hasSecretBinding(h))
	);
	const defaultDenyAllEgress = $derived(!!version.current.mcpDefaultDenyAllEgress);

	function defaultNpxConfig() {
		return { package: '', args: [], egressDomains: [], denyAllEgress: undefined };
	}

	function defaultUvxConfig() {
		return { package: '', command: '', args: [], egressDomains: [], denyAllEgress: undefined };
	}

	function defaultResourceRuntimeConfig() {
		return {
			requests: { cpu: '', memory: '' },
			limits: { cpu: '', memory: '' }
		};
	}

	function defaultContainerizedConfig() {
		return {
			image: '',
			port: 0,
			path: '',
			healthzPath: '',
			command: '',
			args: [],
			egressDomains: [],
			denyAllEgress: undefined
		};
	}

	function normalizeNpxConfig(config?: RuntimeFormData['npxConfig']) {
		return config ? { ...defaultNpxConfig(), ...config } : defaultNpxConfig();
	}

	function normalizeUvxConfig(config?: RuntimeFormData['uvxConfig']) {
		return config ? { ...defaultUvxConfig(), ...config } : defaultUvxConfig();
	}

	function normalizeContainerizedConfig(config?: RuntimeFormData['containerizedConfig']) {
		return config ? { ...defaultContainerizedConfig(), ...config } : defaultContainerizedConfig();
	}

	function normalizeResourceRuntimeConfig(config?: RuntimeFormData['resources']) {
		return config
			? { ...defaultResourceRuntimeConfig(), ...config }
			: defaultResourceRuntimeConfig();
	}

	function convertToFormData(item?: MCPCatalogEntry | MCPCatalogServer): RuntimeFormData {
		if (!item) {
			// Default initialization for new servers
			const isHostedType = type === 'hosted';
			return {
				categories: [''],
				name: '',
				description: '',
				env: [],
				icon: '',
				serverUserType: isHostedType ? 'multiUser' : 'singleUser',
				runtime: 'npx' as Runtime,
				resources:
					type !== 'remote' && type !== 'composite' ? defaultResourceRuntimeConfig() : undefined,
				npxConfig: defaultNpxConfig(),
				uvxConfig: undefined,
				containerizedConfig: undefined,
				remoteConfig: undefined,
				remoteServerConfig: undefined,
				compositeConfig: undefined,
				compositeServerConfig: undefined,
				multiUserConfig: isHostedType ? { userDefinedHeaders: [] } : undefined
			};
		}

		if (item.type === 'mcpserver') {
			// Handle MCPCatalogServer (multi-user servers)
			const server = item as MCPCatalogServer;
			const manifest = server.manifest;

			const formData: RuntimeFormData = {
				categories: manifest.metadata?.categories?.split(',').filter((c) => c.trim()) ?? [''],
				icon: manifest.icon ?? '',
				name: manifest.name ?? '',
				description: manifest.description ?? '',
				serverUserType: 'multiUser',
				env: manifest.env?.map((env) => ({ ...env, value: '' })) ?? [],
				runtime: manifest.runtime,
				resources: normalizeResourceRuntimeConfig(manifest.resources),
				npxConfig: undefined,
				uvxConfig: undefined,
				containerizedConfig: undefined,
				remoteConfig: undefined,
				remoteServerConfig: undefined,
				compositeConfig: undefined,
				compositeServerConfig: undefined,
				multiUserConfig: manifest.multiUserConfig ?? { userDefinedHeaders: [] }
			};

			// Initialize the appropriate runtime config based on the runtime type
			switch (manifest.runtime) {
				case 'npx':
					formData.npxConfig = normalizeNpxConfig(manifest.npxConfig);
					formData.startupTimeoutSeconds = manifest.npxConfig?.startupTimeoutSeconds;
					break;
				case 'uvx':
					formData.uvxConfig = normalizeUvxConfig(manifest.uvxConfig);
					formData.startupTimeoutSeconds = manifest.uvxConfig?.startupTimeoutSeconds;
					break;
				case 'containerized':
					formData.containerizedConfig = normalizeContainerizedConfig(manifest.containerizedConfig);
					formData.startupTimeoutSeconds = manifest.containerizedConfig?.startupTimeoutSeconds;
					break;
				case 'remote':
					formData.remoteServerConfig = manifest.remoteConfig
						? {
								url: manifest.remoteConfig.url,
								headers: manifest.remoteConfig.headers?.map((h) => ({ ...h, value: '' })) ?? []
							}
						: { url: '', headers: [] };
					break;
			}

			return formData;
		} else {
			// Handle MCPCatalogEntry (catalog entries)
			const entry = item as MCPCatalogEntry;
			const manifest = entry.manifest;

			const formData: RuntimeFormData = {
				categories: manifest.metadata?.categories?.split(',').filter((c) => c.trim()) ?? [''],
				name: manifest.name ?? '',
				icon: manifest.icon ?? '',
				env: manifest.env?.map((env) => ({ ...env, value: env.value ?? '' })) ?? [],
				description: manifest.description ?? '',
				serverUserType: manifest.serverUserType,
				runtime: manifest.runtime,
				resources:
					manifest.runtime !== 'composite'
						? normalizeResourceRuntimeConfig(manifest.resources)
						: undefined,
				npxConfig: undefined,
				uvxConfig: undefined,
				containerizedConfig: undefined,
				remoteConfig: undefined,
				remoteServerConfig: undefined,
				multiUserConfig:
					manifest.serverUserType === 'multiUser'
						? (manifest.multiUserConfig ?? { userDefinedHeaders: [] })
						: undefined
			};

			// Initialize the appropriate runtime config based on the runtime type
			switch (manifest.runtime) {
				case 'npx':
					formData.npxConfig = normalizeNpxConfig(manifest.npxConfig);
					formData.startupTimeoutSeconds = manifest.npxConfig?.startupTimeoutSeconds;
					break;
				case 'uvx':
					formData.uvxConfig = normalizeUvxConfig(manifest.uvxConfig);
					formData.startupTimeoutSeconds = manifest.uvxConfig?.startupTimeoutSeconds;
					break;
				case 'containerized':
					formData.containerizedConfig = normalizeContainerizedConfig(manifest.containerizedConfig);
					formData.startupTimeoutSeconds = manifest.containerizedConfig?.startupTimeoutSeconds;
					break;
				case 'remote':
					formData.remoteConfig = manifest.remoteConfig || { fixedURL: '', headers: [] };
					break;
				case 'composite':
					formData.compositeConfig = manifest.compositeConfig || { componentServers: [] };
					break;
			}

			return formData;
		}
	}

	async function revealCatalogServer(id: string, entryId: string, entity: 'workspace' | 'catalog') {
		try {
			const revealFn =
				entity === 'workspace'
					? UserService.revealWorkspaceMCPCatalogServer
					: AdminService.revealMcpCatalogServer;
			const response = await revealFn(id, entryId);

			// Update environment variables with revealed values
			if (formData.env) {
				formData.env = formData.env.map((env) => ({
					...env,
					value: response[env.key] ?? ''
				}));
			}

			// Update headers in the appropriate runtime config based on runtime type
			if (formData.runtime === 'remote') {
				if (formData.remoteConfig?.headers) {
					formData.remoteConfig.headers = formData.remoteConfig.headers.map((header) => ({
						...header,
						value: response[header.key] ?? ''
					}));
				}
				if (formData.remoteServerConfig?.headers) {
					formData.remoteServerConfig.headers = formData.remoteServerConfig.headers.map(
						(header) => ({
							...header,
							value: response[header.key] ?? ''
						})
					);
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

	// Runtime change handler
	function handleRuntimeChange(newRuntime: Runtime) {
		formData.runtime = newRuntime;

		// Clear all runtime configs first
		formData.npxConfig = undefined;
		formData.uvxConfig = undefined;
		formData.containerizedConfig = undefined;
		formData.remoteConfig = undefined;
		formData.remoteServerConfig = undefined;

		if (newRuntime === 'remote' || newRuntime === 'composite') {
			formData.resources = undefined;
		} else if (!formData.resources) {
			formData.resources = defaultResourceRuntimeConfig();
		}

		// Initialize the appropriate config based on the new runtime
		switch (newRuntime) {
			case 'npx':
				formData.npxConfig = defaultNpxConfig();
				break;
			case 'uvx':
				formData.uvxConfig = defaultUvxConfig();
				break;
			case 'containerized':
				formData.containerizedConfig = defaultContainerizedConfig();
				break;
			case 'remote':
				// For remote servers (catalog entries), use remoteConfig
				formData.remoteConfig = { fixedURL: '', headers: [] };
				break;
			case 'composite':
				formData.compositeConfig = { componentServers: [] };
				break;
		}
	}

	onMount(() => {
		if ((type === 'multi' || type === 'remote') && entry && id) {
			revealCatalogServer(id, entry.id, entity);
		}
		if (version.current.engine === 'kubernetes') {
			UserService.getK8sResourceDefaults()
				.then((defaults) => {
					mcpResourceDefaults = defaults;
				})
				.catch((err) => {
					console.error('Failed to load Kubernetes resource defaults:', err);
				});
		}
	});

	function convertToEntryManifest(formData: RuntimeFormData): MCPCatalogEntryServerManifest {
		const { categories, ...baseData } = formData;
		const startupTimeoutSeconds = baseData.startupTimeoutSeconds;
		const startupTimeoutConfig =
			typeof startupTimeoutSeconds === 'number' &&
			Number.isInteger(startupTimeoutSeconds) &&
			startupTimeoutSeconds > 0
				? { startupTimeoutSeconds }
				: {};

		const resources =
			baseData.runtime !== 'remote' && baseData.runtime !== 'composite'
				? sanitizeResourceRuntimeConfig(baseData.resources)
				: undefined;

		// Build base manifest structure
		const manifest: MCPCatalogEntryServerManifest = {
			name: baseData.name,
			description: baseData.description,
			icon: baseData.icon,
			env: baseData.env,
			runtime: baseData.runtime,
			serverUserType: baseData.serverUserType,
			multiUserConfig:
				baseData.serverUserType === 'multiUser' ? baseData.multiUserConfig : undefined,
			...(resources ? { resources } : {}),
			...convertCategoriesToMetadata(categories)
		};

		// Add runtime-specific config based on the runtime type
		switch (baseData.runtime) {
			case 'npx':
				if (baseData.npxConfig) {
					manifest.npxConfig = {
						package: baseData.npxConfig.package,
						args: baseData.npxConfig.args?.filter((arg) => arg.trim()) || [],
						egressDomains: sanitizeEgressDomains(baseData.npxConfig.egressDomains),
						denyAllEgress: baseData.npxConfig.denyAllEgress,
						...startupTimeoutConfig
					};
				}
				break;
			case 'uvx':
				if (baseData.uvxConfig) {
					manifest.uvxConfig = {
						package: baseData.uvxConfig.package,
						command: baseData.uvxConfig.command || undefined,
						args: baseData.uvxConfig.args?.filter((arg) => arg.trim()) || [],
						egressDomains: sanitizeEgressDomains(baseData.uvxConfig.egressDomains),
						denyAllEgress: baseData.uvxConfig.denyAllEgress,
						...startupTimeoutConfig
					};
				}
				break;
			case 'containerized':
				if (baseData.containerizedConfig) {
					manifest.containerizedConfig = {
						image: baseData.containerizedConfig.image,
						port: baseData.containerizedConfig.port,
						path: baseData.containerizedConfig.path,
						healthzPath: baseData.containerizedConfig.healthzPath?.trim() || undefined,
						command: baseData.containerizedConfig.command || undefined,
						args: baseData.containerizedConfig.args?.filter((arg) => arg.trim()) || [],
						egressDomains: sanitizeEgressDomains(baseData.containerizedConfig.egressDomains),
						denyAllEgress: baseData.containerizedConfig.denyAllEgress,
						...startupTimeoutConfig
					};
				}
				break;
			case 'remote':
				if (baseData.remoteConfig) {
					manifest.remoteConfig = {
						fixedURL: baseData.remoteConfig.fixedURL?.trim() || undefined,
						hostname: baseData.remoteConfig.hostname?.trim() || undefined,
						urlTemplate: baseData.remoteConfig.urlTemplate?.trim() || undefined,
						headers: baseData.remoteConfig.headers || [],
						staticOAuthRequired: baseData.remoteConfig.staticOAuthRequired
					};
				}
				break;
			case 'composite':
				if (baseData.compositeConfig) {
					manifest.compositeConfig = {
						componentServers: baseData.compositeConfig.componentServers
					};
				}
				break;
		}

		return manifest;
	}

	function omitSecretValuesFromServerManifest(
		serverManifest: ReturnType<typeof convertServerRuntimeFormDataToManifest>
	): ReturnType<typeof convertServerRuntimeFormDataToManifest> {
		const manifest = { ...serverManifest.manifest };
		if (manifest.env) {
			manifest.env = manifest.env.map(({ value: _value, ...rest }) => {
				return { value: '', ...rest };
			});
		}
		if (manifest.remoteConfig?.headers) {
			manifest.remoteConfig = {
				...manifest.remoteConfig,
				headers: manifest.remoteConfig.headers.map(({ value: _value, ...rest }) => {
					return { value: '', ...rest };
				})
			};
		}
		return { ...serverManifest, manifest };
	}

	async function handleEntrySubmit(id: string) {
		const manifest = convertToEntryManifest(formData);

		let response: MCPCatalogEntry;
		if (entry) {
			const updateEntryFn =
				entity === 'workspace'
					? UserService.updateWorkspaceMCPCatalogEntry
					: AdminService.updateMCPCatalogEntry;
			response = await updateEntryFn(id, entry.id, manifest);
		} else {
			const createEntryFn =
				entity === 'workspace'
					? UserService.createWorkspaceMCPCatalogEntry
					: AdminService.createMCPCatalogEntry;
			response = await createEntryFn(id, manifest);
		}

		// TODO: header fixed values
		return response;
	}

	async function handleServerSubmit(id: string) {
		const serverManifest = convertServerRuntimeFormDataToManifest(formData);

		let response: MCPCatalogServer;
		if (entry) {
			const updateServerFn =
				entity === 'workspace'
					? UserService.updateWorkspaceMCPCatalogServer
					: AdminService.updateMCPCatalogServer;
			response = await updateServerFn(
				id,
				entry.id,
				omitSecretValuesFromServerManifest(serverManifest).manifest
			);
		} else {
			const createServerFn =
				entity === 'workspace'
					? UserService.createWorkspaceMCPCatalogServer
					: AdminService.createMCPCatalogServer;
			response = await createServerFn(id, omitSecretValuesFromServerManifest(serverManifest));
		}

		let configValues: Record<string, string> = {};

		// Add environment variables
		if (serverManifest.manifest.env) {
			const envValues = Object.fromEntries(
				serverManifest.manifest.env
					.filter((env) => env.key && env.value) // Only include env vars with both key and value
					.map((env) => [env.key, env.value])
			);
			configValues = { ...configValues, ...envValues };
		}

		// Add headers from remote config (only for remote runtime)
		if (
			serverManifest.manifest.runtime === 'remote' &&
			serverManifest.manifest.remoteConfig?.headers
		) {
			const headerValues = Object.fromEntries(
				serverManifest.manifest.remoteConfig.headers
					.filter((header) => header.key && header.value) // Only include headers with both key and value
					.map((header) => [header.key, header.value])
			);
			configValues = { ...configValues, ...headerValues };
		}

		// Configure the server with the collected values if any exist
		if (Object.keys(configValues).length > 0) {
			const configureFn =
				entity === 'workspace'
					? UserService.configureWorkspaceMCPCatalogServer
					: AdminService.configureMCPCatalogServer;
			await configureFn(id, response.id, configValues);
		}

		return response;
	}

	async function handleSubmit() {
		if (!id) return;

		showRequired = {}; // reset
		const missingRequiredFields = validateRuntimeForm(formData, type);
		if (Object.keys(missingRequiredFields).length > 0) {
			showRequired = missingRequiredFields;
			return;
		}

		loading = true;
		try {
			const handleFns = {
				hosted: handleEntrySubmit,
				multi: handleServerSubmit,
				remote: handleEntrySubmit,
				composite: handleEntrySubmit
			};
			const entryResponse = await handleFns[type]?.(id);
			savedEntry = entryResponse;

			// Check if OAuth config is needed - redirect to detail page first, then show modal there
			if (!entry && type === 'remote' && formData.remoteConfig?.staticOAuthRequired) {
				loading = false;
				onSubmit?.(entryResponse, 'requires-oauth-config');
				return;
			}

			const existingRules = isAtLeastPowerUserPlus
				? entity === 'workspace'
					? await UserService.listWorkspaceAccessControlRules(id)
					: await AdminService.listAccessControlRules()
				: [];
			const hasEverythingEveryoneRule = existingRules.some(
				(rule) =>
					rule.subjects?.some((s) => s.id === '*') && rule.resources?.some((r) => r.id === '*')
			);
			if (isAtLeastPowerUserPlus && !entry && !hasEverythingEveryoneRule) {
				await selectRulesDialog?.open();
				loading = false;
			} else {
				loading = false;
				formData = convertToFormData(entryResponse);
				onSubmit?.(entryResponse, 'Catalog entry updated successfully!');
			}
		} catch (error) {
			loading = false;
			throw error;
		}
	}

	function updateRequired(field: string) {
		delete showRequired[field];
	}
</script>

<div
	class="dark:bg-base-200 dark:border-base-400 bg-base-100 flex flex-col gap-8 rounded-lg border border-transparent p-4 shadow-sm"
>
	<div class="flex flex-col gap-8">
		{#if readonly && readonlyMessage}
			<div class="notification-info p-3 text-sm font-light">
				<div class="flex items-center gap-3">
					<Info class="size-6" />
					<div>
						{@render readonlyMessage()}
					</div>
				</div>
			</div>
		{/if}

		<div class="flex flex-col gap-1">
			<label
				for="name"
				class={twMerge('text-sm font-light capitalize', showRequired.name && 'error')}>Name</label
			>
			<input
				type="text"
				id="name"
				bind:value={formData.name}
				class={twMerge('text-input-filled dark:bg-base-100', showRequired.name && 'error')}
				disabled={readonly}
				oninput={() => {
					updateRequired('name');
				}}
			/>
		</div>

		<div class="flex flex-col gap-1">
			<label for="name" class="text-sm font-light capitalize"
				>Description <span class="text-muted-content text-xs">(Markdown syntax supported)</span
				></label
			>
			<MarkdownInput
				bind:value={formData.description}
				disabled={readonly}
				placeholder="Provide details about the MCP catalog entry."
			/>
		</div>

		<div class="flex flex-col gap-1">
			<label for="icon" class="text-sm font-light capitalize">Icon URL</label>
			<input
				type="text"
				id="icon"
				bind:value={formData.icon}
				class="text-input-filled dark:bg-base-100"
				disabled={readonly}
			/>
		</div>
	</div>
</div>

{#if type === 'hosted'}
	<div class="paper p-4">
		<h4 class="text-sm font-semibold">Server Tenancy</h4>

		<div class="notification-info">
			<div class="flex items-center gap-2">
				<Info class="size-4" />
				<div>
					<p class="text-xs font-light">
						Once the server tenancy has been set, it cannot be changed. In order to change the
						configuration, you must delete the server and create a new one.
					</p>
				</div>
			</div>
		</div>

		<div class="flex items-center gap-4">
			<label for="server-configuration-selector" class="text-sm font-light">Type</label>
			<div class="w-full">
				<Select
					id="server-configuration-selector"
					class="bg-base-200 dark:bg-base-100 dark:border-base-400 flex-1 border border-transparent shadow-none"
					options={[
						{ id: 'multiUser', label: 'Multi-tenant' },
						{ id: 'singleUser', label: 'Single-tenant' }
					]}
					selected={formData.serverUserType}
					onSelect={(option) => {
						formData.serverUserType = option.id as 'singleUser' | 'multiUser';
						formData.multiUserConfig =
							option.id === 'multiUser' ? { userDefinedHeaders: [] } : undefined;
					}}
					disabled={readonly || !!entry?.id}
				/>
			</div>
		</div>

		<p class="text-muted-content text-xs">
			Set tenancy to <i>Single-tenant</i> if each user should connect to their own private instance
			of the server. <br />
			<i>Multi-tenancy</i> has all users connect to the same server instance.
		</p>
	</div>
{/if}

<!-- Runtime Selection -->
<RuntimeSelector
	bind:runtime={formData.runtime}
	serverType={type}
	{readonly}
	onRuntimeChange={handleRuntimeChange}
/>

<!-- Runtime-specific Forms -->
{#if formData.runtime === 'npx' && formData.npxConfig}
	<NpxRuntimeForm
		bind:config={formData.npxConfig}
		{showEgressDomains}
		{defaultDenyAllEgress}
		bind:startupTimeoutSeconds={formData.startupTimeoutSeconds}
		{readonly}
		{showRequired}
		onFieldChange={updateRequired}
	/>
{:else if formData.runtime === 'uvx' && formData.uvxConfig}
	<UvxRuntimeForm
		bind:config={formData.uvxConfig}
		{showEgressDomains}
		{defaultDenyAllEgress}
		bind:startupTimeoutSeconds={formData.startupTimeoutSeconds}
		{readonly}
		{showRequired}
		onFieldChange={updateRequired}
	/>
{:else if formData.runtime === 'containerized' && formData.containerizedConfig}
	<ContainerizedRuntimeForm
		bind:config={formData.containerizedConfig}
		{showEgressDomains}
		{defaultDenyAllEgress}
		bind:startupTimeoutSeconds={formData.startupTimeoutSeconds}
		{readonly}
		{showRequired}
		onFieldChange={updateRequired}
	/>
{:else if formData.runtime === 'remote' && formData.remoteConfig}
	<RemoteRuntimeForm
		bind:config={formData.remoteConfig}
		{readonly}
		{showRequired}
		onFieldChange={updateRequired}
		isNewEntry={!entry}
		{onConfigureOAuth}
	>
		{#snippet afterHeaders()}
			{#if secretBoundHeaders.length > 0}
				<CustomConfigurationForm
					bind:config={formData.env}
					{readonly}
					serverUserType={formData.serverUserType}
					{secretBoundHeaders}
				/>
			{/if}
		{/snippet}
	</RemoteRuntimeForm>
{:else if formData.runtime === 'composite' && formData.compositeConfig}
	<CompositeRuntimeForm
		bind:config={formData.compositeConfig}
		bind:hasToolNameErrors={compositeHasToolNameErrors}
		{readonly}
		catalogId={id}
		id={entry?.id}
	/>
{/if}

{#if version.current.engine === 'kubernetes' && !['remote', 'composite'].includes(formData.runtime) && formData.resources}
	<ResourceRuntimeForm
		bind:config={formData.resources}
		{readonly}
		defaultResources={mcpResourceDefaults}
	/>
{/if}

<!-- Environment Variables Section -->
{#if !['remote', 'composite'].includes(formData.runtime)}
	<CustomConfigurationForm
		bind:config={formData.env}
		{readonly}
		serverUserType={formData.serverUserType}
		{secretBoundHeaders}
	/>
{/if}

{#if formData.serverUserType === 'multiUser' && formData.multiUserConfig}
	<MultiUserHeadersForm bind:headers={formData.multiUserConfig.userDefinedHeaders} {readonly} />
{/if}

{#if !readonly}
	<div
		class="bg-base-200 dark:bg-base-100 sticky bottom-0 left-0 flex w-[calc(100%+2em)] -translate-x-4 items-center justify-end gap-4 p-4 md:w-[calc(100%+4em)] md:-translate-x-8 md:px-8"
	>
		{#if Object.keys(showRequired).length > 0}
			<span class="text-sm font-medium text-error">Fill out all required fields</span>
		{/if}
		<button class="btn btn-secondary flex items-center gap-1" onclick={() => onCancel?.()}>
			Cancel
		</button>
		<button
			class="btn btn-primary flex items-center gap-1"
			onclick={handleSubmit}
			disabled={loading ||
				(formData.runtime === 'composite' &&
					(!formData.compositeConfig?.componentServers ||
						formData.compositeConfig.componentServers.length === 0 ||
						compositeHasToolNameErrors))}
		>
			{#if loading}
				<Loading class="size-4" />
			{:else}
				{entry ? 'Update' : 'Save'}
			{/if}
		</button>
	</div>
{/if}

<SelectMcpAccessControlRules
	bind:this={selectRulesDialog}
	entry={savedEntry}
	onSubmit={() => {
		if (savedEntry) {
			onSubmit?.(savedEntry, type);
		}
	}}
	{entity}
	{id}
/>
