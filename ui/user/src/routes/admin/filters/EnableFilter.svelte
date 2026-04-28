<script lang="ts">
	import PageLoading from '$lib/components/PageLoading.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import SelectorsAndResourcesFormSegment from '$lib/components/admin/SelectorsAndResourcesFormSegment.svelte';
	import CatalogConfigureForm, {
		type LaunchFormData
	} from '$lib/components/mcp/CatalogConfigureForm.svelte';
	import {
		AdminService,
		type MCPFilter,
		type MCPFilterResource,
		type MCPFilterWebhookSelector,
		type SystemMCPServer,
		type SystemMCPServerCatalogEntry
	} from '$lib/services';
	import { EventStreamService } from '$lib/services/admin/eventstream.svelte';
	import { convertEnvHeadersToRecord, hasEditableConfiguration } from '$lib/services/chat/mcp';

	interface Props {
		configuredFilterServers: SystemMCPServer[];
		onSuccess?: () => void;
		onClose?: () => void;
	}

	let { onSuccess }: Props = $props();

	let server = $state<SystemMCPServer>();
	let entry = $state<SystemMCPServerCatalogEntry>();
	let filterId = $state('');

	let manifest = $derived(server?.manifest || entry?.manifest);
	let isConfigured = $derived(Boolean(entry && server));

	let configDialog = $state<ReturnType<typeof CatalogConfigureForm>>();
	let configureForm = $state<LaunchFormData>();

	let selectorsAndMcpServersDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let selectorsAndMcpServersForm = $state<{
		selectors: MCPFilterWebhookSelector[];
		resources: MCPFilterResource[];
	}>();

	let launchError = $state<string>();
	let launchProgress = $state<number>(0);
	let launchLogsEventStream = $state<EventStreamService<string>>();
	let launchLogs = $state<string[]>([]);
	let launchState = $state<'relaunching' | 'launching' | undefined>();
	let error = $state<string>();
	let saving = $state(false);

	function initConfigureForm(item: SystemMCPServerCatalogEntry) {
		configureForm = {
			name: '',
			envs: item.manifest?.env?.map((env) => ({
				...env,
				value: ''
			})),
			headers: item.manifest?.remoteConfig?.headers?.map((header) => ({
				...header,
				value: '',
				isStatic: header.value !== ''
			})),
			...(item.manifest?.remoteConfig?.hostname
				? { hostname: item.manifest.remoteConfig?.hostname, url: '' }
				: {})
		};
	}

	function initSelectorsAndMcpServersForm() {
		selectorsAndMcpServersForm = {
			selectors: [],
			resources: [{ id: 'default', type: 'mcpCatalog' }]
		};
	}

	function listLaunchLogs(mcpServerId: string) {
		launchLogsEventStream = new EventStreamService<string>();
		launchLogsEventStream.connect(`/api/mcp-servers/${mcpServerId}/logs`, {
			onMessage: (data) => {
				launchLogs = [...launchLogs, data];
			}
		});
	}

	function initUpdatingOrLaunchProgress(existing?: boolean) {
		if (launchLogsEventStream) {
			// reset launch logs
			launchLogsEventStream.disconnect();
			launchLogsEventStream = undefined;
			launchLogs = [];
		}

		launchError = undefined;
		launchProgress = 0;
		launchState = existing ? 'relaunching' : 'launching';

		let timeout1 = setTimeout(() => {
			launchProgress = 10;
		}, 100);

		let timeout2 = setTimeout(() => {
			launchProgress = 30;
		}, 3000);

		let timeout3 = setTimeout(() => {
			launchProgress = 80;
		}, 10000);

		return { timeout1, timeout2, timeout3 };
	}

	async function handleEnableFilter() {
		if (!entry) return;

		if (!entry.manifest) {
			console.error('No server manifest found');
			return;
		}

		const { timeout1, timeout2, timeout3 } = initUpdatingOrLaunchProgress();

		let response: MCPFilter | undefined = undefined;
		try {
			response = await AdminService.createMCPFilter({
				name: entry.manifest.name || '',
				systemMCPServerCatalogEntryID: entry.id,
				allowedToMutate: true,
				selectors: selectorsAndMcpServersForm?.selectors,
				resources: selectorsAndMcpServersForm?.resources
			});
			filterId = response.id;
		} catch (err) {
			console.error('error: ', err);
			launchError = err instanceof Error ? err.message : 'An unknown error occurred';
		}

		if (response) {
			try {
				const lf = configureForm as LaunchFormData | undefined;
				const envs = convertEnvHeadersToRecord(lf?.envs, lf?.headers);
				const configuredResponse = await AdminService.configureMCPFilter(response.id, envs);
				const launchResponse = await AdminService.launchMCPFilter(configuredResponse.id);
				if (!launchResponse.success) {
					launchError = launchResponse.message;
					listLaunchLogs(configuredResponse.id);
				}

				if (!launchError) {
					launchProgress = 100;

					await new Promise((resolve) => setTimeout(resolve, 500));
					launchState = undefined;
					launchProgress = 0;
					onSuccess?.();
				}
			} catch (err) {
				launchError = err instanceof Error ? err.message : 'An unknown error occurred';
			} finally {
				clearTimeout(timeout1);
				clearTimeout(timeout2);
				clearTimeout(timeout3);
			}
		}
	}

	async function handleSave() {
		error = undefined;
		saving = true;
		try {
			await handleEnableFilter();
		} catch (error) {
			console.error('Error during launching', error);
		} finally {
			saving = false;
		}
	}

	async function handleCancel() {
		if (launchLogsEventStream) {
			launchLogsEventStream.disconnect();
		}
		if (filterId) {
			await AdminService.deleteMCPFilter(filterId);
		}

		launchState = undefined;
		launchError = undefined;
	}

	function handleConfigureForm() {
		initSelectorsAndMcpServersForm();
		selectorsAndMcpServersDialog?.open();
	}

	async function handleFinish() {
		configDialog?.close();
		selectorsAndMcpServersDialog?.close();
		if (launchState === 'relaunching' && server && entry) {
			await handleEnableFilter();
			return;
		}

		try {
			await new Promise((resolve) => setTimeout(resolve, 300));
			await handleSave();
		} catch (_error) {
			console.error('Error during configuration:', _error);
			configDialog?.close();
		}
	}

	function initCatalogEntry() {
		if (!entry) return;
		if (hasEditableConfiguration(entry)) {
			initConfigureForm(entry);
			configDialog?.open();
		} else {
			initSelectorsAndMcpServersForm();
			selectorsAndMcpServersDialog?.open();
		}
	}

	export function open({
		server: initServer,
		entry: initEntry
	}: {
		server?: SystemMCPServer;
		entry?: SystemMCPServerCatalogEntry;
	}) {
		server = initServer;
		entry = initEntry;
		filterId = '';
		initCatalogEntry();
	}
