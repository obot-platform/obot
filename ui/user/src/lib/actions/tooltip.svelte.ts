import popover from '$lib/actions/popover.svelte';
import type { Placement } from '@floating-ui/dom';

interface TooltipOptions {
	text: string;
	disablePortal?: boolean;
	classes?: string[];
	placement?: Placement;
}

export function tooltip(node: HTMLElement, opts: TooltipOptions | string | undefined) {
	let tt: ReturnType<typeof popover> | null = null;
	let p: HTMLElement | null = null;
	let isEnabled = false;

	const hasText = (opts: TooltipOptions | string | undefined) => {
		return typeof opts === 'string' ? opts.trim() !== '' : !!opts?.text?.trim();
	};

	const enable = (opts: TooltipOptions | string | undefined) => {
		if (isEnabled) return;

		tt = popover({
			placement: typeof opts === 'object' && opts.placement ? opts?.placement : 'top',
			delay: 300
		});

		p = document.createElement('p');
		// Use word-boundary wrapping and preserve newlines to avoid awkward breaks
		const defaultClasses = ['max-w-64', 'break-words', 'whitespace-pre-wrap'];
		p.classList.add(
			'hidden',
			'tooltip',
			'text-left',
			...(typeof opts === 'object' ? (opts.classes ?? defaultClasses) : defaultClasses)
		);

		if (typeof opts === 'object' && opts?.disablePortal) {
			node.insertAdjacentElement('afterend', p);
		} else {
			document.body.appendChild(p);
		}

		tt.ref(node);
		tt.tooltip(p, {
			hover: true,
			disablePortal: typeof opts === 'object' ? opts.disablePortal : false
		});

		isEnabled = true;
	};

	const disable = () => {
		if (!isEnabled) return;
		p?.remove();
		p = null;
		tt = null;
		isEnabled = false;
	};

	const updateContent = (opts: TooltipOptions | string | undefined) => {
		if (!p) return;

		if (typeof opts === 'string') {
			p.textContent = opts;
		} else if (opts?.text) {
			p.textContent = opts.text;
		}
	};

	const update = (opts: TooltipOptions | string | undefined) => {
		if (hasText(opts)) {
			if (!isEnabled) {
				enable(opts);
			}
			updateContent(opts);
		} else {
			disable();
		}
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
