import popover from '$lib/actions/popover.svelte';
import type { Placement } from '@floating-ui/dom';

interface TooltipOptions {
	text: string;
	disablePortal?: boolean;
	classes?: string[];
	placement?: Placement;
}

export function tooltip(node: HTMLElement, opts: TooltipOptions | string | undefined) {
	const tt = popover({
		placement: typeof opts === 'object' && opts.placement ? opts?.placement : 'top',
		delay: 300
	});

	const p = document.createElement('p');
	const defaultClasses = ['max-w-64', 'break-all'];
	p.classList.add(
		'hidden',
		'tooltip',
		'text-left',
		...(typeof opts === 'object' ? (opts.classes ?? defaultClasses) : defaultClasses)
	);

	const update = (opts: TooltipOptions | string | undefined) => {
		if (typeof opts === 'string') {
			p.textContent = opts;
		} else if (opts?.text) {
			p.textContent = opts.text;
		}
	};

	$effect(() => {
		update(opts);
	});

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

	return {
		update,
		destroy: () => {
			p.remove();
		}
	};
}
