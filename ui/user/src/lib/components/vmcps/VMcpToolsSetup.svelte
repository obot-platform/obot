<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import CompositeEditTools from '$lib/components/mcp/composite/CompositeEditTools.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import { UserService } from '$lib/services';
	import type {
		CompositeServerToolRow,
		MCPCatalogEntry,
		ToolOverride,
		VMCPComponent
	} from '$lib/services';
	import { toolOverridesFromRows } from '$lib/services/user/mcp';
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
		if ((refresh || tools.length === 0) && vmcpID && componentID(component)) {
			void fetchLiveTools();
			return;
		}
		openEditor();
	}

	export function open() {
		cancelToolPreviewRequest();
		error = undefined;
		oauthURL = undefined;
		tools = existingTools;
		toolPrefix = existingToolPrefix ?? component?.toolPrefix ?? '';
		dialogPhase = 'setup';
		setupDialog?.open();
		if (refresh && vmcpID && componentID(component)) {
			void fetchLiveTools();
		}
	}

	export function close() {
		cancelToolPreviewRequest();
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
	{#if configuringEntry}
		{#if oauthURL}
			<p class="mb-4 text-sm">
				MCP server requires OAuth authentication before its tools can be fetched.
			</p>
		{:else if !refresh && tools.length > 0}
			<p class="text-muted-content mb-6 text-sm font-light">
				Tools are read from the catalog-entry snapshot stored on this vMCP. The source catalog entry
				is not queried while editing an existing component.
			</p>
		{:else}
			<p class="text-muted-content mb-6 text-sm font-light">
				Fetch tools using this component's stored configuration before editing.
			</p>
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
				<button class="btn btn-primary" disabled={loading} onclick={configureTools}>
					{#if loading}
						<Loading class="text-primary-content size-4" />
					{:else}
						Configure Tools
					{/if}
				</button>
			{/if}
		</div>
	{/if}
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
