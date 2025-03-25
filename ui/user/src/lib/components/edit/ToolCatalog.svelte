<script lang="ts">
	import { popover } from '$lib/actions';
	import { getToolBundleMap } from '$lib/context/toolReferences.svelte';
	import type { AssistantTool, ToolReference } from '$lib/services/chat/types';
	import { twMerge } from 'tailwind-merge';
	import CollapsePane from './CollapsePane.svelte';
	import { responsive, tools } from '$lib/stores';
	import { ChevronRight, X } from 'lucide-svelte';

	interface Props {
		onSelectTools: (tools: AssistantTool[], oauthApps: Set<string>) => void;
		oauthApps: Set<string>;
		onSubmit?: () => void;
	}

	let { onSelectTools, oauthApps, onSubmit }: Props = $props();

	let input = $state<HTMLInputElement>();
	let search = $state('');
	let dialog = $state<HTMLDialogElement>();

	const bundleMap = getToolBundleMap();

	function getSelectionMap() {
		return tools.current.tools
			.filter((t) => !t.builtin)
			.reduce<Record<string, AssistantTool>>((acc, tool) => {
				acc[tool.id] = {
					...tool,
					supportsOAuthTokenPrompt: tool.supportsOAuthTokenPrompt ?? false,
					oauthApp: tool.oauthApp ?? ''
				};
				return acc;
			}, {});
	}

	let toolSelection = $state<Record<string, AssistantTool>>({});

	$effect(() => {
		toolSelection = getSelectionMap();
	});

	let maxExceeded = $derived(
		Object.values(toolSelection).filter((t) => t.enabled).length > tools.current.maxTools
	);

	let catalog = popover({ fixed: true, slide: responsive.isMobile ? 'up' : undefined });

	let selectedTool = $state<ToolReference | null>(null);

	function handleToolClick(tool: ToolReference) {
		if (tool.metadata?.supportsOAuthTokenPrompt) {
			selectedTool = tool;
			dialog?.showModal();
		}
	}

	function handleAuthSelection(selection: 'oauth' | 'token') {
		if (selection === 'oauth') {
			if (selectedTool?.metadata?.oauth) {
				oauthApps.add(selectedTool.metadata.oauth);
			}
		} else {
			oauthApps.delete(selectedTool?.metadata?.oauth ?? '');
		}
		dialog?.close();
	}

	export function open() {
		catalog.toggle(true);
	}

	function setToolEnabled(toolId: string, val?: boolean) {
		if (toolId in toolSelection) {
			toolSelection[toolId].enabled = val;
		}
	}

	function shouldShowTool(tool: AssistantTool) {
		if (!tool || !toolSelection[tool.id]) return false;

		if (!search) return true;

		return [tool.name, tool.id, tool.description].some((s) =>
			s?.toLowerCase().includes(search.toLowerCase())
		);
	}

	function handleSubmit() {
		onSelectTools(Object.values(toolSelection), oauthApps);
		onSubmit?.();
		catalog.toggle(false);
	}

	function clearBundle(toolRef: ToolReference) {
		if (!toolRef.bundleToolName) return;

		if (toolRef.bundleToolName in toolSelection) {
			toolSelection[toolRef.bundleToolName].enabled = false;
		}
	}

	function isToolEnabled(toolId: string) {
		if (toolId in toolSelection) {
			return toolSelection[toolId].enabled;
		}

		return false;
	}

	function setSubTools(toolId: string, val?: boolean) {
		const toolItem = bundleMap.get(toolId);

		if (!toolItem) return;

		const subtools = toolItem.bundleTools ?? [];

		for (const subtool of subtools) {
			if (subtool.id in toolSelection) {
				toolSelection[subtool.id].enabled = val;
			}
		}
	}

	function allSubtoolsEnabled(toolId: string) {
		const toolItem = bundleMap.get(toolId);

		if (!toolItem) return false;

		const subtools = toolItem.bundleTools ?? [];
		return subtools.every((t) => isToolEnabled(t.id));
	}

	function handleSetSubtool(toolref: ToolReference, val?: boolean) {
		const { bundleToolName } = toolref;

		if (!bundleToolName || !(toolref.id in toolSelection)) return;

		const tool = toolSelection[toolref.id];

		if (!val && isToolEnabled(bundleToolName)) {
			setToolEnabled(bundleToolName, false);
			setSubTools(bundleToolName, true);
		}

		tool.enabled = val;
	}
</script>

