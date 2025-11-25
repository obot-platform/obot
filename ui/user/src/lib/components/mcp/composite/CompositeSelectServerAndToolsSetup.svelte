<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import {
		AdminService,
		ChatService,
		type CatalogComponentServer,
		type CompositeServerToolRow,
		type MCPCatalogEntry,
		type MCPCatalogServer
	} from '$lib/services';
	import { convertEnvHeadersToRecord, hasEditableConfiguration } from '$lib/services/chat/mcp';
	import { LoaderCircle } from 'lucide-svelte';
	import CatalogConfigureForm, { type LaunchFormData } from '../CatalogConfigureForm.svelte';
	import type { AdminMcpServerAndEntriesContext } from '$lib/context/admin/mcpServerAndEntries.svelte';
	import CompositeEditTools from './CompositeEditTools.svelte';
	import SearchMcpServers from '$lib/components/admin/SearchMcpServers.svelte';

	interface Props {
		catalogId?: string;
		onCancel?: () => void;
		onSuccess?: (
			config: CatalogComponentServer,
			entry: MCPCatalogEntry | MCPCatalogServer,
			tools?: CompositeServerToolRow[]
		) => void;
		mcpEntriesContextFn?: () => AdminMcpServerAndEntriesContext;
		configuringEntry?: MCPCatalogEntry | MCPCatalogServer;
		excluded?: string[];
	}

	let {
		catalogId,
		onCancel,
		onSuccess,
		mcpEntriesContextFn,
		excluded,
		configuringEntry: presetConfiguringEntry
	}: Props = $props();
	let searchDialog = $state<ReturnType<typeof SearchMcpServers>>();
	let choiceDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let initConfigureToolsDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let configDialog = $state<ReturnType<typeof CatalogConfigureForm>>();
	let modifyToolsDialog = $state<ReturnType<typeof CompositeEditTools>>();

	let componentConfig = $state<CatalogComponentServer>();
	let configureForm = $state<LaunchFormData>();
	let configuringEntry = $state<MCPCatalogEntry | MCPCatalogServer>();
	let ready = $state(false);
	let loading = $state(false);
	let tools = $state<CompositeServerToolRow[]>([]);
	let oauthURL = $state<string>();
	let listeningOauthVisibility = $state(false);

	function handleVisibilityChange() {
		if (!componentConfig) return;
		if (document.visibilityState === 'visible' && oauthURL && !loading) {
			runPreview();
		}
	}

	$effect(() => {
		if (listeningOauthVisibility) {
			document.addEventListener('visibilitychange', handleVisibilityChange);
		} else {
			document.removeEventListener('visibilitychange', handleVisibilityChange);
		}
		return () => {
			document.removeEventListener('visibilitychange', handleVisibilityChange);
		};
	});

	function resetConfigureTool() {
		ready = false;
		tools = [];
		componentConfig = undefined;
		configuringEntry = undefined;
		oauthURL = undefined;
		listeningOauthVisibility = false;
	}

	export function open() {
		resetConfigureTool();
		if (presetConfiguringEntry) {
			configuringEntry = presetConfiguringEntry;
			componentConfig =
				'isCatalogEntry' in configuringEntry
					? {
							catalogEntryID: configuringEntry.id,
							manifest: configuringEntry.manifest,
							toolOverrides: []
						}
					: ({
							mcpServerID: configuringEntry.id,
							manifest: configuringEntry.manifest,
							toolOverrides: []
						} as CatalogComponentServer);
			runPreview();
			initConfigureToolsDialog?.open();
		} else {
			searchDialog?.open();
		}
	}

	function initConfigureForm(entry: MCPCatalogEntry) {
		configureForm = {
			envs: entry.manifest?.env?.map((env) => ({ ...env, value: '' })),
			headers: entry.manifest?.remoteConfig?.headers?.map((h) => ({ ...h, value: '' })),
			...(entry.manifest?.remoteConfig?.hostname
				? { hostname: entry.manifest.remoteConfig.hostname, url: '' }
				: {})
		};
	}

	async function handleConfigureToolsInit() {
		if (!configuringEntry) return;

		if (presetConfiguringEntry) {
			initConfigureToolsDialog?.close();
		}

		if ('isCatalogEntry' in configuringEntry && hasEditableConfiguration(configuringEntry)) {
			choiceDialog?.close();
			initConfigureForm(configuringEntry);
			configDialog?.open();
			return;
		}

		await runPreview();
	}

	async function fetchSingleRemoteTools(
		entryId: string,
		catalogId: string,
		body: { config?: Record<string, string>; url?: string } = { config: {}, url: '' }
	) {
		const resp = await AdminService.generateMcpCatalogEntryToolPreviews(catalogId, entryId, body, {
			dryRun: true
		});
		const preview = resp?.manifest?.toolPreview || [];
		return preview.map((t) => ({
			id: `${entryId}-${t.id || t.name}`,
			originalName: t.name,
			overrideName: t.name,
			originalDescription: t.description,
			overrideDescription: t.description,
			enabled: true,
			parameters: []
		}));
	}

	async function fetchMultiServerTools(entryId: string) {
		const tools = await ChatService.listMcpCatalogServerTools(entryId);
		return tools.map((t) => ({
			id: `${entryId}-${t.id || t.name}`,
			originalName: t.name,
			overrideName: t.name,
			originalDescription: t.description,
			overrideDescription: t.description,
			enabled: t.enabled !== false
		}));
	}

	async function runPreview(
		body: { config?: Record<string, string>; url?: string } = { config: {}, url: '' }
	) {
		if (!catalogId || !configuringEntry) return;
		loading = true;
		try {
			tools =
				'isCatalogEntry' in configuringEntry
					? await fetchSingleRemoteTools(configuringEntry.id, catalogId, body)
					: await fetchMultiServerTools(configuringEntry.id);
			initConfigureToolsDialog?.close();
			modifyToolsDialog?.open();
		} catch (err: unknown) {
			const msg = err instanceof Error ? err.message : String(err);
			if (msg.includes('OAuth')) {
				const oauthResponse = await AdminService.getMcpCatalogToolPreviewsOauth(
					catalogId!,
					configuringEntry.id,
					body,
					{
						dryRun: true
					}
				);

				if (typeof oauthResponse === 'string') {
					oauthURL = oauthResponse;
				} else if (oauthResponse) {
					oauthURL = undefined;
				}

				if (oauthURL) {
					listeningOauthVisibility = true;
				}
			} else {
				throw err;
			}
		} finally {
			loading = false;
			ready = true;
		}
	}

	async function handleAdd(
		mcpCatalogEntryIds: string[],
		mcpServerIds?: string[],
		_otherSelectors?: string[]
	) {
		if (mcpCatalogEntryIds.length === 1) {
			configuringEntry = await AdminService.getMCPCatalogEntry(catalogId!, mcpCatalogEntryIds[0]);
		} else if (mcpServerIds?.length === 1) {
			configuringEntry = await AdminService.getMCPCatalogServer(catalogId!, mcpServerIds[0]);
		} else {
			console.error('Incorrect type selected.', _otherSelectors);
			return;
		}

		componentConfig =
			'isCatalogEntry' in configuringEntry
				? {
						catalogEntryID: configuringEntry.id,
						manifest: configuringEntry.manifest,
						toolOverrides: []
					}
				: ({
						mcpServerID: configuringEntry.id,
						manifest: configuringEntry.manifest,
						toolOverrides: [],
						disabled: false
					} as CatalogComponentServer);
		choiceDialog?.open();
	}
