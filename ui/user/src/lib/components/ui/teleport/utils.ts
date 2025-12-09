export function port(target: HTMLElement = document.body) {
	return (source: HTMLElement) => {
		if (!target) {
			throw Error('[actions] portal: Target element is undefined !');
		}

		// Check if element is already mounted on target
		if (source.parentElement !== target) {
			// node.hidden = true;

			target.appendChild(source);
			requestAnimationFrame(() => {
				source.hidden = false;
			});
		}

		return () => {
			source.hidden = true;
			source.remove();
		};
	};
}
