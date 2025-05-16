<script lang="ts">
	import { ChatService, type Project, type ProjectShare, type ProjectMCP } from '$lib/services';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import { browser } from '$app/environment';
	import { X, ChevronDown, Server, Pencil } from 'lucide-svelte';
	import { closeSidebarConfig, getLayout } from '$lib/context/layout.svelte';
	import {
		listProjectMCPs,
		deconfigureSharedProjectMCP,
		revealSharedProjectMCP,
		configureSharedProjectMCP
	} from '$lib/services/chat/operations';
	import { onMount } from 'svelte';
	import { clickOutside } from '$lib/actions/clickoutside';
	import {
		isValidMcpConfig,
		type MCPServerInfo,
		isAuthRequiredBundle,
		initConfigFromManifest
	} from '$lib/services/chat/mcp';
	import CredentialAuth from '$lib/components/edit/CredentialAuth.svelte';
	import { getToolBundleMap } from '$lib/context/toolReferences.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import type { ProjectCredential } from '$lib/services/chat/types';
	import { DEFAULT_CUSTOM_SERVER_NAME } from '$lib/constants';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
	let share = $state<ProjectShare>();
	let url = $derived(
		browser && share?.publicID
			? `${window.location.protocol}//${window.location.host}/s/${share.publicID}`
			: ''
	);
	const layout = getLayout();
	const toolBundleMap = getToolBundleMap();

	let mcpServers = $state<ProjectMCP[]>([]);
	let loading = $state(false);
	let selectedMcpConfig = $state<Record<string, string>>({});
	let sharedConfigs = $state<Record<string, boolean>>({});
	let dropdownOpen = $state<Record<string, boolean>>({});
	let showConfigDialog = $state(false);
	let showAuthDialog = $state(false);
	let currentMcpServer = $state<ProjectMCP | null>(null);
	let mcpConfig = $state<MCPServerInfo | null>(null);
	let showSubmitError = $state(false);
	let processingConfig = $state(false);
	let currentCredential = $state<ProjectCredential | null>(null);
	let showDeconfigureConfirm = $state(false);
	let serverToDeconfigure = $state<string | null>(null);

	const configureForEveryone = 'Configure it for everyone';
	const letUserConfigure = 'Let the user configure';

	const getBundleId = (server: ProjectMCP) =>
		server.catalogID && toolBundleMap.get(server.catalogID) ? server.catalogID : null;

	const setShared = (serverId: string) => {
		sharedConfigs = { ...sharedConfigs, [serverId]: true };
		selectedMcpConfig = { ...selectedMcpConfig, [serverId]: configureForEveryone };
	};

	const markUnshared = (serverId: string) => {
		sharedConfigs = { ...sharedConfigs, [serverId]: false };
		selectedMcpConfig = { ...selectedMcpConfig, [serverId]: letUserConfigure };
	};

	const resetConfigDialogState = () => {
		showConfigDialog = false;
		currentMcpServer = null;
		mcpConfig = null;
		showSubmitError = false;
	};

	async function updateShare() {
		share = await ChatService.getProjectShare(project.assistantID, project.id);
	}

	async function loadMcpServers() {
		loading = true;
		try {
			mcpServers = await listProjectMCPs(project.assistantID, project.id);

			selectedMcpConfig = Object.fromEntries(
				mcpServers.map((server) => [server.id, letUserConfigure])
			);

			await Promise.all(
				mcpServers.map(async (server) => {
					if (server.catalogID) await checkToolCredentials(server);
					await checkSharedConfig(server.id);
				})
			);
		} catch (error) {
			console.error('Failed to load MCP servers:', error);
		} finally {
			loading = false;
		}
	}

	async function checkSharedConfig(mcpServerId: string) {
		try {
			const result = await revealSharedProjectMCP(project.assistantID, project.id, mcpServerId);
			if (result && Object.keys(result).length > 0) setShared(mcpServerId);
			else sharedConfigs = { ...sharedConfigs, [mcpServerId]: false };
			return result;
		} catch {
			sharedConfigs = { ...sharedConfigs, [mcpServerId]: false };
			return null;
		}
	}

	async function checkToolCredentials(server: ProjectMCP) {
		try {
			const credentials = await ChatService.listProjectCredentials(project.assistantID, project.id);
			const toolCredential = credentials.items.find(
				(cred) => cred.toolID === server.catalogID && cred.exists
			);
			if (toolCredential) setShared(server.id);
		} catch (error) {
			console.error('Failed to check tool credentials:', error);
		}
	}

	function toggleDropdown(id: string) {
		dropdownOpen = { ...dropdownOpen, [id]: !dropdownOpen[id] };
	}

	function selectOption(serverId: string, option: string) {
		dropdownOpen = { ...dropdownOpen, [serverId]: false };
		const server = mcpServers.find((s) => s.id === serverId);
		if (!server) return;

		if (option === configureForEveryone) {
			openConfigDialog(server);
			return;
		}

		if (
			option === letUserConfigure &&
			(sharedConfigs[serverId] ||
				(server.catalogID && selectedMcpConfig[serverId] === configureForEveryone))
		) {
			serverToDeconfigure = serverId;
			showDeconfigureConfirm = true;
			return;
		}

		selectedMcpConfig = { ...selectedMcpConfig, [serverId]: option };
	}

	async function deconfigureSharedServer() {
		if (!serverToDeconfigure) return;
		const server = mcpServers.find((s) => s.id === serverToDeconfigure);
		if (!server) return;

		try {
			const bundleId = getBundleId(server);
			const isAuthTool = bundleId && isAuthRequiredBundle(bundleId);

			if (isAuthTool && server.catalogID) {
				await ChatService.deleteProjectCredential(
					project.assistantID,
					project.id,
					server.catalogID
				);
			} else {
				await deconfigureSharedProjectMCP(project.assistantID, project.id, server.id);
			}

			markUnshared(server.id);
		} catch (error) {
			console.error('Failed to deconfigure shared server or credential:', error);
		} finally {
			serverToDeconfigure = null;
			showDeconfigureConfirm = false;
		}
	}

	function cancelDeconfigure() {
		serverToDeconfigure = null;
		showDeconfigureConfirm = false;
	}

	function openConfigDialog(server: ProjectMCP) {
		currentMcpServer = server;
		const bundleId = getBundleId(server);
		const isAuthTool = bundleId && isAuthRequiredBundle(bundleId);
		if (isAuthTool) {
			handleToolAuth(server, bundleId);
		} else {
			mcpConfig = initConfigFromManifest(server);
			showConfigDialog = true;
		}
	}

	async function handleToolAuth(server: ProjectMCP, bundleId: string) {
		const credentials = await ChatService.listProjectCredentials(project.assistantID, project.id);
		let credential = credentials.items.find((cred) => cred.toolID === bundleId);
		if (credential?.exists) {
			await ChatService.deleteProjectCredential(project.assistantID, project.id, bundleId);
			credential.exists = false;
		}
		if (!credential) {
			credential = {
				toolID: bundleId,
				toolName: server.name || bundleId,
				icon: server.icon,
				exists: false
			};
		}
		currentCredential = credential;
		showAuthDialog = true;
	}

	function closeAuthDialog() {
		showAuthDialog = false;
		currentCredential = null;
	}

	async function handleConfigSubmit() {
		if (!currentMcpServer || !mcpConfig) return;
		if (!isValidMcpConfig(mcpConfig)) {
			showSubmitError = true;
			return;
		}

		showSubmitError = false;
		processingConfig = true;
		try {
			const keyValues: Record<string, string> = {};
			for (const item of [...(mcpConfig.env || []), ...(mcpConfig.headers || [])]) {
				if (item.key && item.value) keyValues[item.key] = item.value;
			}

			await configureSharedProjectMCP(
				project.assistantID,
				project.id,
				currentMcpServer.id,
				keyValues
			);

			setShared(currentMcpServer.id);
			resetConfigDialogState();
		} catch (error) {
			console.error('Failed to configure shared configuration:', error);
		} finally {
			processingConfig = false;
		}
	}

	onMount(() => {
		if (project) {
			updateShare();
			loadMcpServers();
		}
	});

	async function handleChange(checked: boolean) {
		if (checked) {
			share = await ChatService.createProjectShare(project.assistantID, project.id);
		} else {
			await ChatService.deleteProjectShare(project.assistantID, project.id);
			share = undefined;
		}
	}
