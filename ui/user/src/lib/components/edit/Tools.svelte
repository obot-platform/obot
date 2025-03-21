<script lang="ts">
	import { popover } from '$lib/actions';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { type AssistantTool } from '$lib/services';
	import { Plus, X } from 'lucide-svelte/icons';
	import ToolCatalog from './ToolCatalog.svelte';
	import { responsive, tools as toolsStore } from '$lib/stores';

	interface Props {
		onNewTools: (tools: AssistantTool[]) => Promise<void>;
	}

	let { onNewTools }: Props = $props();
	const tools = toolsStore.current.tools;
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

	let catalog = popover({ fixed: true, slide: responsive.isMobile ? 'left' : undefined });
</script>

{#snippet toolList(tools: AssistantTool[], remove: boolean)}
	<ul class="flex flex-col gap-2">
		{#each tools as tool (tool.id)}
			{@const tt = popover({ hover: true, placement: 'top', delay: 300 })}

			<div
				class="flex w-full cursor-pointer items-start justify-between gap-1 rounded-md bg-surface1 p-2"
				use:tt.ref
			>
				<div class="flex w-full flex-col gap-1">
					<div class="flex w-full items-center justify-between gap-1 text-sm font-medium">
						<div class="flex items-center gap-2">
							{#if tool.icon}
								<div class="rounded-md bg-surface1 p-1 dark:bg-gray-200">
									<img src={tool.icon} class="size-6" alt="tool {tool.name} icon" />
								</div>
							{/if}
							<div class="flex flex-col gap-1">
								<p class="line-clamp-1">{tool.name}</p>
								<span class="line-clamp-2 text-xs font-light text-gray-500">{tool.description}</span
								>
							</div>
						</div>
						<button class="icon-button" onclick={() => modify(tool, remove)}>
							{#if remove}
								<X class="size-5" />
							{:else}
								<Plus class="size-5" />
							{/if}
						</button>
					</div>
				</div>

				<p use:tt.tooltip class="tooltip max-w-64">{tool.description}</p>
			</div>
		{/each}
	</ul>
{/snippet}

<CollapsePane header="Tools">
	<div class="flex flex-col gap-4">
		{@render toolList(enabledList, true)}

		<div class="self-end">
			<button
				class="button flex items-center gap-1 text-sm"
				use:catalog.ref
				onclick={() => catalog.toggle(true)}><Plus class="size-4" /> Tools</button
			>
			<div
				use:catalog.tooltip
				class="default-dialog bottom-0 left-0 h-screen w-full rounded-none p-2 md:bottom-1/2 md:left-1/2 md:h-fit md:w-auto md:-translate-x-1/2 md:translate-y-1/2 md:rounded-xl"
			>
				<ToolCatalog onSelectTools={onNewTools} onSubmit={() => catalog.toggle(false)} />
			</div>
		</div>
	</div>
</CollapsePane>
