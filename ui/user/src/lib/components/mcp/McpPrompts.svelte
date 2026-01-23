<script lang="ts">
	import { ChatService, type MCPServerPrompt, type Project, type ProjectMCP } from '$lib/services';
	import { getProjectMCPs, validateOauthProjectMcps } from '$lib/context/projectMcps.svelte';
	import Menu from '$lib/components/navbar/Menu.svelte';
	import { LoaderCircle, MessageSquarePlus } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	interface Props {
		project: Project;
		variant: 'button' | 'popover' | 'messages';
		filterText?: string;
		onSelect?: (prompt: MCPServerPrompt, mcp: ProjectMCP, params?: Record<string, string>) => void;
		onClickOutside?: () => void;
		limit?: number;
		selectedIndex?: number;
	}

	type PromptSet = {
		mcp: ProjectMCP;
		prompts: MCPServerPrompt[];
	};

	let {
		project,
		variant,
		filterText,
		onSelect,
		limit = $bindable(0),
		selectedIndex = $bindable(0)
	}: Props = $props();
	let menu = $state<ReturnType<typeof Menu>>();
	let popover = $state<HTMLDivElement>();
	let loading = $state(false);
	let mcpPromptSets = $state<PromptSet[]>([]);
	let isHovering = $state(false);

	let params = $state<Record<string, string>>({});
	let selectedPrompt = $state<{ prompt: MCPServerPrompt; mcp: ProjectMCP }>();
	let argsDialog = $state<ReturnType<typeof ResponsiveDialog>>();

	let hasPrompts = $derived(mcpPromptSets.some((mcpPromptSet) => mcpPromptSet.prompts.length > 0));

	function getFilteredSets() {
		if (!filterText) return mcpPromptSets;
		const textToFilter = filterText.slice(1) ?? '';
		return mcpPromptSets
			.map((mcpPromptSet) => ({
				...mcpPromptSet,
				prompts: mcpPromptSet.prompts.filter(
					(prompt) =>
						prompt.name.toLowerCase().includes(textToFilter.toLowerCase()) ||
						prompt.description.toLowerCase().includes(textToFilter.toLowerCase())
				)
			}))
			.filter((mcpPromptSet) => mcpPromptSet.prompts.length > 0);
	}

	let setsToUse = $derived(filterText ? getFilteredSets() : mcpPromptSets);
	let indexMatchedPrompt = $derived(
		setsToUse
			.map((mcpPromptSet) =>
				mcpPromptSet.prompts.map((prompt) => ({ prompt, mcp: mcpPromptSet.mcp }))
			)
			.flat()[selectedIndex]
	);

	const projectMcps = getProjectMCPs();

	$effect(() => {
		if (filterText && filterText.startsWith('/')) {
			popover?.showPopover();
			fetchPrompts();
		} else {
			popover?.hidePopover();
		}
	});

	export function hasPromptHighlighted() {
		return !!indexMatchedPrompt;
	}

	export function triggerSelectPrompt() {
		if (indexMatchedPrompt) {
			handleClick(indexMatchedPrompt.prompt, indexMatchedPrompt.mcp);
		}
	}

	async function fetchPrompts() {
		loading = true;
		mcpPromptSets = [];
		await validateOauthProjectMcps(project.assistantID, project.id, projectMcps.items);
		for (const mcp of projectMcps.items) {
			if (mcp.authenticated) {
				await ChatService.listProjectMcpServerPrompts(project.assistantID, project.id, mcp.id).then(
					(prompts) => {
						mcpPromptSets.push({
							mcp,
							prompts
						});
					}
				);
			}
		}
		limit = mcpPromptSets.reduce((acc, mcpPromptSet) => acc + mcpPromptSet.prompts.length, 0);
		selectedIndex = 0;
		loading = false;
	}

	function handleClick(prompt: MCPServerPrompt, mcp: ProjectMCP) {
		if (variant === 'button') {
			menu?.toggle(false);
		} else {
			popover?.hidePopover();
		}

		if (prompt.arguments) {
			argsDialog?.open();
			selectedPrompt = { prompt, mcp };
		} else {
			onSelect?.(prompt, mcp);
		}
	}
</script>

