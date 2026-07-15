import type { GuideHighlight } from '$lib/services/guides';
import { darkMode } from '$lib/stores';
import Obot from './Obot.svelte';
import { type Config, type Driver, driver } from 'driver.js';
import 'driver.js/dist/driver.css';
import { mount, tick, unmount } from 'svelte';

const ELEMENT_FIND_ATTEMPTS = 10;
const ELEMENT_STABLE_ATTEMPTS = 60;

export interface GuideHighlighterOptions {
	allowClose?: boolean;
	overlayClickBehavior?: Config['overlayClickBehavior'];
	onDestroyed?: () => void;
	onCloseClick?: () => void;
	/** Called when the highlight Obot sprite is shown/hidden so callers can hide their own Obot. */
	onObotVisibilityChange?: (visible: boolean) => void;
}

export interface GuideHighlighter {
	highlight(highlight: GuideHighlight): Promise<void>;
	refresh(): void;
	destroy(): void;
	setOverlayColor(color: string): void;
	getDriver(): Driver;
}

function findElementByIdPrefix(prefix: string): Element | undefined {
	return document.querySelector(`[id^="${prefix}"]`) ?? undefined;
}

function findElement(highlight: GuideHighlight): Element | undefined {
	if (highlight.selector.id) {
		const element = document.getElementById(highlight.selector.id);
		if (element) return element;
	}

	if (highlight.selector.beginsWith) {
		for (const beginsWith of highlight.selector.beginsWith) {
			const match = findElementByIdPrefix(beginsWith);
			if (match) return match;
		}
	}

	return undefined;
}

async function waitForExistingElement(
	highlight: GuideHighlight,
	attempts = ELEMENT_FIND_ATTEMPTS
): Promise<Element | undefined> {
	for (let i = 0; i < attempts; i++) {
		const element = findElement(highlight);
		if (element) {
			return element;
		}

		await tick();
		await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
	}
	return undefined;
}

async function waitForStableElement(
	el: Element,
	stableFrames = 3,
	maxAttempts = ELEMENT_STABLE_ATTEMPTS
): Promise<boolean> {
	let last = { top: -1, left: -1, width: -1, height: -1 };
	let stable = 0;

	for (let attempt = 0; attempt < maxAttempts; attempt++) {
		await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
		const { top, left, width, height } = el.getBoundingClientRect();
		if (
			width > 0 &&
			height > 0 &&
			Math.abs(top - last.top) < 1 &&
			Math.abs(left - last.left) < 1 &&
			Math.abs(width - last.width) < 1 &&
			Math.abs(height - last.height) < 1
		) {
			stable++;
			if (stable >= stableFrames) return true;
		} else {
			stable = 0;
		}
		last = { top, left, width, height };
	}

	return false;
}

function getHighlightLayerNodes(ring?: HTMLElement | null): Element[] {
	return [
		document.querySelector('.driver-overlay'),
		document.querySelector('.driver-popover'),
		ring ?? null
	].filter((node): node is Element => node != null);
}

/** Modal <dialog> lives in the top layer; body-level nodes render underneath it. */
function getHighlightLayerHost(element: Element): HTMLElement {
	const dialog = element.closest('dialog');
	if (dialog instanceof HTMLDialogElement && dialog.open) {
		return dialog;
	}
	return document.body;
}

function moveHighlightLayer(nodes: Element[], host: HTMLElement) {
	for (const node of nodes) {
		if (node.parentElement !== host) {
			host.appendChild(node);
		}
	}
}

