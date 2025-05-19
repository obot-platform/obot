<script lang="ts">
	import { closeSidebarConfig, getLayout } from '$lib/context/layout.svelte';
	import { ChatService, type MCPServerTool, type Project, type ProjectMCP } from '$lib/services';
	import {
		createProjectMcp,
		isValidMcpConfig,
		updateProjectMcp,
		type MCPServerInfo
	} from '$lib/services/chat/mcp';
	import { ChevronsRight, LoaderCircle, PencilLine, Server, X } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import { onMount } from 'svelte';
	import { errors, responsive } from '$lib/stores';
	import HostedMcpForm from '$lib/components/mcp/HostedMcpForm.svelte';
	import RemoteMcpForm from '$lib/components/mcp/RemoteMcpForm.svelte';
	import { clickOutside } from '$lib/actions/clickoutside';
	import McpServerTools from './McpServerTools.svelte';
	import PageLoading from '$lib/components/PageLoading.svelte';
	import { DEFAULT_CUSTOM_SERVER_NAME } from '$lib/constants';
	import { fade } from 'svelte/transition';
	import { getProjectMCPs } from '$lib/context/projectMcps.svelte';

	interface Props {
		projectMcp?: ProjectMCP;
		project?: Project;
		onCreate?: (newProjectMcp: ProjectMCP) => void;
		onUpdate?: (updatedProjectConfig: MCPServerInfo) => void;
		chatbot?: boolean;
	}
	let { projectMcp, project, onCreate, onUpdate, chatbot }: Props = $props();
	let projectMcpServerToolsDialog = $state<HTMLDialogElement>();
	let projectMcpServerTools = $state<MCPServerTool[]>([]);
	let processing = $state(false);
	let savedProjectMcp = $state<ProjectMCP>();

	function isObotHosted(projectMcp: ProjectMCP) {
		// Prioritize command presence for determining if it's Obot-hosted
		// If there's no command but there's a URL, it should be treated as remote
		if (projectMcp.command && projectMcp.command !== '') {
			return true;
		}
		if (projectMcp.url && projectMcp.url !== '') {
			return false;
		}
		// If neither command nor URL is present, fall back to checking other properties
		return (projectMcp.args?.length ?? 0) > 0;
	}

	const initConfig: MCPServerInfo = projectMcp
		? {
				...projectMcp,
				env:
					projectMcp.env?.map((env) => ({
						...env,
						value: ''
					})) ?? [],
				headers:
					projectMcp.headers?.map((header) => ({
						...header,
						value: ''
					})) ?? []
			}
		: {
				description: '',
				icon: '',
				name: '',
				env: [],
				args: [],
				command: '',
				url: '',
				headers: []
			};

	let config = $state<MCPServerInfo>({ ...initConfig });
	let showObotHosted = $state(projectMcp ? isObotHosted(projectMcp) : true);
	let showSubmitError = $state(false);
	let showMcpError = $state(false);
	const layout = getLayout();
	const projectMCPs = getProjectMCPs();

	onMount(() => {
		if (projectMcp && project) {
			ChatService.revealProjectMCPEnvHeaders(project.assistantID, project.id, projectMcp.id)
				.then((response) => {
					if (config.env) {
						config.env = config.env.map((env) => ({
							...env,
							value: response?.[env.key] ?? env.value
						}));
					}
					if (config.headers) {
						config.headers = config.headers.map((header) => ({
							...header,
							value: response?.[header.key] ?? header.value
						}));
					}
				})
				.catch((err) => {
					// if 404, that's expected for reveal -- means no credentials were set
					if (
						(err instanceof Error && err.message.includes('404')) ||
						(typeof err === 'string' && err.includes('404'))
					) {
						return;
					}
					errors.append(err);
				});
		}
	});

	function init(isRemote?: boolean) {
		config = {
			...initConfig
		};
		showObotHosted = !isRemote;
	}

	async function handleSubmit() {
		if (!isValidMcpConfig(config)) {
			showSubmitError = true;
			return;
		}
		if (!project) return;
		if (projectMcp) {
			onUpdate?.(config);
		} else {
			processing = true;
			showMcpError = false;
			try {
				const newProjectMcp = savedProjectMcp
					? await updateProjectMcp(config, savedProjectMcp.id, project)
					: await createProjectMcp(config, project);
				if (!savedProjectMcp) {
					savedProjectMcp = newProjectMcp;
					projectMCPs.items.push(newProjectMcp);
				}
				projectMcpServerTools = await ChatService.listProjectMCPServerTools(
					project.assistantID,
					project.id,
					newProjectMcp.id
				);
				processing = false;
				projectMcpServerToolsDialog?.showModal();
			} catch (error) {
				console.error(error);
				processing = false;
				showMcpError = true;
			}
		}
	}
