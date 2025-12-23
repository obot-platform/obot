<script lang="ts">
	import { ChatService, type MCPCatalogEntry, type MCPCatalogServer } from '$lib/services';
	import type { EventStreamService } from '$lib/services/admin/eventstream.svelte';
	import {
		convertCompositeInfoToLaunchFormData,
		convertCompositeLaunchFormDataToPayload,
		convertEnvHeadersToRecord
	} from '$lib/services/chat/mcp';
	import PageLoading from '../PageLoading.svelte';
	import CatalogConfigureForm, {
		type CompositeLaunchFormData,
		type LaunchFormData
	} from './CatalogConfigureForm.svelte';
	import CatalogEditAliasForm from './CatalogEditAliasForm.svelte';

	interface Props {
		onUpdateConfigure?: () => void;
	}
	let { onUpdateConfigure }: Props = $props();

	let configDialog = $state<ReturnType<typeof CatalogConfigureForm>>();
	let configureForm = $state<LaunchFormData | CompositeLaunchFormData>();
	let editAliasDialog = $state<ReturnType<typeof CatalogEditAliasForm>>();

	let entry = $state<MCPCatalogEntry>();
	let server = $state<MCPCatalogServer>();

	let editingError = $state<string>();
	let editingManifest = $derived(server?.manifest);
	let editing = $state(false);
	let launchError = $state<string>();
	let launchProgress = $state<number>(0);
	let launchLogsEventStream = $state<EventStreamService<string>>();
	let launchLogs = $state<string[]>([]);

	export async function edit({
		server: initServer,
		entry: initEntry
	}: {
		server: MCPCatalogServer;
		entry?: MCPCatalogEntry;
	}) {
		server = initServer;
		entry = initEntry;

		if (entry?.manifest.runtime === 'composite') {
			configureForm = await convertCompositeInfoToLaunchFormData(server);
			configDialog?.open();
			return;
		}

		let values: Record<string, string>;
		try {
			values = await ChatService.revealSingleOrRemoteMcpServer(server.id, {
				dontLogErrors: true
			});
		} catch (error) {
			if (error instanceof Error && !error.message.includes('404')) {
				console.error('Failed to reveal user server values due to unexpected error', error);
			}
			values = {};
		}
		configureForm = {
			envs: server.manifest.env?.map((env) => ({
				...env,
				value: values[env.key] ?? ''
			})),
			headers: server.manifest.remoteConfig?.headers?.map((header) => ({
				...header,
				value: values[header.key] ?? '',
				isStatic: header.value !== ''
			})),
			url: server.manifest.remoteConfig?.url,
			hostname: entry?.manifest.remoteConfig?.hostname
		};
		configDialog?.open();
	}

	export function rename({
		server: initServer,
		entry: initEntry
	}: {
		server: MCPCatalogServer;
		entry?: MCPCatalogEntry;
	}) {
		server = initServer;
		entry = initEntry;

		editAliasDialog?.open();
	}

	function initUpdatingOrLaunchProgress() {
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

		return { timeout1, timeout2, timeout3 };
	}

	async function updateExistingRemoteOrSingleUser(lf: LaunchFormData) {
		if (!server) return;

		if (
			entry &&
			entry.manifest.runtime === 'remote' &&
			// The update-url endpoint should only be called for remote servers that have a hostname set. For ones that have a fixedURL or urlTemplate the update-url endpoint is not supported.
			entry.manifest.remoteConfig?.hostname &&
			lf?.url
		) {
			await ChatService.updateRemoteMcpServerUrl(server.id, lf.url.trim());
		}

		const envs = convertEnvHeadersToRecord(lf.envs, lf.headers);
		await ChatService.configureSingleOrRemoteMcpServer(server.id, envs);
	}

	async function updateExistingComposite(lf: CompositeLaunchFormData) {
		if (!server) return;
		// Composite flow using CatalogConfigureForm data
		if ('componentConfigs' in lf) {
			const payload = convertCompositeLaunchFormDataToPayload(lf);
			await ChatService.configureCompositeMcpServer(server.id, payload);
		}
	}

	async function handleConfigureForm() {
		if (!server) return;
		if (!configureForm) return;

		editing = true;
		try {
			configDialog?.close();
			const { timeout1, timeout2, timeout3 } = initUpdatingOrLaunchProgress();
			// updating existing
			if (entry?.manifest.runtime === 'composite') {
				const lf = configureForm as CompositeLaunchFormData;
				await updateExistingComposite(lf);
			} else {
				const lf = configureForm as LaunchFormData;
				await updateExistingRemoteOrSingleUser(lf);
			}
			launchProgress = 100;
			clearTimeout(timeout1);
			clearTimeout(timeout2);
			clearTimeout(timeout3);
			onUpdateConfigure?.();

			setTimeout(() => {
				editing = false;
			}, 1000);
		} catch (_error) {
			console.error('Error during configuration:', _error);
			configDialog?.close();
		}
	}
</script>

<CatalogConfigureForm
	bind:this={configDialog}
	bind:form={configureForm}
	error={editingError}
	icon={editingManifest?.icon}
	name={server?.alias || server?.manifest?.name || ''}
	onSave={handleConfigureForm}
	submitText="Update"
	loading={editing}
	isNew={false}
/>

<CatalogEditAliasForm bind:this={editAliasDialog} {server} {onUpdateConfigure} />

<PageLoading
	isProgressBar
	show={editing}
	text="Updating and initializing server..."
	progress={launchProgress}
	error={launchError}
	errorClasses={{
		root: 'md:w-[95vw]'
	}}
>
	{#snippet errorPreContent()}
		<h4 class="text-xl font-semibold">MCP Server Update Failed</h4>
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
						launchError = undefined;
						configDialog?.open();
					}}
				>
					Update Configuration and Try Again
				</button>
			{/if}
		</div>
	{/snippet}
</PageLoading>
