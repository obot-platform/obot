<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { getProjectMCPs } from '$lib/context/projectMcps.svelte';
	import { ChatService, type Project, type ProjectMCP, type Thread } from '$lib/services';
	import { Server, Wrench } from 'lucide-svelte';
	import ProjectMcpServerTools from '$lib/components/mcp/ProjectMcpServerTools.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import { fade } from 'svelte/transition';
	import { DEFAULT_CUSTOM_SERVER_NAME } from '$lib/constants';
	import { responsive } from '$lib/stores';
	import PageLoading from '../PageLoading.svelte';

	interface Props {
		project: Project;
		currentThreadID?: string;
	}

	let { project, currentThreadID = $bindable() }: Props = $props();
	let dialog = $state<HTMLDialogElement | undefined>();
	let selectedProjectMcp = $state<ProjectMCP>();
	const projectMCPs = getProjectMCPs();
	let authenticatedMcps = $derived(projectMCPs.items.filter((mcp) => mcp.authenticated));
	let loading = $state(false);

	async function sleep(ms: number): Promise<void> {
		return new Promise((resolve) => setTimeout(resolve, ms));
	}

	async function createThread(): Promise<Thread> {
		let thread = await ChatService.createThread(project.assistantID, project.id);
		while (!thread.ready) {
			await sleep(1000);
			thread = await ChatService.getThread(project.assistantID, project.id, thread.id);
		}
		return thread;
	}
</script>

{#if authenticatedMcps.length > 0}
	<div use:tooltip={'Tools'} in:fade>
		<DotDotDot class="icon-button hover:bg-surface2 hover:text-blue-500">
			{#snippet icon()}
				<Wrench class="h-5 w-5" />
			{/snippet}
			<div class="default-dialog flex min-w-max flex-col p-2">
				{#each authenticatedMcps as projectMcp (projectMcp.id)}
					<button
						class="menu-button"
						onclick={async () => {
							selectedProjectMcp = projectMcp;

							if (!currentThreadID) {
								loading = true;
								const thread = await createThread();
								currentThreadID = thread.id;
								loading = false;
							}
							dialog?.showModal();
						}}
					>
						<div class="flex-shrink-0 rounded-md bg-gray-50 p-1 dark:bg-gray-600">
							{#if projectMcp.icon}
								<img src={projectMcp.icon} alt={projectMcp.name} class="size-4" />
							{:else}
								<Server class="size-4" />
							{/if}
						</div>
						{projectMcp.name || DEFAULT_CUSTOM_SERVER_NAME}
					</button>
				{/each}
			</div>
		</DotDotDot>
	</div>
{/if}

<dialog
	bind:this={dialog}
	use:clickOutside={() => {
		dialog?.close();
	}}
	class="default-dialog w-full max-w-(--breakpoint-xl) p-4 pb-0"
	class:mobile-screen-dialog={responsive.isMobile}
>
	{#if selectedProjectMcp}
		{#key selectedProjectMcp.id}
			<ProjectMcpServerTools
				{currentThreadID}
				{project}
				mcpServer={selectedProjectMcp}
				onSubmit={() => dialog?.close()}
				onClose={() => dialog?.close()}
				submitText="Update"
			/>
		{/key}
	{/if}
</dialog>

<PageLoading show={loading} text="Loading tools..." />