export function createGuideHighlighter(options: GuideHighlighterOptions = {}): GuideHighlighter {
	let highlightObot: ReturnType<typeof mount> | undefined;
	let highlightObotContainer: HTMLElement | undefined;
	let highlightRing: HTMLElement | undefined;
	let highlightRingTarget: Element | undefined;
	let highlightRingRaf = 0;

	function destroyHighlightObot() {
		if (highlightObot) {
			void unmount(highlightObot);
			highlightObot = undefined;
		}
		highlightObotContainer?.remove();
		highlightObotContainer = undefined;
	}

	function destroyHighlightRing() {
		if (highlightRingRaf) {
			cancelAnimationFrame(highlightRingRaf);
			highlightRingRaf = 0;
		}
		highlightRing?.remove();
		highlightRing = undefined;
		highlightRingTarget = undefined;
	}

	function destroyHighlightEffects() {
		destroyHighlightObot();
		destroyHighlightRing();
		options.onObotVisibilityChange?.(true);
		options.onDestroyed?.();
	}

	function positionHighlightRing() {
		if (!highlightRing || !highlightRingTarget) return;
		const rect = highlightRingTarget.getBoundingClientRect();
		const pad = 4;
		highlightRing.style.top = `${rect.top - pad}px`;
		highlightRing.style.left = `${rect.left - pad}px`;
		highlightRing.style.width = `${rect.width + pad * 2}px`;
		highlightRing.style.height = `${rect.height + pad * 2}px`;
		highlightRingRaf = requestAnimationFrame(positionHighlightRing);
	}

	function showHighlightRing(element: Element) {
		destroyHighlightRing();
		highlightRingTarget = element;
		const ring = document.createElement('div');
		ring.className = 'guide-highlight-ring';
		ring.setAttribute('aria-hidden', 'true');
		document.body.appendChild(ring);
		highlightRing = ring;
		positionHighlightRing();
	}

	/** driver.js recreates the popover via document.body.removeChild — keep nodes there first. */
	function restoreHighlightLayerToBody() {
		moveHighlightLayer(getHighlightLayerNodes(highlightRing), document.body);
	}

	function promoteHighlightLayer(element: Element) {
		moveHighlightLayer(getHighlightLayerNodes(highlightRing), getHighlightLayerHost(element));
	}

	const guideDriver = driver({
		showProgress: false,
		// Skip stage animation so overlay/popover exist before we reparent into an open dialog.
		animate: false,
		allowClose: options.allowClose ?? true,
		overlayClickBehavior: options.overlayClickBehavior ?? 'close',
		overlayColor: darkMode.isDark ? 'rgba(0, 0, 0, 1)' : 'rgba(0, 0, 0, 0.35)',
		stagePadding: 12,
		stageRadius: 8,
		onDestroyed: destroyHighlightEffects,
		onCloseClick: options.onCloseClick,
		onHighlighted: (element) => {
			if (element) promoteHighlightLayer(element);
		}
	});

	async function highlight(highlightConfig: GuideHighlight) {
		const element = await waitForExistingElement(highlightConfig);
		if (!element) return;

		const stable = await waitForStableElement(element);
		if (!stable) return;

		const side = highlightConfig.side ?? 'right';

		restoreHighlightLayerToBody();
		guideDriver.highlight({
			element,
			popover: {
				title: highlightConfig.title ?? '',
				description: highlightConfig.description ?? '',
				side,
				align: highlightConfig.align ?? 'start',
				popoverClass: 'max-w-md! w-md! min-h-24!',
				onPopoverRender: (popover) => {
					destroyHighlightObot();

					if (side === 'left' || side === 'right') {
						options.onObotVisibilityChange?.(false);
						popover.title.style['paddingLeft'] = '82px';
						popover.description.style['paddingLeft'] = '82px';

						const container = document.createElement('div');
						container.className = 'guide-highlight-obot';
						container.style.right = 'auto';
						container.style.left = '4px';
						container.style.top = '6px';
						popover.wrapper.appendChild(container);
						highlightObotContainer = container;

						highlightObot = mount(Obot, {
							target: container,
							props: {
								animation: ['enter', 'arrow', 'arrow_idle'],
								size: 84,
								class: side === 'left' ? '-scale-x-100' : ''
							}
						});
					}
				}
			}
		});
		showHighlightRing(element);
		promoteHighlightLayer(element);
		// Overlay is created on the next animation frame when animate is false.
		requestAnimationFrame(() => promoteHighlightLayer(element));
	}

	return {
		highlight,
		refresh: () => {
			guideDriver.refresh();
			const active = guideDriver.getActiveElement();
			if (active) promoteHighlightLayer(active);
		},
		destroy: () => {
			restoreHighlightLayerToBody();
			guideDriver.destroy();
		},
		setOverlayColor: (color: string) => {
			guideDriver.setConfig({
				...guideDriver.getConfig(),
				overlayColor: color
			});
		},
		getDriver: () => guideDriver
	};
}
