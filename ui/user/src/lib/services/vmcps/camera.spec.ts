import {
	clampZoom,
	defaultCamera,
	fitCamera,
	panBy,
	VIEW_BAND_PX,
	viewBand,
	viewportToWorld,
	visibleWorldRect,
	vmcpRowHeight,
	windowRange,
	zoomAt
} from '$lib/services/vmcps/camera';
import { MAX_ZOOM, MIN_ZOOM } from './constants';
import { describe, expect, it } from 'vitest';

describe('clampZoom', () => {
	it('stays within the whiteboard range', () => {
		expect(clampZoom(0.01)).toBe(MIN_ZOOM);
		expect(clampZoom(8)).toBe(MAX_ZOOM);
		expect(clampZoom(1)).toBe(1);
	});
});

describe('zoomAt', () => {
	it('keeps the world point under the cursor', () => {
		const camera = { x: 0, y: 0, zoom: 1 };
		const next = zoomAt(camera, 100, 50, 2);
		expect(viewportToWorld(next, 100, 50)).toEqual(viewportToWorld(camera, 100, 50));
	});
});

describe('panBy', () => {
	it('shifts the camera in screen pixels', () => {
		expect(panBy({ x: 10, y: 20, zoom: 1 }, 5, -3)).toEqual({ x: 15, y: 17, zoom: 1 });
	});
});

describe('fitCamera', () => {
	it('centers a smaller world at zoom 1', () => {
		const camera = fitCamera({ width: 800, height: 600 }, { width: 200, height: 100 }, 0);
		expect(camera.zoom).toBe(1);
		expect(camera.x).toBe(300);
		expect(camera.y).toBe(250);
	});

	it('shrinks to fit a world larger than the viewport', () => {
		const camera = fitCamera({ width: 400, height: 400 }, { width: 2000, height: 400 }, 0);
		expect(camera.zoom).toBe(MIN_ZOOM);
	});
});

describe('defaultCamera', () => {
	it('opens at 100% with the world centered', () => {
		const camera = defaultCamera({ width: 1200, height: 800 }, { width: 900, height: 300 }, 32);
		expect(camera.zoom).toBe(1);
		expect(camera.x).toBe(150);
		expect(camera.y).toBe(250);
	});

	it('anchors to the top left corner when the world is larger than the viewport', () => {
		const camera = defaultCamera({ width: 600, height: 200 }, { width: 900, height: 300 }, 32);
		expect(camera.zoom).toBe(1);
		expect(camera.x).toBe(32);
		expect(camera.y).toBe(32);
	});
});

describe('visibleWorldRect', () => {
	it('maps the viewport through the camera', () => {
		const rect = visibleWorldRect({ x: 10, y: 20, zoom: 2 }, { width: 100, height: 50 });
		expect(rect).toEqual({ left: -5, top: -10, right: 45, bottom: 15 });
	});
});

describe('viewBand', () => {
	it('snaps the view to whole bands so small pans do not re-window rows', () => {
		const viewport = { width: 800, height: 600 };
		const band = viewBand({ x: 0, y: 0, zoom: 1 }, viewport);

		expect(band.top % VIEW_BAND_PX).toBe(0);
		expect(band.bottom % VIEW_BAND_PX).toBe(0);
		expect(viewBand({ x: 0, y: -10, zoom: 1 }, viewport)).toEqual(band);
	});

	it('covers the visible rect', () => {
		const viewport = { width: 800, height: 600 };
		const camera = { x: 0, y: -500, zoom: 1 };
		const view = visibleWorldRect(camera, viewport);
		const band = viewBand(camera, viewport);

		expect(band.top).toBeLessThanOrEqual(view.top);
		expect(band.bottom).toBeGreaterThanOrEqual(view.bottom);
	});
});

describe('windowRange', () => {
	it('returns a slice that covers the view plus overscan', () => {
		expect(
			windowRange({
				viewTop: 200,
				viewBottom: 400,
				originY: 0,
				count: 20,
				itemHeight: 100,
				overscan: 1
			})
		).toEqual({ start: 1, end: 5 });
	});

	it('is empty when there are no items', () => {
		expect(
			windowRange({
				viewTop: 0,
				viewBottom: 100,
				originY: 0,
				count: 0,
				itemHeight: 100
			})
		).toEqual({ start: 0, end: 0 });
	});
});

describe('vmcpRowHeight', () => {
	it('uses the collapsed height until the chain is open', () => {
		expect(vmcpRowHeight(40, false)).toBeLessThan(vmcpRowHeight(40, true));
	});
});
