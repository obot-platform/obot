<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import CompositeEditTools from '$lib/components/mcp/composite/CompositeEditTools.svelte';
	import { isMissingRequiredConfigurationField } from '$lib/components/mcp/configurationOptions';
	import Loading from '$lib/icons/Loading.svelte';
	import { UserService } from '$lib/services';
	import type {
		CompositeServerToolRow,
		MCPCatalogEntry,
		ToolOverride,
		VMCPComponent
	} from '$lib/services';
	import { toolOverridesFromRows } from '$lib/services/user/mcp';
	import { catalogConfigurationFields } from '$lib/services/vmcps/utils';
	import { onDestroy } from 'svelte';
	import type { Snippet } from 'svelte';
	import { fade } from 'svelte/transition';

	interface Props {
		component?: VMCPComponent;
		vmcpID?: string;
		refresh?: boolean;
		existingTools?: CompositeServerToolRow[];
		existingToolPrefix?: string;
		otherEffectiveNames?: string[];
		otherToolPrefixes?: string[];
		onCancel?: () => void;
		onSuccess?: (config: { toolOverrides: ToolOverride[]; toolPrefix: string }) => void;
		additionalActions?: Snippet;
	}

	let {
		component,
		vmcpID,
		refresh = false,
		existingTools = [],
		existingToolPrefix,
		otherEffectiveNames,
		otherToolPrefixes,
		onCancel,
		onSuccess,
		additionalActions: additionalActionsSnippet
	}: Props = $props();

	let setupDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let editDialog = $state<ReturnType<typeof CompositeEditTools>>();
	let tools = $state<CompositeServerToolRow[]>([]);
	let toolPrefix = $state('');
	type DialogPhase = 'closed' | 'setup' | 'editor';
	let dialogPhase = $state<DialogPhase>('closed');
	let loading = $state(false);
	let error = $state<string>();
	let oauthURL = $state<string>();
	let oauthValidating = $state(false);
	let listeningOauthVisibility = $state(false);
	let requestGeneration = 0;
	let requestController: AbortController | undefined;
	let previewConfig = $state<Record<string, string>>({});

	let configuringEntry = $derived<MCPCatalogEntry | undefined>(
		component
			? {
					id: component.id || component.mcpServerCatalogEntryID,
					created: new Date(0).toISOString(),
					manifest: component.catalogEntry.manifest,
					isCatalogEntry: true,
					type: 'catalog-entry',
					unsupportedTools: component.catalogEntry.unsupportedTools
				}
			: undefined
	);
	const userFields = $derived.by(() => {
		if (!configuringEntry) return [];
		const fields = [...catalogConfigurationFields(configuringEntry)];
		const remote = configuringEntry.manifest.remoteConfig;
		const requiresURL = remote?.hostname && !remote.fixedURL;
		if (requiresURL && !fields.some((field) => field.key === '__url')) {
			fields.push({
				key: '__url',
				name: 'Server URL',
				description: `URL must have hostname ${remote.hostname}`,
				usage: 'interpolated',
				required: true,
				sensitive: false,
				value: ''
			});
		}
		return fields.filter((field) => {
			const policy = component?.configuration?.find((policy) => policy.key === field.key);
			return policy ? policy.policy === 'userAllowed' : requiresURL && field.key === '__url';
		});
	});
	const needsLiveTools = $derived(refresh || tools.length === 0 || userFields.length > 0);
	const missingConfiguration = $derived(
		userFields.some((field) =>
			isMissingRequiredConfigurationField({ ...field, value: previewConfig[field.key] ?? '' })
		)
	);

	function componentID(value?: VMCPComponent) {
		return value?.id || value?.mcpServerCatalogEntryID || '';
	}

	function cancelToolPreviewRequest(preserveOauthState = false) {
		requestGeneration += 1;
		requestController?.abort();
		requestController = undefined;
		listeningOauthVisibility = false;
		if (!preserveOauthState) {
			oauthValidating = false;
		}
		loading = false;
	}

	function isCurrentRequest(generation: number, controller: AbortController) {
		return requestGeneration === generation && !controller.signal.aborted;
	}

	function mergePreviewTools(entry: MCPCatalogEntry): CompositeServerToolRow[] {
		const existingByName = new Map(existingTools.map((tool) => [tool.name, tool]));
		const hasStoredOverrides = Boolean(component?.toolOverrides?.length);
		const id = componentID(component);

		return (entry.manifest.toolPreview ?? []).map((previewTool) => {
			const existing = existingByName.get(previewTool.name);
			const description = existing?.description ?? previewTool.description;
			return {
				id: existing?.id ?? `${id}-${previewTool.id || previewTool.name}`,
				name: previewTool.name,
				description,
				overrideName: existing?.overrideName ?? previewTool.name,
				overrideDescription:
					existing?.overrideDescription ?? existing?.description ?? previewTool.description,
				enabled: existing?.enabled ?? !hasStoredOverrides
			};
		});
	}

	function handleVisibilityChange() {
		if (dialogPhase === 'setup' && document.visibilityState === 'visible' && oauthURL && !loading) {
			oauthValidating = true;
			void fetchLiveTools();
		}
	}

	$effect(() => {
		if (listeningOauthVisibility) {
			document.addEventListener('visibilitychange', handleVisibilityChange);
		}
		return () => {
			document.removeEventListener('visibilitychange', handleVisibilityChange);
		};
	});

	async function fetchLiveTools() {
		const id = componentID(component);
		if (!vmcpID || !id) {
			error = 'Unable to fetch tools for this vMCP component.';
			return;
		}

		cancelToolPreviewRequest(oauthValidating);
		const controller = new AbortController();
		requestController = controller;
		const generation = requestGeneration;
		loading = true;
		error = undefined;

		try {
			const entry = await UserService.generateVMCPComponentToolPreviews(vmcpID, id, {
				config: previewConfig,
				signal: controller.signal
			});
			if (!isCurrentRequest(generation, controller)) return;

			tools = mergePreviewTools(entry);
			loading = false;
			error = undefined;
			oauthURL = undefined;
			listeningOauthVisibility = false;
			oauthValidating = false;
			openEditor();
		} catch (err: unknown) {
			if (!isCurrentRequest(generation, controller)) return;

			const message = err instanceof Error ? err.message : String(err);
			if (message.includes('MCP server requires OAuth authentication')) {
				try {
					const nextOauthURL = await UserService.getVMCPComponentToolPreviewsOauth(vmcpID, id, {
						config: previewConfig,
						signal: controller.signal
					});
					if (!isCurrentRequest(generation, controller)) return;

					oauthURL = nextOauthURL;
					if (oauthURL) {
						error = undefined;
						listeningOauthVisibility = true;
					} else {
						error = message;
						listeningOauthVisibility = false;
					}
				} catch (oauthError: unknown) {
					if (!isCurrentRequest(generation, controller)) return;
					error = oauthError instanceof Error ? oauthError.message : message;
					oauthURL = undefined;
					listeningOauthVisibility = false;
				} finally {
					oauthValidating = false;
				}
			} else {
				error = message || 'Failed to fetch tools for this vMCP component.';
				oauthURL = undefined;
				listeningOauthVisibility = false;
				oauthValidating = false;
			}
		} finally {
			if (isCurrentRequest(generation, controller)) {
				loading = false;
				requestController = undefined;
			}
		}
	}

	function configureTools() {
		if (missingConfiguration) return;
		if (needsLiveTools && vmcpID && componentID(component)) {
			void fetchLiveTools();
			return;
		}
		openEditor();
	}

	export function open() {
		cancelToolPreviewRequest();
		previewConfig = {};
		error = undefined;
		oauthURL = undefined;
		tools = existingTools;
		toolPrefix = existingToolPrefix ?? component?.toolPrefix ?? '';
		dialogPhase = 'setup';
		setupDialog?.open();
		if (refresh && userFields.length === 0 && vmcpID && componentID(component)) {
			void fetchLiveTools();
		}
	}

	export function close() {
		cancelToolPreviewRequest();
		previewConfig = {};
		dialogPhase = 'closed';
		setupDialog?.close();
		editDialog?.close();
	}

	function cancel(from: Exclude<DialogPhase, 'closed'>) {
		// ResponsiveDialog invokes onClose from the native close event. Animated closes dispatch that
		// event later, so a setup dialog that is being handed off to the editor must not cancel the
		// flow when its close event arrives.
		if (dialogPhase !== from) return;
		dialogPhase = 'closed';
		close();
		onCancel?.();
	}

	function cancelSetup() {
		cancel('setup');
	}

	function cancelEditor() {
		cancel('editor');
	}

	function openEditor() {
		previewConfig = {};
		dialogPhase = 'editor';
		setupDialog?.close();
		editDialog?.open();
	}

	function save() {
		if (!component) {
			close();
			return;
		}
		cancelToolPreviewRequest();
		dialogPhase = 'closed';
		onSuccess?.({
			toolOverrides: toolOverridesFromRows(tools),
			toolPrefix
		});
		editDialog?.close();
	}

	onDestroy(() => cancelToolPreviewRequest());
