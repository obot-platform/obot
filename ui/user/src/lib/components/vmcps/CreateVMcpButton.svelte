<script lang="ts">
	import { popover } from '$lib/actions';
	import VMcpDragHint from '$lib/components/vmcps/VMcpDragHint.svelte';
	import type { EntryDrag } from '$lib/runes/vmcps/entryDrag.svelte';
	import { CREATE_VMCP_DROP_ID } from '$lib/runes/vmcps/entryDrag.svelte';
	import './vmcpGraph.css';
	import { Layers, Plus } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		drag: EntryDrag;
		embedded?: boolean;
	}

	let { drag, embedded = false }: Props = $props();
	let linked = $derived(drag.isLinked(CREATE_VMCP_DROP_ID));

	const { tooltip, ref, toggle } = popover({
		placement: 'right',
		offset: 12
	});

	$effect(() => {
		if (drag.active) toggle(false);
	});
</script>

<div
	use:drag.createTarget
	class={twMerge(
		'text-primary/50 group relative z-10 w-fit shrink-0 rounded-lg',
		linked && 'aura aura-glow vmcp-drop-target border-primary text-primary'
	)}
>
	<button
		id="create-vmcp-button"
		type="button"
		class={twMerge(
			'cursor-default bg-base-100 group dark:bg-base-300 dark:border-base-400 shadow-md rounded-lg border',
			embedded ? 'border-base-300 border-dashed' : 'border-transparent',
			linked && 'border-primary'
		)}
		use:ref
		onclick={(event) => {
			toggle();
			event.stopPropagation();
		}}
	>
		<div class="p-4 size-full flex flex-col items-center justify-center">
			<div class="size-6 mb-4">
				<Layers class="size-6" />
			</div>
			<p
				class={twMerge(
					'mb-2 uppercase text-muted-content text-xs font-mono flex w-full justify-center items-center gap-1',
					linked && 'text-base-content'
				)}
			>
				<Plus class="size-3 shrink-0" /> Create New vMCP
			</p>
			<p class="text-xs text-muted-content font-extralight">
				Drag a MCP server here to get started.
			</p>
		</div>
	</button>
</div>

<div use:tooltip class="z-40">
	<VMcpDragHint onDismiss={() => toggle(false)} />
</div>
