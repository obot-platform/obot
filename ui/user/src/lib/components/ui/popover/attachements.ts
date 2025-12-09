import { PopoverController } from './controller.svelte';

export function clickOutsidePopoverContent(callback: () => void) {
	return (node: HTMLElement) => {
		const controller = PopoverController.get();

		document.addEventListener(
			'click',
			(event) => {
				if (node.contains(event.target as Node)) {
					return;
				}

				if (controller.dom.trigger?.contains(event.target as Node)) {
					return;
				}

				callback();
			},
			{ passive: true }
		);

		return () => {
			document.removeEventListener('click', callback as EventListener);
		};
	};
}
