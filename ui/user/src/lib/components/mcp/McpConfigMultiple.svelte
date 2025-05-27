<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import { type Project, ChatService } from '$lib/services';
	import { fetchConfigurationStatuses, getKeyValuePairs } from '$lib/services/chat/mcp';
	import { Server, X } from 'lucide-svelte';
	import McpInfoConfig from './McpInfoConfig.svelte';
	import { dialogAnimation } from '$lib/actions/dialogAnimation';
	import { getProjectMCPs } from '$lib/context/projectMcps.svelte';
	import { getToolBundleMap } from '$lib/context/toolReferences.svelte';
	import { onMount } from 'svelte';

	interface Props {
		project: Project;
		chatbot?: boolean;
	}

	let { project, chatbot }: Props = $props();
	let currentIndex = $state(0);
	let view = $state<'start' | 'finish' | 'error' | undefined>();
	let infoDialog = $state<HTMLDialogElement>();

	const projectMcps = getProjectMCPs();
	const toolBundleMap = getToolBundleMap();

	let dialogs = $state<ReturnType<typeof McpInfoConfig>[]>();

	async function initRequiresConfiguration() {
		const response = await fetchConfigurationStatuses(
			project,
			projectMcps.items,
			toolBundleMap,
			chatbot ?? false
		);
		projectMcps.configured = response?.configured || {};
		projectMcps.requiresConfiguration = response?.requiresConfiguration || {};

		// if requires configuration
		const numRequiresConfiguration = Object.keys(projectMcps.requiresConfiguration).length;
		if (numRequiresConfiguration > 0) {
			dialogs = Array(numRequiresConfiguration).fill(undefined); // mcps to how many require configuration
			currentIndex = 0;
			view = 'start';
			infoDialog?.showModal();
		}
	}

	onMount(() => {
		initRequiresConfiguration();
	});
</script>

<dialog
	bind:this={infoDialog}
	class="default-dialog p-6"
	use:clickOutside={() => infoDialog?.close()}
	use:dialogAnimation={{ type: 'fade' }}
>
	<button class="absolute top-0 right-0 p-3" onclick={() => infoDialog?.close()}>
		<X class="icon-default" />
	</button>
	<h3 class="mb-4 text-lg font-semibold">Configure MCP Servers</h3>

	{#if view === 'start'}
		<p class="text-sm text-gray-600">
			To use this agent, you'll need to configure the following MCP servers:
		</p>
		<ul class="mt-2 flex flex-col gap-2">
			{#each Object.values(projectMcps.requiresConfiguration) as mcp}
				<li class="text-md flex items-center gap-2">
					{#if mcp.icon}
						<img src={mcp.icon} alt={mcp.name} class="size-6" />
					{:else}
						<Server class="size-6" />
					{/if}
					{mcp.name}
				</li>
			{/each}
		</ul>
		<button
			class="button-primary mt-6 w-full"
			onclick={() => {
				infoDialog?.close();
				dialogs?.[currentIndex]?.open();
			}}
		>
			Configure Now
		</button>
	{:else if view === 'finish' || view === 'error'}
		<p class="max-w-md text-sm text-gray-600">
			{#if view === 'finish'}
				You're all set! You can now use this agent.
			{:else}
				Looks like there was an issue during configuration. Verify the MCP server configurations
				under MCP Servers.
			{/if}
		</p>
		<button
			class="button-primary mt-6 w-full"
			onclick={() => {
				infoDialog?.close();
			}}
		>
			{view === 'finish' ? "Let's Go!" : 'Continue'}
		</button>
	{/if}
</dialog>

{#if dialogs}
	{@const numToConfigure = Object.keys(projectMcps.requiresConfiguration).length}
	{#each Object.values(projectMcps.requiresConfiguration) as mcp, index (mcp.id)}
		<McpInfoConfig
			bind:this={dialogs[index]}
			animation="slide"
			manifest={mcp}
			{project}
			submitText={index === numToConfigure - 1 ? 'Configure & Finish' : 'Configure & Next'}
			legacyBundleId={mcp.catalogID && toolBundleMap.get(mcp.catalogID) ? mcp.catalogID : undefined}
			onUpdate={async (manifest) => {
				if (!project?.assistantID || !project.id || !mcp) return;

				const keyValuePairs = getKeyValuePairs(manifest);
				await ChatService.configureProjectMCPEnvHeaders(
					project.assistantID,
					project.id,
					mcp.id,
					keyValuePairs
				);

				dialogs?.[currentIndex]?.close();
				currentIndex = currentIndex + 1;

				if (currentIndex < numToConfigure) {
					dialogs?.[currentIndex]?.open();
				} else {
					const response = await fetchConfigurationStatuses(
						project,
						projectMcps.items,
						toolBundleMap,
						chatbot ?? false
					);
					projectMcps.configured = response?.configured || {};
					projectMcps.requiresConfiguration = response?.requiresConfiguration || {};
					if (Object.keys(projectMcps.requiresConfiguration).length > 0) {
						view = 'error';
						setTimeout(() => {
							infoDialog?.showModal();
						}, 200);
					} else {
						view = 'finish';
						setTimeout(() => {
							infoDialog?.showModal();
						}, 200); // waiting for slideout animation to complete from last config dialog
					}
				}
			}}
		/>
	{/each}
{/if}
