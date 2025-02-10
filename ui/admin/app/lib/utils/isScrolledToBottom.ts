export function isScrolledToBottom(container: HTMLDivElement | null) {
	if (!container) return false;

	const { scrollTop, scrollHeight, clientHeight } = container;
	return scrollHeight - scrollTop <= clientHeight;
}
