<script lang="ts">
	import { getToolBundleMap } from '$lib/context/toolReferences.svelte';
	import type { AssistantTool, ToolReference } from '$lib/services/chat/types';
	import CollapsePane from './CollapsePane.svelte';
	import { responsive } from '$lib/stores';
	import {
		ChevronRight,
		ChevronsLeft,
		ChevronsRight,
		Minus,
		SquareMinus,
		Wrench,
		X
	} from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		onSelectTools: (tools: AssistantTool[]) => void;
		onSubmit?: () => void;
		tools: AssistantTool[];
		maxTools: number;
		title?: string;
	}

	type ToolCatalog = {
		tool: ToolReference;
		bundleTools: ToolReference[] | undefined;
		total?: number;
	}[];

	let { onSelectTools, onSubmit, tools, maxTools, title = 'Your Tools' }: Props = $props();

	let input = $state<HTMLInputElement>();
	let searchPopover = $state<HTMLDialogElement>();
	let search = $state('');
	let searchContainer = $state<HTMLDivElement>();
	let direction = $state<'left' | 'right' | null>(null);
	let showAvailableTools = $state(true);
	let toolSelection = $state<Record<string, AssistantTool>>({});
	let maxExceeded = $state(false);

	function getSelectionMap() {
		return tools
			.filter((t) => !t.builtin)
			.reduce<Record<string, AssistantTool>>((acc, tool) => {
				acc[tool.id] = { ...tool };
				return acc;
			}, {});
	}
	$effect(() => {
		toolSelection = getSelectionMap();
	});

	$effect(() => {
		if (responsive.isMobile) {
			showAvailableTools = false;
		} else {
			showAvailableTools = true;
		}
	});

	function handleSearchClickOutside(event: MouseEvent) {
		if (responsive.isMobile) return;
		if (searchContainer && !searchContainer.contains(event.target as Node) && searchPopover?.open) {
			searchPopover.close();
		}
	}

	function handleSubmit() {
		onSelectTools(Object.values(toolSelection));
		onSubmit?.();
	}

	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape') {
			event.preventDefault(); // Prevent default ESC behavior
			handleSubmit();
		}
	}

	onMount(() => {
		document.addEventListener('click', handleSearchClickOutside);
		document.addEventListener('keydown', handleKeydown);
		return () => {
			document.removeEventListener('click', handleSearchClickOutside);
			document.removeEventListener('keydown', handleKeydown);
		};
	});

	const bundles: ToolCatalog = $derived.by(() => {
		if (toolSelection) {
			return Array.from(getToolBundleMap().values()).reduce<ToolCatalog>(
				(acc, { tool, bundleTools }) => {
					if (!toolSelection[tool.id]) return acc;
					acc.push({
						tool,
						bundleTools: bundleTools?.filter((subtool) => toolSelection[subtool.id])
					});
					return acc;
				},
				[]
			);
		}
		return [];
	});

	function getSearchResults() {
		if (!search) return [];

		return bundles.reduce<ToolCatalog>((acc, { tool, bundleTools }) => {
			if (!tool) return acc;

			const subToolMatches =
				bundleTools?.filter((subtool) =>
					[subtool.name, subtool.id, subtool.description].some((s) =>
						s?.toLowerCase().includes(search.toLowerCase())
					)
				) ?? [];

			if (subToolMatches.length > 0) {
				acc.push({ tool, bundleTools: subToolMatches });
				return acc;
			}

			if (
				[tool.name, tool.id, tool.description].some((s) =>
					s?.toLowerCase().includes(search.toLowerCase())
				)
			) {
				acc.push({ tool, bundleTools: undefined });
			}

			return acc;
		}, []);
	}

	function getEnabledTools() {
		return bundles.reduce<ToolCatalog>((acc, { tool, bundleTools }) => {
			if (!tool) return acc;
			if (bundleTools) {
				const bundleEnabled = toolSelection[tool.id]?.enabled;
				const total = bundleTools.length;
				const enabledSubtools = bundleTools.filter((t) => toolSelection[t.id].enabled);
				if (!bundleEnabled && !enabledSubtools.length) return acc;

				acc.push({
					tool,
					bundleTools: bundleEnabled ? bundleTools : enabledSubtools,
					total
				});
			} else if (toolSelection[tool.id].enabled) {
				acc.push({ tool, bundleTools: undefined });
			}

			return acc;
		}, []);
	}

	function getDisabledTools() {
		return bundles.reduce<ToolCatalog>((acc, { tool, bundleTools }) => {
			if (!tool) return acc;
			if (bundleTools) {
				const bundleEnabled = toolSelection[tool.id].enabled;
				if (bundleEnabled) return acc;

				const total = bundleTools.length;
				const disabledSubtools = bundleTools.filter((t) => !toolSelection[t.id].enabled);
				acc.push({ tool, bundleTools: disabledSubtools, total });
			} else if (!toolSelection[tool.id].enabled) {
				acc.push({ tool, bundleTools: undefined });
			}

			return acc;
		}, []);
	}

	function checkMaxExceeded() {
		maxExceeded = Object.values(toolSelection).filter((t) => t.enabled).length > maxTools;
	}

	function toggleBundle(toolBundleId: string, val: boolean, bundleTools: ToolReference[]) {
		for (const subtool of bundleTools) {
			toolSelection[subtool.id].enabled = false;
		}

		toolSelection[toolBundleId].enabled = val;
		checkMaxExceeded();
	}

	function toggleTool(toolId: string, val: boolean, parent?: ToolCatalog[0]) {
		toolSelection[toolId].enabled = val;

		const parentBundleId = parent?.tool.id;
		if (parentBundleId && toolSelection[parentBundleId]?.enabled && !val) {
			// If parent bundle is enabled and we're turning off a subtool,
			// parent bundle is no longer enabled, but subtools other than the one
			// being turned off are enabled

			toolSelection[parentBundleId].enabled = false;
			for (const subtool of parent?.bundleTools ?? []) {
				if (subtool.id !== toolId) {
					toolSelection[subtool.id].enabled = true;
				}
			}
			return;
		}

		if (parentBundleId && val && (parent?.bundleTools ?? []).length === 1) {
			// If this is the last item in the bundle that is being enabled,
			// enable the parent bundle
			toolSelection[parentBundleId].enabled = true;
			const bundleTools = bundles.find((b) => b.tool.id === parentBundleId)?.bundleTools;
			if (bundleTools) {
				for (const subtool of bundleTools) {
					toolSelection[subtool.id].enabled = false;
				}
			}
		}

		checkMaxExceeded();
	}
