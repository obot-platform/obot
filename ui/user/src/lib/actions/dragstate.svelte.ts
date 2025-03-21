import type { Action } from 'svelte/action';

export const dragstate = () => {
	let dragging = $state(false);

	const dragRef: Action<HTMLElement> = (node) => {
		const ondragover = (e: DragEvent): void => {
			e.preventDefault();
			e.stopPropagation();
			dragging = true;
		};

		const ondragleave = (e: DragEvent): void => {
			e.preventDefault();
			e.stopPropagation();
			const rect = node.getBoundingClientRect();
			const x = e.clientX;
			const y = e.clientY;

			// Only set dragging to false if the mouse is outside the container
			if (x < rect.left || x > rect.right || y < rect.top || y > rect.bottom) {
				dragging = false;
			}
		};

		node.addEventListener('dragover', ondragover);
		node.addEventListener('dragleave', ondragleave);

		return {
			destroy() {
				node.removeEventListener('dragover', ondragover);
				node.removeEventListener('dragleave', ondragleave);
			}
		};
	};

	return {
		get dragging() {
			return dragging;
		},
		toggle(val?: boolean) {
			dragging = val ?? !dragging;
		},
		dragRef
	};
};
