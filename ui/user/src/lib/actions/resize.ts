export function columnResize(handle: HTMLElement, column: HTMLElement) {
	const resizeMove = (e: MouseEvent) => {
		const w = e.clientX - column.getBoundingClientRect().left;
		column.style.width = w + 'px';
	};

	const resizeDone = () => {
		window.document.removeEventListener('mousemove', resizeMove);
		window.document.removeEventListener('mouseup', resizeDone);
	};

	handle.onmousedown = (e) => {
		e.preventDefault();
		window.document.addEventListener('mousemove', resizeMove);
		window.document.addEventListener('mouseup', () => {
			window.document.removeEventListener('mousemove', resizeMove);
			window.document.removeEventListener('mouseup', resizeDone);
		});
	};
}
