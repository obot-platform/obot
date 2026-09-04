<script lang="ts">
	import type { EntryDrag } from '../../runes/vmcps/entryDrag.svelte';
	import McpServerIcon from './McpServerIcon.svelte';
	import './vmcpGraph.css';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		drag: EntryDrag;
	}

	let { drag }: Props = $props();
</script>

<svelte:window
	onkeydown={(event) => {
		if (event.key === 'Escape' && drag.started) drag.cancel();
	}}
/>

{#if drag.active}
	{@const wire = drag.wire}
	<div class="pointer-events-none fixed inset-0 z-above-dialog" data-vmcp-drag-overlay>
		{#if wire}
			<svg class="text-primary absolute inset-0 size-full" aria-hidden="true">
				<path class="vmcp-link-halo" d={wire.path} />
				<path class="vmcp-link-core" d={wire.path} />
				<circle class="vmcp-link-node" cx={wire.from.x} cy={wire.from.y} r="4" />
				<circle class="vmcp-link-node" cx={wire.to.x} cy={wire.to.y} r="3" />
			</svg>
		{/if}
		<div
			class={twMerge(
				'bg-base-100 dark:bg-base-300 dark:border-base-400 absolute flex size-11 -translate-x-1/2 -translate-y-1/2 flex-col items-center gap-2 rounded-lg border border-transparent p-1 shadow-lg',
				wire && 'border-primary text-primary vmcp-drop-target'
			)}
			style="left: {drag.x}px; top: {drag.y}px;"
		>
			<McpServerIcon icon={drag.entry?.manifest.icon} />
		</div>
	</div>
{/if}
