<script lang="ts">
	import { ChatService, type MCPServerPrompt, type Project, type ProjectMCP } from '$lib/services';
	import { getProjectMCPs } from '$lib/context/projectMcps.svelte';
	import Menu from '$lib/components/navbar/Menu.svelte';
	import { Plus } from 'lucide-svelte';
	import { responsive } from '$lib/stores';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	interface Props {
		project: Project;
		variant: 'button' | 'popover';
		filterText?: string;
	}

	type PromptSet = {
		mcp: ProjectMCP;
		prompts: MCPServerPrompt[];
	};

	let { project, variant, filterText }: Props = $props();
	let menu = $state<ReturnType<typeof Menu>>();
	let ref = $state<HTMLDivElement>();
	let loading = $state(false);
	let mcpPromptSets = $state<PromptSet[]>([]);
	const projectMcps = getProjectMCPs();

	$effect(() => {
		if (filterText && filterText.startsWith('/')) {
			ref?.classList.remove('hidden');
			fetchPrompts();
		} else {
			ref?.classList.add('hidden');
		}
	});

	function fetchPrompts() {
		loading = true;
		mcpPromptSets = [];
		for (const mcp of projectMcps.items) {
			ChatService.listProjectMcpServerPrompts(project.assistantID, project.id, mcp.id).then(
				(prompts) => {
					mcpPromptSets.push({
						mcp,
						prompts
					});
				}
			);
		}
		loading = false;
	}

	function handleClick(prompt: MCPServerPrompt, mcp: ProjectMCP) {
		if (variant === 'button') {
			menu?.toggle(false);
			// opens arguments dialog if arguments
		} else {
			ref?.classList.add('hidden');
			// same thing?
		}
	}
</script>

{#snippet content(filteredByNameDescription?: PromptSet[])}
	{@const setsToUse = filteredByNameDescription ?? mcpPromptSets}
	{#if setsToUse.length === 0}
		<div class="flex h-full flex-col items-center justify-center">
			<p class="text-sm text-gray-500">No prompts found</p>
		</div>
	{:else}
		{#each setsToUse as mcpPromptSet (mcpPromptSet.mcp.id)}
			{#each mcpPromptSet.prompts as prompt (prompt.name)}
				<button
					class="menu-button flex h-full w-full items-center gap-2 text-left"
					onclick={() => handleClick(prompt, mcpPromptSet.mcp)}
				>
					<img src={mcpPromptSet.mcp.icon} alt={mcpPromptSet.mcp.name} class="size-6 rounded-sm" />
					<div class="flex flex-col">
						<p class="text-xs font-semibold">
							{prompt.name}
							{#if variant === 'popover' && prompt.arguments}
								{#each prompt.arguments as argument}
									<span class="text-xs text-gray-500">
										[{argument.name}]
									</span>
								{/each}
							{/if}
						</p>
						<p class="text-xs text-gray-500">{prompt.description}</p>
					</div>
				</button>
			{/each}
		{/each}
	{/if}
{/snippet}

{#if variant === 'button'}
	<div use:tooltip={'Add Prompt'}>
		<Menu
			bind:this={menu}
			title=""
			classes={{
				button: 'button-icon-primary',
				dialog: responsive.isMobile
					? 'rounded-none max-h-[calc(100vh-64px)] left-0 bottom-0 w-full'
					: 'p-2'
			}}
			onLoad={fetchPrompts}
			slide={responsive.isMobile ? 'up' : undefined}
			fixed={responsive.isMobile}
		>
			{#snippet body()}
				{@render content()}
			{/snippet}
			{#snippet icon()}
				<Plus class="size-5" />
			{/snippet}
		</Menu>
	</div>
{:else if variant === 'popover'}
	{@const textToFilter = filterText?.slice(1) ?? ''}
	{@const filteredByNameDescription = filterText
		? mcpPromptSets
				.map((mcpPromptSet) => ({
					...mcpPromptSet,
					prompts: mcpPromptSet.prompts.filter(
						(prompt) =>
							prompt.name.toLowerCase().includes(textToFilter.toLowerCase()) ||
							prompt.description.toLowerCase().includes(textToFilter.toLowerCase())
					)
				}))
				.filter((mcpPromptSet) => mcpPromptSet.prompts.length > 0)
		: mcpPromptSets}
	<div
		bind:this={ref}
		class="default-dialog absolute top-0 left-0 hidden w-full -translate-y-full p-2"
	>
		{@render content(filteredByNameDescription)}
	</div>
{/if}
