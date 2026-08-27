import type { MCPCatalogEntry } from '$lib/services';
import { borderAnchor, buildWirePath, distanceToRect } from '../../services/vmcps/utils';
import type { Action } from 'svelte/action';
import type { Attachment } from 'svelte/attachments';

export const CREATE_VMCP_DROP_ID = '__create__';

/** Pointer travel that turns a press on a server card into a drag rather than a click. */
const DRAG_ACTIVATE_DISTANCE_PX = 6;
/** How near a drag has to come to a drop target before it links to it. */
const VMCP_LINK_DISTANCE_PX = 220;

type ComponentTarget = { vmcpId: string; key: string };

type DropTarget = {
	vmcpId: string;
	el: HTMLElement;
	isComponentsPanel: boolean;
	componentKey?: string;
};

export interface EntryDragOptions {
	composites: () => MCPCatalogEntry[];
	panelEl: () => HTMLElement | undefined;
	openEntry: (entry: MCPCatalogEntry) => void;
	createEntry: (target?: { vmcp?: MCPCatalogEntry }) => void;
	dropOnCreate: (entry: MCPCatalogEntry) => void;
	dropOnVMcp: (entry: MCPCatalogEntry, vmcp: MCPCatalogEntry) => void;
}

/**
 * Drag state shared by the servers panel that starts a drag and the canvas that receives it.
 *
 * The panel owns the drag sources, the canvas owns the drop targets, and neither can resolve a
 * drop on its own, so both read the same instance of this rather than passing handlers across the
 * layout. Must be created during component initialization.
 */