</script>

<div class="flex w-full flex-col">
	<div
		class="dark:border-surface2 flex w-full justify-center border-b border-transparent px-4 py-4 md:px-8"
	>
		<div class="flex w-full items-start justify-between md:max-w-[1200px]">
			<h4 class="text-xl font-semibold">ChatBot Configuration</h4>
			<button onclick={() => closeSidebarConfig(layout)} class="icon-button">
				<X class="size-6" />
			</button>
		</div>
	</div>
	<div class="flex w-full justify-center px-4 py-8 md:px-8">
		<div class="flex w-full flex-col items-start gap-6 md:max-w-[1200px]">
			<div class="flex w-full items-center justify-between">
				<h5 class="text-lg font-semibold">Enable</h5>
				<Toggle label="Toggle ChatBot" checked={!!share?.publicID} onChange={handleChange} />
			</div>

			<div class="w-full">
				<p class="text-sm text-gray-600">
					Create a simplified chat-only view of this agent to share with others. Users of your
					chatbot will get see their own threads and files.
				</p>
			</div>

			{#if share?.publicID}
				<div class="w-full">
					<h5 class="mb-2 font-medium">Share this URL with your users</h5>
					<div class="bg-surface1 flex w-full items-center gap-2 rounded-md p-2">
						<CopyButton text={url} />
						<a href={url} class="overflow-hidden text-ellipsis hover:underline">{url}</a>
					</div>
				</div>

				<div class="mt-4 w-full">
					<h5 class="mb-3 text-lg font-semibold">MCP Servers</h5>
					<p class="mb-4 text-sm text-gray-600">
						How do you want to configure this chatbot's MCP Servers? Copying your configuration will
						copy all configuration including sensitive items such as API keys. The user will not be
						able to see these values, but the MCP servers will use them.
					</p>

					{#if loading}
						<p class="text-sm text-gray-500 italic">Loading MCP servers...</p>
					{:else if mcpServers.length === 0}
						<p class="text-sm text-gray-500">No MCP servers configured.</p>
					{:else}
						<div class="flex w-full flex-col gap-2">
							{#each mcpServers as server}
								<div
									class="group hover:bg-surface3 flex w-full items-center justify-between rounded-md p-2 transition-colors duration-200"
								>
									<div class="flex items-center gap-2">
										<div class="rounded-md bg-gray-50 p-1 dark:bg-gray-600">
											{#if server.icon}
												<img src={server.icon} class="size-4" alt={server.name} />
											{:else}
												<Server class="size-4" />
											{/if}
										</div>
										<span class="text-sm">{server.name || DEFAULT_CUSTOM_SERVER_NAME}</span>
									</div>

									<div class="relative flex items-center gap-2">
										{#if (sharedConfigs[server.id] || server.catalogID) && selectedMcpConfig[server.id] === configureForEveryone}
											<button
												class="icon-button text-blue-500 hover:text-blue-700"
												onclick={() => openConfigDialog(server)}
											>
												<Pencil class="size-4" />
											</button>
										{/if}
										<button
											class="border-surface3 flex min-w-40 items-center justify-between rounded border px-3 py-1.5"
											onclick={() => toggleDropdown(server.id)}
										>
											<span class="text-sm">
												{selectedMcpConfig[server.id] || '--'}
											</span>
											<ChevronDown class="size-4" />
										</button>
										{#if dropdownOpen[server.id]}
											<div
												class="border-surface3 bg-surface1 absolute top-full right-0 z-10 mt-1 w-full rounded border shadow-md"
											>
												<div class="flex flex-col p-1">
													<button
														class="hover:bg-surface2 rounded px-3 py-2 text-left text-sm"
														onclick={() => selectOption(server.id, configureForEveryone)}
													>
														{configureForEveryone}
													</button>
													<button
														class="hover:bg-surface2 rounded px-3 py-2 text-left text-sm"
														onclick={() => selectOption(server.id, letUserConfigure)}
													>
														{letUserConfigure}
													</button>
												</div>
											</div>
										{/if}
									</div>
								</div>
							{/each}
						</div>
					{/if}
				</div>
			{/if}
		</div>
	</div>
</div>

{#if showConfigDialog && currentMcpServer && mcpConfig}
	<dialog open class="default-dialog w-full max-w-xl p-6" use:clickOutside={resetConfigDialogState}>
		<div class="flex flex-col gap-4">
			<div class="flex items-center justify-between">
				<div class="flex items-center gap-2">
					<div class="h-fit flex-shrink-0 self-start rounded-md bg-gray-50 p-1 dark:bg-gray-600">
						{#if currentMcpServer.icon}
							<img src={currentMcpServer.icon} alt={currentMcpServer.name} class="size-6" />
						{:else}
							<Server class="size-6" />
						{/if}
					</div>
					<h3 class="text-lg font-semibold">
						Configure {currentMcpServer.name || 'MCP Server'}
					</h3>
				</div>
				<button class="icon-button" onclick={resetConfigDialogState}>
					<X class="size-6" />
				</button>
			</div>

			<p class="text-sm text-gray-600">
				Configure the shared credentials for this MCP server. These will be used by all chatbot
				users.
			</p>

			<div class="max-h-[60vh] overflow-y-auto">
				{#if mcpConfig}
					<div class="flex flex-col gap-4">
						{#if mcpConfig.env && mcpConfig.env.length > 0}
							<div class="flex flex-col gap-1">
								<h4 class="text-base font-semibold">Environment Variables</h4>
								{#each mcpConfig.env as env}
									<div class="flex w-full items-center gap-2">
										<div class="flex grow flex-col gap-1">
											{#if env.custom}
												<input
													class="ghost-input w-full py-0"
													bind:value={env.key}
													placeholder="Key (ex. API_KEY)"
												/>
											{:else}
												<label for={env.name} class="flex items-center gap-1 text-sm font-light">
													{env.required
														? `${env.name || env.key}*`
														: `${env.name || env.key} (optional)`}
													{#if env.description}
														<span class="text-xs text-gray-500" title={env.description}>ⓘ</span>
													{/if}
												</label>
											{/if}
											{#if env.sensitive}
												<input
													data-1p-ignore
													id={env.name}
													name={env.name}
													class="text-input-filled w-full"
													class:error={showSubmitError && !env.value && env.required}
													bind:value={env.value}
													type="password"
												/>
											{:else}
												<input
													data-1p-ignore
													id={env.name}
													name={env.name}
													class="text-input-filled w-full"
													class:error={showSubmitError && !env.value && env.required}
													bind:value={env.value}
													type="text"
												/>
											{/if}

											<div class="min-h-4 text-xs text-red-500">
												{#if showSubmitError && !env.value && env.required}
													This field is required.
												{/if}
											</div>
										</div>
									</div>
								{/each}
							</div>
						{/if}

						{#if mcpConfig.headers && mcpConfig.headers.length > 0}
							<div class="flex flex-col gap-1">
								<h4 class="text-base font-semibold">Headers</h4>
								{#each mcpConfig.headers as header}
									<div class="flex w-full items-center gap-2">
										<div class="flex grow flex-col gap-1">
											{#if header.custom}
												<input
													class="ghost-input w-full py-0"
													bind:value={header.key}
													placeholder="Key (ex. Authorization)"
												/>
											{:else}
												<label for={header.name} class="flex items-center gap-1 text-sm font-light">
													{header.required
														? `${header.name || header.key}*`
														: `${header.name || header.key} (optional)`}
													{#if header.description}
														<span class="text-xs text-gray-500" title={header.description}>ⓘ</span>
													{/if}
												</label>
											{/if}
											{#if header.sensitive}
												<input
													data-1p-ignore
													id={header.name}
													name={header.name}
													class="text-input-filled w-full"
													class:error={showSubmitError && !header.value && header.required}
													bind:value={header.value}
													type="password"
												/>
											{:else}
												<input
													data-1p-ignore
													id={header.name}
													name={header.name}
													class="text-input-filled w-full"
													class:error={showSubmitError && !header.value && header.required}
													bind:value={header.value}
													type="text"
												/>
											{/if}

											<div class="min-h-4 text-xs text-red-500">
												{#if showSubmitError && !header.value && header.required}
													This field is required.
												{/if}
											</div>
										</div>
									</div>
								{/each}
							</div>
						{/if}

						{#if (!mcpConfig.env || mcpConfig.env.length === 0) && (!mcpConfig.headers || mcpConfig.headers.length === 0)}
							<p class="text-sm text-gray-500">This server does not require any configuration.</p>
						{/if}
					</div>
				{/if}
			</div>

			<div class="flex justify-end gap-2 pt-2">
				<button class="button-secondary" onclick={resetConfigDialogState}>Cancel</button>
				<button class="button-primary" onclick={handleConfigSubmit} disabled={processingConfig}>
					{#if processingConfig}
						<span class="animate-spin">⟳</span> Saving...
					{:else}
						Save Configuration
					{/if}
				</button>
			</div>
		</div>
	</dialog>
{/if}

{#if showAuthDialog && currentMcpServer && currentCredential}
	<dialog open class="default-dialog w-full max-w-xl" use:clickOutside={closeAuthDialog}>
		<CredentialAuth
			inline
			toolID={currentCredential.toolID}
			{project}
			credential={currentCredential}
			onClose={(error) => {
				if (!error && currentMcpServer) {
					// Update UI after successful authentication
					sharedConfigs = { ...sharedConfigs, [currentMcpServer.id]: true };
					selectedMcpConfig = {
						...selectedMcpConfig,
						[currentMcpServer.id]: configureForEveryone
					};
				}
				closeAuthDialog();
			}}
		/>
	</dialog>
{/if}

{#if showDeconfigureConfirm && serverToDeconfigure}
	<Confirm
		show={showDeconfigureConfirm}
		msg="Are you sure you want to remove the shared configuration? Users will need to configure this service themselves."
		onsuccess={deconfigureSharedServer}
		oncancel={cancelDeconfigure}
	/>
{/if}