</script>

<div class="flex h-full w-full flex-col">
	<div class="mt-8 flex w-full flex-col gap-2 px-4 md:px-8">
		<div class="flex w-full flex-col gap-2 self-center md:max-w-[900px]">
			<div class="flex items-center justify-between gap-8">
				{#if projectMcp}
					<div class="flex flex-col gap-4">
						<div class="flex max-w-sm items-center gap-2">
							<div
								class="h-fit flex-shrink-0 self-start rounded-md bg-gray-50 p-1 dark:bg-gray-600"
							>
								{#if projectMcp.icon}
									<img src={projectMcp.icon} alt={projectMcp.name} class="size-6" />
								{:else}
									<Server class="size-6" />
								{/if}
							</div>
							<div class="flex flex-col gap-1">
								<h3 class="text-lg leading-4.5 font-semibold">
									{projectMcp.name || DEFAULT_CUSTOM_SERVER_NAME}
								</h3>
							</div>
						</div>
						{#if projectMcp.description}
							<p class="mb-4 text-sm font-light text-gray-500">
								{projectMcp.description}
							</p>
						{/if}
					</div>
				{:else}
					<h3 class="flex items-center gap-2 text-xl font-semibold">
						<PencilLine class="size-5" /> Create MCP Config
					</h3>
				{/if}
				<button
					class="icon-button h-fit w-fit flex-shrink-0 self-start"
					onclick={() => closeSidebarConfig(layout)}
				>
					<X class="size-6" />
				</button>
			</div>
		</div>
	</div>

	{#if !projectMcp?.catalogID && !chatbot}
		<div
			class="dark:bg-gray-980 mt-4 flex w-full flex-col gap-2 bg-gray-50 px-4 pt-4 pb-2 shadow-inner md:px-8"
		>
			<div class="flex w-full self-center md:max-w-[900px]">
				<div class="flex w-full gap-1">
					<button
						class={twMerge(
							'dark:bg-gray-980 flex-1 bg-gray-50 py-3',
							showObotHosted &&
								'dark:bg-surface2 dark:border-surface3 rounded-md bg-white shadow-sm dark:border'
						)}
						onclick={() => init()}
					>
						Obot Hosted
					</button>
					<button
						class={twMerge(
							'dark:bg-gray-980 flex-1 bg-gray-50 py-3',
							!showObotHosted &&
								'dark:bg-surface2 dark:border-surface3 rounded-md bg-white shadow-sm dark:border'
						)}
						onclick={() => init(true)}
					>
						Remote
					</button>
				</div>
			</div>
		</div>
	{/if}

	<div
		class="dark:bg-gray-980 relative flex flex-col gap-4 bg-gray-50 px-4 pb-4 md:px-8"
		class:pt-4={projectMcp?.catalogID}
	>
		<div
			class="dark:bg-surface2 dark:border-surface3 flex w-full flex-col gap-4 self-center rounded-lg bg-white px-4 py-8 shadow-sm md:max-w-[900px] md:px-8 dark:border"
		>
			{#if showMcpError}
				<p class="notification-error" in:fade>
					Failed to get tools, please check your configuration and try again.
				</p>
			{/if}
			{#if showObotHosted}
				<HostedMcpForm bind:config {showSubmitError} custom {chatbot} />
			{:else}
				<RemoteMcpForm bind:config {showSubmitError} custom {chatbot} />
			{/if}
		</div>
	</div>

	<div class="flex grow"></div>

	<div class="flex w-full flex-col gap-2 self-center md:max-w-[900px]">
		<div class="flex justify-end p-4 md:p-8">
			<button
				disabled={processing}
				class="button-primary flex items-center gap-1"
				onclick={handleSubmit}
			>
				{#if processing}
					<LoaderCircle class="size-4 animate-spin" />
				{:else}
					{projectMcp ? 'Update' : 'Configure'} server <ChevronsRight class="size-4" />
				{/if}
			</button>
		</div>
	</div>
</div>

<PageLoading show={processing} text="Launching and connecting to MCP server..." />

<dialog
	bind:this={projectMcpServerToolsDialog}
	class="default-dialog w-full max-w-(--breakpoint-xl) p-4 pb-0"
	class:mobile-screen-dialog={responsive.isMobile}
	use:clickOutside={() => {
		projectMcpServerToolsDialog?.close();
	}}
>
	{#if savedProjectMcp && project}
		{#key savedProjectMcp.id}
			<McpServerTools
				tools={projectMcpServerTools}
				mcpServer={savedProjectMcp}
				{project}
				onSubmit={async () => {
					if (savedProjectMcp) {
						projectMcpServerToolsDialog?.close();
						onCreate?.(savedProjectMcp);
					}
				}}
				isNew
			/>
		{/key}
	{/if}
</dialog>
