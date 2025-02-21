export function resize(node: HTMLTextAreaElement) {
	node.style.height = 'auto';
	node.style.height = node.scrollHeight + 'px';
}

export function autoHeight(node: HTMLTextAreaElement) {
	if ('fieldSizing' in node.style) {
		if (node.value === '') {
			// This is so that rows=2 works
			node.style.fieldSizing = 'fixed';
		} else {
			node.style.fieldSizing = 'content';
		}
	}
	node.classList.add('scrollbar-none');
	node.onkeyup = () => resize(node);
	node.onfocus = () => resize(node);
	node.oninput = () => resize(node);
	node.onresize = () => resize(node);
	node.onchange = () => resize(node);
}
