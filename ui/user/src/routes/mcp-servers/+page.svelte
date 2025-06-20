<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import Table from '$lib/components/Table.svelte';
	import { createProjectMcp, type MCPServerInfo } from '$lib/services/chat/mcp';
	import {
		ChatService,
		type MCP,
		type MCPCatalogServer,
		type ProjectMCP
	} from '$lib/services/index.js';
	import { responsive } from '$lib/stores';
	import { ChevronRight, LoaderCircle, Server, Trash2, Unplug, X } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';

	let { data } = $props();
	const { project } = data;

	let projectServers = $state<ProjectMCP[]>([]);
	let servers = $state<MCPCatalogServer[]>([]);
	let entries = $state<MCP[]>([]);
	let loading = $state(true);

	let deletingProjectMcp = $state<string>();
	let connectToEntry = $state<{
		matchingProject?: ProjectMCP;
		entry: MCP;
		envs: MCPServerInfo['env'];
		headers: MCPServerInfo['headers'];
		connectURL?: string;
		launching: boolean;
	}>();
	let connectToServer = $state<MCPCatalogServer>();
	let configDialog = $state<HTMLDialogElement>();

	let search = $state('');
	let connectedProjects = $derived(new Map(projectServers?.map((s) => [s.catalogEntryID, s])));
	let tableData = $derived([
		...(entries.map((e) => ({
			...e,
			connectedProject: connectedProjects.get(e.id)
		})) ?? []),
		...(servers ?? [])
	]);
	let filteredTableData = $derived(
		search
			? tableData.filter((item) => {
					const nameToUse =
						'name' in item
							? item.name
							: (item.commandManifest?.server.name ?? item.urlManifest?.server.name);
					return nameToUse?.toLowerCase().includes(search.toLowerCase());
				})
			: tableData
	);

	async function reloadProjectServers(assistantID: string, projectID: string) {
		const response = await ChatService.listProjectMCPs(assistantID, projectID);
		return response.filter((s) => !s.deleted);
	}

	async function loadData() {
		if (project) {
			loading = true;
			try {
				const [projectServersResult, entriesResult, serversResult] = await Promise.all([
					reloadProjectServers(project.assistantID, project.id),
					ChatService.listMCPs(),
					ChatService.listMCPCatalogServers()
				]);
				projectServers = projectServersResult.filter((s) => !s.deleted);
				entries = entriesResult;
				servers = serversResult;
			} catch (error) {
				console.error('Failed to load data:', error);
			} finally {
				loading = false;
			}
		}
	}

	onMount(() => {
		loadData();
	});

	function closeConfigDialog() {
		connectToServer = undefined;
		connectToEntry = undefined;
		configDialog?.close();
	}

	async function handleMcpServer(server: MCPCatalogServer) {
		connectToServer = server;
	}

	async function handleMcpEntry(entry: MCP, connectedProject?: ProjectMCP) {
		const envs = (
			(entry.commandManifest ? entry.commandManifest.server.env : entry.urlManifest?.server.env) ??
			[]
		).map((env) => ({ ...env, value: '' }));

		const headers = (
			(entry.commandManifest
				? entry.commandManifest.server.headers
				: entry.urlManifest?.server.headers) ?? []
		).map((header) => ({ ...header, value: '' }));

		connectToEntry = { entry, matchingProject: connectedProject, envs, headers, launching: false };

		if (connectedProject) {
			connectToEntry.connectURL = connectedProject.connectURL;
		} else if (envs.length === 0) {
			handleLaunch();
		}
	}

	async function handleLaunch() {
		if (connectToEntry && project) {
			connectToEntry.launching = true;
			const serverManifest =
				connectToEntry.entry.commandManifest?.server ?? connectToEntry.entry.urlManifest?.server;

			if (!serverManifest) {
				console.error('No server manifest found');
				return;
			}

			const mcpServerInfo: MCPServerInfo = {
				...serverManifest,
				env: connectToEntry.envs,
				headers: connectToEntry.headers
			};

			const response = await createProjectMcp(mcpServerInfo, project, connectToEntry.entry.id);
			projectServers = await reloadProjectServers(project.assistantID, project.id);
			connectToEntry.connectURL = response.connectURL;
			connectToEntry.launching = false;
		}
	}
</script>

