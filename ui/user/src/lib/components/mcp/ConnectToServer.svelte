<script lang="ts">
	import { ExternalLink, Plus, Server } from 'lucide-svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import CopyButton from '../CopyButton.svelte';
	import HowToConnect from './HowToConnect.svelte';
	import {
		ChatService,
		EditorService,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type MCPServerInstance
	} from '$lib/services';
	import {
		convertCompositeLaunchFormDataToPayload,
		convertEnvHeadersToRecord,
		createProjectMcp,
		hasEditableConfiguration
	} from '$lib/services/chat/mcp';
	import { goto } from '$app/navigation';
	import PageLoading from '../PageLoading.svelte';
	import CatalogConfigureForm, {
		type CompositeLaunchFormData,
		type LaunchFormData
	} from './CatalogConfigureForm.svelte';
	import { EventStreamService } from '$lib/services/admin/eventstream.svelte';
	import { resolve } from '$app/paths';

	interface Props {
		userConfiguredServers: MCPCatalogServer[];
		onConnect?: ({
			server,
			entry,
			instance
		}: {
			server?: MCPCatalogServer;
			entry?: MCPCatalogEntry;
			instance?: MCPServerInstance;
		}) => void;
		onClose?: () => void;
		skipConnectDialog?: boolean;
		hideActions?: boolean;
	}

	let { userConfiguredServers, onConnect, onClose, skipConnectDialog, hideActions }: Props =
		$props();

	let server = $state<MCPCatalogServer>();
	let entry = $state<MCPCatalogEntry>();
	let instance = $state<MCPServerInstance>();
	let manifest = $derived(server?.manifest || entry?.manifest);
	let isConfigured = $derived(Boolean((entry && server) || (server && instance)));

	let connectDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let configDialog = $state<ReturnType<typeof CatalogConfigureForm>>();
	let configureForm = $state<LaunchFormData | CompositeLaunchFormData>();

	let chatLoading = $state(false);
	let chatLoadingProgress = $state(0);
	let chatLaunchError = $state<string>();

	let launchError = $state<string>();
	let launchProgress = $state<number>(0);
	let launchLogsEventStream = $state<EventStreamService<string>>();
	let launchLogs = $state<string[]>([]);
	let launchState = $state<'relaunching' | 'launching' | undefined>();
	let error = $state<string>();
	let saving = $state(false);

	let existingServerNames = $derived(
		userConfiguredServers
			.flatMap((server) => [server.manifest?.name || '', server.alias || ''])
			.filter(Boolean)
			.map((name) => name.toLowerCase())
	);
	let name = $derived(server?.alias || server?.manifest.name || '');

	function handleConnect() {
		if (onConnect) {
			onConnect({ server, entry, instance });
		}

		if (!skipConnectDialog) {
			connectDialog?.open();
		}
	}

	function getUniqueAlias(serverName: string): string | undefined {
		const nameLower = serverName.toLowerCase();

		// Return undefined if no conflict
		if (!existingServerNames.includes(nameLower)) {
			return undefined;
		}

		// Generate unique alias with counter
		let counter = 1;
		let candidateAlias: string;
		do {
			candidateAlias = `${serverName} ${counter}`;
			counter++;
		} while (existingServerNames.includes(candidateAlias.toLowerCase()));

		return candidateAlias;
	}

	function initConfigureForm(item: MCPCatalogEntry) {
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

	function initCompositeForm(item: MCPCatalogEntry) {
		// For composite: open form first to collect per-component URLs before creating
		if (item.manifest.runtime === 'composite') {
			const components = item.manifest?.compositeConfig?.componentServers || [];
			const componentConfigs: Record<
				string,
				{
					name?: string;
					icon?: string;
					hostname?: string;
					url?: string;
					disabled?: boolean;
					isMultiUser?: boolean;
					envs?: Array<Record<string, unknown> & { key: string; value: string }>;
					headers?: Array<Record<string, unknown> & { key: string; value: string }>;
				}
			> = {};
			for (const c of components) {
				const id = c.catalogEntryID || c.mcpServerID;
				if (!id || !c.manifest) continue;
				const m = c.manifest;
				const isMultiUser = !!c.mcpServerID && !c.catalogEntryID;
				componentConfigs[id] = {
					name: m.name,
					icon: m.icon,
					hostname: isMultiUser ? undefined : m.remoteConfig?.hostname,
					url: isMultiUser ? undefined : (m.remoteConfig?.fixedURL ?? ''),
					disabled: false,
					isMultiUser,
					envs: isMultiUser
						? []
						: (m.env ?? []).map((e) => ({
								...(e as unknown as Record<string, unknown>),
								key: e.key,
								value: ''
							})),
					headers: isMultiUser
						? []
						: (m.remoteConfig?.headers ?? []).map((h) => ({
								...(h as unknown as Record<string, unknown>),
								key: h.key,
								value: '',
								isStatic: h.value !== ''
							}))
				};
			}
			configureForm = { componentConfigs } as CompositeLaunchFormData;
			configDialog?.open();
		}
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

	async function handleLaunchCatalogEntry() {
		if (!entry) return;

		if (!entry.manifest) {
			console.error('No server manifest found');
			return;
		}

		const { timeout1, timeout2, timeout3 } = initUpdatingOrLaunchProgress();
		const url =
			entry.manifest.runtime === 'remote'
				? (
						(configureForm as LaunchFormData | undefined)?.url ||
						entry.manifest.remoteConfig?.fixedURL
					)?.trim()
				: undefined;
		const serverName = entry.manifest.name || '';

		// Generate unique alias if there's a naming conflict
		const aliasToUse = configureForm?.name || getUniqueAlias(serverName);

		let response: MCPCatalogServer | undefined = undefined;
		try {
			response = await ChatService.createSingleOrRemoteMcpServer({
				catalogEntryID: entry.id,
				manifest: url ? { remoteConfig: { url } } : {},
				alias: aliasToUse
			});
		} catch (err) {
			console.error('error: ', err);
			launchError = err instanceof Error ? err.message : 'An unknown error occurred';
		}

		if (response) {
			try {
				const lf = configureForm as LaunchFormData | undefined;
				const envs = convertEnvHeadersToRecord(lf?.envs, lf?.headers);
				const configuredResponse = await ChatService.configureSingleOrRemoteMcpServer(
					response.id,
					envs
				);
				server = configuredResponse;

				const launchResponse = await ChatService.validateSingleOrRemoteMcpServerLaunched(
					configuredResponse.id
				);
				if (!launchResponse.success) {
					launchError = launchResponse.message;
					listLaunchLogs(configuredResponse.id);
				} else {
					launchProgress = 100;
				}

				if (!launchError) {
					setTimeout(() => {
						launchState = undefined;
						launchProgress = 0;
						handleConnect();
					}, 1000);
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

	async function handleLaunchCompositeServer() {
		if (!entry) return;

		// If no configureForm yet, initialize the composite form so user can enable/disable components.
		if (!configureForm || !('componentConfigs' in configureForm)) {
			initCompositeForm(entry);
			return;
		}

		if (!entry.manifest) {
			console.error('No server manifest found');
			return;
		}

		if (launchLogsEventStream) {
			// reset launch logs
			launchLogsEventStream.disconnect();
			launchLogsEventStream = undefined;
			launchLogs = [];
		}

		launchError = undefined;
		launchProgress = 0;
		launchState = 'launching';

		let timeout1 = setTimeout(() => {
			launchProgress = 10;
		}, 100);

		let timeout2 = setTimeout(() => {
			launchProgress = 30;
		}, 3000);

		let timeout3 = setTimeout(() => {
			launchProgress = 80;
		}, 10000);

		try {
			const aliasToUse =
				(configureForm as { name?: string } | undefined)?.name ||
				getUniqueAlias(entry.manifest.name || '');
			const componentServersForCreate: Array<{
				catalogEntryID: string;
				manifest: Record<string, unknown>;
				disabled?: boolean;
			}> = [];
			const payload: Record<
				string,
				{ config: Record<string, string>; url?: string; disabled?: boolean }
			> = {};
			for (const [id, comp] of Object.entries(configureForm.componentConfigs)) {
				const url = comp.url?.trim();
				componentServersForCreate.push({
					catalogEntryID: id,
					manifest: url
						? { remoteConfig: { url: url.startsWith('http') ? url : `https://${url}` } }
						: {},
					disabled: comp.disabled ?? false
				});
				const config: Record<string, string> = {};
				for (const f of [
					...(comp.envs ?? ([] as Array<{ key: string; value: string }>)),
					...(comp.headers ?? ([] as Array<{ key: string; value: string }>))
				]) {
					if (f.value) config[f.key] = f.value;
				}
				payload[id] = { config, url, disabled: comp.disabled ?? false };
			}

			const created = await ChatService.createCompositeMcpServer({
				catalogEntryID: entry.id,
				alias: aliasToUse,
				manifest: {
					compositeConfig: { componentServers: componentServersForCreate }
				}
			});
			server = created;

			await ChatService.configureCompositeMcpServer(created.id, payload);

			const launchResponse = await ChatService.validateSingleOrRemoteMcpServerLaunched(created.id);
			if (!launchResponse.success) {
				launchError = launchResponse.message;
			} else {
				launchProgress = 100;
			}

			if (!launchError) {
				setTimeout(() => {
					launchState = undefined;
					launchProgress = 0;
					handleConnect();
				}, 1000);
			}
		} catch (err) {
			launchError = err instanceof Error ? err.message : 'An unknown error occurred';
		} finally {
			clearTimeout(timeout1);
			clearTimeout(timeout2);
			clearTimeout(timeout3);
		}
	}

	async function handleMultiUserServer() {
		if (!server || server.catalogEntryID) return;
		try {
			const response = await ChatService.createMcpServerInstance(server.id);
			instance = response;
			handleConnect();
		} catch (err) {
			error = err instanceof Error ? err.message : 'An unknown error occurred';
		}
	}

	async function handleLaunch() {
		error = undefined;
		saving = true;
		try {
			if (entry && entry.manifest?.runtime === 'composite') {
				await handleLaunchCompositeServer();
			} else if (entry) {
				await handleLaunchCatalogEntry();
			} else {
				await handleMultiUserServer();
			}
		} catch (error) {
			console.error('Error during launching', error);
		} finally {
			saving = false;
		}
	}

	async function handleCancelLaunch() {
		if (launchLogsEventStream) {
			launchLogsEventStream.disconnect();
		}
		if (server && entry) {
			await ChatService.deleteSingleOrRemoteMcpServer(server.id);
		}

		launchState = undefined;
		launchError = undefined;
	}

	async function updateExistingRemoteOrSingleUser(lf: LaunchFormData) {
		if (!entry || !server) return;
		if (
			entry &&
			entry.manifest.runtime === 'remote' &&
			entry.manifest.remoteConfig?.hostname &&
			lf?.url
		) {
			await ChatService.updateRemoteMcpServerUrl(server.id, lf.url.trim());
		}

		const envs = convertEnvHeadersToRecord(lf.envs, lf.headers);
		await ChatService.configureSingleOrRemoteMcpServer(server.id, envs);

		server = await ChatService.getSingleOrRemoteMcpServer(server.id);
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
		if (!configureForm) return;

		if (launchState === 'relaunching' && server && entry) {
			configDialog?.close();
			await handleLaunchCatalogEntry();
			return;
		}

		try {
			if (server?.id) {
				configDialog?.close();
				const { timeout1, timeout2, timeout3 } = initUpdatingOrLaunchProgress(true);
				// updating existing
				if (entry?.id === 'composite') {
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
				// onUpdate?.();

				setTimeout(() => {
					launchState = undefined;
				}, 1000);
			} else {
				// launching new
				configDialog?.close();
				await new Promise((resolve) => setTimeout(resolve, 300));
				await handleLaunch();
			}
		} catch (_error) {
			console.error('Error during configuration:', _error);
			configDialog?.close();
		}
	}

	function initCatalogEntry() {
		if (!entry) return;
		if (hasEditableConfiguration(entry) && entry.manifest?.runtime === 'composite') {
			initCompositeForm(entry);
		} else if (hasEditableConfiguration(entry)) {
			initConfigureForm(entry);
			configDialog?.open();
		} else {
			handleLaunch();
		}
	}

	export function open({
		server: initServer,
		entry: initEntry,
		instance: initInstance
	}: {
		server?: MCPCatalogServer;
		entry?: MCPCatalogEntry;
		instance?: MCPServerInstance;
	}) {
		server = initServer;
		entry = initEntry;
		instance = initInstance;

		if ((entry && server) || (server && instance)) {
			handleConnect();
		} else {
			if (initEntry && !initServer) {
				initCatalogEntry();
			} else {
				handleLaunch();
			}
		}
	}

	export async function handleSetupChat(
		connectedServer: MCPCatalogServer,
		instance?: MCPServerInstance
	) {
		connectDialog?.close();
		chatLaunchError = undefined;
		chatLoading = true;
		chatLoadingProgress = 0;

		let timeout1 = setTimeout(() => {
			chatLoadingProgress = 10;
		}, 1000);
		let timeout2 = setTimeout(() => {
			chatLoadingProgress = 50;
		}, 5000);
		let timeout3 = setTimeout(() => {
			chatLoadingProgress = 80;
		}, 10000);

		const projects = await ChatService.listProjects();
		const name = [
			connectedServer.alias || connectedServer.manifest.name || '',
			connectedServer.id
		].join(' - ');
		const match = projects.items.find((project) => project.name === name);

		let project = match;
		if (!match) {
			// if no project match, create a new one w/ mcp server connected to it
			project = await EditorService.createObot({
				name: name
			});
		}

		try {
			const mcpId = instance ? instance.id : connectedServer.id;
			if (
				project &&
				!(await ChatService.listProjectMCPs(project.assistantID, project.id)).find(
					(mcp) => mcp.mcpID === mcpId
				)
			) {
				await createProjectMcp(project, mcpId);
			}
		} catch (err) {
			chatLaunchError = err instanceof Error ? err.message : 'An unknown error occurred';
		} finally {
			clearTimeout(timeout1);
			clearTimeout(timeout2);
			clearTimeout(timeout3);
		}

		chatLoadingProgress = 100;
		setTimeout(() => {
			chatLoading = false;
			goto(resolve(`/o/${project?.id}`));
		}, 1000);
	}
</script>

<ResponsiveDialog bind:this={connectDialog} animate="slide" {onClose}>
	{#snippet titleContent()}
		{#if server}
			{@const icon = server.manifest.icon ?? ''}

			<div class="bg-surface1 rounded-sm p-1 dark:bg-gray-600">
				{#if icon}
					<img src={icon} alt={name} class="size-8" />
				{:else}
					<Server class="size-8" />
				{/if}
			</div>
			{name}
		{/if}
	{/snippet}

	{#if server}
		{@const url = instance ? instance.connectURL : server.connectURL}
		<div class="flex items-center gap-4">
			<div class="mb-4 flex grow flex-col gap-1">
				<label for="connectURL" class="font-light">Connection URL</label>
				<div class="mock-input-btn flex w-full items-center justify-between gap-2 shadow-inner">
					<p>
						{url}
					</p>
					<CopyButton
						showTextLeft
						text={url}
						classes={{
							button: 'flex-shrink-0 flex items-center gap-1 text-xs font-light hover:text-blue-500'
						}}
					/>
				</div>
			</div>
			{#if !hideActions}
				<div class="w-32">
					<button
						class="button-primary flex h-fit w-full grow items-center justify-center gap-2 text-sm"
						onclick={() => handleSetupChat(server!, instance)}
					>
						Chat <ExternalLink class="size-4" />
					</button>
				</div>
			{/if}
		</div>

		{#if url}
			<HowToConnect servers={[{ url, name }]} />
		{/if}

		{#if entry && !hideActions}
			<p class="text-on-surface1 flex items-center justify-end gap-2 text-sm font-light">
				Need to set up a different instance?
				<button
					class="button-small button-primary hover:bg-primary px-3 text-xs"
					onclick={() => {
						server = undefined;
						initCatalogEntry();
						connectDialog?.close();
					}}
				>
					<Plus class="size-3" /> Connect to New Server
				</button>
			</p>
		{/if}
	{/if}
</ResponsiveDialog>

<PageLoading
	show={chatLoading}
	isProgressBar
	progress={chatLoadingProgress}
	text="Loading chat..."
	error={chatLaunchError}
	longLoadMessage="Connecting MCP Server to chat..."
	longLoadDuration={10000}
	onClose={() => {
		chatLoading = false;
	}}
/>

<CatalogConfigureForm
	bind:this={configDialog}
	bind:form={configureForm}
	{error}
	icon={manifest?.icon}
	name={server?.alias || manifest?.name || ''}
	onSave={handleConfigureForm}
	submitText={isConfigured ? 'Update' : 'Launch'}
	loading={saving}
	isNew={!isConfigured}
	showAlias={isConfigured}
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
	onClose={handleCancelLaunch}
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
						if (hasEditableConfiguration(entry!)) {
							configDialog?.open();
						} else {
							handleLaunch();
						}
					}}
				>
					Update Configuration and Try Again
				</button>
			{/if}
			<button class="button w-full md:w-1/2 md:flex-1" onclick={handleCancelLaunch}>
				Cancel and Delete Server
			</button>
		</div>
	{/snippet}
</PageLoading>