<div class="w-full">
	<h4
		class="border-surface3 relative mx-2 mb-2 flex items-center justify-center border-b py-4 text-lg font-semibold md:justify-start"
	>
		Modify Tools
		<button class="icon-button absolute top-1 right-0" onclick={() => onSubmit?.()}>
			{#if responsive.isMobile}
				<ChevronRight class="size-6" />
			{:else}
				<X class="size-6" />
			{/if}
		</button>
	</h4>
	<div class="flex w-full items-center justify-between">
		<div class="flex grow rounded-t-lg p-2">
			<input
				class="bg-surface1 w-full rounded-lg p-2"
				type="text"
				placeholder="Search tools"
				bind:this={input}
				bind:value={search}
			/>
		</div>
	</div>

	<dialog
		bind:this={dialog}
		class="top-1/3 left-1/3 min-h-[200px] w-1/4 -translate-x-1/2 -translate-y-1/2 overflow-visible p-5"
	>
		<div class="flex h-full flex-col">
			<button class="absolute top-0 right-0 p-3" onclick={() => dialog?.close()}>
				<X class="icon-default" />
			</button>
			<h1 class="mb-4 text-xl font-semibold">Authentication Method</h1>
			<p class="mb-4 text-sm text-gray-500">
				This tool has personal access token (PAT) and OAuth support. Select the authentication
				method you would like to use for this tool.
			</p>
			<div class="flex flex-col gap-2">
				<button class="button" onclick={() => handleAuthSelection('oauth')}> OAuth </button>
				<button class="button" onclick={() => handleAuthSelection('token')}>
					Personal Access Token (PAT)
				</button>
			</div>
		</div>
	</dialog>

	<div class="default-scrollbar-thin flex max-h-[50vh] grow flex-col">
		{#each Array.from(bundleMap.values()).sort( (a, b) => a.tool.name.localeCompare(b.tool.name) ) as { tool, bundleTools }}
			{@const hasBundle = tool.id in toolSelection}
			{@const visibleBundleTools = bundleTools
				.filter((t) => t.id in toolSelection && shouldShowTool(toolSelection[t.id]))
				.sort((a, b) => a.name.localeCompare(b.name))}
			{@const selectedSubtools = bundleTools.filter(
				(t) => t.id in toolSelection && toolSelection[t.id].enabled
			).length}

			{#if visibleBundleTools.length || shouldShowTool(toolSelection[tool.id])}
				<CollapsePane
					showDropdown={visibleBundleTools.length > 0}
					classes={{ header: 'py-0 pl-0 pr-3', content: 'border-none py-0 px-7' }}
				>
					{#each visibleBundleTools as subTool (subTool.id)}
						{@render toolItem(subTool)}
					{/each}

					{#snippet header()}
						{@const bundleTool = toolSelection[tool.id]}
						{@const allSelected = allSubtoolsEnabled(tool.id)}
						{@const tt = popover({ hover: true, placement: 'left' })}

						<label
							class={twMerge(
								'hover:bg-surface3 flex grow cursor-pointer items-center gap-2 rounded-lg p-2'
							)}
							onclickcapture={(e) => e.stopPropagation()}
							use:tt.ref
						>
							{#if !!bundleTool}
								<input
									type="checkbox"
									onchange={() => {
										setSubTools(tool.id, false);
										if (bundleTool.enabled) {
											handleToolClick(tool);
										} else {
											if (bundleTool.oauthApp) {
												oauthApps.delete(bundleTool.oauthApp);
											}
										}
									}}
									indeterminate={selectedSubtools > 0}
									bind:checked={bundleTool.enabled}
								/>
							{:else}
								<input
									indeterminate={selectedSubtools > 0 && !allSelected}
									checked={allSelected}
									onchange={(e) => {
										setSubTools(tool.id, e.currentTarget.checked);
										if (allSelected) {
											handleToolClick(tool);
										}
									}}
									type="checkbox"
								/>
							{/if}
							<p class="flex items-center gap-2">
								<img
									src={tool.metadata?.icon}
									alt={tool.name}
									class="size-6 rounded-full bg-white p-1"
								/>
								{tool.name}
							</p>

							{#if selectedSubtools > 0}
								<span class="justify-self-end text-xs text-gray-500">
									{selectedSubtools} / {bundleTools.length} Selected
								</span>
							{/if}
						</label>

						<p use:tt.tooltip class="tooltip max-w-64">
							{#if hasBundle}
								{tool.description}
							{:else}
								No bundle tool available for {tool.name}
							{/if}
						</p>
					{/snippet}
				</CollapsePane>
			{/if}
		{/each}
	</div>

	<div class="flex justify-between gap-2 p-2">
		<p class={twMerge('max-w-72 text-left text-sm text-red-500', !maxExceeded && 'invisible')}>
			Maximum number of tools exceeded for this Assistant. (Max: {tools.current.maxTools})
		</p>
		<button onclick={handleSubmit} disabled={maxExceeded} class="button">Apply</button>
	</div>
</div>

{#snippet toolItem(toolReference: ToolReference)}
	{@const tool = toolSelection[toolReference.id]}
	{@const bundleToolSelected =
		!!toolReference.bundleToolName && !!toolSelection[toolReference.bundleToolName]?.enabled}
	{@const { tooltip, ref } = popover({ hover: true, placement: 'left' })}

	<label
		class="hover:bg-surface3 flex cursor-pointer items-center justify-between gap-2 rounded-lg p-2"
		use:ref
	>
		<p class={twMerge('flex items-center gap-2')}>
			<input
				type="checkbox"
				onchange={() => {
					clearBundle(toolReference);
					if (tool.enabled) {
						handleToolClick(toolReference);
					}
				}}
				bind:checked={
					() => (bundleToolSelected ? true : tool.enabled),
					(val) => handleSetSubtool(toolReference, val)
				}
			/>
			<img src={tool.icon} alt={tool.name} class="size-6 rounded-full bg-white p-1" />
			{toolReference.name}
		</p>
	</label>

	<p use:tooltip class="tooltip max-w-64">
		{toolReference.description}
	</p>
{/snippet}
