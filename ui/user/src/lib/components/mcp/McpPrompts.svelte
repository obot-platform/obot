<script lang="ts">
	import { ChatService, type MCPServerPrompt, type Project, type ProjectMCP } from '$lib/services';
	import { getProjectMCPs } from '$lib/context/projectMcps.svelte';
	import Menu from '$lib/components/navbar/Menu.svelte';
	import { ChevronRight, LoaderCircle, Plus, X } from 'lucide-svelte';
	import { responsive } from '$lib/stores';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { twMerge } from 'tailwind-merge';
	import { onMount } from 'svelte';
	import { clickOutside } from '$lib/actions/clickoutside';
	interface Props {
		project: Project;
		variant: 'button' | 'popover' | 'messages';
		filterText?: string;
		onSelect?: (prompt: MCPServerPrompt, mcp: ProjectMCP, params?: Record<string, string>) => void;
		onClickOutside?: () => void;
	}

	type PromptSet = {
		mcp: ProjectMCP;
		prompts: MCPServerPrompt[];
	};

	let { project, variant, filterText, onSelect, onClickOutside }: Props = $props();
	let menu = $state<ReturnType<typeof Menu>>();
	let ref = $state<HTMLDivElement>();
	let loading = $state(false);
	let mcpPromptSets = $state<PromptSet[]>([]);

	let params = $state<Record<string, string>>({});
	let selectedPrompt = $state<{ prompt: MCPServerPrompt; mcp: ProjectMCP }>();
	let argsDialog = $state<HTMLDialogElement>();

	const projectMcps = getProjectMCPs();

	$effect(() => {
		if (filterText && filterText.startsWith('/')) {
			ref?.classList.remove('hidden');
			fetchPrompts();
		} else {
			ref?.classList.add('hidden');
		}
	});

	function handleClickOutside() {
		if (ref?.classList.contains('hidden')) return; // already hidden
		ref?.classList.add('hidden');
		onClickOutside?.();
	}

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
		} else {
			ref?.classList.add('hidden');
		}

		if (prompt.arguments) {
			argsDialog?.showModal();
			selectedPrompt = { prompt, mcp };
		} else {
			onSelect?.(prompt, mcp);
		}
	}

	function handleCloseArgsDialog() {
		selectedPrompt = undefined;
		params = {};
		argsDialog?.close();
	}

	onMount(() => {
		if (variant === 'messages') {
			fetchPrompts();
		}
	});
</script>

{#snippet content(filteredByNameDescription?: PromptSet[])}
	{@const setsToUse = filteredByNameDescription ?? mcpPromptSets}
	{#if loading}
		<div class="flex h-full flex-col items-center justify-center">
			<LoaderCircle class="size-4 animate-spin" />
		</div>
	{:else if setsToUse.length === 0 && variant !== 'messages'}
		<div class="flex h-full flex-col items-center justify-center">
			<p class="text-sm text-gray-500">No prompts found</p>
		</div>
	{:else}
		{#each setsToUse as mcpPromptSet (mcpPromptSet.mcp.id)}
			<div
				class={twMerge(
					'w-full text-xs font-semibold',
					variant === 'messages' && 'flex items-center gap-2 pt-8 pb-4 first:pt-0',
					variant !== 'messages' && 'border-0 px-2 py-2 first:pt-0'
				)}
			>
				{#if variant === 'messages'}
					<img src={mcpPromptSet.mcp.icon} alt={mcpPromptSet.mcp.name} class="size-4 rounded-sm" />
				{/if}
				{mcpPromptSet.mcp.name}
			</div>

			{#if variant === 'messages'}
				<div class="flex flex-wrap gap-4 px-5">
					{#each mcpPromptSet.prompts as prompt (prompt.name)}
						<button
							class="border-surface3 hover:bg-surface2 w-fit max-w-full rounded-xl border bg-transparent p-4 text-left text-sm font-light transition-all duration-300 md:max-w-72"
							onclick={() => handleClick(prompt, mcpPromptSet.mcp)}
						>
							<p class="mb-1 flex items-center gap-1.5 text-xs">
								{prompt.name}
							</p>
							<span class="line-clamp-3 text-xs font-light text-gray-400 dark:text-gray-600">
								{prompt.description}
							</span>
						</button>
					{/each}
				</div>
			{:else}
				<div
					class="dark:border-surface3 flex flex-col border-0 bg-gray-50 p-2 shadow-inner dark:bg-gray-950"
				>
					{#each mcpPromptSet.prompts as prompt (prompt.name)}
						<button
							class="menu-button flex h-full w-full items-center gap-2 border-0 text-left"
							onclick={() => handleClick(prompt, mcpPromptSet.mcp)}
						>
							<img
								src={mcpPromptSet.mcp.icon}
								alt={mcpPromptSet.mcp.name}
								class="size-6 rounded-sm"
							/>
							<div class="flex flex-col">
								<p class="text-xs font-light">
									{prompt.name}
									{#if variant === 'popover' && prompt.arguments}
										{#each prompt.arguments as argument}
											<span class="text-xs text-gray-500">
												[{argument.name}]
											</span>
										{/each}
									{/if}
								</p>
								<p class="text-xs font-light text-gray-400 dark:text-gray-600">
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

{#if variant === 'button'}
	<div use:tooltip={'Add Prompt'}>
		<Menu
			bind:this={menu}
			title=""
			classes={{
				button: 'button-icon-primary',
				dialog: responsive.isMobile
					? 'rounded-none max-h-[calc(100vh-64px)] left-0 bottom-0 w-full py-2 px-0'
					: 'py-2 px-0'
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
		class="default-dialog absolute top-0 left-0 w-full -translate-y-full py-2"
		use:clickOutside={handleClickOutside}
	>
		{@render content(filteredByNameDescription)}
	</div>
{:else if variant === 'messages'}
	<div>
		{@render content()}
	</div>
{/if}

<dialog
	bind:this={argsDialog}
	class={twMerge('p-4 md:w-md', responsive.isMobile && 'mobile-screen-dialog')}
	use:clickOutside={handleCloseArgsDialog}
>
	<h3 class="default-dialog-title" class:default-dialog-mobile-title={responsive.isMobile}>
		Prompt Arguments
		<button
			class:mobile-header-button={responsive.isMobile}
			onclick={handleCloseArgsDialog}
			class="icon-button"
		>
			{#if responsive.isMobile}
				<ChevronRight class="size-6" />
			{:else}
				<X class="size-5" />
			{/if}
		</button>
	</h3>
	{#if selectedPrompt?.prompt.arguments}
		{#each selectedPrompt.prompt.arguments as argument}
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
</dialog>
