<script lang="ts">
	import { getProjectMCPs, validateOauthProjectMcps } from '$lib/context/projectMcps.svelte';
	import { getLayout } from '$lib/context/chatLayout.svelte';
	import { onMount, tick } from 'svelte';
	import { Server, X } from 'lucide-svelte';
	import CatalogConfigureForm, { type LaunchFormData } from '../mcp/CatalogConfigureForm.svelte';
	import { ChatService, type MCPCatalogEntry, type MCPCatalogServer } from '$lib/services';
	import { convertEnvHeadersToRecord } from '$lib/services/chat/mcp';
	import { SvelteSet } from 'svelte/reactivity';

	interface Props {
		assistantId: string;
		projectId: string;
	}

	const { assistantId, projectId }: Props = $props();

	const projectMcps = getProjectMCPs();
	const layout = getLayout();
	const isInMcp = $derived(
		layout.sidebarConfig === 'mcp-server-tools' || layout.sidebarConfig === 'mcp-server'
	);

	let oauthDialogs = $state<HTMLDialogElement[]>([]);
	let oauthIndex = $state(0);
	let oauthQueue = $derived(projectMcps.items.filter((m) => !m.authenticated && m.oauthURL));

	let userServers = $state<MCPCatalogServer[]>([]);
	let entries = $state<MCPCatalogEntry[]>([]);
	let userServersById = $derived(new Map(userServers.map((s) => [s.id, s])));
	let entriesById = $derived(new Map(entries.map((e) => [e.id, e])));
	let configQueue = $derived(
		projectMcps.items
			.map((m) => userServersById.get(m.mcpID))
			.filter((s): s is MCPCatalogServer => Boolean(s))
			.filter((s) => s.needsURL || s.needsUpdate || s.configured === false)
	);

	let closed = new SvelteSet<string>();
	let configDialog = $state<ReturnType<typeof CatalogConfigureForm>>();
	let configIndex = $state(0);
	let configureForm = $state<LaunchFormData>();
	let configuring = $state(false);
	let configError = $state<string>();
	let configName = $state<string>('');
	let configIcon = $state<string>('');
	let configServerId = $state<string>('');

	let ready = $state(false);

	async function refreshUserServers() {
		try {
			const [singleOrRemoteUserServers, entriesResult] = await Promise.all([
				ChatService.listSingleOrRemoteMcpServers(),
				ChatService.listMCPs()
			]);
			userServers = singleOrRemoteUserServers;
			entries = entriesResult;
		} catch (err) {
			console.error('Failed to refresh user servers:', err);
		}
	}

	async function checkOauths() {
		const updated = await validateOauthProjectMcps(assistantId, projectId, projectMcps.items);
		if (updated.length > 0) {
			projectMcps.items = updated;
		}
	}

	onMount(() => {
		const handleVisibilityChange = async () => {
			if (isInMcp) return;
			if (document.visibilityState === 'visible') {
				ready = false;
				await checkOauths();
				await refreshUserServers();
				ready = true;
			}
		};
		document.addEventListener('visibilitychange', handleVisibilityChange);
		return () => document.removeEventListener('visibilitychange', handleVisibilityChange);
	});

	onMount(async () => {
		ready = false;
		await checkOauths();
		await refreshUserServers();
		ready = true;
		maybeOpenDialogs();
	});

	$effect(() => {
		if (isInMcp || !ready) return;
		if (oauthQueue.length > 0 || configQueue.length > 0) {
			maybeOpenDialogs();
		}
	});

	async function maybeOpenDialogs() {
		await tick();

		if (oauthQueue.length > 0) {
			oauthIndex = oauthQueue.findIndex((m) => !closed.has(m.id));
			oauthDialogs[oauthIndex]?.showModal();
			return;
		}

		if (configQueue.length > 0) {
			openConfigAt(configQueue.findIndex((s) => !closed.has(s.id)));
		}
	}

	async function next() {
		// Close the current dialog first
		const currentDialog = oauthDialogs[oauthIndex];
		if (currentDialog) {
			currentDialog.close();
		}

		// Wait for the close operation to complete
		await tick();

		// Move to next dialog if available
		const nextOauthIndex = oauthQueue.findIndex(
			(m, index) => !closed.has(m.id) && index > oauthIndex
		);
		if (nextOauthIndex < oauthQueue.length - 1 && nextOauthIndex !== -1) {
			oauthIndex = nextOauthIndex;
			oauthDialogs[oauthIndex]?.showModal();
			return;
		}

		const nextConfigIndex = configQueue.findIndex((s) => !closed.has(s.id));
		if (nextConfigIndex !== -1) {
			openConfigAt(nextConfigIndex);
		}
	}

	async function openConfigAt(index: number) {
		configIndex = index;
		const server = configQueue[configIndex];
		if (!server) return;
		const parent = server.catalogEntryID ? entriesById.get(server.catalogEntryID) : undefined;
		await prepareConfigureForm(server, parent);
		configDialog?.open();
	}

	async function prepareConfigureForm(server: MCPCatalogServer, parent?: MCPCatalogEntry) {
		configError = '';
		configuring = false;
		configName = server.alias || server.manifest?.name || '';
		configIcon = server.manifest?.icon || '';
		configServerId = server.id;

		let values: Record<string, string> = {};
		try {
			values = await ChatService.revealSingleOrRemoteMcpServer(server.id, { dontLogErrors: true });
		} catch (error) {
			if (error instanceof Error && !error.message.includes('404')) {
				console.error('Failed to reveal user server values:', error);
			}
			values = {};
		}

		configureForm = {
			envs: server.manifest.env?.map((env) => ({ ...env, value: values[env.key] ?? '' })),
			headers: server.manifest.remoteConfig?.headers?.map((header) => ({
				...header,
				value: values[header.key] ?? ''
			})),
			url: server.manifest.remoteConfig?.url,
			hostname: parent?.manifest.remoteConfig?.hostname
		};
	}

	async function handleSaveConfig() {
		const server = configQueue[configIndex];
		if (!server || !configureForm) return;
		configuring = true;
		configError = '';
		const parent = server.catalogEntryID ? entriesById.get(server.catalogEntryID) : undefined;
		try {
			if (parent?.manifest.runtime === 'remote' && configureForm.url) {
				await ChatService.updateRemoteMcpServerUrl(server.id, configureForm.url.trim());
			}
			const secretValues = convertEnvHeadersToRecord(configureForm.envs, configureForm.headers);
			await ChatService.configureSingleOrRemoteMcpServer(server.id, secretValues);
			configDialog?.close();
			await refreshUserServers();
			// Refresh project MCPs to clear warnings in the sidebar
			try {
				const refreshed = await ChatService.listProjectMCPs(assistantId, projectId);
				const validated = await validateOauthProjectMcps(assistantId, projectId, refreshed);
				projectMcps.items = validated.length > 0 ? validated : refreshed;
			} catch {
				// ignore refresh errors
			}
			const nextConfigIndex = configQueue.findIndex(
				(s, index) => !closed.has(s.id) && index > configIndex
			);
			if (nextConfigIndex !== -1) {
				await openConfigAt(nextConfigIndex);
			}
		} catch (error) {
			configError = error instanceof Error ? error.message : 'Unknown error';
		} finally {
			configuring = false;
		}
	}
