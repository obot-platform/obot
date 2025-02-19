import { tick } from 'svelte';

export function opacityIn(node: HTMLElement) {
	node.classList.add('opacity-0');
	node.classList.add('transition-opacity');
	node.classList.add('duration-300');
	tick().then(() => {
		node.classList.remove('opacity-0');
	});
}
