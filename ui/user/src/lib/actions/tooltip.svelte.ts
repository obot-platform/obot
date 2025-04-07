import popover from '$lib/actions/popover.svelte';

interface TooltipOptions {
	text: string;
	disablePortal?: boolean;
}

export function tooltip(node: HTMLElement, opts: TooltipOptions | string | undefined) {
	const tt = popover({ placement: 'top', delay: 300 });

	const p = document.createElement('p');
	p.classList.add('hidden', 'tooltip', 'max-w-64');

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
