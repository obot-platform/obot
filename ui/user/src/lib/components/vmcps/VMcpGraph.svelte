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
	import {
		VMCP_CREATE_HEIGHT,
		VMCP_OVERSCAN_ROWS,
		VMCP_ROW_GAP,
		ZOOM_STEP
	} from '$lib/services/vmcps/constants';
	import type { Camera, RowContext } from '$lib/services/vmcps/types';
	import { Maximize2, Minus, Plus } from '@lucide/svelte';
	import type { Snippet } from 'svelte';

	let {
		items,
		actions,
		expandedIds = [],
		dragActive = false,
		estimateHeight,
		row,
		footer
	}: {
		items: T[];
		actions?: Snippet;
		expandedIds?: string[];
		dragActive?: boolean;
		estimateHeight: (item: T, expanded: boolean) => number;
		row: Snippet<[T, RowContext]>;
		footer?: Snippet;
	} = $props();

	let viewportEl = $state<HTMLElement>();
	let camera = $state<Camera>({ x: 0, y: 0, zoom: 1 });
	let viewportSize = $state({ width: 1, height: 1 });
	let measuredHeights = $state<Record<string, number>>({});
	let measuredFooterHeight = $state(0);
	let didFit = $state(false);
	let panPointer = $state<{ id: number; x: number; y: number }>();
	let interacting = $state(false);

	const WORLD_MIN_WIDTH = 900;
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

	function heightFor(item: T) {
		return measuredHeights[item.id] ?? estimateHeight(item, expandedIds.includes(item.id));
	}

	let offsets = $derived.by(() => {
		const tops: number[] = [];
		let y = 0;
		for (const item of items) {
			tops.push(y);
			y += heightFor(item) + VMCP_ROW_GAP;
		}
		return { tops, contentHeight: y };
	});

	let footerHeight = $derived(measuredFooterHeight || (footer ? VMCP_CREATE_HEIGHT : 0));
	let worldHeight = $derived(offsets.contentHeight + footerHeight);
	let measuredWidth = $state(WORLD_MIN_WIDTH);
	let worldWidth = $derived(Math.max(WORLD_MIN_WIDTH, measuredWidth));

	// Split into scalars so a pan that stays inside the current band compares equal and stops
	// propagating: rows keep their slice and the frame costs one transform write.
	let rawBand = $derived(viewBand(camera, viewportSize));
	let bandTop = $derived(rawBand.top);
	let bandBottom = $derived(rawBand.bottom);

	let visible = $derived.by(() => {
		const view = { top: bandTop, bottom: bandBottom };
		const start = items.findIndex((_, index) => {
			const top = offsets.tops[index] ?? 0;
			const bottom = top + heightFor(items[index]) + VMCP_ROW_GAP;
			return bottom >= view.top;
		});
		if (start < 0) {
			return { start: 0, end: Math.min(items.length, VMCP_OVERSCAN_ROWS + 1) };
		}
		let end = start;
		while (end < items.length) {
			const top = offsets.tops[end] ?? 0;
			if (top > view.bottom) break;
			end += 1;
		}
		return {
			start: Math.max(0, start - VMCP_OVERSCAN_ROWS),
			end: Math.min(items.length, end + VMCP_OVERSCAN_ROWS)
		};
	});

	let visibleStart = $derived(visible.start);
	let visibleEnd = $derived(visible.end);
	let visibleItems = $derived(items.slice(visibleStart, visibleEnd));

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

	function applyFit() {
		if (!viewportEl) return;
		commitCamera(fitCamera(viewportSize, { width: worldWidth, height: Math.max(1, worldHeight) }));
	}

	function zoomToward(viewportX: number, viewportY: number, factor: number) {
		const current = liveCamera();
		commitCamera(zoomAt(current, viewportX, viewportY, current.zoom * factor));
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
		commitCamera(panBy(liveCamera(), -event.deltaX, -event.deltaY));
	}

	function onPointerDown(event: PointerEvent) {
		if (dragActive || event.button !== 0) return;
		if (!isCanvasBackground(event.target)) return;
		panPointer = { id: event.pointerId, x: event.clientX, y: event.clientY };
		viewportEl?.setPointerCapture(event.pointerId);
	}

	function onPointerMove(event: PointerEvent) {
		if (!panPointer || event.pointerId !== panPointer.id) return;
		commitCamera(panBy(liveCamera(), event.clientX - panPointer.x, event.clientY - panPointer.y));
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
		if (didFit || viewportSize.width < 64 || viewportSize.height < 64 || worldHeight < 10) return;
		if (items.length === 0) return;
		commitCamera(defaultCamera(viewportSize, { width: worldWidth }));
		didFit = true;
	});

	function bindRow(id: string) {
		return (node: HTMLElement) => {
			const observer = new ResizeObserver(() => {
				const next = node.offsetHeight;
				if (measuredHeights[id] !== next) {
					measuredHeights = { ...measuredHeights, [id]: next };
				}
				if (node.offsetWidth > measuredWidth) measuredWidth = node.offsetWidth;
			});
			observer.observe(node);
			return () => observer.disconnect();
		};
	}

	function bindFooter(node: HTMLElement) {
		const observer = new ResizeObserver(() => {
			measuredFooterHeight = node.offsetHeight;
			if (node.offsetWidth > measuredWidth) measuredWidth = node.offsetWidth;
		});
		observer.observe(node);
		return () => observer.disconnect();
	}
</script>

<div
	bind:this={viewportEl}
	data-vmcp-canvas
	role="application"
	aria-label="vMCP canvas"
	class="relative h-full min-h-0 w-full touch-none overflow-hidden {panPointer
		? 'cursor-grabbing'
		: 'cursor-grab'}"
	onpointerdown={onPointerDown}
	onpointermove={onPointerMove}
	onpointerup={onPointerUp}
	onpointercancel={onPointerUp}
>
	<div
		class="absolute left-0 top-22 @md:top-14 @3xl:top-0 @3xl:left-auto @3xl:right-0 z-20 flex items-center gap-4"
	>
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
		{#if actions}
			{@render actions()}
		{/if}
	</div>

	<div
		data-vmcp-world
		class="relative origin-top-left"
		style:will-change={interacting ? 'transform' : null}
		style:width="{worldWidth}px"
		style:height="{worldHeight}px"
		style:transform="translate({camera.x}px, {camera.y}px) scale({camera.zoom})"
		aria-label="vMCPs"
	>
		{#each visibleItems as item, sliceIndex (item.id)}
			{@const index = visibleStart + sliceIndex}
			{@const rowY = offsets.tops[index] ?? 0}
			<div
				data-vmcp-node
				class="absolute top-0 left-0 cursor-auto"
				style:transform="translateY({rowY}px)"
				{@attach bindRow(item.id)}
			>
				{@render row(item, {
					rowY,
					viewTop: bandTop,
					viewBottom: bandBottom
				})}
			</div>
		{/each}
		{#if footer}
			<div
				data-vmcp-node
				class="absolute top-0 left-0 cursor-auto"
				style:transform="translateY({offsets.contentHeight}px)"
				{@attach bindFooter}
			>
				{@render footer()}
			</div>
		{/if}
	</div>
</div>