</script>

<div class="flex h-full w-full flex-col overflow-hidden md:h-[75vh]">
	<h4
		class="border-surface3 relative mx-4 flex items-center justify-center border-b py-4 text-lg font-semibold md:justify-start"
	>
		{title}
		<button class="icon-button absolute top-2 right-0" onclick={() => handleSubmit()}>
			{#if responsive.isMobile}
				<ChevronRight class="size-6" />
			{:else}
				<X class="size-6" />
			{/if}
		</button>
	</h4>
	<div class="flex w-full items-center justify-between">
		<div class="flex grow rounded-t-lg p-4 md:relative" bind:this={searchContainer}>
			{#if responsive.isMobile}
				<button class="mock-input-btn" onclick={() => searchPopover?.show()}>
					Search tools...
				</button>
			{:else}
				{@render searchInput()}
			{/if}
			<dialog
				bind:this={searchPopover}
				class="default-scrollbar-thin absolute top-0 left-0 z-10 h-full w-full rounded-sm md:top-11 md:h-[50vh] md:w-[calc(100%-1rem)] md:overflow-y-auto"
				class:hidden={!responsive.isMobile && !search}
			>
				<div class="flex h-full flex-col">
					{#if responsive.isMobile}
						<div class="flex w-full justify-between gap-2 p-4">
							<div class="flex grow">
								{@render searchInput()}
							</div>
							<div class="flex flex-shrink-0">
								<button class="icon-button" onclick={() => searchPopover?.close()}>
									<ChevronRight class="size-6" />
								</button>
							</div>
						</div>
					{/if}
					<div class="default-scrollbar-thin flex min-h-0 grow flex-col overflow-y-auto">
						{#each getSearchResults() as result}
							{@render searchResult(result)}
						{/each}
						{#if getSearchResults().length === 0 && search}
							<p class="px-4 py-2 text-sm text-gray-500">No results found</p>
						{/if}
					</div>
				</div>
			</dialog>
		</div>
	</div>
	<div class="flex min-h-0 w-full grow items-stretch px-4">
		<!-- Selected Tools Column -->
		{#if !responsive.isMobile || (responsive.isMobile && !showAvailableTools)}
			<div
				class="border-surface2 dark:border-surface1 flex flex-1 flex-col rounded-sm border-2"
				transition:fly={showAvailableTools
					? { x: 250, duration: 300, delay: 0 }
					: { x: 250, duration: 300, delay: 300 }}
			>
				<h4 class="bg-surface1 flex px-4 py-2 text-base font-semibold">Selected Tools</h4>
				<div class="default-scrollbar-thin h-inherit flex min-h-0 flex-1 flex-col overflow-y-auto">
					{#each getEnabledTools() as enabledCatalogItem (enabledCatalogItem.tool.id)}
						<div transition:fly={{ x: 250, duration: 300 }}>
							{@render catalogItem(enabledCatalogItem, true)}
						</div>
					{/each}
				</div>
			</div>
		{/if}

		<!-- Directional Bar -->
		{#if responsive.isMobile && !showAvailableTools}
			<button
				transition:fly={showAvailableTools
					? { x: 250, duration: 300, delay: 0 }
					: { x: 250, duration: 300, delay: 300 }}
				onclick={() => (showAvailableTools = !showAvailableTools)}
				class="bg-surface1 h-inherit dark:border-surface2 flex min-h-0 w-8 flex-col items-center justify-center gap-2 border-l border-white px-2"
			>
				<ChevronsRight class="size-6 text-black dark:text-white" />
			</button>
		{:else if !responsive.isMobile}
			<div
				class="h-inherit bg-surface1 dark:border-surface2 mx-2 flex min-h-0 w-8 flex-col items-center justify-center gap-2 rounded-sm border-x border-white px-2"
			>
				{#if !direction}
					<div class="flex flex-col">
						<ChevronsRight class="size-6 text-gray-500" />
						<ChevronsLeft class="size-6 text-gray-500" />
					</div>
				{:else if direction === 'right'}
					<div>
						<ChevronsRight class="text-blue size-6" />
						<ChevronsLeft class="size-6 text-gray-500" />
					</div>
				{:else if direction === 'left'}
					<div>
						<ChevronsRight class="size-6 text-gray-500" />
						<ChevronsLeft class="text-blue size-6" />
					</div>
				{/if}
			</div>
		{/if}

		<!-- Unselected Tools Column -->
		{#if !responsive.isMobile || (responsive.isMobile && showAvailableTools)}
			<div
				class="border-surface2 dark:border-surface1 flex flex-1 rounded-sm border-2"
				transition:fly={showAvailableTools
					? { x: 250, duration: 300, delay: 300 }
					: { x: 250, duration: 300, delay: 0 }}
			>
				<div class="flex flex-1 flex-col">
					<h4 class="bg-surface1 flex px-4 py-2 text-base font-semibold">Available Tools</h4>
					<div
						class="default-scrollbar-thin h-inherit flex min-h-0 flex-1 flex-col overflow-y-auto"
					>
						{#each getDisabledTools() as disabledCatalogItem (disabledCatalogItem.tool.id)}
							<div transition:fly={{ x: -250, duration: 300 }}>
								{@render catalogItem(disabledCatalogItem, false)}
							</div>
						{/each}
					</div>
				</div>
			</div>
			{#if responsive.isMobile}
				<button
					transition:fly={showAvailableTools
						? { x: 250, duration: 300, delay: 300 }
						: { x: 250, duration: 300, delay: 0 }}
					onclick={() => (showAvailableTools = !showAvailableTools)}
					class="bg-surface1 text:border-black h-inherit dark:border-surface2 flex min-h-0 w-8 flex-col items-center justify-center gap-2 border-l border-white px-2"
				>
					<ChevronsLeft class="size-6 text-black dark:text-white" />
				</button>
			{/if}
		{/if}
	</div>

	<div class="flex flex-col items-center gap-2 p-2 md:flex-row">
		{#if maxExceeded}
			<p class="text-left text-sm text-red-500">
				Maximum number of tools exceeded for this Assistant. (Max: {maxTools})
			</p>
		{/if}
	</div>
</div>

{#snippet toolInfo(tool: ToolReference, headerLabel?: string)}
	{#if tool.metadata?.icon}
		<img
			class="size-8 flex-shrink-0 rounded-md bg-white p-1 dark:bg-gray-600"
			src={tool.metadata?.icon}
			alt="message icon"
		/>
	{:else}
		<Wrench class="size-8 flex-shrink-0 rounded-md bg-gray-100 p-1 text-black" />
	{/if}
	<span class="flex grow flex-col px-2 text-left">
		<span>
			{tool.name}
			{#if headerLabel}
				<span class="text-xs text-gray-500">{headerLabel}</span>
			{/if}
		</span>
		<span class="text-gray text-xs font-normal dark:text-gray-300">
			{tool.description}
		</span>
	</span>
{/snippet}

{#snippet catalogItem(item: ToolCatalog[0], toggleValue: boolean)}
	{@const { tool, bundleTools, total: subtoolsTotal } = item}
	<CollapsePane
		showDropdown={bundleTools && bundleTools.length > 0}
		classes={{
			header: 'py-0 pl-0 pr-3 hover:bg-surface2 dark:hover:bg-surface3',
			content: 'border-none p-0 bg-surface2 shadow-none'
		}}
	>
		{#if bundleTools && bundleTools.length > 0}
			{#each bundleTools as subTool (subTool.id)}
				{@render subToolItem(subTool, item)}
			{/each}
		{/if}

		{#snippet header()}
			{@const isEnabled = toolSelection[tool.id]?.enabled}
			{@const total = subtoolsTotal ?? 0}
			{@const subToolsSelectedCount = isEnabled ? total : (bundleTools?.length ?? 0)}

			<button
				onclick={(e) => {
					e.stopPropagation();

					if (bundleTools) {
						toggleBundle(tool.id, !toggleValue, bundleTools);
					} else {
						toggleTool(tool.id, true);
					}
				}}
				onmouseenter={() => (direction = toggleValue ? 'right' : 'left')}
				onmouseleave={() => (direction = null)}
				class="group flex grow items-center justify-between gap-2 rounded-lg p-2 px-4"
			>
				{@render toolInfo(
					tool,
					subToolsSelectedCount !== total ? `${subToolsSelectedCount}/${total}` : undefined
				)}
				{@render chevronAction(toggleValue, 'translate-x-6')}
			</button>
		{/snippet}
	</CollapsePane>
{/snippet}

{#snippet chevronAction(isEnabled?: boolean, containerClass?: string)}
	<span
		class={twMerge(
			'flex items-center justify-center opacity-0 transition-opacity duration-200 group-hover:opacity-100',
			containerClass
		)}
	>
		{#if isEnabled}
			<ChevronsRight class="text-blue animate-bounce-x size-10" />
		{:else}
			<ChevronsLeft class="text-blue animate-bounce-x size-10" />
		{/if}
	</span>
{/snippet}

{#snippet subToolItem(toolReference: ToolReference, parent: ToolCatalog[0])}
	{@const isEnabled =
		(parent.tool.id && toolSelection[parent.tool.id]?.enabled) ||
		toolSelection[toolReference.id]?.enabled}
	<button
		transition:fly={isEnabled ? { x: 250, duration: 300 } : { x: -250, duration: 300 }}
		onclick={() => {
			toggleTool(toolReference.id, !isEnabled, parent);
		}}
		class="dark:bg-surface2 group hover:bg-surface2 dark:hover:bg-surface3 flex grow items-center gap-2 bg-white p-2 px-4 transition-opacity duration-200"
		onmouseenter={() => (direction = isEnabled ? 'right' : 'left')}
		onmouseleave={() => (direction = null)}
	>
		{@render toolInfo(toolReference)}
		{@render chevronAction(isEnabled)}
	</button>
{/snippet}

{#snippet searchInput()}
	<input
		class="bg-surface1 w-full rounded-lg p-2"
		type="text"
		placeholder="Search tools..."
		bind:this={input}
		bind:value={search}
		onmousedown={() => {
			if (!responsive.isMobile) {
				searchPopover?.show();
			}
		}}
	/>
{/snippet}

{#snippet searchResult({ tool, bundleTools, total }: ToolCatalog[0])}
	{@const val = toolSelection[tool.id]?.enabled}
	<button
		class="hover:bg-surface2 dark:hover:bg-surface3 flex w-full px-4 py-2"
		onclick={(e) => {
			e.stopPropagation();
			if (bundleTools && bundleTools.length > 0) {
				toggleBundle(tool.id, !val, bundleTools);
			} else {
				toggleTool(tool.id, !val, { tool, bundleTools, total });
			}
		}}
	>
		{@render toolInfo(tool)}
		{#if val}
			<div class="mr-4 flex items-center">
				<Minus class="size-6" />
			</div>
		{/if}
	</button>
	{#if bundleTools}
		{#each bundleTools as subTool (subTool.id)}
			{@const subToolVal = toolSelection[subTool.id]?.enabled}
			<button
				class="hover:bg-surface2 dark:hover:bg-surface3 flex w-full px-4 py-2"
				onclick={(e) => {
					e.stopPropagation();
					toggleTool(subTool.id, val ? false : !subToolVal, { tool, bundleTools });
				}}
			>
				{@render toolInfo(subTool)}
				{#if val}
					<div class="mr-4 flex items-center">
						<SquareMinus class="size-5" />
					</div>
				{:else if subToolVal}
					<div class="mr-4 flex items-center">
						<Minus class="size-6" />
					</div>
				{/if}
			</button>
		{/each}
	{/if}
{/snippet}

<style lang="postcss">
	@keyframes bounce-x {
		0%,
		100% {
			transform: translateX(-25%);
		}
		50% {
			transform: translateX(0);
		}
	}

	:global(.animate-bounce-x) {
		animation: bounce-x 1s infinite ease-in-out;
	}
</style>