</script>

<ResponsiveDialog
	bind:this={setupDialog}
	animate="slide"
	title={`Configure ${configuringEntry?.manifest.name ?? 'MCP Server'} Tools`}
	class="md:w-md"
	onClose={cancelSetup}
>
	<form
		class="flex grow flex-col p-4 md:p-0"
		onsubmit={(event) => {
			event.preventDefault();
			configureTools();
		}}
	>
		{#if configuringEntry}
			{#if oauthURL}
				<p class="mb-4 text-sm">
					MCP server requires OAuth authentication before its tools can be fetched.
				</p>
			{:else if userFields.length > 0}
				<p class="text-muted-content mb-6 text-sm font-light">
					Enter credentials to discover tools. These values are used only for this tool preview;
					users will still provide their own values when connecting.
				</p>
			{:else if !needsLiveTools}
				<p class="text-muted-content mb-6 text-sm font-light">
					Tools are read from the catalog-entry snapshot stored on this vMCP. The source catalog
					entry is not queried while editing an existing component.
				</p>
			{:else}
				<p class="text-muted-content mb-6 text-sm font-light">
					The MCP server's stored configuration will be used to fetch the tool list. In order to
					discover the MCP server's tools, you may need to temporarily authenticate.
				</p>
			{/if}

			{#if !oauthURL}
				{#each userFields as field (field.key)}
					<div class="mb-4 flex flex-col gap-2">
						<label for={`preview-${field.key}`} class="text-sm font-medium">
							{field.name || field.key}{field.required ? ' *' : ''}
						</label>
						{#if field.options?.length}
							<select
								id={`preview-${field.key}`}
								class="select w-full"
								bind:value={previewConfig[field.key]}
								required={field.required}
								disabled={loading}
							>
								<option value="">Select an option</option>
								{#each field.options as option (option.value)}
									<option value={option.value}>{option.name}</option>
								{/each}
							</select>
						{:else if field.sensitive}
							<SensitiveInput
								name={`preview-${field.key}`}
								textarea={field.usage === 'file' || field.usage === 'dynamicFile'}
								growable
								bind:value={
									() => previewConfig[field.key] ?? '',
									(value) => (previewConfig[field.key] = value)
								}
								required={field.required}
								disabled={loading}
							/>
						{:else if field.usage === 'file' || field.usage === 'dynamicFile'}
							<textarea
								id={`preview-${field.key}`}
								class="input-text-filled w-full min-h-32 resize-y"
								bind:value={previewConfig[field.key]}
								required={field.required}
								disabled={loading}
							></textarea>
						{:else}
							<input
								id={`preview-${field.key}`}
								type={field.key === '__url' ? 'url' : 'text'}
								class="input-text-filled w-full"
								bind:value={previewConfig[field.key]}
								required={field.required}
								disabled={loading}
							/>
						{/if}
						{#if field.description}
							<p class="text-muted-content text-xs">{field.description}</p>
						{/if}
					</div>
				{/each}
			{/if}

			{#if error}
				<p class="text-error mb-4 text-sm" role="alert">{error}</p>
			{/if}
			<div class="flex w-full flex-col gap-2">
				{#if oauthURL}
					{#if oauthValidating}
						<button
							in:fade
							class="btn btn-primary flex items-center justify-center gap-2"
							disabled
							type="button"
						>
							<Loading class="text-primary size-4" />
							Validating authentication...
						</button>
					{:else}
						<a
							in:fade
							href={oauthURL}
							rel="external noopener noreferrer"
							target="_blank"
							class="btn btn-primary"
						>
							Authenticate
						</a>
					{/if}
				{:else}
					<button class="btn btn-primary" disabled={loading || missingConfiguration} type="submit">
						{#if loading}
							<Loading class="text-primary-content size-4" />
						{:else}
							Configure Tools
						{/if}
					</button>
				{/if}
			</div>
		{/if}
	</form>
</ResponsiveDialog>

<CompositeEditTools
	bind:this={editDialog}
	{configuringEntry}
	{tools}
	bind:toolPrefix
	{otherEffectiveNames}
	{otherToolPrefixes}
	onCancel={cancelEditor}
	onClose={cancelEditor}
	onSuccess={save}
>
	{#snippet additionalActions()}
		{#if additionalActionsSnippet}
			{@render additionalActionsSnippet()}
		{/if}
	{/snippet}
</CompositeEditTools>
