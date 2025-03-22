import popover from './popover.svelte';
import type { Placement } from '@floating-ui/dom';
import type { Action } from 'svelte/action';
import { twMerge } from 'tailwind-merge';

interface TooltipOptions {
	text: string;
	className?: string;
	placement?: Placement;
	offset?: number;
	delay?: number;
	disabled?: boolean;
}

export const tooltip: Action<HTMLElement, TooltipOptions> = (node, options) => {
	if (!options?.text) return;

	// the tooltip element
	const tooltipEl = document.createElement('div');
	tooltipEl.className = twMerge('tooltip', options.className);
	tooltipEl.textContent = options.text;

	if (!options.disabled) {
		document.body.appendChild(tooltipEl);
	}

	const pop = popover({
		hover: true,
		placement: options.placement ?? 'top',
		offset: options.offset ?? 8,
		delay: options.delay ?? 150
	});

	pop.ref(node);
	pop.tooltip(tooltipEl);

	return {
		update(newOptions: TooltipOptions) {
			if (newOptions.text) {
				tooltipEl.textContent = newOptions.text;
			}

			if (newOptions.disabled) {
				tooltipEl.remove();
			} else {
				document.body.appendChild(tooltipEl);
			}
		},
		destroy() {
			tooltipEl.remove();
		}
	};
};