</script>

<SearchMcpServers
	bind:this={searchDialog}
	onAdd={(mcpCatalogEntryIds, mcpServerIds, otherSelectors) =>
		handleAdd(mcpCatalogEntryIds, mcpServerIds, otherSelectors)}
	exclude={['*', 'default', ...(excluded ?? [])]}
	type="acr"
	mcpEntriesContextFn={(): AdminMcpServerAndEntriesContext => {
		const ctx = mcpEntriesContextFn?.() ?? {
			entries: [],
			servers: [],
			loading: false
		};
		return {
			...ctx,
			entries: ctx.entries.filter((e) => e.manifest?.runtime !== 'composite')
		};
	}}
	singleSelect
	title="Select MCP Server"
/>

<ResponsiveDialog
	bind:this={choiceDialog}
	animate="slide"
	title={`Configure ${configuringEntry?.manifest?.name ?? 'MCP Server'} Tools`}
	class="bg-surface1 md:w-md"
>
	<p class="text-on-surface1 text-sm font-light">
		By default, the tools for <i>{configuringEntry?.manifest?.name ?? 'MCP Server'}</i> are enabled by
		default. Would you like to further modify any tool availability or details?
	</p>
	<p class="text-on-surface1 mt-2 mb-6 text-sm font-light">
		You can also choose to skip and make these changes at a later time.
	</p>

	<div class="flex w-full flex-col gap-2">
		<button
			class="button"
			onclick={() => {
				if (!componentConfig || !configuringEntry) return;
				onSuccess?.(componentConfig, configuringEntry);
				choiceDialog?.close();
			}}
		>
			Skip, I'll Do Later
		</button>
		<button
			class="button-primary"
			onclick={() => {
				if (!configuringEntry) return;
				ready = false;
				runPreview();
				choiceDialog?.close();
				initConfigureToolsDialog?.open();
			}}
		>
			Configure Tools
		</button>
	</div>
