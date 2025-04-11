<script lang="ts">
	import { getToolBundleMap } from '$lib/context/toolReferences.svelte';
	import type { AssistantTool, ToolReference } from '$lib/services/chat/types';
	import CollapsePane from './CollapsePane.svelte';
	import { responsive } from '$lib/stores';
	import { ChevronRight, ChevronsLeft, ChevronsRight, Search, Wrench, X } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';
	import SearchInput from '../Search.svelte';
	import { IGNORED_BUILTIN_TOOLS } from '$lib/constants';

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

	let { onSelectTools, onSubmit, tools, maxTools, title = 'Tool Catalog' }: Props = $props();

	let searchPopover = $state<HTMLDialogElement>();
	let search = $state('');
	let searchContainer = $state<HTMLDivElement>();
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

	const builtInTools = $derived.by(() => {
		const builtInToolMap = new Map<string, AssistantTool>(
			tools.filter((t) => t.builtin && !IGNORED_BUILTIN_TOOLS.has(t.id)).map((t) => [t.id, t])
		);
		return Array.from(getToolBundleMap().values()).reduce<ToolCatalog>(
			(acc, { tool, bundleTools }) => {
				if (builtInToolMap.has(tool.id)) {
					acc.push({ tool, bundleTools, total: bundleTools?.length });
					return acc;
				}

				const builtInSubtools =
					bundleTools?.filter((subtool) => builtInToolMap.has(subtool.id)) ?? [];
				if (builtInSubtools.length > 0) {
					acc.push({
						tool,
						bundleTools: builtInSubtools,
						total: builtInSubtools.length
					});
				}
				return acc;
			},
			[]
		);
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
		class="border-surface3 relative mx-4 flex items-center justify-center border-b py-4 text-lg font-semibold md:mb-4 md:justify-start"
	>
		{#if responsive.isMobile}
			<button class="icon-button absolute top-2 left-0" onclick={() => searchPopover?.show()}>
				<Search class="size-6" />
			</button>
		{/if}
		{title}
		<button class="icon-button absolute top-2 right-0" onclick={() => handleSubmit()}>
			{#if responsive.isMobile}
				<ChevronRight class="size-6" />
			{:else}
				<X class="size-6" />
			{/if}
		</button>
	</h4>
	<div class="flex min-h-0 w-full grow items-stretch md:gap-2 md:px-4">
		<!-- Selected Tools Column -->
		{#if !responsive.isMobile || (responsive.isMobile && !showAvailableTools)}
			{@const enabledTools = getEnabledTools()}
			<div
				class="border-surface2 dark:border-surface3 flex flex-1 flex-col md:rounded-md md:border-2"
				transition:fly={showAvailableTools
					? { x: 250, duration: 300, delay: 0 }
					: { x: 250, duration: 300, delay: 300 }}
			>
				<h4 class="flex px-4 py-2 text-base font-semibold">Selected Tools</h4>
				<div
					class="default-scrollbar-thin h-inherit flex min-h-0 flex-1 grow flex-col overflow-x-hidden overflow-y-auto"
				>
					{#each enabledTools as enabledCatalogItem (enabledCatalogItem.tool.id)}
						<div transition:fly={{ x: 250, duration: 300 }}>
							{@render catalogItem(enabledCatalogItem, true)}
						</div>
					{/each}
					{#if enabledTools.length === 0}
						<p class="p-4 text-sm text-gray-500">No tools selected.</p>
					{/if}
				</div>
				{@render readOnlyTools()}
			</div>
		{/if}

		<!-- Mobile Directional Bar -->
		{#if responsive.isMobile && !showAvailableTools}
			<button
				transition:fly={showAvailableTools
					? { x: 250, duration: 300, delay: 0 }
					: { x: 250, duration: 300, delay: 300 }}
				onclick={() => (showAvailableTools = !showAvailableTools)}
				class="h-inherit border-surface1 dark:border-surface3 flex min-h-0 w-8 flex-col items-center justify-center gap-2 border-l bg-transparent px-2"
			>
				<ChevronsRight class="size-6 text-black dark:text-white" />
			</button>
		{/if}

		<!-- Unselected Tools Column -->
		{#if !responsive.isMobile || (responsive.isMobile && showAvailableTools)}
			{@const disabledTools = getDisabledTools()}
			<div
				class="border-surface2 dark:border-surface3 flex flex-1 md:rounded-md md:border-2"
				transition:fly={showAvailableTools
					? { x: 250, duration: 300, delay: 300 }
					: { x: 250, duration: 300, delay: 0 }}
			>
				<div class="flex flex-1 flex-col">
					<h4 class="flex px-4 py-2 text-base font-semibold">Available Tools</h4>
					<div
						class="default-scrollbar-thin h-inherit flex min-h-0 flex-1 flex-col overflow-y-auto"
					>
						{#each disabledTools as disabledCatalogItem (disabledCatalogItem.tool.id)}
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
					class="text:border-black h-inherit dark:border-surface3 border-surface1 flex min-h-0 w-8 flex-col items-center justify-center gap-2 border-l px-2"
				>
					<ChevronsLeft class="size-6 text-black dark:text-white" />
				</button>
			{/if}
		{/if}
	</div>

	<div class="flex w-full items-center justify-between">
		<div class="flex grow md:relative" bind:this={searchContainer}>
			{#if !responsive.isMobile}
				<div class="w-full p-4">
					{@render searchInput()}
				</div>
			{/if}

			{@render searchDialog()}
		</div>
	</div>

	<div class="flex flex-col items-center gap-2 md:flex-row">
		{#if maxExceeded}
			<p class="p-2 text-left text-sm text-red-500">
				Maximum number of tools exceeded for this Assistant. (Max: {maxTools})
			</p>
		{/if}
	</div>
</div>

{#snippet readOnlyTools()}
	{#if builtInTools.length > 0}
		<CollapsePane
			classes={{
				content:
					'default-scrollbar-thin flex min-h-0 flex-col overflow-y-auto p-0 pr-2 bg-surface1',
				header: 'border-t-2 border-surface1 dark:border-surface3 px-5',
				root: 'min-h-0'
			}}
		>
			{#snippet header()}
				<span class="grow text-left text-sm font-medium">Built-in Tools</span>
			{/snippet}
			{#each builtInTools as builtInTool (builtInTool.tool.id)}
				{@render catalogItem(builtInTool, true, true)}
			{/each}
		</CollapsePane>
	{/if}
{/snippet}

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

{#snippet catalogItem(item: ToolCatalog[0], toggleValue: boolean, readOnly?: boolean)}
	{@const { tool, bundleTools, total: subtoolsTotal } = item}
	<CollapsePane
		showDropdown={bundleTools && bundleTools.length > 0}
		classes={{
			header: twMerge(
				'group py-0 pl-0 pr-3',
				!readOnly && 'hover:bg-surface2 dark:hover:bg-surface3'
			),
			content: 'border-none p-0 bg-transparent shadow-none'
		}}
	>
		{#if bundleTools && bundleTools.length > 0}
			{#each bundleTools as subTool (subTool.id)}
				{@render subToolItem(subTool, item, toggleValue, readOnly)}
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
				class="flex grow items-center justify-between gap-2 rounded-lg p-2 px-4 disabled:cursor-not-allowed"
				disabled={readOnly}
			>
				{#if !toggleValue}
					<div
						class="w-0 -translate-x-2 opacity-0 transition-all duration-200 group-hover:w-8 group-hover:opacity-100"
					>
						{@render chevronAction(toggleValue)}
					</div>
				{/if}
				<div class="flex grow items-center" class:-translate-x-3={!toggleValue}>
					{@render toolInfo(
						tool,
						subToolsSelectedCount !== total ? `${subToolsSelectedCount}/${total}` : undefined
					)}
				</div>
			</button>
		{/snippet}

		{#snippet endContent()}
			{#if toggleValue && !readOnly}
				<div
					class="w-0 opacity-0 transition-all duration-200 group-hover:w-8 group-hover:opacity-100"
				>
					{@render chevronAction(toggleValue, 'translate-x-2')}
				</div>
			{/if}
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
			<ChevronsRight class="text-blue/65 animate-bounce-x size-6" />
		{:else}
			<ChevronsLeft class="text-blue/65 animate-bounce-x size-6" />
		{/if}
	</span>
{/snippet}

{#snippet subToolItem(
	toolReference: ToolReference,
	parent: ToolCatalog[0],
	toggleValue: boolean,
	readOnly?: boolean
)}
	{@const isEnabled =
		(parent.tool.id && toolSelection[parent.tool.id]?.enabled) ||
		toolSelection[toolReference.id]?.enabled}
	<button
		transition:fly={toggleValue ? { x: 250, duration: 300 } : { x: -250, duration: 300 }}
		onclick={() => {
			toggleTool(toolReference.id, !isEnabled, parent);
		}}
		class={twMerge(
			'group flex grow items-center gap-2 p-2 px-4 transition-opacity duration-200',
			readOnly && 'cursor-not-allowed',
			!readOnly && 'dark:bg-surface2 hover:bg-surface2 dark:hover:bg-surface3 bg-white'
		)}
	>
		{#if !isEnabled && !readOnly}
			<div
				class="-mr-1 -ml-2 w-0 opacity-0 transition-all duration-200 group-hover:w-8 group-hover:opacity-100"
			>
				{@render chevronAction(isEnabled)}
			</div>
		{/if}
		{@render toolInfo(toolReference)}
		{#if isEnabled && !readOnly}
			{@render chevronAction(isEnabled, 'translate-x-3')}
		{/if}
	</button>
{/snippet}

{#snippet searchDialog()}
	<dialog
		bind:this={searchPopover}
		class="default-scrollbar-thin absolute bottom-0 left-0 z-10 h-full w-full rounded-sm md:bottom-16 md:h-fit md:max-h-[50vh] md:w-[calc(100%-1rem)] md:overflow-y-auto"
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
					<p class="px-4 py-2 text-sm font-light text-gray-500">No results found.</p>
				{/if}
			</div>
		</div>
	</dialog>
{/snippet}

{#snippet searchInput()}
	<SearchInput
		onChange={(val) => {
			search = val;
		}}
		onMouseDown={() => {
			if (!responsive.isMobile) {
				searchPopover?.show();
			}
		}}
		placeholder="Search tools..."
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

			searchPopover?.close();
		}}
	>
		{@render toolInfo(tool)}
		{#if val}
			<div class="mr-4 flex items-center">
				<div class="remove-pill">Remove</div>
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
					searchPopover?.close();
				}}
			>
				{@render toolInfo(subTool)}
				{#if val || subToolVal}
					<div class="mr-4 flex items-center">
						<div class="remove-pill">Remove</div>
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
			transform: translateX(-20%);
		}
		50% {
			transform: translateX(0);
		}
	}

	:global(.animate-bounce-x) {
		animation: bounce-x 1s infinite ease-in-out;
	}

	.remove-pill {
		font-size: var(--text-xs);
		padding: 0.25rem 0.75rem;
		border-radius: var(--radius-2xl);
		background-color: var(--surface3);
		color: var(--text-primary);
		font-weight: 200;
	}
</style>
