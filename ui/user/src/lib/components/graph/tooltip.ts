import { autoUpdate, computePosition, flip, offset } from '@floating-ui/dom';

/** Positions a floating chart tooltip above `reference` and keeps it in place while visible. */
export function chartTooltip(reference: Element, floating: HTMLElement) {
	const compute = async () => {
		const { x, y } = await computePosition(reference, floating, {
			placement: 'top',
			middleware: [
				offset(8),
				flip({
					padding: { top: 0, right: 40, left: 40, bottom: 0 },
					boundary: document.documentElement,
					fallbackPlacements: ['top', 'top-end', 'top-start', 'left-start', 'right-start']
				})
			]
		});

		floating.style.transform = `translate(${x}px, ${y}px)`;
	};

	return autoUpdate(reference, floating, compute, {
		animationFrame: true,
		ancestorScroll: true,
		ancestorResize: true
	});
}
