<script lang="ts">
	import { popover } from '$lib/actions';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { type Assistant, type AssistantTool } from '$lib/services';
	import { Plus, X } from 'lucide-svelte/icons';
	import ToolCatalog from './ToolCatalog.svelte';

	interface Props {
		tools: AssistantTool[];
		onNewTools: (tools: AssistantTool[]) => Promise<void>;
		assistant?: Assistant;
	}

	let { tools, onNewTools, assistant }: Props = $props();
	let enabledList = $derived(tools.filter((t) => !t.builtin && t.enabled));

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

{#snippet toolList(tools: AssistantTool[], remove: boolean)}
	<ul class="flex flex-col">
		{#each tools as tool (tool.id)}
			{@const tt = popover({ hover: true, placement: 'top', delay: 300 })}

			<div class="flex w-full cursor-pointer items-start justify-between gap-1 p-2" use:tt.ref>
				<div class="flex w-full flex-col gap-1">
					<span class="flex w-full items-center justify-between gap-1 text-sm font-medium">
						<span class="flex items-center gap-2">
							{#if tool.icon}
								<img
									src={tool.icon}
									class="size-6 rounded-full bg-white p-1"
									alt="tool {tool.name} icon"
								/>
							{/if}
							<p class="line-clamp-1">{tool.name}</p>
						</span>
						<button class="icon-button-small" onclick={() => modify(tool, remove)}>
							{#if remove}
								<X class="size-5" />
							{:else}
								<Plus class="size-5" />
							{/if}
						</button>
					</span>
					<span class="line-clamp-2 text-xs text-gray-500">{tool.description}</span>
				</div>

				<p use:tt.tooltip class="tooltip max-w-64">{tool.description}</p>
			</div>
		{/each}
	</ul>
{/snippet}

<CollapsePane header="Tools">
	<div class="flex flex-col gap-2">
		<ul class="flex flex-col gap-2">
			{@render toolList(enabledList, true)}
		</ul>

		<div class="self-end">
			<ToolCatalog {tools} onSelectTools={onNewTools} maxTools={assistant?.maxTools} />
		</div>
	</div>
</CollapsePane>
