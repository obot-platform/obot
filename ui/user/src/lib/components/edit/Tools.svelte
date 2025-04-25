<script lang="ts">
	import { popover } from '$lib/actions';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { type AssistantTool } from '$lib/services';
	import ToolCatalog from './ToolCatalog.svelte';
	import { Plus, X } from 'lucide-svelte/icons';
	import { getProjectTools } from '$lib/context/projectTools.svelte';
	import { IGNORED_BUILTIN_TOOLS } from '$lib/constants';
	import { twMerge } from 'tailwind-merge';
	import { clickOutside } from '$lib/actions/clickoutside';
	import { tooltip } from '$lib/actions/tooltip.svelte';

	interface Props {
		onNewTools: (tools: AssistantTool[]) => Promise<void>;
	}

	let { onNewTools }: Props = $props();
	const projectTools = getProjectTools();

	let enabledList = $derived(
		projectTools.tools.filter((t) => t.enabled && t.id && !t.toolType && !t.builtin)
	);
	let builtInList = $derived(
		projectTools.tools.filter((t) => t.builtin && t.id && !IGNORED_BUILTIN_TOOLS.has(t.id))
	);

	async function remove(tool: AssistantTool) {
		if (tool.toolType) {
			projectTools.tools = projectTools.tools.filter((t) => t.id !== tool.id);
		} else {
			onNewTools(projectTools.tools.filter((t) => t.id !== tool.id));
		}
	}

	let toolCatalog = $state<HTMLDialogElement>();
</script>

{#snippet toolList(tools: AssistantTool[])}
	<ul class="flex flex-col gap-2">
		{#each tools as tool (tool.id)}
			{@const tt = popover({ placement: 'top', delay: 300 })}

			<div
				class={twMerge(
					'flex w-full cursor-pointer items-start justify-between gap-1 rounded-md bg-white p-2 shadow-sm',
					tool.builtin && 'bg-surface1/70 cursor-default'
				)}
				use:tt.ref
			>
				<div class="flex w-full flex-col gap-1">
					<div class="flex w-full items-center justify-between gap-1 text-sm font-medium">
						<div class="flex items-center gap-2">
							{#if tool.icon}
								<div class="bg-surface1 flex-shrink-0 rounded-md p-1 dark:bg-gray-200">
									<img src={tool.icon} class="size-4" alt="tool {tool.name} icon" />
								</div>
							{/if}
							<div class="flex flex-col">
								<p class="line-clamp-1 text-xs">{tool.name || 'Untitled'}</p>
								<span class="line-clamp-1 text-xs font-light text-gray-500">{tool.description}</span
								>
							</div>
						</div>
						{#if !tool.builtin}
							<button class="icon-button-small" onclick={() => remove(tool)}>
								<X class="size-5" />
							</button>
						{/if}
					</div>
				</div>

				<p use:tt.tooltip={{ hover: true }} class="tooltip max-w-64">
					{tool.description}
					{tool.builtin ? '(Built-in)' : ''}
				</p>
			</div>
		{/each}
	</ul>
{/snippet}

<div class="flex flex-col gap-2">
	<div class="mb-1 flex items-center justify-between">
		<p class="grow text-sm font-semibold">Tools</p>
		<button
			class="icon-button"
			onclick={() => toolCatalog?.showModal()}
			use:tooltip={'Modify Tools'}
		>
			<Plus class="size-5" />
		</button>
	</div>

	<p class="text-xs text-gray-500">Tools added here are available to all threads.</p>
	<div class="flex flex-col gap-2">
		{@render toolList(enabledList)}
		{@render toolList(builtInList)}
	</div>
</div>

<dialog
	bind:this={toolCatalog}
	class="h-full max-h-[100vh] w-full max-w-[100vw] rounded-none md:h-fit md:w-[1200px] md:rounded-xl"
	use:clickOutside={() => toolCatalog?.close()}
>
	<ToolCatalog
		onSelectTools={onNewTools}
		onSubmit={() => toolCatalog?.close()}
		tools={projectTools.tools}
		maxTools={projectTools.maxTools}
	/>
</dialog>
