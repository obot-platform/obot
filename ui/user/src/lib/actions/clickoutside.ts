export function clickOutside(element: HTMLElement, onClickOutside: (event: Event) => void) {
	// <dialog> called with showModal()
	const isModalDialog =
		element.tagName.toLowerCase() === 'dialog' &&
		(element as HTMLDialogElement).showModal !== undefined;

	if (!isModalDialog) {
		const onclick = (event: MouseEvent) => {
			if (element?.contains(event.target as HTMLElement)) return;

			onClickOutside(event);
		};

		document.addEventListener('click', onclick);

		return {
			destroy() {
				document.removeEventListener('click', onclick);
			}
		};
	} else {
		const onclick = (event: MouseEvent) => {
			const target = event.target as HTMLElement;

			// click inside dialog content
			if (element?.contains(target) && element !== target) return;

			// click outside dialog
			onClickOutside(event);
		};

		element.addEventListener('click', onclick);

		return {
			destroy() {
				element.removeEventListener('click', onclick);
			}
		};
	}
}
