<script lang="ts">
	import {
		type MCPCatalogEntry,
		type RuntimeFormData,
		type MCPCatalogEntryServerManifest,
		type MCPCatalogServerManifest
	} from '$lib/services/admin/types';
	import type { Runtime } from '$lib/services/chat/types';
	import { Info, LoaderCircle, Plus, Trash2 } from 'lucide-svelte';
	import RuntimeSelector from '../mcp/RuntimeSelector.svelte';
	import NpxRuntimeForm from '../mcp/NpxRuntimeForm.svelte';
	import UvxRuntimeForm from '../mcp/UvxRuntimeForm.svelte';
	import ContainerizedRuntimeForm from '../mcp/ContainerizedRuntimeForm.svelte';
	import RemoteRuntimeForm from '../mcp/RemoteRuntimeForm.svelte';
	import { AdminService, type MCPCatalogServer } from '$lib/services';
	import { getContext, onMount, tick, type Snippet } from 'svelte';
	import MarkdownInput from '../MarkdownInput.svelte';
	import SelectMcpAccessControlRules from './SelectMcpAccessControlRules.svelte';
	import { twMerge } from 'tailwind-merge';
	import CategorySelectInput from './CategorySelectInput.svelte';
	import { CONTEXT_KEY_MCP_SERVERS_CATEGORIES } from '$lib/context/admin/keys';

	interface Props {
		catalogId?: string;
		entry?: MCPCatalogEntry | MCPCatalogServer;
		type?: 'single' | 'multi' | 'remote';
		readonly?: boolean;
		onCancel?: () => void;
		onSubmit?: (id: string, type: 'single' | 'multi' | 'remote') => void;
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
			return catalogEntry.manifest.runtime === 'remote' ? 'remote' : 'single';
		}
	}

	let {
		catalogId,
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

	const categoriesContext = getContext(CONTEXT_KEY_MCP_SERVERS_CATEGORIES) as {
		readonly current: string[];
	};
	const categories = $derived(categoriesContext?.current ?? []);

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
				remoteServerConfig: undefined
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
			}

			return formData;
		}
	}
	let formData = $state<RuntimeFormData>(convertToFormData(entry));

	async function revealCatalogServer(catalogId: string, entryId: string) {
		try {
			const response = await AdminService.revealMcpCatalogServer(catalogId, entryId);

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
					// For remote catalog entries, either fixedURL or hostname is required
					if (
						!formData.remoteConfig?.fixedURL?.trim() &&
						!formData.remoteConfig?.hostname?.trim()
					) {
						missingFields.fixedURL = true;
						missingFields.hostname = true;
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
		if ((type === 'multi' || type === 'remote') && entry && catalogId) {
			revealCatalogServer(catalogId, entry.id);
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
						fixedURL: baseData.remoteConfig.fixedURL || undefined,
						hostname: baseData.remoteConfig.hostname || undefined,
						headers: baseData.remoteConfig.headers || []
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

	async function handleEntrySubmit(catalogId: string) {
		const manifest = convertToEntryManifest(formData);

		let response: MCPCatalogEntry;
		if (entry) {
			response = await AdminService.updateMCPCatalogEntry(catalogId, entry.id, manifest);
		} else {
			response = await AdminService.createMCPCatalogEntry(catalogId, manifest);
		}

		// TODO: header fixed values
		return response;
	}

	async function handleServerSubmit(catalogId: string) {
		const serverManifest = convertToServerManifest(formData);

		let response: MCPCatalogServer;
		if (entry) {
			response = await AdminService.updateMCPCatalogServer(
				catalogId,
				entry.id,
				serverManifest.manifest
			);
		} else {
			response = await AdminService.createMCPCatalogServer(catalogId, serverManifest);
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
			await AdminService.configureMCPCatalogServer(catalogId, response.id, configValues);
		}

		return response;
	}

	async function handleSubmit() {
		if (!catalogId) return;

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
				remote: handleEntrySubmit
			};
			const entryResponse = await handleFns[type]?.(catalogId);
			savedEntry = entryResponse;

			if (!entry) {
				await selectRulesDialog?.open();
				loading = false;
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

{#if !hideTitle}
	<h1 class="text-2xl font-semibold capitalize">
		{#if entry}
			{formData.name}
		{:else}
			Create {type} Server
		{/if}
	</h1>
{/if}

<div
	class="dark:bg-surface1 dark:border-surface3 flex flex-col gap-8 rounded-lg border border-transparent bg-white p-4 shadow-sm"
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
				class={twMerge('text-input-filled dark:bg-black', showRequired.name && 'error')}
				disabled={readonly}
				oninput={() => {
					updateRequired('name');
				}}
			/>
		</div>

		<div class="flex flex-col gap-1">
			<label for="name" class="text-sm font-light capitalize"
				>Description <span class="text-xs text-gray-400 dark:text-gray-600"
					>(Markdown syntax supported)</span
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
				class="text-input-filled dark:bg-black"
				disabled={readonly}
			/>
		</div>

		<div class="flex flex-col gap-1">
			<span class="text-sm font-light capitalize">Categories</span>
			<CategorySelectInput
				categories={formData.categories.join(',')}
				options={categories.map((d) => ({ label: d, id: d }))}
				{readonly}
				onDelete={() => formData.categories.splice(0, 1)}
				onCreate={async (category) => {
					await tick();

					formData.categories = [category, ...formData.categories].filter(Boolean);
				}}
				onUpdate={async (categories) => {
					formData.categories = categories
						.split(',')
						.map((c) => c.trim())
						.filter(Boolean);
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
{/if}

<!-- Environment Variables Section -->
{#if !readonly || (readonly && formData.env && formData.env.length > 0)}
	<div
		class="dark:bg-surface1 dark:border-surface3 flex flex-col gap-4 rounded-lg border border-transparent bg-white p-4 shadow-sm"
	>
		<h4 class="text-sm font-semibold">
			{type === 'single' ? 'User Supplied Configuration' : 'Configuration'}
		</h4>
		<p class="text-xs font-light text-gray-400 dark:text-gray-600">
			{type === 'single' ? 'User supplied config' : 'Config'} values will be available as environment
			variables in the MCP server and can be referenced in the runtime configuration using the syntax
			$KEY_NAME.
		</p>

		{#if formData.env}
			{#each formData.env as _, i (i)}
				<div
					class="dark:border-surface3 flex w-full items-center gap-4 rounded-lg border border-transparent bg-gray-50 p-4 dark:bg-gray-900"
				>
					{#if type === 'single'}
						<div class="flex w-full flex-col gap-4">
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
						</div>
					{:else}
						<div class="flex w-full flex-col gap-4">
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
								<input
									id={`env-value-${i}`}
									class="text-input-filled w-full"
									bind:value={formData.env[i].value}
									placeholder="e.g. 123abcdef456"
									disabled={readonly}
									type={formData.env[i].sensitive ? 'password' : 'text'}
								/>
							</div>
							<div>
								<label class="flex items-center gap-2">
									<input
										type="checkbox"
										bind:checked={formData.env[i].sensitive}
										disabled={readonly}
									/>
									<span class="text-sm">Sensitive</span>
								</label>
							</div>
						</div>
					{/if}

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
							sensitive: false
						})}
				>
					<Plus class="size-4" />
					{type === 'single' ? 'User Configuration' : 'Configuration'}
				</button>
			</div>
		{/if}
	</div>
{/if}

{#if !readonly}
	<div
		class="bg-surface1 sticky bottom-0 left-0 flex w-[calc(100%+2em)] -translate-x-4 items-center justify-end gap-4 p-4 md:w-[calc(100%+4em)] md:-translate-x-8 md:px-8 dark:bg-black"
	>
		{#if Object.keys(showRequired).length > 0}
			<span class="text-sm font-medium text-red-500">Fill out all required fields</span>
		{/if}
		<button class="button flex items-center gap-1" onclick={() => onCancel?.()}> Cancel </button>
		<button class="button-primary flex items-center gap-1" onclick={handleSubmit}>
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
/>
