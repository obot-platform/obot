<script lang="ts">
	import { getDraggableItemContext } from './contextItem';
	import { getDraggableContext } from './contextRoot';
	import { GripVerticalIcon } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	const rootContext = getDraggableContext();
	const itemContext = getDraggableItemContext();

	let { class: klass = '' } = $props();

	const isDisabled = $derived(rootContext.state.disabled ?? false);
</script>

<button
	class={twMerge(
		'draggable-handle flex h-10 cursor-move touch-none items-center justify-center select-none',
		isDisabled && 'pointer-events-none opacity-50',
		klass
	)}
	type="button"
	onpointerdown={(ev) => {
		if (isDisabled) return;

		return itemContext?.state?.onPointerDown?.(ev);
	}}
	onpointerenter={(ev) => {
		if (isDisabled) return;

		return itemContext?.state?.onPointerEnter?.(ev);
	}}
	onpointerleave={(ev) => {
		if (isDisabled) return;

		return itemContext?.state?.onPointerLeave?.(ev);
	}}
>
	<GripVerticalIcon class="aspect-square h-full" />
</button>
