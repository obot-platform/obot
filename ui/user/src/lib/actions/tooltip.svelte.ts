import popover from '$lib/actions/popover.svelte';
import SnippetComponent from '$lib/components/primitives/Snippet.svelte';
import type { Placement } from '@floating-ui/dom';
import { mount, unmount, type Snippet } from 'svelte';

export interface TooltipOptions {
	text?: string;
	snippet?: Snippet;
	interactive?: boolean;
	disablePortal?: boolean;
	classes?: string[];
	placement?: Placement;
}

function resolveDataTheme(trigger: HTMLElement): string {
	return (
		trigger.closest('[data-theme]')?.getAttribute('data-theme') ??
		document.querySelector<HTMLElement>('.nanobot[data-theme]')?.getAttribute('data-theme') ??
		'nanobotlight'
	);
}

function placementAttr(placement: Placement | undefined): string {
	return placement ?? 'top';
}

export function tooltip(node: HTMLElement, opts: TooltipOptions | string | undefined) {
	let tt: ReturnType<typeof popover> | null = null;
	let portalRoot: HTMLElement | null = null;
	let isEnabled = false;
	let snippetMount: ReturnType<typeof mount> | null = null;
	let popoverTooltipParams: { update: (p?: Record<string, unknown>) => void } | null = null;

	function capturePopoverTooltipHandle(ret: unknown) {
		popoverTooltipParams =
			ret && typeof ret === 'object' && 'update' in ret
				? (ret as { update: (p?: Record<string, unknown>) => void })
				: null;
	}

	const hasText = (opts: TooltipOptions | string | undefined) => {
		if (typeof opts === 'string') return opts.trim() !== '';
		if (!opts) return false;
		if (opts.snippet) return true;
		return !!opts.text?.trim();
	};

	function applyLookClasses(el: HTMLElement, o: TooltipOptions | string | undefined) {
		el.classList.remove(
			'tooltip-portal-daisy-host',
			'tooltip-portal-daisy',
			'tooltip',
			'text-left'
		);
		const extra = typeof o === 'object' ? (o.classes ?? []) : [];
		el.classList.add('tooltip-portal-daisy-host', ...extra);
	}

	function syncDaisyPortal(
		trigger: HTMLElement,
		host: HTMLElement,
		o: TooltipOptions | string | undefined
	) {
		host.setAttribute('data-theme', resolveDataTheme(trigger));
		const placement = typeof o === 'object' ? o.placement : undefined;
		host.setAttribute('data-placement', placementAttr(placement));
	}

	function buildDaisyPortal(): HTMLElement {
		const host = document.createElement('div');
		const bubble = document.createElement('div');
		const caret = document.createElement('span');
		bubble.className = 'tooltip-portal-daisy-host__bubble';
		caret.className = 'tooltip-portal-daisy-host__caret';
		caret.setAttribute('aria-hidden', 'true');
		host.append(bubble, caret);
		return host;
	}

	const enable = (init: TooltipOptions | string | undefined) => {
		if (isEnabled) return;

		const placement =
			typeof init === 'object' && init.placement ? init.placement : ('top' as Placement);

		tt = popover({
			placement,
			delay: 0,
			strategy: 'fixed'
		});

		portalRoot = buildDaisyPortal();
		portalRoot.classList.add('opacity-0', 'tooltip-portal-daisy-host--inactive');
		applyLookClasses(portalRoot, init);
		syncDaisyPortal(node, portalRoot, init);

		if (typeof init === 'object' && init?.disablePortal) {
			node.insertAdjacentElement('afterend', portalRoot);
		} else {
			document.body.appendChild(portalRoot);
		}

		tt.ref(node);
		capturePopoverTooltipHandle(
			tt.tooltip(portalRoot, {
				hover: true,
				disablePortal: typeof init === 'object' ? init.disablePortal : false,
				interactiveHover: typeof init === 'object' ? !!init.interactive : false,
				enterTransition: 'daisy'
			})
		);

		isEnabled = true;
	};

	const clearSnippetMount = () => {
		if (snippetMount) {
			unmount(snippetMount);
			snippetMount = null;
		}
	};

	const disable = () => {
		if (!isEnabled) return;
		clearSnippetMount();
		popoverTooltipParams = null;
		portalRoot?.remove();
		portalRoot = null;
		tt = null;
		isEnabled = false;
	};

	const updateContent = (o: TooltipOptions | string | undefined) => {
		if (!portalRoot) return;
		const bubble = portalRoot.querySelector('.tooltip-portal-daisy-host__bubble');
		if (typeof o === 'object' && o?.snippet) {
			clearSnippetMount();
			const target = (bubble ?? portalRoot) as HTMLElement;
			target.replaceChildren();
			snippetMount = mount(SnippetComponent, {
				target,
				props: { children: o.snippet }
			});
			return;
		}
		clearSnippetMount();
		const text = typeof o === 'string' ? o : (o?.text ?? '');
		if (bubble) {
			bubble.textContent = text;
		} else {
			portalRoot.textContent = text;
		}
	};

	const applyOptions = (o: TooltipOptions | string | undefined) => {
		if (!hasText(o)) {
			disable();
			return;
		}

		if (!isEnabled) {
			enable(o);
			updateContent(o);
			return;
		}

		if (portalRoot) {
			syncDaisyPortal(node, portalRoot, o);
			applyLookClasses(portalRoot, o);
		}

		if (typeof o === 'object') {
			popoverTooltipParams?.update({
				disablePortal: o.disablePortal,
				interactiveHover: !!o.interactive
			});
		}

		updateContent(o);
	};

	function tooltipOptionsEqual(
		a: TooltipOptions | string | undefined,
		b: TooltipOptions | string | undefined
	): boolean {
		if (a === b) return true;
		if (typeof a === 'string' || typeof b === 'string') return a === b;
		if (!a || !b) return !a && !b;
		const ao = a as TooltipOptions;
		const bo = b as TooltipOptions;
		return (
			ao.text === bo.text &&
			ao.placement === bo.placement &&
			ao.interactive === bo.interactive &&
			ao.disablePortal === bo.disablePortal &&
			ao.snippet === bo.snippet &&
			JSON.stringify(ao.classes ?? []) === JSON.stringify(bo.classes ?? [])
		);
	}

	let lastOpts: TooltipOptions | string | undefined = opts;
	applyOptions(opts);

	return {
		update(newOpts: TooltipOptions | string | undefined) {
			if (tooltipOptionsEqual(lastOpts, newOpts)) return;
			lastOpts = newOpts;
			applyOptions(newOpts);
		},
		destroy: () => {
			disable();
		}
	};
}
