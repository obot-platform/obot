<script lang="ts">
	import { tick, type Snippet } from 'svelte';
	import { on } from 'svelte/events';
	import { twMerge } from 'tailwind-merge';
	import { getDraggableContext } from './contextRoot';
	import { setDraggableItemContext } from './contextItem';

	type Props = {
		rootClass?: string;
		class?: string;
		as?: string;
		index?: number;
		id?: string;
		data?: unknown;
		children?: Snippet;
	};

	const draggableContext = getDraggableContext();

	if (!draggableContext) {
		throw new Error('Draggable context was not found');
	}

	let {
		rootClass,
		class: klass,
		as,
		index = 0,
		id = (Date.now() * Math.random() + index).toString(16),
		data,
		children
	}: Props = $props();

	let isPointerDown = $state(false);

	let isPointerEntered = $state(false);

	let isDragOver = $derived(draggableContext.state.targetItemId === id);

	let isActive = $derived(
		draggableContext.state.sourceItemId && draggableContext.state.sourceItemId === id
	);

	// Calculate if this item should shift up or down during drag
	let shiftDirection = $derived.by(() => {
		const { sourceItemId, sourceItemIndex, targetItemIndex } = draggableContext.state;

		// No dragging happening or this is the dragged item
		if (!sourceItemId || isActive) return 0;

		// Get current item's index
		const currentIndex = draggableContext.methods.getItemIndex(id);
		if (currentIndex === -1) return 0;

		// No valid target yet
		if (targetItemIndex === -1) return 0;

		// Moving DOWN (source is above target)
		if (sourceItemIndex < targetItemIndex) {
			// Items between source and target (inclusive) should shift UP
			if (currentIndex > sourceItemIndex && currentIndex <= targetItemIndex) {
				return -1; // shift up
			}
		}
		// Moving UP (source is below target)
		else if (sourceItemIndex > targetItemIndex) {
			// Items between target and source (inclusive) should shift DOWN
			if (currentIndex >= targetItemIndex && currentIndex < sourceItemIndex) {
				return 1; // shift down
			}
		}

		return 0;
	});

	let top: number | undefined = $state();

	let pageY: number | undefined = $state(0);

	let dy = $state(0);

	let rootElement: HTMLElement | undefined = $state();
	let containerElement: HTMLElement | undefined = $state();

	const item = {
		id,
		get data() {
			return data;
		}
	};

	// Share item context
	setDraggableItemContext({
		get state() {
			return {
				get id() {
					return item.id;
				},
				get data() {
					return item.data;
				},
				onPointerDown,
				onPointerEnter,
				onPointerLeave
			};
		}
	});

	// Ensure it runs only once;
	const unmount = draggableContext.methods.mount(item.id, item);
	$effect(() => unmount);

	$effect(() => {
		const onPointerMove = (ev: PointerEvent) => {
			ev.preventDefault();

			if (!isActive) return;

			requestAnimationFrame(() => {
				dy = ev.pageY - (pageY ?? 0);
			});
		};

		return on(window, 'pointermove', onPointerMove);
	});

	// Attach pointer up handler to the window object to assure it will be call when user release the pointer
	$effect(() => {
		const onPointerUp = async () => {
			isPointerDown = false;
			isPointerEntered = false;
			top = 0;
			pageY = 0;
			dy = 0;

			await tick();

			draggableContext.methods.reorder();
		};
		return on(window, 'pointerup', onPointerUp);
	});

	function onPointerDown(ev: PointerEvent) {
		ev.preventDefault();

		const target = ev.currentTarget as HTMLElement;

		if (target.contains(rootElement!)) {
			return;
		}

		if (!rootElement) return;

		top = containerElement?.offsetTop ?? 0;

		pageY = ev.pageY;

		// Report this item's height before setting as source
		draggableContext.methods.setSourceItemHeight(rootElement.offsetHeight);
		draggableContext.methods.setSourceItem(id);

		isPointerDown = true;
	}

	function onPointerEnter(ev: PointerEvent) {
		ev.preventDefault();

		if (draggableContext.state.sourceItemId) return;

		isPointerEntered = true;
	}

	function onPointerLeave(ev: PointerEvent) {
		ev.preventDefault();

		if (isActive) return;

		isPointerEntered = false;
	}
</script>

<svelte:element
	this={as ?? 'div'}
	bind:this={rootElement}
	class={twMerge(
		'draggable-element relative min-w-full touch-none',
		isActive && 'pointer-events-none z-10 cursor-move',
		shiftDirection !== 0 && !isActive && 'transition-transform duration-200 ease-out',
		rootClass
	)}
	data-id={id}
	style:top={`${top ?? 0}px`}
	style:transform={isPointerDown
		? `translateY(${dy + draggableContext.state.scrollDelta}px)`
		: shiftDirection !== 0
			? `translateY(${shiftDirection * (draggableContext.state.sourceItemHeight + draggableContext.state.gap)}px)`
			: ''}
>
	<div
		bind:this={containerElement}
		class={twMerge(
			'draggable-inner-element relative isolate z-[1] flex justify-start gap-2 rounded-sm border border-transparent transition-colors duration-200',
			isPointerEntered && 'border-primary bg-primary/5',
			!isActive && isDragOver && 'bg-surface2 pointer-events-none',
			klass
		)}
	>
		{@render children?.()}
	</div>
</svelte:element>
