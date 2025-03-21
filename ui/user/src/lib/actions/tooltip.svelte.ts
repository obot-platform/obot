import popover from '$lib/actions/popover.svelte';

export function tooltip(node: HTMLElement, opts: string | undefined) {
	const tt = popover({ hover: true, placement: 'top', delay: 300 });

	const p = document.createElement('p');
	p.classList.add('hidden', 'tooltip', 'max-w-64');
	document.body.appendChild(p);

	const update = (opts: string | undefined) => {
		console.log('effect');
		if (opts) {
			p.textContent = opts;
		}
		console.log('update');
	};

	$effect(() => {
		update(opts);
	});

	tt.ref(node);
	tt.tooltip(p);

	return {
		update,
		destroy: () => {
			p.remove();
		}
	};
}
