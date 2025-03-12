<script lang="ts">
	import { popover } from '$lib/actions';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { type AssistantTool } from '$lib/services';
	import { Plus, X } from 'lucide-svelte/icons';
	import ToolCatalog from './ToolCatalog.svelte';

	interface Props {
		tools: AssistantTool[];
		onNewTools: (tools: AssistantTool[]) => Promise<void>;
	}

	let { tools, onNewTools }: Props = $props();
	let enabledList = $derived(tools.filter((t) => !t.builtin && t.enabled));
	let disabledList = $derived(tools.filter((t) => !t.builtin && !t.enabled));
	let { ref, tooltip, toggle } = popover();

	async function modify(tool: AssistantTool, remove: boolean) {
		let newTools = enabledList;
		if (remove) {
			newTools = newTools.filter((t) => t.id !== tool.id);
		} else {
			newTools = [...newTools, { ...tool, enabled: true }];
		}
		await onNewTools(newTools);
	}
</script>

{#snippet toolList(tools: AssistantTool[], remove: boolean, bg: string)}
	<ul class="flex flex-col gap-2">
		{#each tools as tool}
			{#key tool.id}
				<div class="flex items-center justify-between gap-1 {bg} rounded-3xl px-5 py-4">
					<div class="flex flex-col gap-1">
						<div class="flex items-center gap-2">
							{#if tool.icon}
								<img
									src={tool.icon}
									class="h-6 rounded-md bg-white p-1"
									alt="tool {tool.name} icon"
								/>
							{/if}
							<span class="text-sm font-medium">{tool.name}</span>
						</div>
						<span class="text-xs">{tool.description}</span>
					</div>
					<button class="icon-button" onclick={() => modify(tool, remove)}>
						{#if remove}
							<X class="h-5 w-5" />
						{:else}
							<Plus class="h-5 w-5" />
						{/if}
					</button>
				</div>
			{/key}
		{/each}
	</ul>
{/snippet}

<CollapsePane header="Tools">
	<div class="flex flex-col gap-2">
		<ul class="flex flex-col gap-2">
			{@render toolList(enabledList, true, 'bg-surface2')}
		</ul>

		<div class="self-end"><ToolCatalog {tools} onSelectTools={onNewTools} /></div>
	</div>
</CollapsePane>
