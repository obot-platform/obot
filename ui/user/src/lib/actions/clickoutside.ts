export function clickOutside(element: HTMLElement, onClickOutside: () => void) {
	function checkClickOutside(event: Event) {
		if (element.contains(event.target as Node)) return;
		onClickOutside();
	}

	function checkDialogClickOutside(event: Event) {
		if (!(element as HTMLDialogElement).open) return;
		if (!(event.target as HTMLElement)?.contains(element)) return;
		onClickOutside();
	}

	// <dialog> called with showModal()
	const isModalDialog =
		element.tagName.toLowerCase() === 'dialog' &&
		(element as HTMLDialogElement).showModal !== undefined;

	if (!isModalDialog) {
		document.addEventListener('click', checkClickOutside);
	} else {
		element.addEventListener('click', checkDialogClickOutside);
	}

	return {
		destroy() {
			if (!isModalDialog) {
				document.removeEventListener('click', checkClickOutside);
			} else {
				element.removeEventListener('click', checkDialogClickOutside);
			}
		}
	};
}
