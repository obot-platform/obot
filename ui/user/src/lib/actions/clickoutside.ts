export function clickOutside(element: HTMLElement, params: (() => void) | [() => void, boolean]) {
	const [onClickOutside, inline = false] = Array.isArray(params) ? params : [params, false];

	let ignoreNextClick = false;
	let observer: MutationObserver | undefined;

	function checkClickOutside(event: Event) {
		if (element.contains(event.target as Node)) return;
		onClickOutside();
	}

	function checkDialogClickOutside(event: Event) {
		if (!(element as HTMLDialogElement).open) return;
		if (!(event.target as HTMLElement)?.contains(element)) return;
		onClickOutside();
	}

	function checkInlineDialogClickOutside(event: Event) {
		if (element.contains(event.target as Node)) return;

		if (ignoreNextClick) {
			ignoreNextClick = false;
			return;
		}

		onClickOutside();
	}

	// <dialog> called with showModal()
	const isModalDialog =
		element.tagName.toLowerCase() === 'dialog' &&
		(element as HTMLDialogElement).showModal !== undefined;

	if (!isModalDialog) {
		document.addEventListener('click', checkClickOutside);
	} else if (inline && isModalDialog) {
		// Was called with dialog.show();
		document.addEventListener('click', checkInlineDialogClickOutside);

		// Set up mutation observer to detect when dialog opens
		observer = new MutationObserver((mutations) => {
			mutations.forEach((mutation) => {
				if (mutation.type === 'attributes' && mutation.attributeName === 'open') {
					if ((element as HTMLDialogElement).open) {
						ignoreNextClick = true;
					}
				}
			});
		});

		observer.observe(element, { attributes: true, attributeFilter: ['open'] });
	} else {
		element.addEventListener('click', checkDialogClickOutside);
	}

	return {
		destroy() {
			if (!isModalDialog) {
				document.removeEventListener('click', checkClickOutside);
			} else if (inline && isModalDialog) {
				document.removeEventListener('click', checkInlineDialogClickOutside);
				observer?.disconnect();
			} else {
				element.removeEventListener('click', checkDialogClickOutside);
			}
		}
	};
}