</script>

<CatalogConfigureForm
	bind:this={configDialog}
	bind:form={configureForm}
	{error}
	icon={manifest?.icon}
	name={manifest?.name || ''}
	onSave={handleConfigureForm}
	submitText="Next"
	loading={saving}
	isNew={!isConfigured}
	showAlias={isConfigured}
	displayDescriptionInline
/>

<PageLoading
	isProgressBar
	show={typeof launchState !== 'undefined'}
	text="Configuring and initializing server..."
	progress={launchProgress}
	error={launchError}
	errorClasses={{
		root: 'md:w-[95vw]'
	}}
	onClose={handleCancel}
>
	{#snippet errorPreContent()}
		<h4 class="text-xl font-semibold">MCP Server Launch Failed</h4>
	{/snippet}
	{#snippet errorPostContent()}
		{#if launchLogs.length > 0}
			<div
				class="default-scrollbar-thin bg-surface1 max-h-[50vh] w-full overflow-y-auto rounded-lg p-4 shadow-inner"
			>
				{#each launchLogs as log, i (i)}
					<div class="font-mono text-sm">
						<span class="text-on-surface1">{log}</span>
					</div>
				{/each}
			</div>
		{:else}
			<p class="text-md self-start">An issue occurred while launching the MCP server.</p>
		{/if}

		<div class="flex w-full flex-col items-center gap-2 md:flex-row">
			{#if entry}
				<button
					class="button-primary w-full md:w-1/2 md:flex-1"
					onclick={() => {
						launchState = 'relaunching';
						launchError = undefined;
						if (entry && hasEditableConfiguration(entry)) {
							configDialog?.open();
						} else {
							selectorsAndMcpServersDialog?.open();
						}
					}}
				>
					Update Configuration and Try Again
				</button>
			{/if}
			<button class="button w-full md:w-1/2 md:flex-1" onclick={handleCancel}> Cancel </button>
		</div>
	{/snippet}
</PageLoading>

<ResponsiveDialog
	bind:this={selectorsAndMcpServersDialog}
	title="Modify Selectors & MCP Servers"
	class="max-w-3xl"
	animate="slide"
>
	<div class="flex flex-col gap-6">
		{#if selectorsAndMcpServersForm}
			<SelectorsAndResourcesFormSegment bind:form={selectorsAndMcpServersForm} inDialog />
		{/if}
		<div class="flex w-full justify-end gap-2">
			{#if entry && hasEditableConfiguration(entry)}
				<button
					class="button text-sm"
					onclick={() => {
						selectorsAndMcpServersDialog?.close();
						configDialog?.open();
					}}>Go Back</button
				>
			{/if}
			<button class="button-primary text-sm" onclick={handleFinish}>Save</button>
		</div>
	</div>
</ResponsiveDialog>
