<script lang="ts">
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { HELPER_TEXTS } from '$lib/context/helperMode.svelte';
	import { ChatService, type Project } from '$lib/services';
	import Toggle from '$lib/components/Toggle.svelte';
	import { getProjectTools } from '$lib/context/projectTools.svelte';
	import { getToolReferenceMap } from '$lib/context/toolReferences.svelte';

	interface Props {
		project: Project;
	}

	let { project = $bindable() }: Props = $props();
	const projectTools = getProjectTools();
	const toolReferences = getToolReferenceMap();
	let projectToolsMap = $derived(new Map(projectTools.tools.map((x) => [x.id, x])));

	const toolsToInclude = ['database', 'memory', 'knowledge', 'time'];
	const builtInTools = $derived([
		...toolsToInclude.map((tool) => ({
			id: tool,
			type: 'toolReference' as const,
			builtin: false,
			...projectToolsMap.get(tool)
		}))
	]);

	function sortBuiltInLast<T extends { builtin: boolean }>(a: T, b: T) {
		return a.builtin ? 1 : b.builtin ? -1 : 0;
	}
</script>

<CollapsePane
	classes={{ header: 'pl-3 py-2 text-md', content: 'p-0' }}
	iconSize={5}
	header="Built-In Capabilities"
	helpText={HELPER_TEXTS.general}
>
	<div class="flex flex-col overflow-x-hidden p-2">
		{#each builtInTools.sort(sortBuiltInLast) as tool (tool.id)}
			{@const toolReference = toolReferences.get(tool.id)}
			<div
				class="flex min-h-9 items-center justify-between rounded-md bg-transparent p-2 pr-3 text-xs transition-colors duration-200"
			>
				<span class="flex items-center gap-2" class:opacity-50={tool.builtin}>
					<div class="bg-surface1 flex-shrink-0 rounded-sm p-1 dark:bg-gray-600">
						<img
							src={tool?.icon ?? toolReference?.metadata?.icon}
							class="size-4"
							alt={toolReference?.name}
						/>
					</div>

					<div class="flex flex-col">
						<p class="flex items-center gap-1">
							{tool?.name ?? toolReference?.name}
							{#if tool.builtin}
								<span
									class="rounded-full border border-gray-400 px-1.5 py-0.5 text-[9px] text-gray-500 dark:border-gray-600"
								>
									Non-optional
								</span>
							{/if}
						</p>
						<span class="text-xs text-gray-500"
							>{tool?.description ?? toolReference?.description}</span
						>
					</div>
				</span>
				<div class="w-9">
					{#if !tool.builtin}
						<Toggle
							label="Toggle Capability"
							checked={!!tool.enabled}
							onChange={async (checked) => {
								const matchingIndex = projectTools.tools.findIndex(
									(tool) => tool.id === toolReference?.id
								);
								if (!toolReference || !matchingIndex) return;

								projectTools.tools[matchingIndex].enabled = checked;
								await ChatService.updateProjectTools(project.assistantID, project.id, {
									items: projectTools.tools
								});
							}}
						/>
					{/if}
				</div>
			</div>
		{/each}
	</div>
</CollapsePane>