</ResponsiveDialog>

<ResponsiveDialog
	bind:this={initConfigureToolsDialog}
	animate="slide"
	title={`Configure ${configuringEntry?.manifest?.name ?? 'MCP Server'} Tools`}
	class="md:w-sm"
	onClose={() => {
		listeningOauthVisibility = false;
	}}
>
	{#if configuringEntry}
		<div class="flex h-full min-h-32 flex-col items-center justify-center">
			{#if loading && !ready}
				<div class="mb-8 flex items-center justify-center gap-1">
					<LoaderCircle class="text-on-surface1 size-4 animate-spin" />
					<p class="text-on-surface1 text-sm font-light">Fetching tools...</p>
				</div>
			{:else}
				<div class="mb-6 h-full text-left">
					{#if 'isCatalogEntry' in configuringEntry && hasEditableConfiguration(configuringEntry)}
						<p>
							In order to request tools from the server, you'll need to pass some configuration
							information first.
						</p>
					{:else if oauthURL}
						<p>
							In order to request tools from the server, OAuth authentication is required first.
						</p>
						<p class="mt-2">
							<b>Note:</b> This will only be used to fetch the tools for this server; end users would
							still need to login when consuming this composite server and must have the appropriate
							permissions to access these tools.
						</p>
					{:else}
						<p>
							You're set up to begin fine-tuning the tools for this MCP server. Click the button
							below to begin!
						</p>
					{/if}
				</div>
				{#if oauthURL}
					<a href={oauthURL} target="_blank" class="button-primary flex w-full justify-center">
						{#if loading}
							<LoaderCircle class="size-4 animate-spin" />
						{:else}
							Authenticate
						{/if}
					</a>
				{:else}
					<button
						class="button-primary flex w-full justify-center"
						disabled={loading}
						onclick={handleConfigureToolsInit}
					>
						{#if loading}
							<LoaderCircle class="size-4 animate-spin" />
						{:else}
							Get Started
						{/if}
					</button>
				{/if}
			{/if}
		</div>
	{/if}
</ResponsiveDialog>

<CompositeEditTools
	bind:this={modifyToolsDialog}
	{configuringEntry}
	{tools}
	onCancel={() => {
		resetConfigureTool();
		if (configuringEntry) {
			onCancel?.();
		}
	}}
	onSuccess={() => {
		if (!componentConfig || !configuringEntry) return;
		onSuccess?.(
			{
				...componentConfig,
				toolOverrides: tools.map((t) => ({
					name: t.originalName,
					overrideName: t.overrideName,
					overrideDescription: t.overrideDescription,
					enabled: t.enabled
				}))
			},
			configuringEntry,
			tools
		);
	}}
/>

<CatalogConfigureForm
	bind:this={configDialog}
	bind:form={configureForm}
	name={configuringEntry?.manifest?.name}
	icon={configuringEntry?.manifest?.icon}
	submitText="Continue"
	cancelText="Cancel"
	onSave={async () => {
		if (!configuringEntry) return;
		const configValues = convertEnvHeadersToRecord(configureForm?.envs, configureForm?.headers);
		await runPreview({ config: configValues, url: configureForm?.url });
		configDialog?.close();
		modifyToolsDialog?.open();
	}}
	onCancel={() => {
		if (configuringEntry) {
			onCancel?.();
		}
		configDialog?.close();
	}}
	{loading}
	error={undefined}
	isNew
	disableOutsideClick
	animate="slide"
>
	{#snippet loadingContent()}
		<div class="mb-8 flex items-center justify-center gap-1">
			<LoaderCircle class="text-on-surface1 size-4 animate-spin" />
			<p class="text-on-surface1 text-sm font-light">Fetching tools...</p>
		</div>
	{/snippet}
</CatalogConfigureForm>
