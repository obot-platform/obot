<script lang="ts" generics="T extends { id: string }">
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import {
		defaultCamera,
		fitCamera,
		panBy,
		viewBand,
		wheelZoomFactor,
		zoomAt
	} from '$lib/services/vmcps/camera';
	import { ZOOM_STEP } from '$lib/services/vmcps/constants';
	import type { Camera, RowContext } from '$lib/services/vmcps/types';
	import { Maximize2, Minus, Plus } from '@lucide/svelte';
	import { untrack, type Snippet } from 'svelte';

	let {
		item,
		actions,
		expanded = false,
		dragActive = false,
		estimateHeight,
		row,
		empty,
		viewportEl = $bindable()
	}: {
		item?: T;
		actions?: Snippet;
		expanded?: boolean;
		dragActive?: boolean;
		estimateHeight: (item: T, expanded: boolean) => number;
		row: Snippet<[T, RowContext]>;
		empty?: Snippet;
		viewportEl?: HTMLElement;
	} = $props();
	let camera = $state<Camera>({ x: 0, y: 0, zoom: 1 });
	let viewportSize = $state({ width: 1, height: 1 });
	let measuredHeights = $state<Record<string, number>>({});
	let centeredId = $state<string>();
	let cameraMovedByUser = $state(false);
	let panPointer = $state<{ id: number; x: number; y: number }>();
	let interacting = $state(false);

	/** Stands in for the card's width until it has been measured, so the first frame is close. */
	const ESTIMATED_WORLD_WIDTH = 900;
	const IDLE_AFTER_MS = 300;

	// Trackpads emit wheel/pointer events faster than the display refreshes. Coalescing them into
	// one commit per frame keeps the transform to a single DOM write per frame, and lets the next
	// event in the same frame accumulate onto the pending camera rather than the painted one.
	let pendingCamera: Camera | undefined;
	let commitFrame = 0;
	let idleTimer: ReturnType<typeof setTimeout> | undefined;

	function liveCamera() {
		return pendingCamera ?? camera;
	}

	function commitCamera(next: Camera) {
		pendingCamera = next;
		interacting = true;
		clearTimeout(idleTimer);
		idleTimer = setTimeout(() => (interacting = false), IDLE_AFTER_MS);

		if (commitFrame) return;
		commitFrame = requestAnimationFrame(() => {
			commitFrame = 0;
			if (pendingCamera) camera = pendingCamera;
			pendingCamera = undefined;
		});
	}

	let worldHeight = $derived(
		item ? (measuredHeights[item.id] ?? estimateHeight(item, expanded)) : 0
	);
	let measuredWidth = $state(0);
	let worldWidth = $derived(measuredWidth || ESTIMATED_WORLD_WIDTH);

	let rawBand = $derived(viewBand(camera, viewportSize));
	let bandTop = $derived(rawBand.top);
	let bandBottom = $derived(rawBand.bottom);

	function isCanvasBackground(target: EventTarget | null) {
		if (!(target instanceof Element)) return false;
		if (!target.closest('[data-vmcp-canvas]')) return false;
		return !target.closest('[data-vmcp-node], [data-vmcp-ui]');
	}

	function viewportPoint(event: { clientX: number; clientY: number }) {
		const rect = viewportEl?.getBoundingClientRect();
		if (!rect) return { x: 0, y: 0 };
		return { x: event.clientX - rect.left, y: event.clientY - rect.top };
	}

	function moveCamera(next: Camera) {
		cameraMovedByUser = true;
		commitCamera(next);
	}

	function applyFit() {
		if (!viewportEl) return;
		moveCamera(fitCamera(viewportSize, { width: worldWidth, height: Math.max(1, worldHeight) }));
	}

	function zoomToward(viewportX: number, viewportY: number, factor: number) {
		const current = liveCamera();
		moveCamera(zoomAt(current, viewportX, viewportY, current.zoom * factor));
	}

	function zoomFromButton(factor: number) {
		zoomToward(viewportSize.width / 2, viewportSize.height / 2, factor);
	}

	function onWheel(event: WheelEvent) {
		if (dragActive) {
			if (event.ctrlKey || event.metaKey) event.preventDefault();
			return;
		}
		event.preventDefault();
		const point = viewportPoint(event);
		if (event.ctrlKey || event.metaKey) {
			zoomToward(point.x, point.y, wheelZoomFactor(event.deltaY, event.deltaMode));
			return;
		}
		moveCamera(panBy(liveCamera(), -event.deltaX, -event.deltaY));
	}

	function onPointerDown(event: PointerEvent) {
		if (dragActive || event.button !== 0) return;
		if (!isCanvasBackground(event.target)) return;
		panPointer = { id: event.pointerId, x: event.clientX, y: event.clientY };
		viewportEl?.setPointerCapture(event.pointerId);
	}

	function onPointerMove(event: PointerEvent) {
		if (!panPointer || event.pointerId !== panPointer.id) return;
		moveCamera(panBy(liveCamera(), event.clientX - panPointer.x, event.clientY - panPointer.y));
		panPointer = { ...panPointer, x: event.clientX, y: event.clientY };
	}

	function onPointerUp(event: PointerEvent) {
		if (!panPointer || event.pointerId !== panPointer.id) return;
		panPointer = undefined;
	}

	$effect(() => {
		const el = viewportEl;
		if (!el) return;
		const observer = new ResizeObserver(() => {
			const rect = el.getBoundingClientRect();
			viewportSize = { width: rect.width, height: rect.height };
		});
		observer.observe(el);
		void dragActive;
		const handleWheel = (event: WheelEvent) => onWheel(event);
		el.addEventListener('wheel', handleWheel, { passive: false });
		return () => {
			observer.disconnect();
			el.removeEventListener('wheel', handleWheel);
		};
	});

	$effect(() => {
		if (dragActive) panPointer = undefined;
	});

	$effect(() => {
		return () => {
			if (commitFrame) cancelAnimationFrame(commitFrame);
			clearTimeout(idleTimer);
		};
	});

	$effect(() => {
		const id = item?.id;
		if (!id) return;
		const viewport = { width: viewportSize.width, height: viewportSize.height };
		const world = { width: worldWidth, height: worldHeight };

		untrack(() => {
			if (centeredId !== id) {
				centeredId = id;
				cameraMovedByUser = false;
			}
			if (cameraMovedByUser) return;
			if (viewport.width < 64 || viewport.height < 64 || world.height < 10) return;
			commitCamera(defaultCamera(viewport, world));
		});
	});

	function bindRow(id: string) {
		return (node: HTMLElement) => {
			const observer = new ResizeObserver(() => {
				const next = node.offsetHeight;
				if (measuredHeights[id] !== next) {
					measuredHeights = { ...measuredHeights, [id]: next };
				}
				const width = node.offsetWidth;
				if (width > 0 && width !== measuredWidth) measuredWidth = width;
			});
			observer.observe(node);
			return () => observer.disconnect();
		};
	}
