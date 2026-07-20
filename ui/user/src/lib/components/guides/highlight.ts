import type { GuideHighlight } from '$lib/services/guides';
import { darkMode } from '$lib/stores';
import Obot from './Obot.svelte';
import { type Config, type Driver, driver } from 'driver.js';
import 'driver.js/dist/driver.css';
import { mount, tick, unmount } from 'svelte';

// Route navigations need wall-clock waits; rAF-only polling finishes before the next page mounts.
const ELEMENT_FIND_ATTEMPTS = 50;
const ELEMENT_FIND_INTERVAL_MS = 100;
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
		await new Promise<void>((resolve) => setTimeout(resolve, ELEMENT_FIND_INTERVAL_MS));
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

function getOpenGuidePanel(): HTMLElement | undefined {
	return document.querySelector<HTMLElement>('.guide-panel-popover:popover-open') ?? undefined;
}

function getOpenDialogHost(element: Element): HTMLDialogElement | undefined {
	const dialog = element.closest('dialog');
	if (dialog instanceof HTMLDialogElement && dialog.open) {
		return dialog;
	}
	return undefined;
}

/**
 * Overlay stays in the dialog/body so the guide panel (top layer) remains interactive.
 * Popover may need the guide panel itself — see getHighlightPopoverHost.
 */
function getHighlightOverlayHost(element: Element): HTMLElement {
	return getOpenDialogHost(element) ?? document.body;
}

/**
 * When the guide panel is open, app dialogs use non-modal `show()` so they are not in the
 * top layer. Reparenting the tip into that dialog both clips it (`overflow: clip` + inset
 * for the panel) and leaves it under the panel's top-layer popover. Host the tip in the
 * open guide panel instead so it paints above it without covering its controls.
 */
function getHighlightPopoverHost(element: Element): HTMLElement {
	const dialog = getOpenDialogHost(element);
	const guidePanel = getOpenGuidePanel();
	if (guidePanel && dialog && !dialog.matches(':modal') && !guidePanel.contains(dialog)) {
		return guidePanel;
	}
	return dialog ?? document.body;
}

function moveHighlightLayer(nodes: Element[], host: HTMLElement) {
	for (const node of nodes) {
		if (node.parentElement !== host) {
			host.appendChild(node);
		}
	}
}

function moveHighlightLayerSplit(
	nodes: Element[],
	overlayHost: HTMLElement,
	popoverHost: HTMLElement
) {
	for (const node of nodes) {
		// Only the tip needs the guide-panel top layer. Overlay/ring stay with the dialog so
		// they cannot sit above it and steal clicks from the highlighted content.
		const host = node.classList.contains('driver-popover') ? popoverHost : overlayHost;
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
	let highlightGeneration = 0;

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
		moveHighlightLayerSplit(
			getHighlightLayerNodes(highlightRing),
			getHighlightOverlayHost(element),
			getHighlightPopoverHost(element)
		);
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
		const generation = ++highlightGeneration;
		const element = await waitForExistingElement(highlightConfig);
		if (generation !== highlightGeneration || !element) return;

		await waitForStableElement(element);
		if (generation !== highlightGeneration) return;
		// Layout may keep shifting (guide panel resize); still highlight after the wait.

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
					if (generation !== highlightGeneration) return;
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

		// Click may have destroyed the tour while we awaited; roll back if superseded.
		if (generation !== highlightGeneration) {
			guideDriver.destroy();
			destroyHighlightEffects();
			return;
		}

		showHighlightRing(element);
		promoteHighlightLayer(element);
		// Overlay is created on the next animation frame when animate is false.
		requestAnimationFrame(() => {
			if (generation !== highlightGeneration) return;
			promoteHighlightLayer(element);
		});
	}

	return {
		highlight,
		refresh: () => {
			guideDriver.refresh();
			const active = guideDriver.getActiveElement();
			if (active) promoteHighlightLayer(active);
		},
		destroy: () => {
			highlightGeneration++;
			restoreHighlightLayerToBody();
			guideDriver.destroy();
			// Driver may skip onDestroyed if it never highlighted — always clear our effects.
			destroyHighlightEffects();
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
