export function columnResize(
	handle: HTMLElement,
	{ column, direction = 'left' }: { column: HTMLElement; direction?: 'left' | 'right' }
) {
	const getClientX = (e: MouseEvent | TouchEvent): number => {
		if ('touches' in e && e.touches.length > 0) {
			return e.touches[0].clientX;
		}
		return (e as MouseEvent).clientX;
	};

	const resizeMove = (e: MouseEvent | TouchEvent) => {
		e.stopPropagation();
		const clientX = getClientX(e);
		const w =
			direction === 'right'
				? column.getBoundingClientRect().right - clientX
				: clientX - column.getBoundingClientRect().left;
		column.style.width = w + 'px';
	};

	const resizeDone = (e: MouseEvent | TouchEvent) => {
		e.stopPropagation();
		window.document.removeEventListener('mousemove', resizeMove);
		window.document.removeEventListener('mouseup', resizeDone);
		window.document.removeEventListener('touchmove', resizeMove);
		window.document.removeEventListener('touchend', resizeDone);
	};

	const resizeStart = (e: MouseEvent | TouchEvent): void => {
		e.preventDefault();
		e.stopPropagation();
		window.document.addEventListener('mousemove', resizeMove);
		window.document.addEventListener('mouseup', resizeDone);
		window.document.addEventListener('touchmove', resizeMove, { passive: false });
		window.document.addEventListener('touchend', resizeDone);
	};

	// Mouse events
	handle.addEventListener('mousedown', resizeStart);

	// Touch events
	handle.addEventListener('touchstart', resizeStart, { passive: false });

	return {
		destroy() {
			handle.removeEventListener('mousedown', resizeStart);
			handle.removeEventListener('touchstart', resizeStart);
		}
	};
}
