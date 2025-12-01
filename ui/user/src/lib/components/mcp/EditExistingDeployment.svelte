<script lang="ts">
	import {
		ChatService,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type MCPServerInstance
	} from '$lib/services';
	import type { EventStreamService } from '$lib/services/admin/eventstream.svelte';
	import {
		convertCompositeInfoToLaunchFormData,
		convertCompositeLaunchFormDataToPayload,
		convertEnvHeadersToRecord
	} from '$lib/services/chat/mcp';
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
	let instance = $state<MCPServerInstance>();

	let editingError = $state<string>();
	let editingManifest = $derived(server?.manifest);
	let editing = $state(false);
	let launchError = $state<string>();
	let launchProgress = $state<number>(0);
	let launchLogsEventStream = $state<EventStreamService<string>>();
	let launchLogs = $state<string[]>([]);

	export async function edit({
		server: initServer,
		instance: initInstance,
		entry: initEntry
	}: {
		server: MCPCatalogServer;
		instance?: MCPServerInstance;
		entry?: MCPCatalogEntry;
	}) {
		server = initServer;
		instance = initInstance;
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
				value: values[header.key] ?? ''
			})),
			url: server.manifest.remoteConfig?.url,
			hostname: entry?.manifest.remoteConfig?.hostname
		};
		configDialog?.open();
	}

	export function rename({
		server: initServer,
		instance: initInstance,
		entry: initEntry
	}: {
		server: MCPCatalogServer;
		instance?: MCPServerInstance;
		entry?: MCPCatalogEntry;
	}) {
		server = initServer;
		instance = initInstance;
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
			entry.manifest.remoteConfig?.urlTemplate === undefined &&
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
