import {
	createDragAnchor,
	createDragSession,
	floatDragClone,
	scrollableAncestor
} from '$lib/services/dashboard/dragScroll';
import type { DashboardConfig, DropTarget, WidgetId } from '$lib/services/dashboard/layout';
import { moveHeader, placeWidget } from '$lib/services/dashboard/layout';
import { onDestroy } from 'svelte';

export type GridDragSubject = { kind: 'widget'; id: WidgetId } | { kind: 'header'; id: string };

export interface GridDragOptions {
	/** The saved layout a drag starts from. */
	layout: () => DashboardConfig;
	/** Saves the layout a drag ends on, when it differs from where the drag started. */
	commit: (next: DashboardConfig) => void;
}

export type GridDrag = ReturnType<typeof createGridDrag>;

/**
 * Pointer drag of widgets and headers on the dashboard grid. While a drag is held the grid renders
 * `draft`, a live preview of the drop, so cards move under the pointer; the drop commits it.
 *
 * Cards and headers are found through their data-dashboard-* attributes. Must be created during
 * component initialization.
 */
export function createGridDrag(options: GridDragOptions) {
	let dragging = $state<GridDragSubject | null>(null);
	let draft = $state<DashboardConfig | null>(null);
	let dragOrigin = $state<string | null>(null);
	let dragClone: HTMLElement | null = null;
	let dragOffsetX = 0;
	let dragOffsetY = 0;
	const previewDwellMs = 120;
	const previewTravelPx = 28;
	const cardGutterPx = 24;
	let previewTimer = 0;
	let appliedKey = '';
	let heldKey = '';
	const dragAnchor = createDragAnchor(() => dashboardScroller()?.scrollTop ?? 0);
	const drag = createDragSession({
		scroller: () => dashboardScroller(),
		onMove: () => placeDraggedAtPointer(),
		onDrop: () => finishDrag(true),
		onCancel: () => finishDrag(false)
	});

	function parsePath(value: string | undefined): number[] {
		if (!value) return [];
		return value.split('-').map((part) => Number(part));
	}

	function draggedNode(): HTMLElement | null {
		if (!dragging || typeof document === 'undefined') return null;
		const selector =
			dragging.kind === 'widget'
				? `[data-dashboard-card][data-widget-id="${CSS.escape(dragging.id)}"]`
				: `[data-dashboard-header][data-header-id="${CSS.escape(dragging.id)}"]`;
		for (const match of document.querySelectorAll(selector)) {
			if (match instanceof HTMLElement && !match.closest('[data-drag-clone]')) return match;
		}
		return null;
	}

	type Probe = { dragged: HTMLElement | null; rect: (node: HTMLElement) => DOMRect };

	function createProbe(): Probe {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity
		const rects = new Map<HTMLElement, DOMRect>();
		return {
			dragged: draggedNode(),
			rect(node) {
				let rect = rects.get(node);
				if (!rect) {
					rect = node.getBoundingClientRect();
					rects.set(node, rect);
				}
				return rect;
			}
		};
	}

	function pointInside(rect: DOMRect, x: number, y: number): boolean {
		return x >= rect.left && x <= rect.right && y >= rect.top && y <= rect.bottom;
	}

	function coversPointer(rect: DOMRect, x: number, y: number, gutter = 0): boolean {
		return x >= rect.left - gutter && x <= rect.right && y >= rect.top && y <= rect.bottom;
	}

	function smallest(nodes: HTMLElement[], probe: Probe): HTMLElement | undefined {
		let best: HTMLElement | undefined;
		let bestArea = Number.POSITIVE_INFINITY;
		for (const node of nodes) {
			const rect = probe.rect(node);
			const area = rect.width * rect.height;
			if (area < bestArea) {
				best = node;
				bestArea = area;
			}
		}
		return best;
	}

	function targetKey(action: DropTarget | null): string {
		if (!action) return '';
		const path = action.path.join('.');
		return action.kind === 'slot' ? `slot:${path}:${action.index}` : `row:${path}:${action.index}`;
	}

	type RowBand = { index: number; top: number; bottom: number; nodes: HTMLElement[] };

	function rowBands(box: HTMLElement, probe: Probe): RowBand[] {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity
		const bands = new Map<number, RowBand>();
		for (const node of box.querySelectorAll(
			'[data-dashboard-card], [data-dashboard-header], [data-dashboard-box]'
		)) {
			if (!(node instanceof HTMLElement)) continue;
			if (node.parentElement?.closest('[data-dashboard-box]') !== box) continue;
			const index = node.hasAttribute('data-dashboard-box')
				? (parsePath(node.dataset.boxPath).at(-1) ?? 0)
				: Number(node.dataset.row);
			const rect = probe.rect(node);
			const band = bands.get(index);
			if (!band) {
				bands.set(index, { index, top: rect.top, bottom: rect.bottom, nodes: [node] });
				continue;
			}
			band.top = Math.min(band.top, rect.top);
			band.bottom = Math.max(band.bottom, rect.bottom);
			band.nodes.push(node);
		}
		return [...bands.values()].sort((a, b) => a.top - b.top || a.index - b.index);
	}

	function closestNode(nodes: HTMLElement[], x: number, probe: Probe): HTMLElement | null {
		let best: HTMLElement | null = null;
		let bestDist = Number.POSITIVE_INFINITY;
		for (const node of nodes) {
			const rect = probe.rect(node);
			const dist = x < rect.left ? rect.left - x : x > rect.right ? x - rect.right : 0;
			if (dist < bestDist) {
				best = node;
				bestDist = dist;
			}
		}
		return best;
	}

	function actionFor(
		node: HTMLElement,
		x: number,
		y: number,
		kind: GridDragSubject['kind'],
		probe: Probe
	): DropTarget | null {
		const rect = probe.rect(node);
		if (node.hasAttribute('data-dashboard-card')) {
			const parent = parsePath(node.dataset.parent);
			const row = Number(node.dataset.row);
			const index = Number(node.dataset.index);
			const edgeBand = Math.min(16, rect.height * 0.12);
			const edge = y < rect.top + edgeBand || y > rect.bottom - edgeBand;
			if (kind === 'header' || edge) {
				return {
					kind: 'new-row',
					path: parent,
					index: y < rect.top + rect.height / 2 ? row : row + 1
				};
			}
			return {
				kind: 'slot',
				path: [...parent, row],
				index: x < rect.left + rect.width / 2 ? index : index + 1
			};
		}
		if (node.hasAttribute('data-dashboard-header')) {
			const parent = parsePath(node.dataset.parent);
			const row = Number(node.dataset.row);
			return {
				kind: 'new-row',
				path: parent,
				index: y < rect.top + rect.height / 2 ? row : row + 1
			};
		}
		return actionForBox(node, x, y, kind, probe);
	}

	function actionForBox(
		box: HTMLElement,
		x: number,
		y: number,
		kind: GridDragSubject['kind'],
		probe: Probe
	): DropTarget | null {
		const path = parsePath(box.dataset.boxPath);
		const bands = rowBands(box, probe);
		if (bands.length === 0) return { kind: 'new-row', path, index: 0 };
		const first = bands[0];
		const last = bands[bands.length - 1];
		if (y <= first.top) return { kind: 'new-row', path, index: first.index };
		if (y >= last.bottom) return { kind: 'new-row', path, index: last.index + 1 };
		for (let i = 0; i < bands.length; i++) {
			const band = bands[i];
			const above = i === 0 ? Number.NEGATIVE_INFINITY : (bands[i - 1].bottom + band.top) / 2;
			const below =
				i === bands.length - 1 ? Number.POSITIVE_INFINITY : (band.bottom + bands[i + 1].top) / 2;
			if (y < above || y > below) continue;
			const node = closestNode(band.nodes, x, probe);
			if (!node || node === probe.dragged) return null;
			return actionFor(node, x, y, kind, probe);
		}
		return { kind: 'new-row', path, index: last.index + 1 };
	}

	function hitNodes(): HTMLElement[] {
		return [
			...document.querySelectorAll(
				'[data-dashboard-card], [data-dashboard-header], [data-dashboard-box]'
			)
		].filter(
			(node): node is HTMLElement =>
				node instanceof HTMLElement && !node.closest('[data-drag-clone]')
		);
	}

	function actionAt(x: number, y: number, subject: GridDragSubject): DropTarget | null {
		const probe = createProbe();
		const dragged = probe.dragged;
		if (dragged && coversPointer(probe.rect(dragged), x, y, cardGutterPx)) return null;
		const nodes = hitNodes().filter((node) => node !== dragged);
		const cards = nodes.filter((node) => !node.hasAttribute('data-dashboard-box'));
		const hit = smallest(
			cards.filter((node) => pointInside(probe.rect(node), x, y)),
			probe
		);
		if (hit) return actionFor(hit, x, y, subject.kind, probe);
		const beside = smallest(
			cards.filter((node) => coversPointer(probe.rect(node), x, y, cardGutterPx)),
			probe
		);
		if (beside) return actionFor(beside, x, y, subject.kind, probe);
		const box = smallest(
			nodes.filter(
				(node) => node.hasAttribute('data-dashboard-box') && pointInside(probe.rect(node), x, y)
			),
			probe
		);
		return box ? actionForBox(box, x, y, subject.kind, probe) : null;
	}

	function clearDragClone() {
		dragClone?.remove();
		dragClone = null;
	}

	function mountDragClone(event: PointerEvent) {
		if (typeof document === 'undefined') return;
		const current = event.currentTarget;
		const source =
			current instanceof Element
				? current.closest('[data-dashboard-card], [data-dashboard-header]')
				: null;
		if (!(source instanceof HTMLElement)) return;
		const heading = source.querySelector('h2, h3, h4, h5, h6');
		const anchor = heading instanceof HTMLElement ? heading : source;
		const rect = anchor.getBoundingClientRect();
		const clone = document.createElement('div');

		clearDragClone();
		clone.textContent = anchor.textContent?.trim() ?? '';
		clone.className = source.hasAttribute('data-dashboard-header')
			? 'bg-base-100 dark:bg-base-300 border-base-300 rounded-md border px-3 py-2 text-lg font-semibold shadow-xl'
			: 'bg-base-100 dark:bg-base-300 border-base-300 rounded-md border px-3 py-2 text-xs font-medium tracking-wider uppercase shadow-xl';
		floatDragClone(clone, { zIndex: '70', maxWidth: '420px', opacity: '0.95' });
		document.body.appendChild(clone);
		const cloneRect = clone.getBoundingClientRect();
		dragOffsetX = Math.min(
			Math.max(event.clientX - rect.left, 0),
			Math.max(cloneRect.width - 8, 0)
		);
		dragOffsetY = Math.min(
			Math.max(event.clientY - rect.top, 0),
			Math.max(cloneRect.height - 8, 0)
		);
		dragClone = clone;
		moveDragClone(event.clientX, event.clientY);
	}

	function moveDragClone(x: number, y: number) {
		if (!dragClone) return;
		dragClone.style.left = `${x - dragOffsetX}px`;
		dragClone.style.top = `${y - dragOffsetY}px`;
	}

	function dashboardScroller(): HTMLElement | null {
		if (typeof document === 'undefined') return null;
		const anchor = document.querySelector(
			'[data-dashboard-card], [data-dashboard-header], [data-dashboard-box]'
		);
		return scrollableAncestor(anchor?.parentElement);
	}

	function travelFromAnchor(): number {
		return dragAnchor.travel(drag.x, drag.y);
	}

	function rememberAnchor() {
		dragAnchor.remember(drag.x, drag.y);
	}

	function clearPreviewTimer() {
		if (!previewTimer || typeof clearTimeout === 'undefined') return;
		clearTimeout(previewTimer);
		previewTimer = 0;
	}

	function applyPreview(action: DropTarget) {
		if (!dragging) return;
		const layout = draft ?? options.layout();
		const next = moveSubject(layout, dragging, action);
		if (JSON.stringify(next.rows) !== JSON.stringify(layout.rows)) draft = next;
		appliedKey = targetKey(action);
		heldKey = appliedKey;
		rememberAnchor();
	}

	function placeDraggedAtPointer() {
		if (!dragging) return;
		moveDragClone(drag.x, drag.y);
		const action = actionAt(drag.x, drag.y, dragging);
		const key = targetKey(action);
		if (!action || key === appliedKey) {
			clearPreviewTimer();
			heldKey = key;
			return;
		}
		if (travelFromAnchor() < previewTravelPx) {
			clearPreviewTimer();
			return;
		}
		if (key === heldKey) return;
		heldKey = key;
		clearPreviewTimer();
		previewTimer = window.setTimeout(() => {
			previewTimer = 0;
			if (!dragging || heldKey !== key || travelFromAnchor() < previewTravelPx) return;
			const latest = actionAt(drag.x, drag.y, dragging);
			if (!latest || targetKey(latest) !== key) return;
			applyPreview(latest);
		}, previewDwellMs);
	}

	function commitPreview() {
		clearPreviewTimer();
		if (!dragging || travelFromAnchor() < previewTravelPx) return;
		const action = actionAt(drag.x, drag.y, dragging);
		if (!action || targetKey(action) === appliedKey) return;
		applyPreview(action);
	}

	function moveSubject(layout: DashboardConfig, subject: GridDragSubject, action: DropTarget) {
		if (subject.kind === 'widget') return placeWidget(layout, subject.id, action);
		if (action.kind !== 'new-row') return layout;
		return moveHeader(layout, subject.id, action.index, action.path);
	}

	function detachDragListeners() {
		drag.stop();
		clearPreviewTimer();
		clearDragClone();
	}

	function finishDrag(commit: boolean) {
		const scroller = dashboardScroller();
		const scrollTop = scroller?.scrollTop ?? 0;
		if (commit) commitPreview();
		if (!dragging && !draft) {
			detachDragListeners();
			return;
		}
		const next = draft;
		const origin = dragOrigin;
		dragging = null;
		draft = null;
		dragOrigin = null;
		appliedKey = '';
		heldKey = '';
		detachDragListeners();
		if (commit && next && origin && JSON.stringify(next.rows) !== origin) options.commit(next);
		if (!scroller) return;
		const restore = () => {
			scroller.scrollTop = scrollTop;
		};
		restore();
		if (typeof requestAnimationFrame === 'undefined') return;
		requestAnimationFrame(() => {
			restore();
			requestAnimationFrame(restore);
		});
	}

	function beginDrag(event: PointerEvent, subject: GridDragSubject) {
		if (event.button !== 0) return;
		event.preventDefault();
		event.stopPropagation();
		mountDragClone(event);
		dragging = subject;
		dragOrigin = JSON.stringify((draft ?? options.layout()).rows);
		draft = null;
		appliedKey = '';
		heldKey = '';
		drag.start(event.clientX, event.clientY);
		rememberAnchor();
	}

	onDestroy(detachDragListeners);

	return {
		get dragging() {
			return dragging;
		},
		get draft() {
			return draft;
		},
		/** Keeps an in-progress drag on a layout changed from outside it (a widget's own menu). */
		follow(next: DashboardConfig) {
			if (dragging) draft = next;
		},
		begin: beginDrag
	};
}
