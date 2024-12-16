function resize(node: HTMLTextAreaElement) {
	node.style.height = 'auto';
	node.style.height = node.scrollHeight + 'px';
}

export function autoHeight(node: HTMLTextAreaElement) {
	node.onkeyup = () => resize(node);
	node.onfocus = () => resize(node);
	node.oninput = () => resize(node);
	node.onresize = () => resize(node);
	node.onchange = () => resize(node);
}
