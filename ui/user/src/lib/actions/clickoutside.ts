export function clickOutside(element: HTMLElement, onClickOutside: () => void) {
	function checkClickOutside(event: Event) {
		if (!(event.target as HTMLElement)?.contains(element)) return;
		onClickOutside();
	}
	element.addEventListener('click', checkClickOutside);

	return {
		destroy() {
			element.removeEventListener('click', checkClickOutside);
		}
	};
}
