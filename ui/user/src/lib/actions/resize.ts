export function columnResize(
	handle: HTMLElement,
	{ column, direction = 'left' }: { column: HTMLElement; direction?: 'left' | 'right' }
) {
	const resizeMove = (e: MouseEvent) => {
		const w =
			direction === 'right'
				? column.getBoundingClientRect().right - e.clientX
				: e.clientX - column.getBoundingClientRect().left;
		column.style.width = w + 'px';
	};

	const resizeDone = () => {
		window.document.removeEventListener('mousemove', resizeMove);
		window.document.removeEventListener('mouseup', resizeDone);
	};

	const resizeStart = (e: MouseEvent): void => {
		e.preventDefault();
		window.document.addEventListener('mousemove', resizeMove);
		window.document.addEventListener('mouseup', resizeDone);
	};

	handle.addEventListener('mousedown', resizeStart);

	return {
		destroy() {
			handle.removeEventListener('mousedown', resizeStart);
		}
	};
}