{#snippet content()}
	{#if loading}
		<div class="flex h-full flex-col items-center justify-center">
			<LoaderCircle class="size-4 animate-spin" />
		</div>
	{:else if !hasPrompts && variant !== 'messages'}
		<div class="flex h-full flex-col items-center justify-center">
			<p class="text-on-surface1 text-sm">No prompts available</p>
		</div>
	{:else}
		{#each setsToUse as mcpPromptSet (mcpPromptSet.mcp.id)}
			{#if mcpPromptSet.prompts.length > 0}
				<div
					class={twMerge(
						'w-full text-xs font-semibold',
						variant === 'messages' && 'flex items-center gap-2 pt-8 pb-4 first:pt-0',
						variant !== 'messages' && 'border-0 px-2 py-2 first:pt-0'
					)}
				>
					<div class="flex-shrink-0 rounded-sm">
						{#if variant === 'messages'}
							{#if mcpPromptSet.mcp.icon}
								<img src={mcpPromptSet.mcp.icon} alt={mcpPromptSet.mcp.name} class="size-4" />
							{:else}
								<MessageSquarePlus class="text-on-surface1 size-4" />
							{/if}
						{/if}
					</div>
					{mcpPromptSet.mcp.name}
				</div>

				<div
					class="dark:border-surface3 flex flex-col border-0 bg-gray-50 p-2 shadow-inner dark:bg-gray-950"
				>
					{#each mcpPromptSet.prompts as prompt (prompt.name)}
						<button
							class={twMerge(
								'menu-button flex h-full w-full items-center gap-2 border-0 text-left',
								indexMatchedPrompt?.prompt.name === prompt.name &&
									indexMatchedPrompt?.mcp.id === mcpPromptSet.mcp.id &&
									!isHovering &&
									'bg-surface2 dark:bg-surface3 hover:bg-surface2 dark:hover:bg-surface3'
							)}
							onclick={() => handleClick(prompt, mcpPromptSet.mcp)}
						>
							<div class="flex-shrink-0 rounded-sm">
								{#if mcpPromptSet.mcp.icon}
									<img src={mcpPromptSet.mcp.icon} alt={mcpPromptSet.mcp.name} class="size-6" />
								{:else}
									<MessageSquarePlus class="text-on-surface1 size-5" />
								{/if}
							</div>
							<div class="flex flex-col">
								<p class="text-xs font-light">
									{prompt.name}
									{#if variant === 'popover' && prompt.arguments}
										{#each prompt.arguments as argument (argument.name)}
											<span class="text-on-surface1 text-xs">
												[{argument.name}]
											</span>
										{/each}
									{/if}
								</p>
								<p class="text-on-surface1 text-xs font-light">
									{prompt.description}
								</p>
							</div>
						</button>
					{/each}
				</div>
			{/if}
		{/each}
	{/if}
{/snippet}

<div
	bind:this={popover}
	class="dropdown-menu default-scrollbar-thin max-h-[300px] overflow-y-auto py-2"
	onmouseenter={() => (isHovering = true)}
	onmouseleave={() => (isHovering = false)}
	role="listbox"
	tabindex={0}
	popover
	id="mcp-prompts-popover"
	style="--anchor-v: top; --anchor-h: span-left; position-anchor: --input-anchor; width: anchor-size(width);"
>
	{@render content()}
</div>

<ResponsiveDialog bind:this={argsDialog} title="Prompt Arguments" class="p-4 md:w-md">
	{#if selectedPrompt?.prompt.arguments}
		{#each selectedPrompt.prompt.arguments as argument (argument.name)}
			<div class="my-4 flex flex-col gap-1">
				<label for={argument.name} class="text-md font-semibold">{argument.name}</label>
				<input
					id={argument.name}
					name={argument.name}
					class="text-input-filled w-full"
					type="text"
					placeholder={argument.description}
					onchange={(e) => {
						params[argument.name] = (e.target as HTMLInputElement).value;
					}}
				/>
			</div>
		{/each}
	{/if}
	<div class="flex grow"></div>
	<div class="flex justify-end">
		<button
			class="button-primary"
			onclick={() => {
				if (selectedPrompt) {
					onSelect?.(selectedPrompt.prompt, selectedPrompt.mcp, params);
				}
				selectedPrompt = undefined;
				params = {};
				argsDialog?.close();
			}}
		>
			Submit
		</button>
	</div>
</ResponsiveDialog>
