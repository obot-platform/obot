<script lang="ts">
	import {
		type MCPCatalogEntry,
		type RuntimeFormData,
		type MCPCatalogEntryServerManifest,
		type MCPCatalogServerManifest,
		Group
	} from '$lib/services/admin/types';
	import type { Runtime } from '$lib/services/chat/types';
	import { Info, LoaderCircle, Plus, Trash2 } from 'lucide-svelte';
	import RuntimeSelector from '../mcp/RuntimeSelector.svelte';
	import NpxRuntimeForm from '../mcp/NpxRuntimeForm.svelte';
	import UvxRuntimeForm from '../mcp/UvxRuntimeForm.svelte';
	import CompositeRuntimeForm from '../mcp/CompositeRuntimeForm.svelte';
	import ContainerizedRuntimeForm from '../mcp/ContainerizedRuntimeForm.svelte';
	import RemoteRuntimeForm from '../mcp/RemoteRuntimeForm.svelte';
	import { AdminService, ChatService, type MCPCatalogServer } from '$lib/services';
	import { onMount, tick, type Snippet } from 'svelte';
	import MarkdownInput from '../MarkdownInput.svelte';
	import SelectMcpAccessControlRules from './SelectMcpAccessControlRules.svelte';
	import { twMerge } from 'tailwind-merge';
	import CategorySelectInput from './CategorySelectInput.svelte';
	import Select from '../Select.svelte';
	import { profile } from '$lib/stores';
	import { getAdminMcpServerAndEntries } from '$lib/context/admin/mcpServerAndEntries.svelte';

	interface Props {
		id?: string;
		entity?: 'workspace' | 'catalog';
		entry?: MCPCatalogEntry | MCPCatalogServer;
		type?: 'single' | 'multi' | 'remote' | 'composite';
		readonly?: boolean;
		onCancel?: () => void;
		onSubmit?: (id: string, type: 'single' | 'multi' | 'remote' | 'composite') => void;
		hideTitle?: boolean;
		readonlyMessage?: Snippet;
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
					: 'single';
		}
	}

	let {
		id,
		entity = 'catalog',
		entry,
		readonly,
		type: newType = 'single',
		onCancel,
		onSubmit,
		hideTitle,
		readonlyMessage
	}: Props = $props();
	let type = $derived(getType(entry) ?? newType);

	let savedEntry = $state<MCPCatalogEntry | MCPCatalogServer>();
	let selectRulesDialog = $state<ReturnType<typeof SelectMcpAccessControlRules>>();
	let showRequired = $state<Record<string, boolean>>({});
	let loading = $state(false);

	let formData = $state<RuntimeFormData>(convertToFormData(entry));

	let remoteCategories = $state<string[]>([]);

	let categories = $derived([...remoteCategories, ...(formData?.categories ?? [])]);
	const isAtLeastPowerUserPlus = $derived(profile.current?.groups.includes(Group.POWERUSER_PLUS));

	onMount(() => {
		if (!id || entity === 'workspace') return;
		// TODO: do we have categories for workspace catalog?
		AdminService.listCatalogCategories(id).then((res) => {
			remoteCategories = res;
		});
	});

	function convertToFormData(item?: MCPCatalogEntry | MCPCatalogServer): RuntimeFormData {
		if (!item) {
			// Default initialization for new servers
			return {
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
				remoteServerConfig: undefined,
				compositeConfig: undefined,
				compositeServerConfig: undefined
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
				env: manifest.env?.map((env) => ({ ...env, value: '' })) ?? [],
				runtime: manifest.runtime,
				npxConfig: undefined,
				uvxConfig: undefined,
				containerizedConfig: undefined,
				remoteConfig: undefined,
				remoteServerConfig: undefined,
				compositeConfig: undefined,
				compositeServerConfig: undefined
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
								url: manifest.remoteConfig.url,
								headers: manifest.remoteConfig.headers?.map((h) => ({ ...h, value: '' })) ?? []
							}
						: { url: '', headers: [] };
					break;
			}

			return formData;
		} else {
			// Handle MCPCatalogEntry (single-user servers)
			const entry = item as MCPCatalogEntry;
			const manifest = entry.manifest;

			const formData: RuntimeFormData = {
				categories: manifest.metadata?.categories?.split(',').filter((c) => c.trim()) ?? [''],
				name: manifest.name ?? '',
				icon: manifest.icon ?? '',
				env: manifest.env?.map((env) => ({ ...env, value: '' })) ?? [],
				description: manifest.description ?? '',
				runtime: manifest.runtime,
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
					? ChatService.revealWorkspaceMCPCatalogServer
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
			if (error instanceof Error && error.message.includes('404')) {
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

		// Initialize the appropriate config based on the new runtime
		switch (newRuntime) {
			case 'npx':
				formData.npxConfig = { package: '', args: [] };
				break;
			case 'uvx':
				formData.uvxConfig = { package: '', command: '', args: [] };
				break;
			case 'containerized':
				formData.containerizedConfig = {
					image: '',
					port: 0,
					path: '',
					command: '',
					args: []
				};
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

	// Form validation
	function validateForm(): Record<string, boolean> {
		let missingFields: Record<string, boolean> = {};
		// Basic validation - name is required
		if (!formData.name.trim()) {
			missingFields.name = true;
		}

		// Runtime-specific validation
		switch (formData.runtime) {
			case 'npx':
				if (!formData.npxConfig?.package?.trim()) {
					missingFields.package = true;
				}
				break;
			case 'uvx':
				if (!formData.uvxConfig?.package?.trim()) {
					missingFields.package = true;
				}
				break;
			case 'containerized':
				if (!formData.containerizedConfig?.image?.trim()) {
					missingFields.image = true;
				}
				if (!formData.containerizedConfig?.path?.trim()) {
					missingFields.path = true;
				}
				if ((formData.containerizedConfig?.port ?? 0) <= 0) {
					missingFields.port = true;
				}
				break;
			case 'remote':
				if (type === 'remote') {
					// For remote catalog entries, one of fixedURL, hostname, or urlTemplate is required
					if (
						!formData.remoteConfig?.fixedURL?.trim() &&
						!formData.remoteConfig?.hostname?.trim() &&
						!formData.remoteConfig?.urlTemplate?.trim()
					) {
						missingFields.fixedURL = true;
						missingFields.hostname = true;
						missingFields.urlTemplate = true;
					}
					break;
				} else {
					// For multi-user servers with remote runtime, URL is required
					if (!formData.remoteServerConfig?.url?.trim()) {
						missingFields.url = true;
					}
					break;
				}
			default:
				break;
		}

		return missingFields;
	}

	onMount(() => {
		if ((type === 'multi' || type === 'remote') && entry && id) {
			revealCatalogServer(id, entry.id, entity);
		}
	});

	function convertCategoriesToMetadata(categories: string[]) {
		const validCategories = categories.filter((c) => c);
		return validCategories
			? {
					metadata: {
						categories: validCategories.join(',')
					}
				}
			: undefined;
	}

	function convertToEntryManifest(formData: RuntimeFormData): MCPCatalogEntryServerManifest {
		const { categories, ...baseData } = formData;

		// Build base manifest structure
		const manifest: MCPCatalogEntryServerManifest = {
			name: baseData.name,
			description: baseData.description,
			icon: baseData.icon,
			env: baseData.env,
			runtime: baseData.runtime,
			...convertCategoriesToMetadata(categories)
		};

		// Add runtime-specific config based on the runtime type
		switch (baseData.runtime) {
			case 'npx':
				if (baseData.npxConfig) {
					manifest.npxConfig = {
						package: baseData.npxConfig.package,
						args: baseData.npxConfig.args?.filter((arg) => arg.trim()) || []
					};
				}
				break;
			case 'uvx':
				if (baseData.uvxConfig) {
					manifest.uvxConfig = {
						package: baseData.uvxConfig.package,
						command: baseData.uvxConfig.command || undefined,
						args: baseData.uvxConfig.args?.filter((arg) => arg.trim()) || []
					};
				}
				break;
			case 'containerized':
				if (baseData.containerizedConfig) {
					manifest.containerizedConfig = {
						image: baseData.containerizedConfig.image,
						port: baseData.containerizedConfig.port,
						path: baseData.containerizedConfig.path,
						command: baseData.containerizedConfig.command || undefined,
						args: baseData.containerizedConfig.args?.filter((arg) => arg.trim()) || []
					};
				}
				break;
			case 'remote':
				if (baseData.remoteConfig) {
					manifest.remoteConfig = {
						fixedURL: baseData.remoteConfig.fixedURL?.trim() || undefined,
						hostname: baseData.remoteConfig.hostname?.trim() || undefined,
						urlTemplate: baseData.remoteConfig.urlTemplate?.trim() || undefined,
						headers: baseData.remoteConfig.headers || []
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

	function convertToServerManifest(formData: RuntimeFormData): MCPCatalogServerManifest {
		const { categories, ...baseData } = formData;

		// Build base manifest structure for server
		const serverManifest: MCPCatalogServerManifest = {
			manifest: {
				name: baseData.name,
				description: baseData.description,
				icon: baseData.icon,
				env: baseData.env,
				runtime: baseData.runtime,
				...convertCategoriesToMetadata(categories)
			}
		};

		// Add runtime-specific config based on the runtime type
		switch (baseData.runtime) {
			case 'npx':
				if (baseData.npxConfig) {
					serverManifest.manifest.npxConfig = {
						package: baseData.npxConfig.package,
						args: baseData.npxConfig.args?.filter((arg) => arg.trim()) || []
					};
				}
				break;
			case 'uvx':
				if (baseData.uvxConfig) {
					serverManifest.manifest.uvxConfig = {
						package: baseData.uvxConfig.package,
						command: baseData.uvxConfig.command || undefined,
						args: baseData.uvxConfig.args?.filter((arg) => arg.trim()) || []
					};
				}
				break;
			case 'containerized':
				if (baseData.containerizedConfig) {
					serverManifest.manifest.containerizedConfig = {
						image: baseData.containerizedConfig.image,
						port: baseData.containerizedConfig.port,
						path: baseData.containerizedConfig.path,
						command: baseData.containerizedConfig.command || undefined,
						args: baseData.containerizedConfig.args?.filter((arg) => arg.trim()) || []
					};
				}
				break;
			case 'remote':
				if (baseData.remoteServerConfig) {
					serverManifest.manifest.remoteConfig = {
						url: baseData.remoteServerConfig.url,
						headers: baseData.remoteServerConfig.headers || []
					};
				}
				break;
		}

		return serverManifest;
	}

	async function handleEntrySubmit(id: string) {
		const manifest = convertToEntryManifest(formData);

		let response: MCPCatalogEntry;
		if (entry) {
			const updateEntryFn =
				entity === 'workspace'
					? ChatService.updateWorkspaceMCPCatalogEntry
					: AdminService.updateMCPCatalogEntry;
			response = await updateEntryFn(id, entry.id, manifest);
		} else {
			const createEntryFn =
				entity === 'workspace'
					? ChatService.createWorkspaceMCPCatalogEntry
					: AdminService.createMCPCatalogEntry;
			response = await createEntryFn(id, manifest);
		}

		// TODO: header fixed values
		return response;
	}

	async function handleServerSubmit(id: string) {
		const serverManifest = convertToServerManifest(formData);

		let response: MCPCatalogServer;
		if (entry) {
			const updateServerFn =
				entity === 'workspace'
					? ChatService.updateWorkspaceMCPCatalogServer
					: AdminService.updateMCPCatalogServer;
			response = await updateServerFn(id, entry.id, serverManifest.manifest);
		} else {
			const createServerFn =
				entity === 'workspace'
					? ChatService.createWorkspaceMCPCatalogServer
					: AdminService.createMCPCatalogServer;
			response = await createServerFn(id, serverManifest);
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
					? ChatService.configureWorkspaceMCPCatalogServer
					: AdminService.configureMCPCatalogServer;
			await configureFn(id, response.id, configValues);
		}

		return response;
	}

	async function handleSubmit() {
		if (!id) return;

		showRequired = {}; // reset
		const missingRequiredFields = validateForm();
		if (Object.keys(missingRequiredFields).length > 0) {
			showRequired = missingRequiredFields;
			return;
		}

		loading = true;
		try {
			const handleFns = {
				single: handleEntrySubmit,
				multi: handleServerSubmit,
				remote: handleEntrySubmit,
				composite: handleEntrySubmit
			};
			const entryResponse = await handleFns[type]?.(id);
			savedEntry = entryResponse;
			if (isAtLeastPowerUserPlus) {
				const existingRules =
					entity === 'workspace'
						? await ChatService.listWorkspaceAccessControlRules(id)
						: await AdminService.listAccessControlRules();
				const hasEverythingEveryoneRule = existingRules.some(
					(rule) =>
						rule.subjects?.some((s) => s.id === '*') && rule.resources?.some((r) => r.id === '*')
				);

				if (!entry && !hasEverythingEveryoneRule) {
					await selectRulesDialog?.open();
					loading = false;
				} else {
					loading = false;
					onSubmit?.(entryResponse.id, type);
				}
			} else {
				loading = false;
				onSubmit?.(entryResponse.id, type);
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
	class="dark:bg-surface1 dark:border-surface3 bg-background flex flex-col gap-8 rounded-lg border border-transparent p-4 shadow-sm"
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
				class={twMerge('text-input-filled dark:bg-background', showRequired.name && 'error')}
				disabled={readonly}
				oninput={() => {
					updateRequired('name');
				}}
			/>
		</div>

		<div class="flex flex-col gap-1">
			<label for="name" class="text-sm font-light capitalize"
				>Description <span class="text-on-surface1 text-xs">(Markdown syntax supported)</span
				></label
			>
			<MarkdownInput
				bind:value={formData.description}
				disabled={readonly}
				placeholder="Provide details about the MCP server."
			/>
		</div>

		<div class="flex flex-col gap-1">
			<label for="icon" class="text-sm font-light capitalize">Icon URL</label>
			<input
				type="text"
				id="icon"
				bind:value={formData.icon}
				class="text-input-filled dark:bg-background"
				disabled={readonly}
			/>
		</div>

		<div class="flex flex-col gap-1">
			<span class="text-sm font-light capitalize">Categories</span>
			<CategorySelectInput
				categories={formData.categories.join(',')}
				options={categories.map((d) => ({ label: d, id: d }))}
				{readonly}
				onCreate={async (category) => {
					await tick();

					formData.categories = [category, ...formData.categories].filter(Boolean);
				}}
				onUpdate={async (categories) => {
					formData.categories = [
						// Avoid duplicates
						...new Set(
							categories
								.split(',')
								.map((c) => c.trim())
								.filter(Boolean)
						)
					];
				}}
			/>
		</div>
	</div>
</div>

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
		{readonly}
		{showRequired}
		onFieldChange={updateRequired}
	/>
{:else if formData.runtime === 'uvx' && formData.uvxConfig}
	<UvxRuntimeForm
		bind:config={formData.uvxConfig}
		{readonly}
		{showRequired}
		onFieldChange={updateRequired}
	/>
{:else if formData.runtime === 'containerized' && formData.containerizedConfig}
	<ContainerizedRuntimeForm
		bind:config={formData.containerizedConfig}
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
	/>
{:else if formData.runtime === 'composite' && formData.compositeConfig}
	<CompositeRuntimeForm
		bind:config={formData.compositeConfig}
		{readonly}
		catalogId={id}
		mcpEntriesContextFn={getAdminMcpServerAndEntries}
		id={entry?.id}
	/>
{/if}

<!-- Environment Variables Section -->
{#if !['remote', 'composite'].includes(formData.runtime)}
	{#if !readonly || (readonly && formData.env && formData.env.length > 0)}
		<div
			class="dark:bg-surface1 dark:border-surface3 bg-background flex flex-col gap-4 rounded-lg border border-transparent p-4 shadow-sm"
		>
			<h4 class="text-sm font-semibold">
				{type === 'single' ? 'User Supplied Configuration' : 'Configuration'}
			</h4>

			{#if formData.env}
				{#each formData.env as _, i (i)}
					<div
						class="dark:border-surface3 flex w-full items-center gap-4 rounded-lg border border-transparent bg-gray-50 p-4 dark:bg-gray-900"
					>
						<div class="flex w-full flex-col gap-4">
							<div class="flex w-full flex-col gap-1">
								<label for={`env-type-${i}`} class="text-sm font-light">Type</label>
								<Select
									class="bg-surface1 dark:border-surface3 dark:bg-surface1 border border-transparent shadow-inner"
									classes={{
										root: 'flex grow'
									}}
									options={[
										{ label: 'Environment Variable', id: 'environment_variable_type' },
										{ label: 'File', id: 'file_type' }
									]}
									disabled={readonly}
									selected={formData.env[i].file ? 'file_type' : 'environment_variable_type'}
									onSelect={(option) => {
										if (option.id === 'file_type') {
											formData.env[i].file = true;
										} else {
											formData.env[i].file = false;
										}
									}}
									id={`env-type-${i}`}
								/>
							</div>

							<p class="text-on-surface1 text-xs font-light">
								{#if formData.env[i].file}
									The value {type === 'single' ? 'the user supplies' : 'you provide'} will be written
									to a file. An environment variable will be created using the name you specify in the
									Key field and its value will be the path to that file. This environment variable will
									be set inside your MCP server and you can reference it in the arguments section above
									using the syntax ${'{KEY_NAME}'}.
								{:else}
									{type === 'single' ? 'The value the user supplies' : 'The value you provide'} will
									be set as an environment variable using the name you specify in the Key field. This
									environment variable will be set inside your MCP server and you can reference it in
									the arguments section above using the syntax ${'{KEY_NAME}'}.
								{/if}
							</p>

							{#if type === 'single'}
								<p class="text-on-surface1 text-xs font-light">
									The Name and Description fields will be displayed to the user when configuring
									this server. The Key field will not.
								</p>
								<div class="flex w-full flex-col gap-1">
									<label for={`env-name-${i}`} class="text-sm font-light">Name</label>
									<input
										id={`env-name-${i}`}
										class="text-input-filled w-full"
										bind:value={formData.env[i].name}
										disabled={readonly}
									/>
								</div>
								<div class="flex w-full flex-col gap-1">
									<label for={`env-description-${i}`} class="text-sm font-light">Description</label>
									<input
										id={`env-description-${i}`}
										class="text-input-filled w-full"
										bind:value={formData.env[i].description}
										disabled={readonly}
									/>
								</div>
								<div class="flex w-full flex-col gap-1">
									<label for={`env-key-${i}`} class="text-sm font-light">Key</label>
									<input
										id={`env-key-${i}`}
										class="text-input-filled w-full"
										bind:value={formData.env[i].key}
										placeholder="e.g. CUSTOM_API_KEY"
										disabled={readonly}
									/>
								</div>
								<div class="flex gap-8">
									<label class="flex items-center gap-2">
										<input
											type="checkbox"
											bind:checked={formData.env[i].sensitive}
											disabled={readonly}
										/>
										<span class="text-sm">Sensitive</span>
									</label>
									<label class="flex items-center gap-2">
										<input
											type="checkbox"
											bind:checked={formData.env[i].required}
											disabled={readonly}
										/>
										<span class="text-sm">Required</span>
									</label>
								</div>
							{:else}
								<div class="flex w-full flex-col gap-1">
									<label for={`env-key-${i}`} class="text-sm font-light">Key</label>
									<input
										id={`env-key-${i}`}
										class="text-input-filled w-full"
										bind:value={formData.env[i].key}
										placeholder="e.g. CUSTOM_API_KEY"
										disabled={readonly}
									/>
								</div>
								<div class="flex w-full flex-col gap-1">
									<label for={`env-value-${i}`} class="text-sm font-light">Value</label>
									{#if formData.env[i].file}
										<textarea
											id={`env-value-${i}`}
											class="text-input-filled min-h-24 w-full resize-y"
											bind:value={formData.env[i].value}
											disabled={readonly}
											rows={formData.env[i].value.split('\n').length + 1}
										></textarea>
									{:else}
										<input
											id={`env-value-${i}`}
											class="text-input-filled w-full"
											bind:value={formData.env[i].value}
											placeholder="e.g. 123abcdef456"
											disabled={readonly}
											type={formData.env[i].sensitive ? 'password' : 'text'}
										/>
									{/if}
								</div>
								<div class="flex w-full gap-4">
									<label class="flex items-center gap-2">
										<input
											type="checkbox"
											bind:checked={formData.env[i].sensitive}
											disabled={readonly}
										/>
										<span class="text-sm">Sensitive</span>
									</label>
								</div>
							{/if}
						</div>

						{#if !readonly}
							<button
								class="icon-button"
								onclick={() => {
									formData.env.splice(i, 1);
								}}
							>
								<Trash2 class="size-4" />
							</button>
						{/if}
					</div>
				{/each}
			{/if}

			{#if !readonly}
				<div class="flex justify-end">
					<button
						class="button flex items-center gap-1 text-xs"
						onclick={() =>
							formData.env.push({
								key: '',
								description: '',
								name: '',
								value: '',
								required: false,
								sensitive: false,
								file: false
							})}
					>
						<Plus class="size-4" />
						{type === 'single' ? 'User Configuration' : 'Configuration'}
					</button>
				</div>
			{/if}
		</div>
	{/if}
{/if}

{#if !readonly}
	<div
		class="bg-surface1 dark:bg-background sticky bottom-0 left-0 flex w-[calc(100%+2em)] -translate-x-4 items-center justify-end gap-4 p-4 md:w-[calc(100%+4em)] md:-translate-x-8 md:px-8"
	>
		{#if Object.keys(showRequired).length > 0}
			<span class="text-sm font-medium text-red-500">Fill out all required fields</span>
		{/if}
		<button class="button flex items-center gap-1" onclick={() => onCancel?.()}> Cancel </button>
		<button
			class="button-primary flex items-center gap-1"
			onclick={handleSubmit}
			disabled={loading ||
				(formData.runtime === 'composite' &&
					(!formData.compositeConfig?.componentServers ||
						formData.compositeConfig.componentServers.length === 0))}
		>
			{#if loading}
				<LoaderCircle class="size-4 animate-spin" />
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
			onSubmit?.(savedEntry.id, type);
		}
	}}
	{entity}
	{id}
/>
