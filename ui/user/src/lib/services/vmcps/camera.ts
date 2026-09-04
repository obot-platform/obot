import {
	MAX_ZOOM,
	MIN_ZOOM,
	VMCP_CARD_HEIGHT,
	VMCP_COLLAPSED_ROW_HEIGHT,
	VMCP_COMPONENT_HEIGHT,
	VMCP_OVERSCAN_ROWS
} from './constants';
import type { Camera, WorldRect } from './types';

export function clampZoom(zoom: number) {
	return Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, zoom));
}

export function zoomAt(
	camera: Camera,
	viewportX: number,
	viewportY: number,
	nextZoom: number
): Camera {
	const zoom = clampZoom(nextZoom);
	const worldX = (viewportX - camera.x) / camera.zoom;
	const worldY = (viewportY - camera.y) / camera.zoom;
	return {
		zoom,
		x: viewportX - worldX * zoom,
		y: viewportY - worldY * zoom
	};
}

export function panBy(camera: Camera, dx: number, dy: number): Camera {
	return { ...camera, x: camera.x + dx, y: camera.y + dy };
}

export function fitCamera(
	viewport: { width: number; height: number },
	world: { width: number; height: number },
	padding = 32
): Camera {
	const availableWidth = Math.max(1, viewport.width - padding * 2);
	const availableHeight = Math.max(1, viewport.height - padding * 2);
	const zoom = clampZoom(
		Math.min(
			1,
			availableWidth / Math.max(1, world.width),
			availableHeight / Math.max(1, world.height)
		)
	);
	return {
		zoom,
		x: (viewport.width - world.width * zoom) / 2,
		y: (viewport.height - world.height * zoom) / 2
	};
}

/**
 * Where the canvas opens: actual size, centered in the viewport. A world too large to fit anchors
 * at `padding` on that axis, so its top-left corner stays in view rather than going off-screen.
 */
export function defaultCamera(
	viewport: { width: number; height: number },
	world: { width: number; height: number },
	padding = 32
): Camera {
	return {
		zoom: 1,
		x: Math.max(padding, (viewport.width - world.width) / 2),
		y: Math.max(padding, (viewport.height - world.height) / 2)
	};
}

export function viewportToWorld(camera: Camera, viewportX: number, viewportY: number) {
	return {
		x: (viewportX - camera.x) / camera.zoom,
		y: (viewportY - camera.y) / camera.zoom
	};
}

export const VIEW_BAND_PX = 240;

export function visibleWorldRect(
	camera: Camera,
	viewport: { width: number; height: number }
): WorldRect {
	const topLeft = viewportToWorld(camera, 0, 0);
	const bottomRight = viewportToWorld(camera, viewport.width, viewport.height);
	return {
		left: topLeft.x,
		top: topLeft.y,
		right: bottomRight.x,
		bottom: bottomRight.y
	};
}

export function viewBand(
	camera: Camera,
	viewport: { width: number; height: number },
	step = VIEW_BAND_PX
) {
	const view = visibleWorldRect(camera, viewport);
	return {
		top: Math.floor(view.top / step) * step,
		bottom: Math.ceil(view.bottom / step) * step
	};
}

export function windowRange({
	viewTop,
	viewBottom,
	originY,
	count,
	itemHeight,
	overscan = VMCP_OVERSCAN_ROWS
}: {
	viewTop: number;
	viewBottom: number;
	originY: number;
	count: number;
	itemHeight: number;
	overscan?: number;
}) {
	if (count <= 0 || itemHeight <= 0) return { start: 0, end: 0 };
	const start = Math.max(0, Math.floor((viewTop - originY) / itemHeight) - overscan);
	const end = Math.min(count, Math.ceil((viewBottom - originY) / itemHeight) + overscan);
	if (start >= count) return { start: 0, end: 0 };
	return { start, end: Math.max(start, end) };
}

export function vmcpRowHeight(componentCount: number, expanded: boolean) {
	if (!expanded) return VMCP_COLLAPSED_ROW_HEIGHT;
	const stack = Math.max(1, componentCount) * VMCP_COMPONENT_HEIGHT;
	return Math.max(VMCP_CARD_HEIGHT, stack);
}

export function wheelZoomFactor(deltaY: number, deltaMode: number) {
	const pixels = deltaMode === 1 ? deltaY * 16 : deltaY;
	return Math.exp(-pixels * 0.0015);
}
