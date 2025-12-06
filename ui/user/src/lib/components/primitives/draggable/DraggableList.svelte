<script lang="ts">
	import type { Snippet } from 'svelte';
	import { setDraggableContext, type DraggableContext, type DraggableItem } from './contextRoot';
	import { on } from 'svelte/events';
	import { twMerge } from 'tailwind-merge';

	type Props = {
		class: string;
		order: string[];
		as: string;
		disabled?: boolean;
		gap?: number;
		onChange?: (items: unknown[]) => void;
		children?: Snippet<[]>;
	};
	let {
		class: klass,
		as,
		order = [],
		disabled = false,
		gap = 0,
		onChange,
		children
	}: Props = $props();

	let internalItems: DraggableItem<unknown>[] = $state([]);

	// Drag source Id
	let sourceItemId: string | undefined = $state();
	const sourceItem = $derived(internalItems.find((d) => d.id === sourceItemId));
	const sourceItemIndex = $derived(sourceItem ? internalItems.indexOf(sourceItem) : -1);

	// Drag target Id
	let targetItemId: string | undefined = $state();
	const targetItem = $derived(internalItems.find((d) => d.id === targetItemId));
	const targetItemIndex = $derived(targetItem ? internalItems.indexOf(targetItem) : -1);

	// Source item height for shift calculations
	let sourceItemHeight: number = $state(0);

	// timeout id before updating bindable data
	let synchTimeoutId: number | undefined = undefined;

	// sync number to trigger effect
	// I don't want to call change fn when items are mounting
	let sync: number | undefined = $state(undefined);

	// Reference to the list element for position calculations
	let listElement: HTMLElement | undefined = $state();

	// Overlap threshold (30% = 0.3)
	const OVERLAP_THRESHOLD = 0.3;

	// Auto-scroll configuration
	const SCROLL_EDGE_THRESHOLD = 80; // pixels from edge to start scrolling
	const SCROLL_SPEED = 12; // base pixels per frame
	let autoScrollAnimationId: number | null = null;
	let lastPointerY: number = 0;
	let scrollableParent: HTMLElement | Window | null = null;

	// Track scroll delta for dragged item position correction
	let initialScrollTop: number = 0;
	let scrollDelta: number = $state(0);

	// Find the nearest scrollable parent element
	function findScrollableParent(element: HTMLElement | null): HTMLElement | Window {
		if (!element) return window;

		let current: HTMLElement | null = element.parentElement;

		while (current) {
			const style = getComputedStyle(current);
			const overflowY = style.overflowY;

			// Check if element is scrollable (has overflow-y auto or scroll and can actually scroll)
			if (
				(overflowY === 'auto' || overflowY === 'scroll') &&
				current.scrollHeight > current.clientHeight
			) {
				return current;
			}

			current = current.parentElement;
		}

		return window;
	}

	// Share context with children
	const context: DraggableContext<unknown> = {
		get state() {
			return {
				items: internalItems,
				sourceItemId,
				sourceItemIndex,
				sourceItemHeight,
				targetItemId,
				targetItemIndex,
				disabled,
				gap,
				scrollDelta
			};
		},
		methods: {
			reorder: () => {
				if (!sourceItem || !targetItem) {
					sourceItemId = undefined;
					targetItemId = undefined;
					sourceItemHeight = 0;
					return;
				}

				clearTimeout(synchTimeoutId);

				// take a snapshot of items
				let array = [...$state.snapshot(internalItems)] as DraggableItem<unknown>[];

				const reorderedArray = [];

				for (let i = 0; i < array.length; i++) {
					const item = array[i];

					// skip the source item
					if (i === sourceItemIndex) continue;

					// add item to new array
					reorderedArray.push(item);
				}

				reorderedArray.splice(targetItemIndex, 0, $state.snapshot(sourceItem));

				internalItems = [...reorderedArray];

				sourceItemId = undefined;
				targetItemId = undefined;
				sourceItemHeight = 0;

				synchTimeoutId = setTimeout(() => {
					// sync array
					sync = Date.now();
				}, 1000 / 60);
			},
			mount: (id, item) => {
				clearTimeout(synchTimeoutId);

				if (sync) {
					const preOrderArray = [...internalItems, item];

					const obj = preOrderArray.reduce(
						(acc, val) => {
							acc[val.id] = val;
							return acc;
						},
						{} as Record<string, DraggableItem<unknown>>
					);

					const orderedArray = [];

					for (const id of order) {
						const item = obj[id];
						if (item) {
							orderedArray.push(obj[id]);
						}
					}

					internalItems = [...orderedArray];
				} else {
					internalItems = [...internalItems, item];
				}

				synchTimeoutId = setTimeout(() => {
					// sync array
					sync = Date.now();
				}, 1000 / 60);

				return () => context.methods.unmount(id);
			},
			unmount: (id) => {
				clearTimeout(synchTimeoutId);

				internalItems = internalItems.filter((d) => d.id !== id);

				synchTimeoutId = setTimeout(() => {
					// sync arraysetSourceItem
					sync = Date.now();
				}, 1000 / 60);
			},
			setSourceItem: (id) => {
				sourceItemId = id;
			},
			setTargetItem: (id) => {
				targetItemId = id;
			},
			getItemIndex: (id) => {
				return internalItems.findIndex((item) => item.id === id);
			},
			setSourceItemHeight: (height) => {
				sourceItemHeight = height;
			}
		}
	};

	setDraggableContext(context);

	// only react if length changed
	const length = $derived(order.length);

	$effect(() => {
		if (sync === undefined) return;
		if (length === 0) return;
		if (length > internalItems.length) return;

		onChange?.(internalItems.map((d) => d.data));
	});

	// Calculate target based on dragged element position with threshold
	function calculateTargetFromPosition() {
		if (!listElement || !sourceItemId) return;

		// Use pointer position as the source center since the dragged element follows the pointer
		// This avoids timing issues where DOM hasn't re-rendered with new scrollDelta yet
		const sourceElement = listElement.querySelector(
			`.draggable-element[data-id="${sourceItemId}"]`
		) as HTMLElement;
		if (!sourceElement) return;

		const sourceRect = sourceElement.getBoundingClientRect();
		// Use lastPointerY as the center point, but fall back to element center if pointer hasn't moved
		const sourceCenter = lastPointerY > 0 ? lastPointerY : sourceRect.top + sourceRect.height / 2;

		// Get all draggable items except the source
		const items = listElement.querySelectorAll('.draggable-element') as NodeListOf<HTMLElement>;

		let newTargetId: string | undefined = sourceItemId; // Default to source position (no change)

		let lowestTargetIndex = sourceItemIndex; // Track lowest valid target when dragging up

		for (const item of items) {
			const itemId = item.dataset['id'];
			if (!itemId || itemId === sourceItemId) continue;

			const itemRect = item.getBoundingClientRect();
			const itemIndex = internalItems.findIndex((i) => i.id === itemId);

			// Calculate threshold positions
			const thresholdTop = itemRect.top + itemRect.height * OVERLAP_THRESHOLD;
			const thresholdBottom = itemRect.top + itemRect.height * (1 - OVERLAP_THRESHOLD);

			// If source is being dragged DOWN (source index < item index)
			if (sourceItemIndex < itemIndex) {
				// Source center needs to pass the top threshold of the item
				if (sourceCenter > thresholdTop) {
					newTargetId = itemId;
				}
			}
			// If source is being dragged UP (source index > item index)
			else if (sourceItemIndex > itemIndex) {
				// Source center needs to pass the bottom threshold of the item
				if (sourceCenter < thresholdBottom) {
					// Track the lowest index (furthest up) that satisfies the condition
					if (itemIndex < lowestTargetIndex) {
						lowestTargetIndex = itemIndex;
						newTargetId = itemId;
					}
				}
			}
		}

		if (newTargetId !== targetItemId) {
			targetItemId = newTargetId;
		}
	}

	// Get current scroll position of the scrollable parent
	function getCurrentScrollTop(): number {
		if (!scrollableParent) return 0;
		if (scrollableParent === window) {
			return window.scrollY;
		}
		return (scrollableParent as HTMLElement).scrollTop;
	}

	// Auto-scroll function that runs during drag
	function startAutoScroll() {
		// Find scrollable parent when drag starts
		scrollableParent = findScrollableParent(listElement ?? null);
		// Record initial scroll position
		initialScrollTop = getCurrentScrollTop();
		scrollDelta = 0;

		function scrollStep() {
			if (!sourceItemId || !scrollableParent) {
				autoScrollAnimationId = null;
				return;
			}

			const containerTop = 0;
			const containerBottom = window.innerHeight;

			const distanceFromTop = lastPointerY - containerTop;
			const distanceFromBottom = containerBottom - lastPointerY;

			let scrollAmount = 0;

			if (distanceFromTop < SCROLL_EDGE_THRESHOLD && distanceFromTop >= 0) {
				// Scroll up - faster when closer to edge
				const intensity = 1 - distanceFromTop / SCROLL_EDGE_THRESHOLD;
				scrollAmount = -SCROLL_SPEED * intensity * intensity;
			} else if (distanceFromBottom < SCROLL_EDGE_THRESHOLD && distanceFromBottom >= 0) {
				// Scroll down - faster when closer to edge
				const intensity = 1 - distanceFromBottom / SCROLL_EDGE_THRESHOLD;
				scrollAmount = SCROLL_SPEED * intensity * intensity;
			}

			if (scrollAmount !== 0) {
				if (scrollableParent === window) {
					window.scrollBy(0, scrollAmount);
				} else {
					(scrollableParent as HTMLElement).scrollTop += scrollAmount;
				}
				// Update scroll delta for dragged item position
				scrollDelta = getCurrentScrollTop() - initialScrollTop;
				// Recalculate target position after scroll
				requestAnimationFrame(calculateTargetFromPosition);
			}

			autoScrollAnimationId = requestAnimationFrame(scrollStep);
		}

		if (autoScrollAnimationId === null) {
			autoScrollAnimationId = requestAnimationFrame(scrollStep);
		}
	}

	function stopAutoScroll() {
		if (autoScrollAnimationId !== null) {
			cancelAnimationFrame(autoScrollAnimationId);
			autoScrollAnimationId = null;
		}
		scrollableParent = null;
		initialScrollTop = 0;
		scrollDelta = 0;
	}

	// Calculate target based on position during pointer/touch move
	$effect(() => {
		if (!sourceItemId) return;

		const onPointerMove = (e: PointerEvent) => {
			lastPointerY = e.clientY;
			requestAnimationFrame(calculateTargetFromPosition);
		};

		return on(window, 'pointermove', onPointerMove);
	});

	// This code detects target id on touch-based devices
	$effect(() => {
		if (!sourceItemId) return;
		// check if current device supports touch events
		if (!navigator.maxTouchPoints) return;

		const onTouchMove = (e: TouchEvent) => {
			if (e.touches.length > 0) {
				lastPointerY = e.touches[0].clientY;
			}
			requestAnimationFrame(calculateTargetFromPosition);
		};

		return on(window, 'touchmove', onTouchMove);
	});

	// Start/stop auto-scrolling when drag state changes
	$effect(() => {
		if (sourceItemId) {
			startAutoScroll();
		} else {
			stopAutoScroll();
		}

		return () => {
			stopAutoScroll();
		};
	});

	// Track manual scrolling (e.g., mouse wheel) during drag
	$effect(() => {
		if (!sourceItemId || !scrollableParent) return;

		const onScroll = () => {
			scrollDelta = getCurrentScrollTop() - initialScrollTop;
			requestAnimationFrame(calculateTargetFromPosition);
		};

		const target = scrollableParent === window ? window : scrollableParent;
		return on(target as Window | HTMLElement, 'scroll', onScroll);
	});

	$effect(() => {
		if (!sourceItemId) return;

		const style = document.createElement('style');
		style.textContent = '* { cursor: grabbing !important; }';
		document.head.appendChild(style);

		return () => {
			style.remove();
		};
	});
</script>

<svelte:element
	this={as ?? 'div'}
	bind:this={listElement}
	class={twMerge('draggable-list flex flex-col', klass)}
>
	{@render children?.()}
</svelte:element>
