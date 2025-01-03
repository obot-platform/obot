import { tick } from 'svelte';

export function resize(node: HTMLTextAreaElement) {
	node.style.height = 'auto';
	node.style.height = node.scrollHeight + 'px';
}

export function autoHeight(node: HTMLTextAreaElement) {
	node.onkeyup = () => resize(node);
	node.onfocus = () => resize(node);
	node.oninput = () => resize(node);
	node.onresize = () => resize(node);
	node.onchange = () => resize(node);
	tick().then(() => resize(node));

	// I don't have a great solution when the textarea is loaded on demand because it doesn't
	// seem to fire any event. I'm sure there is one.
	setTimeout(() => resize(node), 500);
}