</script>

<div
	bind:this={viewportEl}
	data-vmcp-canvas
	role="application"
	aria-label="vMCP canvas"
	class="h-full min-h-0 w-full touch-none overflow-hidden {panPointer
		? 'cursor-grabbing'
		: 'cursor-grab'}"
	onpointerdown={onPointerDown}
	onpointermove={onPointerMove}
	onpointerup={onPointerUp}
	onpointercancel={onPointerUp}
>
	<div class="absolute right-3 top-3 z-20 flex items-center gap-2">
		<div
			class="bg-base-100/80 dark:bg-base-300/80 flex gap-1 rounded-md border border-transparent p-1 shadow-sm"
			data-vmcp-ui
			role="toolbar"
			tabindex="-1"
			aria-label="Canvas zoom"
			onpointerdown={(event) => event.stopPropagation()}
		>
			<IconButton
				class="btn-sm"
				tooltip={{ text: 'Zoom in', placement: 'bottom' }}
				onclick={() => zoomFromButton(ZOOM_STEP)}
			>
				<Plus class="size-4" />
			</IconButton>
			<IconButton
				class="btn-sm"
				tooltip={{ text: 'Zoom out', placement: 'bottom' }}
				onclick={() => zoomFromButton(1 / ZOOM_STEP)}
			>
				<Minus class="size-4" />
			</IconButton>
			<IconButton
				class="btn-sm"
				tooltip={{ text: 'Fit to view', placement: 'bottom' }}
				onclick={applyFit}
			>
				<Maximize2 class="size-4" />
			</IconButton>
		</div>
		{#if actions && item}
			<div
				class="bg-base-100/80 dark:bg-base-300/80 flex gap-1 rounded-md border border-transparent p-1 shadow-sm"
				data-vmcp-ui
				role="toolbar"
				tabindex="-1"
				aria-label="vMCP actions"
				onpointerdown={(event) => event.stopPropagation()}
			>
				{@render actions()}
			</div>
		{/if}
	</div>

	{#if item}
		<div
			data-vmcp-world
			class="relative origin-top-left"
			style:will-change={interacting ? 'transform' : null}
			style:width="{worldWidth}px"
			style:height="{worldHeight}px"
			style:transform="translate({camera.x}px, {camera.y}px) scale({camera.zoom})"
			aria-label="vMCP"
		>
			<div
				data-vmcp-node
				class="absolute top-0 left-0 w-max cursor-auto"
				{@attach bindRow(item.id)}
			>
				{@render row(item, {
					rowY: 0,
					viewTop: bandTop,
					viewBottom: bandBottom
				})}
			</div>
		</div>
	{:else if empty}
		<div data-vmcp-node class="flex h-full cursor-auto items-center justify-center">
			{@render empty()}
		</div>
	{/if}
</div>