<Layout>
	<div class="flex flex-col gap-8 py-8 pt-4" in:fade>
		<h1 class="text-2xl font-semibold">MCP Servers</h1>
		{#if loading}
			<div class="my-2 flex items-center justify-center">
				<LoaderCircle class="size-6 animate-spin" />
			</div>
		{:else}
			<Search
				class="dark:bg-surface1 dark:border-surface3 bg-white shadow-sm dark:border"
				onChange={(val) => {
					search = val;
				}}
				placeholder="Search by name..."
			/>
			<div class="flex flex-col gap-4">
				<Table data={filteredTableData} fields={['name']} pageSize={50}>
					{#snippet onRenderColumn(property, d)}
						{#if property === 'name'}
							<span class="flex items-center gap-1">
								{'name' in d
									? d.name
									: (d.commandManifest?.server.name ?? d.urlManifest?.server.name)}
							</span>
						{/if}
					{/snippet}
					{#snippet actions(d)}
						{#if 'connectedProject' in d && d.connectedProject}
							<button
								class="icon-button hover:text-red-500"
								onclick={async () => {
									if (!d.connectedProject) return;
									deletingProjectMcp = d.connectedProject.id;
									closeConfigDialog();
								}}
								use:tooltip={'Delete Server'}
							>
								<Trash2 class="size-4" />
							</button>
						{/if}
						<button
							class="icon-button hover:text-blue-500"
							use:tooltip={'Connect'}
							onclick={(e) => {
								e.stopPropagation();
								if (d.type === 'mcpserver') {
									handleMcpServer(d as MCPCatalogServer);
								} else {
									handleMcpEntry(d as MCP, connectedProjects.get(d.id));
								}
								configDialog?.showModal();
							}}
						>
							<Unplug class="size-4" />
						</button>
					{/snippet}
				</Table>
			</div>
		{/if}
	</div>
</Layout>

<dialog
	bind:this={configDialog}
	use:clickOutside={() => closeConfigDialog()}
	class="w-full md:w-lg"
	class:p-4={!responsive.isMobile}
	class:mobile-screen-dialog={responsive.isMobile}
>
	<h3 class="default-dialog-title" class:default-dialog-mobile-title={responsive.isMobile}>
		<span class="flex items-center gap-2">
			{#if connectToEntry}
				{@const name =
					connectToEntry.entry.commandManifest?.server.name ??
					connectToEntry.entry.urlManifest?.server.name}
				{@const icon =
					connectToEntry.entry.commandManifest?.server.icon ??
					connectToEntry.entry.urlManifest?.server.icon}
				{#if icon}
					<img src={icon} alt={name} class="size-6" />
				{:else}
					<Server class="size-6" />
				{/if}
				{name}
			{:else if connectToServer}
				{#if connectToServer.icon}
					<img src={connectToServer.icon} alt={connectToServer.name} class="size-6" />
				{:else}
					<Server class="size-6" />
				{/if}
				{connectToServer.name}
			{/if}
		</span>
		<button
			class:mobile-header-button={responsive.isMobile}
			onclick={() => closeConfigDialog()}
			class="icon-button"
		>
			{#if responsive.isMobile}
				<ChevronRight class="size-6" />
			{:else}
				<X class="size-5" />
			{/if}
		</button>
	</h3>

	{#if connectToEntry}
		{#if connectToEntry.launching}
			<div class="my-4 flex flex-col justify-center gap-4">
				<LoaderCircle class="size-6 animate-spin" />
			</div>
		{:else if connectToEntry.connectURL}
			<div class="my-4 flex flex-col gap-1">
				<label for="connectURL">Connect URL</label>
				<div class="mock-input-btn min-h-9">
					{connectToEntry.connectURL}
				</div>
			</div>
		{:else}
			<div class="my-4 flex flex-col gap-4">
				{#if connectToEntry.envs && connectToEntry.envs.length > 0}
					{#each connectToEntry.envs as env, i}
						<div class="flex flex-col gap-1">
							<label for={env.key}>{env.name}</label>
							<input
								type="text"
								id={env.key}
								bind:value={connectToEntry.envs[i].value}
								class="text-input-filled"
								placeholder={env.description}
							/>
						</div>
					{/each}
				{/if}
			</div>
			<div class="flex justify-end">
				<button class="button-primary" onclick={handleLaunch}>Launch</button>
			</div>
		{/if}
	{:else if connectToServer}
		<div class="my-4 flex flex-col gap-1">
			<label for="connectURL">Connect URL</label>
			<div class="mock-input-btn min-h-9">
				{connectToServer.connectURL}
			</div>
		</div>
	{/if}
</dialog>

<Confirm
	msg={'Are you sure you want to delete this server?'}
	show={Boolean(deletingProjectMcp)}
	onsuccess={async () => {
		if (deletingProjectMcp && project) {
			await ChatService.deleteProjectMCP(project.assistantID, project.id, deletingProjectMcp);
			projectServers = await reloadProjectServers(project.assistantID, project.id);
			deletingProjectMcp = undefined;
		}
	}}
	oncancel={() => (deletingProjectMcp = undefined)}
/>

<svelte:head>
	<title>Obot | MCP Servers</title>
</svelte:head>
