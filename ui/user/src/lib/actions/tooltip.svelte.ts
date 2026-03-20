import popover from '$lib/actions/popover.svelte';
import type { Placement } from '@floating-ui/dom';

const DEFAULT_LAYOUT_CLASSES = ['max-w-64', 'break-words', 'whitespace-pre-wrap'] as const;

export type TooltipVariant = 'default' | 'daisy';

export interface TooltipOptions {
	text: string;
	disablePortal?: boolean;
	classes?: string[];
	placement?: Placement;
	/** `daisy`: same surface, typography, and tail as DaisyUI `tooltip` + `data-tip` (neutral). */
	variant?: TooltipVariant;
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

function isDaisyVariant(o: TooltipOptions | string | undefined): boolean {
	return typeof o === 'object' && o.variant === 'daisy';
}

export function tooltip(node: HTMLElement, opts: TooltipOptions | string | undefined) {
	let tt: ReturnType<typeof popover> | null = null;
	let portalRoot: HTMLElement | null = null;
	let isEnabled = false;

	const hasText = (opts: TooltipOptions | string | undefined) => {
		return typeof opts === 'string' ? opts.trim() !== '' : !!opts?.text?.trim();
	};

	function applyLookClasses(el: HTMLElement, o: TooltipOptions | string | undefined) {
		el.classList.remove(
			'tooltip-portal-daisy-host',
			'tooltip-portal-daisy',
			'tooltip',
			'text-left',
			...DEFAULT_LAYOUT_CLASSES
		);
		if (typeof o === 'string') {
			el.classList.add('tooltip', 'text-left', ...DEFAULT_LAYOUT_CLASSES);
			return;
		}
		if (!o) return;
		const extra = o.classes ?? (o.variant === 'daisy' ? [] : [...DEFAULT_LAYOUT_CLASSES]);
		if (o.variant === 'daisy') {
			el.classList.add('tooltip-portal-daisy-host', ...extra);
		} else {
			el.classList.add('tooltip', 'text-left', ...extra);
		}
	}

	function syncDaisyPortal(trigger: HTMLElement, host: HTMLElement, o: TooltipOptions) {
		host.setAttribute('data-theme', resolveDataTheme(trigger));
		host.setAttribute('data-placement', placementAttr(o.placement));
	}

	function buildDaisyPortal(): HTMLElement {
		const host = document.createElement('div');
		const bubble = document.createElement('span');
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
			delay: isDaisyVariant(init) ? 0 : 300,
			strategy: 'fixed'
		});

		if (isDaisyVariant(init)) {
			portalRoot = buildDaisyPortal();
			portalRoot.classList.add('opacity-0', 'tooltip-portal-daisy-host--inactive');
			applyLookClasses(portalRoot, init);
			if (typeof init === 'object') syncDaisyPortal(node, portalRoot, init);
		} else {
			portalRoot = document.createElement('p');
			portalRoot.classList.add('hidden');
			applyLookClasses(portalRoot, init);
		}

		if (typeof init === 'object' && init?.disablePortal) {
			node.insertAdjacentElement('afterend', portalRoot);
		} else {
			document.body.appendChild(portalRoot);
		}

		tt.ref(node);
		tt.tooltip(portalRoot, {
			hover: true,
			disablePortal: typeof init === 'object' ? init.disablePortal : false,
			enterTransition: isDaisyVariant(init) ? 'daisy' : undefined
		});

		isEnabled = true;
	};

	const disable = () => {
		if (!isEnabled) return;
		portalRoot?.remove();
		portalRoot = null;
		tt = null;
		isEnabled = false;
	};

	const updateContent = (o: TooltipOptions | string | undefined) => {
		if (!portalRoot) return;
		const text = typeof o === 'string' ? o : (o?.text ?? '');
		const bubble = portalRoot.querySelector('.tooltip-portal-daisy-host__bubble');
		if (bubble) {
			bubble.textContent = text;
		} else {
			portalRoot.textContent = text;
		}
	};

	const update = (o: TooltipOptions | string | undefined) => {
		if (!hasText(o)) {
			disable();
			return;
		}

		if (!isEnabled) {
			enable(o);
			updateContent(o);
			return;
		}

		const wasDaisy = portalRoot?.classList.contains('tooltip-portal-daisy-host') ?? false;
		const nowDaisy = isDaisyVariant(o);
		if (wasDaisy !== nowDaisy) {
			disable();
			enable(o);
			updateContent(o);
			return;
		}

		if (portalRoot && typeof o === 'object' && nowDaisy) {
			syncDaisyPortal(node, portalRoot, o);
			applyLookClasses(portalRoot, o);
		} else if (portalRoot) {
			applyLookClasses(portalRoot, o);
		}

		updateContent(o);
	};

	$effect(() => {
		update(opts);
	});

	return {
		update,
		destroy: () => {
			disable();
		}
	};
}
