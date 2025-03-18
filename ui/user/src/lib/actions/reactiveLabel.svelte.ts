import type { Action } from 'svelte/action';

interface ReactiveLabelParams {
	value?: string;
	height?: string;
}

export const reactiveLabel: Action<HTMLElement, ReactiveLabelParams> = (node, params) => {
	// Add default classes that don't change
	node.classList.add('origin-top', 'overflow-hidden', 'transition-all', 'duration-200');

	$effect(() => {
		const { value, height = 'h-6' } = params;
		// Remove previous height class if it exists
		node.classList.remove('h-0', height);
		// Remove previous transform classes
		node.classList.remove('translate-y-0', 'translate-y-full');
		// Remove previous opacity class
		node.classList.remove('opacity-0');

		// Add appropriate classes based on value
		if (value) {
			node.classList.add(height, 'translate-y-0');
		} else {
			node.classList.add('h-0', 'translate-y-full', 'opacity-0');
		}
	});
};