export function createEntryDrag(options: EntryDragOptions) {
	let drag = $state<{
		entry?: MCPCatalogEntry;
		pointerId: number;
		x: number;
		y: number;
		active: boolean;
	}>();
	let dragOrigin = { x: 0, y: 0 };
	let linkedVMcpId = $state<string>();
	let linkedViaComponentsPanel = $state(false);
	let linkedComponentKey = $state<string>();
	let wavePhase = $state(0);

	let createEl = $state<HTMLElement>();
	const vmcpEls = $state<Record<string, HTMLElement>>({});
	const componentEls = $state<Record<string, { target: ComponentTarget; el: HTMLElement }>>({});

	const linkedVMcp = $derived(options.composites().find((vmcp) => vmcp.id === linkedVMcpId));

	const wire = $derived.by(() => {
		if (!drag?.active || !linkedVMcpId) return undefined;
		const target = linkedVMcpId === CREATE_VMCP_DROP_ID ? createEl : vmcpEls[linkedVMcpId];
		if (!target) return undefined;

		const to = { x: drag.x, y: drag.y };
		const from = borderAnchor(target.getBoundingClientRect(), to);
		return { from, to, path: buildWirePath(from, to, wavePhase) };
	});

	// Depends only on whether a link exists, so the loop is not torn down every frame.
	$effect(() => {
		if (!drag?.active || !linkedVMcpId) return;
		if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return;

		let frame = requestAnimationFrame(function tick(time) {
			wavePhase = time / 750;
			frame = requestAnimationFrame(tick);
		});
		return () => cancelAnimationFrame(frame);
	});

	function isOutsidePanel(x: number, y: number) {
		const rect = options.panelEl()?.getBoundingClientRect();
		if (!rect) return true;
		return x < rect.left || x > rect.right || y < rect.top || y > rect.bottom;
	}

	function dropTargets() {
		const targets: DropTarget[] = [];
		if (createEl) {
			targets.push({ vmcpId: CREATE_VMCP_DROP_ID, el: createEl, isComponentsPanel: false });
		}
		for (const vmcp of options.composites()) {
			const card = vmcpEls[vmcp.id];
			if (card) targets.push({ vmcpId: vmcp.id, el: card, isComponentsPanel: false });
		}
		for (const { target, el } of Object.values(componentEls)) {
			targets.push({
				vmcpId: target.vmcpId,
				el,
				isComponentsPanel: true,
				componentKey: target.key
			});
		}
		return targets;
	}

	function updateLinkTarget() {
		if (!drag?.active || !isOutsidePanel(drag.x, drag.y)) {
			clearLinkTarget();
			return;
		}

		let closest: { vmcpId: string; isComponentsPanel: boolean; componentKey?: string } | undefined;
		let closestDistance = VMCP_LINK_DISTANCE_PX;
		for (const target of dropTargets()) {
			const distance = distanceToRect(target.el.getBoundingClientRect(), drag);
			if (distance <= closestDistance) {
				closestDistance = distance;
				closest = target;
			}
		}
		linkedVMcpId = closest?.vmcpId;
		linkedViaComponentsPanel = closest?.isComponentsPanel ?? false;
		linkedComponentKey = closest?.componentKey;
	}

	function clearLinkTarget() {
		linkedVMcpId = undefined;
		linkedViaComponentsPanel = false;
		linkedComponentKey = undefined;
	}

	function cancel() {
		drag = undefined;
		clearLinkTarget();
	}

	function activate(entry?: MCPCatalogEntry) {
		if (entry) {
			options.openEntry(entry);
		} else {
			options.createEntry();
		}
	}

	function pointerDown(event: PointerEvent, entry?: MCPCatalogEntry) {
		if (event.button !== 0) return;

		const card = event.currentTarget as HTMLButtonElement;
		card.setPointerCapture(event.pointerId);
		dragOrigin = { x: event.clientX, y: event.clientY };
		drag = { entry, pointerId: event.pointerId, x: event.clientX, y: event.clientY, active: false };
	}

	function pointerMove(event: PointerEvent) {
		if (!drag || event.pointerId !== drag.pointerId) return;

		drag.x = event.clientX;
		drag.y = event.clientY;
		if (!drag.active) {
			const travelled = Math.hypot(drag.x - dragOrigin.x, drag.y - dragOrigin.y);
			if (travelled < DRAG_ACTIVATE_DISTANCE_PX) return;
			drag.active = true;
		}

		event.preventDefault();
		updateLinkTarget();
	}

	function pointerUp(event: PointerEvent) {
		if (!drag || event.pointerId !== drag.pointerId) return;

		const dropped = drag;
		const vmcp = linkedVMcp;
		const dropOnCreate = linkedVMcpId === CREATE_VMCP_DROP_ID;
		cancel();

		if (!dropped.active) {
			activate(dropped.entry);
			return;
		}

		if (!dropped.entry) {
			if (!dropOnCreate && !vmcp) return;
			options.createEntry({ vmcp: dropOnCreate ? undefined : vmcp });
			return;
		}

		if (dropOnCreate) {
			options.dropOnCreate(dropped.entry);
			return;
		}

		if (vmcp) {
			options.dropOnVMcp(dropped.entry, vmcp);
		}
	}

	const createTarget: Action<HTMLElement> = (node) => {
		createEl = node;
		return {
			destroy() {
				if (createEl === node) createEl = undefined;
			}
		};
	};

	const vmcpTarget: Action<HTMLElement, string> = (node, vmcpId) => {
		let current = vmcpId;
		vmcpEls[current] = node;
		return {
			update(next: string) {
				if (next === current) return;
				if (vmcpEls[current] === node) delete vmcpEls[current];
				current = next;
				vmcpEls[current] = node;
			},
			destroy() {
				if (vmcpEls[current] === node) delete vmcpEls[current];
			}
		};
	};

	/**
	 * Attachment form of {@link vmcpTarget}, for drop targets a parent can only reach through a
	 * prop rather than a directive, such as a row rendered by a shared table component.
	 */
	const vmcpTargetAttachment =
		(vmcpId: string): Attachment<HTMLElement> =>
		(node) => {
			const registered = vmcpTarget(node, vmcpId);
			return () => registered?.destroy?.();
		};

	const componentTarget: Action<HTMLElement, ComponentTarget> = (node, target) => {
		let current = `${target.vmcpId}::${target.key}`;
		componentEls[current] = { target, el: node };
		return {
			update(next: ComponentTarget) {
				const key = `${next.vmcpId}::${next.key}`;
				if (key === current) return;
				if (componentEls[current]?.el === node) delete componentEls[current];
				current = key;
				componentEls[current] = { target: next, el: node };
			},
			destroy() {
				if (componentEls[current]?.el === node) delete componentEls[current];
			}
		};
	};

	return {
		/** A drag that has travelled far enough to be more than a click. */
		get active() {
			return drag?.active === true;
		},
		/** A pointer is held on a drag source, whether or not it has travelled yet. */
		get started() {
			return drag !== undefined;
		},
		get entry() {
			return drag?.entry;
		},
		get x() {
			return drag?.x ?? 0;
		},
		get y() {
			return drag?.y ?? 0;
		},
		get wire() {
			return wire;
		},
		isLinked(vmcpId: string) {
			return linkedVMcpId === vmcpId;
		},
		isComponentLinked(vmcpId: string, componentKey: string) {
			return (
				linkedVMcpId === vmcpId && linkedViaComponentsPanel && linkedComponentKey === componentKey
			);
		},
		isDragging(entry: MCPCatalogEntry) {
			return drag?.active === true && drag.entry?.id === entry.id;
		},
		get isDraggingNewEntry() {
			return drag?.active === true && !drag.entry;
		},
		activate,
		cancel,
		pointerDown,
		pointerMove,
		pointerUp,
		createTarget,
		vmcpTarget,
		vmcpTargetAttachment,
		componentTarget
	};
}

export type EntryDrag = ReturnType<typeof createEntryDrag>;