</script>

{#each oauthQueue as mcpServer, i (mcpServer.id)}
	<dialog bind:this={oauthDialogs[i]} class="p-4 md:w-sm">
		<div class="flex w-full flex-col gap-4">
			<div class="absolute top-2 right-2">
				<button
					class="icon-button"
					onclick={() => {
						closed.add(mcpServer.id);
						next();
					}}
				>
					<X class="size-4" />
				</button>
			</div>
			<div class="flex items-center gap-2">
				<div class="h-fit flex-shrink-0 self-start rounded-md bg-gray-50 p-1 dark:bg-gray-600">
					{#if mcpServer.icon}
						<img src={mcpServer.icon} alt={mcpServer.name} class="size-6" />
					{:else}
						<Server class="size-6" />
					{/if}
				</div>
				<h3 class="text-lg leading-5.5 font-semibold">{mcpServer.name}</h3>
			</div>
			<p>In order to use {mcpServer.name}, authentication with the MCP server is required.</p>
			<p>Click the link below to authenticate.</p>
			<a
				href={mcpServer.oauthURL}
				target="_blank"
				class="button-primary text-center text-sm outline-none"
				onclick={next}
			>
				Authenticate
			</a>
		</div>
	</dialog>
{/each}

<CatalogConfigureForm
	bind:this={configDialog}
	bind:form={configureForm}
	disableOutsideClick
	name={configName}
	icon={configIcon}
	serverId={configServerId}
	submitText="Update"
	loading={configuring}
	error={configError}
	onSave={handleSaveConfig}
	onClose={async () => {
		closed.add(configServerId);
		// wait 300ms -- the close animation length of the dialog
		await new Promise((resolve) => setTimeout(resolve, 300));
		const nextConfigIndex = configQueue.findIndex(
			(s, index) => !closed.has(s.id) && index > configIndex
		);
		if (nextConfigIndex !== -1) {
			await tick();
			await openConfigAt(nextConfigIndex);
		}
	}}
/>
