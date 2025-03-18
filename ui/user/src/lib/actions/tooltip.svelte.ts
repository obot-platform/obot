import popover from '$lib/actions/popover.svelte';

export function tooltip(node: HTMLElement, opts: string | undefined) {
	const tt = popover({ placement: 'top', delay: 300 });

	const p = document.createElement('p');
	p.classList.add('hidden', 'tooltip', 'max-w-64');
	document.body.appendChild(p);

	const update = (opts: string | undefined) => {
		if (opts) {
			p.textContent = opts;
		}
	};

	$effect(() => {
		update(opts);
	});

	tt.ref(node);
	tt.tooltip(p, { hover: true });

	return {
		update,
		destroy: () => {
			p.remove();
		}
	};
}
