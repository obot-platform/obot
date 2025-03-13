<script lang="ts">
	import { popover } from '$lib/actions';
	import { getToolBundleMap } from '$lib/context/toolReferences.svelte';
	import type { AssistantTool, ToolReference } from '$lib/services/chat/types';
	import { twMerge } from 'tailwind-merge';
	import CollapsePane from './CollapsePane.svelte';
	import { Plus } from 'lucide-svelte';

	interface Props {
		tools: AssistantTool[];
		onSelectTools: (tools: AssistantTool[]) => void;
		maxTools?: number;
	}

	let { tools, onSelectTools, maxTools = Number.MAX_SAFE_INTEGER }: Props = $props();

	let input = $state<HTMLInputElement>();
	let search = $state('');

	const bundleMap = getToolBundleMap();

	function getSelectionMap() {
		return tools
			.filter((t) => !t.builtin)
			.reduce<Record<string, AssistantTool>>((acc, tool) => {
				acc[tool.id] = { ...tool };
				return acc;
			}, {});
	}

	let toolSelection = $state<Record<string, AssistantTool>>({});

	$effect(() => {
		if (catalog.open) {
			toolSelection = getSelectionMap();
		}
	});

	let maxExceeded = $derived(
		Object.values(toolSelection).filter((t) => t.enabled).length > maxTools
	);

	let catalog = popover({ fixed: true });

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
		onSelectTools(Object.values(toolSelection));
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
			toolSelection[subtool.id].enabled = val;
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

<button class="button flex items-center gap-2" use:catalog.ref onclick={() => catalog.toggle(true)}
	><Plus class="size-4" /> Tools</button
>

<div
	use:catalog.tooltip
	class="default-dialog left-1/2 top-[15%] flex min-h-[500px] w-[500px] -translate-x-1/2 flex-col"
>
	<div class="rounded-t-lg p-2">
		<input
			class="w-full rounded-lg bg-surface1 p-2"
			type="text"
			placeholder="Search tools"
			bind:this={input}
			bind:value={search}
		/>
	</div>

	<p class={twMerge('text-center text-sm text-red-500', !maxExceeded && 'invisible')}>
		Maximum number of tools exceeded for this Assistant. (Max: {maxTools})
	</p>

	<div class="default-scrollbar-thin flex max-h-[50vh] grow flex-col p-3">
		{#each bundleMap.values() as { tool, bundleTools }}
			{@const hasBundle = tool.id in toolSelection}
			{@const visibleBundleTools = bundleTools.filter(
				(t) => t.id in toolSelection && shouldShowTool(toolSelection[t.id])
			)}
			{@const selectedSubtools = bundleTools.filter(
				(t) => t.id in toolSelection && toolSelection[t.id].enabled
			).length}

			{#if visibleBundleTools.length || shouldShowTool(toolSelection[tool.id])}
				<CollapsePane
					showDropdown={visibleBundleTools.length > 0}
					classes={{ header: 'py-0', content: 'border-none py-0 px-7' }}
				>
					{#each visibleBundleTools as subTool (subTool.id)}
						{@render toolItem(subTool)}
					{/each}

					{#snippet header()}
						{@const bundleTool = toolSelection[tool.id]}
						{@const allSelected = allSubtoolsEnabled(tool.id)}
						{@const tt = popover({ hover: true, placement: 'left', fixed: false })}

						<label
							class={twMerge(
								'flex grow cursor-pointer items-center gap-2 rounded-lg p-2 hover:bg-surface3'
							)}
							onclickcapture={(e) => e.stopPropagation()}
							use:tt.ref
						>
							{#if !!bundleTool}
								<input
									type="checkbox"
									onchange={() => setSubTools(tool.id, false)}
									indeterminate={selectedSubtools > 0}
									bind:checked={bundleTool.enabled}
								/>
							{:else}
								<input
									indeterminate={selectedSubtools > 0 && !allSelected}
									checked={allSelected}
									onchange={(e) => setSubTools(tool.id, e.currentTarget.checked)}
									type="checkbox"
								/>
							{/if}
							<p class="flex items-center gap-2">
								<img
									src={tool.metadata?.icon}
									alt={tool.name}
									class="size-6 rounded-lg bg-white p-1"
								/>
								{tool.name}
							</p>

							{#if tool.bundle && isToolEnabled(tool.id)}
								<span class="justify-self-end text-xs text-gray-500">Bundle Selected</span>
							{/if}

							{#if selectedSubtools > 0}
								<span class="justify-self-end text-xs text-gray-500">
									{selectedSubtools} / {bundleTools.length} Selected
								</span>
							{/if}
						</label>

						<p use:tt.tooltip class="w-64 rounded-xl bg-surface3 p-2 text-start">
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

	<div class="flex justify-end gap-2 p-2">
		<button onclick={handleSubmit} disabled={maxExceeded} class="button">Apply</button>
	</div>
</div>

{#snippet toolItem(toolReference: ToolReference)}
	{@const tool = toolSelection[toolReference.id]}
	{@const bundleToolSelected =
		!!toolReference.bundleToolName && !!toolSelection[toolReference.bundleToolName]?.enabled}
	{@const { tooltip, ref } = popover({ hover: true, placement: 'left' })}

	<label
		class="flex cursor-pointer items-center justify-between gap-2 rounded-lg p-2 hover:bg-surface3"
		use:ref
	>
		<p class={twMerge('flex items-center gap-2')}>
			<input
				type="checkbox"
				onchange={() => clearBundle(toolReference)}
				bind:checked={
					() => (bundleToolSelected ? true : tool.enabled),
					(val) => handleSetSubtool(toolReference, val)
				}
			/>
			<img src={tool.icon} alt={tool.name} class="size-6 rounded-lg bg-white p-1" />
			{toolReference.name}
		</p>
	</label>

	<p use:tooltip class="w-64 rounded-xl bg-surface3 p-2 text-start">
		{toolReference.description}
	</p>
{/snippet}
