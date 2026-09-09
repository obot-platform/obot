<script lang="ts">
	import { CATALOG_SERVER_FIELD_IDS } from '$lib/constants';
	import { HttpError } from '$lib/errors';
	import { highlightFirstAvailableField } from '$lib/form';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		UserService,
		type MCPCatalogServer,
		type LaunchServerType,
		type MCPCatalogEntry,
		type MCPResourceRequirements,
		type MCPAllowedSecretBindingTarget,
		type MCPTunnel,
		type RuntimeFormData,
		type MCPCatalogEntryServerManifest,
		type Runtime,
		Group
	} from '$lib/services';
	import { MAX_CATALOG_ENTRY_SHORT_DESCRIPTION_LENGTH } from '$lib/services/user/constants';
	import { getManifestConfiguration } from '$lib/services/user/mcp';
	import {
		convertCategoriesToMetadata,
		convertServerRuntimeFormDataToManifest,
		hasSecretBinding,
		isKubernetesRuntimeBackend,
		sanitizeEgressDomains,
		sanitizeResourceRuntimeConfig,
		validateRuntimeForm
	} from '$lib/services/user/mcp';
	import { errors, profile, version } from '$lib/stores';
	import MarkdownInput from '../MarkdownInput.svelte';
	import CatalogConfigurationForm from '../mcp/CatalogConfigurationForm.svelte';
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
		canConfigureOAuthCredentials?: boolean;
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
		onConfigureOAuth,
		canConfigureOAuthCredentials = true
	}: Props = $props();
	function getType(entry?: MCPCatalogEntry | MCPCatalogServer) {
		if (!entry) return undefined;
		if (entry.type === 'mcpserver') {
			return 'multi';
		} else {
			// For catalog entries, determine type based on runtime
			const catalogEntry = entry as MCPCatalogEntry;
			return catalogEntry.manifest.runtime === 'remote' ? 'remote' : 'hosted';
		}
	}

	let type = $derived(getType(entry) ?? newType);
	let savedEntry = $state<MCPCatalogEntry | MCPCatalogServer>();
	let selectRulesDialog = $state<ReturnType<typeof SelectMcpAccessControlRules>>();
	let showRequired = $state<Record<string, boolean>>({});
	let showInvalid = $state<Record<string, boolean>>({});
	let loading = $state(false);
	let mcpResourceDefaults = $state<MCPResourceRequirements>();
	let mcpTunnels = $state<MCPTunnel[]>();
	let secretBindingTargets = $state<MCPAllowedSecretBindingTarget[]>();

	let formData = $state<RuntimeFormData>(untrack(() => convertToFormData(entry)));

	const isAtLeastPowerUserPlus = $derived(profile.current?.groups.includes(Group.POWERUSER_PLUS));
	const canConfigureTunnels = $derived(
		entity === 'catalog' && type === 'remote' && Boolean(profile.current.isAdmin?.())
	);
	const showEgressDomains = $derived(!!version.current.mcpNetworkPolicyEnabled);
	const secretBoundHeaders = $derived(
		(type === 'multi'
			? (formData.remoteServerConfig?.headers ?? [])
			: (formData.config ?? []).filter((field) => field.usage === 'header')
		).filter((h) => hasSecretBinding(h))
	);
	const secretBindingsSupported = $derived(isKubernetesRuntimeBackend(version.current.engine));
	const canEditSecretBindings = $derived(
		secretBindingsSupported &&
			entity === 'catalog' &&
			profile.current?.isAdmin?.() &&
			!readonly &&
			(type === 'multi' || entity === 'catalog')
	);
	const editableSecretBindingTargets = $derived(
		canEditSecretBindings ? secretBindingTargets : undefined
	);
	const defaultDenyAllEgress = $derived(!!version.current.mcpDefaultDenyAllEgress);
	const shortDescriptionError = $derived(
		showRequired.shortDescription
			? 'Short description is required'
			: `Must be less than or equal to ${MAX_CATALOG_ENTRY_SHORT_DESCRIPTION_LENGTH} characters`
	);
	const hasShortDescriptionError = $derived(
		showRequired.shortDescription ?? showInvalid.shortDescription
	);

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
				metadata: undefined,
				name: '',
				shortDescription: '',
				description: '',
				env: [],
				icon: '',
				serverUserType: isHostedType && entity === 'catalog' ? 'multiUser' : 'singleUser',
				runtime: 'npx' as Runtime,
				config: type !== 'multi' ? [] : undefined,
				resources: type !== 'remote' ? defaultResourceRuntimeConfig() : undefined,
				npxConfig: defaultNpxConfig(),
				uvxConfig: undefined,
				containerizedConfig: undefined,
				remoteConfig: undefined,
				remoteServerConfig: undefined,
				multiUserConfig: isHostedType ? { userDefinedHeaders: [] } : undefined
			};
		}

		if (item.type === 'mcpserver') {
			// Handle MCPCatalogServer (multi-user servers)
			const server = item as MCPCatalogServer;
			const manifest = server.manifest;

			const formData: RuntimeFormData = {
				categories: manifest.metadata?.categories?.split(',').filter((c) => c.trim()) ?? [''],
				metadata: manifest.metadata,
				icon: manifest.icon ?? '',
				name: manifest.name ?? '',
				shortDescription: manifest.shortDescription ?? '',
				description: manifest.description ?? '',
				serverUserType: 'multiUser',
				env: getManifestConfiguration(manifest).env?.map((env) => ({ ...env, value: '' })) ?? [],
				runtime: manifest.runtime,
				resources: normalizeResourceRuntimeConfig(manifest.resources),
				npxConfig: undefined,
				uvxConfig: undefined,
				containerizedConfig: undefined,
				remoteConfig: undefined,
				remoteServerConfig: undefined,
				multiUserConfig: {
					userDefinedHeaders: (manifest.config ?? []).filter(
						(field) => field.usage === 'header' && field.userAllowed
					)
				}
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
								headers:
									getManifestConfiguration(manifest).headers.map((h) => ({ ...h, value: '' })) ??
									[],
								tunnelName: manifest.remoteConfig.tunnelName
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
				metadata: manifest.metadata,
				name: manifest.name ?? '',
				icon: manifest.icon ?? '',
				shortDescription: manifest.shortDescription ?? '',
				env: [],
				config: manifest.config?.map((config) => ({ ...config, value: config.value ?? '' })) ?? [],
				description: manifest.description ?? '',
				serverUserType: 'multiUser',
				runtime: manifest.runtime,
				resources: normalizeResourceRuntimeConfig(manifest.resources),
				npxConfig: undefined,
				uvxConfig: undefined,
				containerizedConfig: undefined,
				remoteConfig: undefined,
				remoteServerConfig: undefined,
				multiUserConfig: undefined
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
					formData.remoteConfig = manifest.remoteConfig || { fixedURL: '' };
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

		if (newRuntime === 'remote') {
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
				if (type === 'multi') {
					formData.remoteServerConfig = { url: '', headers: [] };
				} else {
					formData.remoteConfig = { fixedURL: '', headers: [] };
				}
				break;
		}
	}

	function loadSecretBindingTargets() {
		AdminService.listMCPSecretBindingTargets({ dontLogErrors: true })
			.then((targets) => {
				secretBindingTargets = targets;
			})
			.catch((err) => {
				secretBindingTargets = [];
				errors.append(`Failed to load Kubernetes Secrets for binding: ${err}`);
			});
	}

	function stripSecretBindingSource<T extends object>(field: T) {
		const rest = { ...field } as T & {
			secretBindingSource?: string;
			userAllowed?: boolean;
		};
		delete rest.secretBindingSource;
		delete rest.userAllowed;
		return rest;
	}

	function hasDuplicateConfigKeys() {
		const keys = (formData.config ?? []).map(({ key }) => key.trim()).filter(Boolean);
		return new Set(keys).size !== keys.length;
	}

	onMount(() => {
		if (entry?.type === 'mcpserver' && id) {
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
		if (canEditSecretBindings) {
			loadSecretBindingTargets();
		}
		if (canConfigureTunnels) {
			AdminService.listMCPTunnels()
				.then((tunnels) => {
					mcpTunnels = tunnels;
				})
				.catch((err) => {
					mcpTunnels = [];
					console.error('Failed to load MCP tunnels:', err);
				});
		}
	});

	function convertToEntryManifest(formData: RuntimeFormData): MCPCatalogEntryServerManifest {
		const { categories, metadata, ...baseData } = formData;
		const startupTimeoutSeconds = baseData.startupTimeoutSeconds;
		const startupTimeoutConfig =
			typeof startupTimeoutSeconds === 'number' &&
			Number.isInteger(startupTimeoutSeconds) &&
			startupTimeoutSeconds > 0
				? { startupTimeoutSeconds }
				: {};

		const resources =
			baseData.runtime !== 'remote' ? sanitizeResourceRuntimeConfig(baseData.resources) : undefined;

		// Build base manifest structure
		const manifest: MCPCatalogEntryServerManifest = {
			name: baseData.name,
			description: baseData.description,
			...(baseData.shortDescription !== undefined
				? { shortDescription: baseData.shortDescription }
				: {}),
			icon: baseData.icon,
			config: baseData.config?.map(stripSecretBindingSource),
			runtime: baseData.runtime,
			...(resources ? { resources } : {}),
			...convertCategoriesToMetadata(categories, metadata)
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
						staticOAuthRequired: baseData.remoteConfig.staticOAuthRequired,
						tunnelName: baseData.remoteConfig.tunnelName?.trim() || undefined
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
		if (manifest.config) {
			manifest.config = manifest.config.map(({ value: _value, ...rest }) => {
				return { value: '', ...rest };
			});
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

		const configValues = Object.fromEntries(
			(serverManifest.manifest.config ?? [])
				.filter((field) => !field.userAllowed && field.key && field.value)
				.map((field) => [field.key, field.value])
		);

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

		// reset
		showRequired = {};
		showInvalid = {};

		const validationData =
			type !== 'multi' ? { ...formData, env: formData.config ?? [] } : formData;
		const { required, invalid } = validateRuntimeForm(validationData, type);
		if (type !== 'multi' && hasDuplicateConfigKeys()) {
			invalid.env = true;
		}
		if (Object.keys(required).length > 0 || Object.keys(invalid).length > 0) {
			showRequired = required;
			showInvalid = invalid;
			highlightFirstAvailableField({ ...required, ...invalid }, CATALOG_SERVER_FIELD_IDS);
			return;
		}

		loading = true;
		try {
			const handleFns = {
				hosted: handleEntrySubmit,
				multi: handleServerSubmit,
				remote: handleEntrySubmit
			};
			const entryResponse = await handleFns[type](id);
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
			const showSetAccessPolicy = isAtLeastPowerUserPlus && !entry && !hasEverythingEveryoneRule;
			if (showSetAccessPolicy) {
				await selectRulesDialog?.open();
				loading = false;
				return;
			}
			loading = false;
			formData = convertToFormData(entryResponse);
			onSubmit?.(entryResponse, 'Catalog entry updated successfully!');
		} catch (error) {
			loading = false;
			throw error;
		}
	}

	function updateRequired(field: string) {
		delete showRequired[field];
	}

	function updateInvalid(field: string) {
		delete showInvalid[field];
	}

	function handleFormSubmit(e: SubmitEvent) {
		e.preventDefault();
		if (
			e.submitter instanceof HTMLElement &&
			e.submitter.getAttribute('data-form-action') !== 'save'
		) {
			return;
		}
		handleSubmit();
	}
</script>

<form
	class="flex flex-col gap-8"
	novalidate
	onsubmit={handleFormSubmit}
	aria-describedby={Object.keys(showRequired).length > 0
		? CATALOG_SERVER_FIELD_IDS.formError
		: undefined}
>
	<section class="paper p-4" id={CATALOG_SERVER_FIELD_IDS.serverFormDetails}>
		<div class="flex flex-col gap-8">
			{#if readonly && readonlyMessage}
				<div class="notification-info p-3 text-sm font-light" role="status">
					<div class="flex items-center gap-3">
						<Info class="size-6" aria-hidden="true" />
						<div>
							{@render readonlyMessage()}
						</div>
					</div>
				</div>
			{/if}

			<div class="flex flex-col gap-1" id={`${CATALOG_SERVER_FIELD_IDS.name}-container`}>
				<label
					for={CATALOG_SERVER_FIELD_IDS.name}
					class={twMerge('text-sm font-light capitalize', showRequired.name && 'error')}
				>
					Name
					{#if !readonly}
						<span class={showRequired.name ? 'text-error' : ''} aria-hidden="true">*</span>
						<span class="sr-only">(required)</span>
					{/if}
				</label>
				<input
					type="text"
					id={CATALOG_SERVER_FIELD_IDS.name}
					name="name"
					bind:value={formData.name}
					class={twMerge('text-input-filled dark:bg-base-100', showRequired.name && 'error')}
					disabled={readonly}
					aria-required={!readonly ? 'true' : undefined}
					aria-invalid={showRequired.name ? 'true' : undefined}
					aria-describedby={showRequired.name ? CATALOG_SERVER_FIELD_IDS.nameError : undefined}
					oninput={() => {
						updateRequired('name');
					}}
				/>
				{#if showRequired.name}
					<p id={CATALOG_SERVER_FIELD_IDS.nameError} class="text-xs text-error" role="alert">
						Name is required
					</p>
				{/if}
			</div>

			<div class="flex flex-col gap-1" id={`${CATALOG_SERVER_FIELD_IDS.description}-container`}>
				<span id={CATALOG_SERVER_FIELD_IDS.description} class="text-sm font-light capitalize">
					Description
					<span id={CATALOG_SERVER_FIELD_IDS.descriptionHint} class="text-muted-content text-xs">
						(Markdown syntax supported)
					</span>
				</span>
				<MarkdownInput
					bind:value={formData.description}
					disabled={readonly}
					placeholder="Provide details about the MCP catalog entry."
					labelledBy={CATALOG_SERVER_FIELD_IDS.description}
					describedBy={CATALOG_SERVER_FIELD_IDS.descriptionHint}
				/>
			</div>

			<div
				class="flex flex-col gap-1"
				id={`${CATALOG_SERVER_FIELD_IDS.shortDescription}-container`}
			>
				<label
					for={CATALOG_SERVER_FIELD_IDS.shortDescription}
					class={twMerge('text-sm font-light capitalize', hasShortDescriptionError && 'error')}
				>
					Short Description
					{#if !readonly}
						<span class={hasShortDescriptionError ? 'text-error' : ''} aria-hidden="true">*</span>
						<span class="sr-only">(required)</span>
					{/if}
					<span
						id={CATALOG_SERVER_FIELD_IDS.shortDescriptionHint}
						class="text-muted-content text-xs"
					>
						(max {MAX_CATALOG_ENTRY_SHORT_DESCRIPTION_LENGTH} characters)
					</span>
				</label>
				<input
					type="text"
					id={CATALOG_SERVER_FIELD_IDS.shortDescription}
					name="shortDescription"
					bind:value={formData.shortDescription}
					class={twMerge('text-input-filled dark:bg-base-100', hasShortDescriptionError && 'error')}
					disabled={readonly}
					placeholder="Provide a brief summary that will be shown in catalog listings."
					maxlength={MAX_CATALOG_ENTRY_SHORT_DESCRIPTION_LENGTH}
					aria-required={!readonly ? 'true' : undefined}
					aria-describedby={`${CATALOG_SERVER_FIELD_IDS.shortDescriptionHint} ${CATALOG_SERVER_FIELD_IDS.shortDescriptionCount}`}
					aria-invalid={hasShortDescriptionError ? 'true' : undefined}
					aria-errormessage={hasShortDescriptionError
						? CATALOG_SERVER_FIELD_IDS.shortDescriptionError
						: undefined}
					oninput={() => {
						updateInvalid('shortDescription');
						updateRequired('shortDescription');
					}}
				/>
				<div class="flex justify-between gap-4">
					{#if hasShortDescriptionError}
						<p
							id={CATALOG_SERVER_FIELD_IDS.shortDescriptionError}
							class="text-xs text-error"
							role="alert"
						>
							{shortDescriptionError}
						</p>
					{:else}
						<p></p>
					{/if}
					<span
						id={CATALOG_SERVER_FIELD_IDS.shortDescriptionCount}
						class={twMerge(
							'pl-0.5 text-xs text-muted-content flex justify-end',
							showInvalid.shortDescription && 'text-error'
						)}
						aria-live="polite"
					>
						{Array.from(formData.shortDescription ?? '').length} / {MAX_CATALOG_ENTRY_SHORT_DESCRIPTION_LENGTH}
					</span>
				</div>
			</div>

			<div class="flex flex-col gap-1" id={`${CATALOG_SERVER_FIELD_IDS.icon}-container`}>
				<label for={CATALOG_SERVER_FIELD_IDS.icon} class="text-sm font-light capitalize"
					>Icon URL</label
				>
				<input
					type="text"
					id={CATALOG_SERVER_FIELD_IDS.icon}
					name="icon"
					bind:value={formData.icon}
					class="text-input-filled dark:bg-base-100"
					disabled={readonly}
					inputmode="url"
					autocomplete="off"
				/>
			</div>
		</div>
	</section>

	<!-- Runtime Selection -->
	<RuntimeSelector
		bind:runtime={formData.runtime}
		serverType={type}
		{readonly}
		onRuntimeChange={handleRuntimeChange}
	/>

	<!-- Runtime-specific Forms -->
	<div class="flex flex-col gap-8" id={`${CATALOG_SERVER_FIELD_IDS.runtimeConfiguration}`}>
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
		{:else if formData.runtime === 'remote' && type === 'multi' && formData.remoteServerConfig}
			<RemoteRuntimeForm
				bind:config={formData.remoteServerConfig}
				variant="server"
				{readonly}
				{showRequired}
				{showInvalid}
				onFieldChange={updateRequired}
				isNewEntry={!entry}
				{onConfigureOAuth}
				{canConfigureOAuthCredentials}
				secretBindingTargets={editableSecretBindingTargets}
			>
				{#snippet afterHeaders()}
					{#if secretBoundHeaders.length > 0}
						<CustomConfigurationForm
							bind:config={formData.env}
							{readonly}
							serverUserType={formData.serverUserType}
							{secretBoundHeaders}
							showRequired={showRequired.env}
							showInvalid={showInvalid.env}
						/>
					{/if}
				{/snippet}
			</RemoteRuntimeForm>
		{:else if formData.runtime === 'remote' && formData.remoteConfig}
			<RemoteRuntimeForm
				bind:config={formData.remoteConfig}
				hideHeaders={type !== 'multi'}
				tunnels={canConfigureTunnels ? mcpTunnels : undefined}
				tunnelsLoading={canConfigureTunnels && mcpTunnels === undefined}
				{readonly}
				{showRequired}
				{showInvalid}
				onFieldChange={updateRequired}
				isNewEntry={!entry}
				{onConfigureOAuth}
				{canConfigureOAuthCredentials}
			/>
		{/if}
	</div>

	{#if version.current.engine === 'kubernetes' && formData.runtime !== 'remote' && formData.resources}
		<ResourceRuntimeForm
			bind:config={formData.resources}
			{readonly}
			defaultResources={mcpResourceDefaults}
		/>
	{/if}

	{#if type !== 'multi'}
		<CatalogConfigurationForm
			bind:config={formData.config}
			{readonly}
			secretBindingTargets={editableSecretBindingTargets}
			showRequired={showRequired.env}
			showInvalid={showInvalid.env}
		/>
	{:else if formData.runtime !== 'remote'}
		<CustomConfigurationForm
			bind:config={formData.env}
			{readonly}
			serverUserType={formData.serverUserType}
			{secretBoundHeaders}
			secretBindingTargets={editableSecretBindingTargets}
			showRequired={showRequired.env}
			showInvalid={showInvalid.env}
		/>
	{/if}

	{#if type === 'multi' && formData.multiUserConfig}
		<MultiUserHeadersForm bind:headers={formData.multiUserConfig.userDefinedHeaders} {readonly} />
	{/if}

	{#if !readonly}
		<div
			class="bg-base-200 dark:bg-base-100 sticky bottom-0 left-0 flex w-[calc(100%+2em)] -translate-x-4 items-center justify-end gap-4 p-4 md:w-[calc(100%+4em)] md:-translate-x-8 md:px-8"
		>
			{#if Object.keys(showRequired).length > 0}
				<span
					id={CATALOG_SERVER_FIELD_IDS.formError}
					class="text-sm font-medium text-error"
					role="alert"
					tabindex="-1"
				>
					Fill out all required fields
				</span>
			{/if}
			<button
				type="button"
				class="btn btn-secondary flex items-center gap-1"
				onclick={() => onCancel?.()}
				id={CATALOG_SERVER_FIELD_IDS.cancelBtn}
			>
				Cancel
			</button>
			<button
				type="submit"
				data-form-action="save"
				class="btn btn-primary flex items-center gap-1"
				disabled={loading}
				aria-busy={loading}
				id={CATALOG_SERVER_FIELD_IDS.submitBtn}
			>
				{#if loading}
					<span aria-hidden="true">
						<Loading class="size-4" />
					</span>
					<span class="sr-only">Saving</span>
				{:else}
					{entry ? 'Update' : 'Save'}
				{/if}
			</button>
		</div>
	{/if}
</form>

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
