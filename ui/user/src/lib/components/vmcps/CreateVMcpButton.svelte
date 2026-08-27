<script lang="ts">
	import type { EntryDrag } from '$lib/runes/vmcps/entryDrag.svelte';
	import { CREATE_VMCP_DROP_ID } from '$lib/runes/vmcps/entryDrag.svelte';
	import './vmcpGraph.css';
	import { Layers, Plus } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		drag: EntryDrag;
		embedded?: boolean;
		onCreate: () => void;
	}

	let { drag, embedded = false }: Props = $props();
	let linked = $derived(drag.isLinked(CREATE_VMCP_DROP_ID));
</script>

<div
	use:drag.createTarget
	class={twMerge(
		'text-primary/50 hover:text-primary group relative z-10 w-fit shrink-0 rounded-lg',
		'aura aura-glow',
		linked && 'vmcp-drop-target border-primary'
	)}
>
	<button
		id="create-vmcp-button"
		class={twMerge(
			'bg-base-100 group dark:bg-base-300 dark:border-base-400 shadow-md rounded-lg border',
			embedded ? 'border-base-300 border-dashed' : 'border-transparent',
			linked && 'border-primary'
		)}
		onclick={() => {
			// onCreate();
			// temporarily commented out, existing composite catalog entry cannot be created with empty componentServers
		}}
	>
		<div class="p-4 size-full flex flex-col items-center justify-center">
			<div class="size-6 mb-4">
				<Layers class="size-6" />
			</div>
			<p
				class="mb-2 uppercase text-muted-content group-hover:text-base-content text-xs font-mono flex w-full justify-center items-center gap-1"
			>
				<Plus class="size-3 shrink-0" /> Create New vMCP
			</p>
			<p class="text-xs text-muted-content font-extralight">
				Drag a MCP server here to get started.
			</p>
		</div>
	</button>
</div>
