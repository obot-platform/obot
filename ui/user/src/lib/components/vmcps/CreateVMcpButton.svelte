<script lang="ts">
	import { popover } from '$lib/actions';
	import VMcpDragHint from '$lib/components/vmcps/VMcpDragHint.svelte';
	import type { EntryDrag } from '$lib/runes/vmcps/entryDrag.svelte';
	import { CREATE_VMCP_DROP_ID } from '$lib/runes/vmcps/entryDrag.svelte';
	import './vmcpGraph.css';
	import { Layers, Plus } from '@lucide/svelte';
	import { tick } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	const CREATE_VMCP_DRAG_HINT_ID = 'create-vmcp-drag-hint';
	const CREATE_VMCP_DRAG_HINT_TITLE_ID = 'create-vmcp-drag-hint-title';
	const CREATE_VMCP_DRAG_HINT_DESCRIPTION_ID = 'create-vmcp-drag-hint-description';

	interface Props {
		drag: EntryDrag;
		embedded?: boolean;
	}

	let { drag, embedded = false }: Props = $props();
	let linked = $derived(drag.isLinked(CREATE_VMCP_DROP_ID));
	let triggerButton = $state<HTMLButtonElement | null>(null);
	let hintPanelEl = $state<HTMLElement | null>(null);

	const { tooltip, ref, toggle, open } = popover({
		placement: 'right',
		offset: 12,
		onOpenChange(isOpen) {
			if (!isOpen) triggerButton?.focus();
		}
	});

	$effect(() => {
		if (drag.active) toggle(false);
	});

	$effect(() => {
		if (!open || !hintPanelEl) return;
		void tick().then(() => {
			hintPanelEl
				?.querySelector<HTMLButtonElement>('button[aria-label="Dismiss drag and drop tip"]')
				?.focus();
		});
	});

	$effect(() => {
		if (!open) return;
		const onKeyDown = (event: KeyboardEvent) => {
			if (event.key !== 'Escape') return;
			event.stopPropagation();
			toggle(false);
		};
		document.addEventListener('keydown', onKeyDown);
		return () => document.removeEventListener('keydown', onKeyDown);
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
		bind:this={triggerButton}
		aria-haspopup="dialog"
		aria-expanded={open}
		aria-controls={CREATE_VMCP_DRAG_HINT_ID}
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

<div
	bind:this={hintPanelEl}
	use:tooltip
	id={CREATE_VMCP_DRAG_HINT_ID}
	role="dialog"
	aria-labelledby={CREATE_VMCP_DRAG_HINT_TITLE_ID}
	aria-describedby={CREATE_VMCP_DRAG_HINT_DESCRIPTION_ID}
	tabindex="-1"
	class="z-40"
>
	<VMcpDragHint
		titleId={CREATE_VMCP_DRAG_HINT_TITLE_ID}
		descriptionId={CREATE_VMCP_DRAG_HINT_DESCRIPTION_ID}
		onDismiss={() => toggle(false)}
	/>
</div>
