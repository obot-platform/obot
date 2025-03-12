<script lang="ts" module>
	const MAX_TOOLS = 5;
</script>

<script lang="ts">
	import { popover } from '$lib/actions';
	import { getToolBundleMap } from '$lib/context/toolReferences.svelte';
	import type { AssistantTool, ToolReference } from '$lib/services/chat/types';
	import { twMerge } from 'tailwind-merge';
	import CollapsePane from './CollapsePane.svelte';

	interface Props {
		tools: AssistantTool[];
		onSelectTools: (tools: AssistantTool[]) => void;
	}

	let { tools, onSelectTools }: Props = $props();

	let input = $state<HTMLInputElement>();

	const bundleMap = getToolBundleMap();

	let search = $state('');

	let toolSelection = $state(
		tools
			.filter((t) => !t.builtin)
			.reduce<Record<string, AssistantTool>>((acc, tool) => {
				acc[tool.id] = { ...tool };
				return acc;
			}, {})
	);

	let canSelectMore = $derived(
		Object.values(toolSelection).filter((t) => t.enabled).length < MAX_TOOLS
	);

	let catalog = popover({ fixed: true });

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

	function clearSubTools(toolRef: ToolReference) {
		if (!toolRef.bundle) return;

		const subtools =
			bundleMap.get(toolRef.id)?.bundleTools.filter((t) => t.id in toolSelection) ?? [];

		for (const subtool of subtools) {
			toolSelection[subtool.id].enabled = false;
		}
	}

	function clearBundle(toolRef: ToolReference) {
		if (!toolRef.bundleToolName) return;

		if (toolRef.bundleToolName in toolSelection) {
			toolSelection[toolRef.bundleToolName].enabled = false;
		}
	}
</script>

<button class="button" use:catalog.ref onclick={() => catalog.toggle()}>Open Catalog</button>

<div
	use:catalog.tooltip
	class="left-1/2 top-[15%] flex min-h-[500px] w-[500px] -translate-x-1/2 flex-col rounded-lg rounded-b-lg border border-surface3 bg-surface1 shadow-lg"
>
	<div class="rounded-t-lg p-2">
		<input
			bind:this={input}
			class="w-full rounded-lg p-2"
			bind:value={search}
			type="text"
			placeholder="Search"
		/>
	</div>

	<p class={twMerge('text-center text-sm', canSelectMore && 'invisible')}>
		Maximum number of tools selected
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
						{@const disabled = !bundleTool?.enabled && !canSelectMore}
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
									{disabled}
									onchange={() => clearSubTools(tool)}
									bind:checked={bundleTool.enabled}
								/>
							{:else}
								<input disabled type="checkbox" />
							{/if}
							<p class="flex items-center gap-2">
								<img src={tool.metadata?.icon} alt={tool.name} class="size-6" />
								{tool.name}
							</p>

							{#if selectedSubtools > 0}
								<span class="justify-self-end text-xs">
									({selectedSubtools} Selected)
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
		<button onclick={() => catalog.toggle(false)} class="button-secondary">Cancel</button>
		<button onclick={handleSubmit} class="button">Apply</button>
	</div>
</div>

{#snippet toolItem(toolReference: ToolReference)}
	{@const tool = toolSelection[toolReference.id]}
	{@const disabled = !tool.enabled && !canSelectMore}
	{@const bundleToolSelected =
		!!toolReference.bundleToolName && !!toolSelection[toolReference.bundleToolName]?.enabled}
	{@const { tooltip, ref } = popover({ hover: true, placement: 'left' })}

	<label
		class="flex cursor-pointer items-center justify-between gap-2 rounded-lg p-2 hover:bg-surface3"
		use:ref
	>
		<p class={twMerge('flex items-center gap-2', disabled && 'opacity-25')}>
			<input
				type="checkbox"
				disabled={disabled || bundleToolSelected}
				onchange={() => clearBundle(toolReference)}
				bind:checked={
					() => (bundleToolSelected ? true : tool.enabled), (val) => (tool.enabled = val)
				}
			/>
			<img src={tool.icon} alt={tool.name} class="size-6" />
			{toolReference.name}
		</p>
	</label>

	<p use:tooltip class="w-64 rounded-xl bg-surface3 p-2 text-start">
		{toolReference.description}
	</p>
{/snippet}
