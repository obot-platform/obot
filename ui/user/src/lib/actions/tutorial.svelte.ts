import { computePosition, flip, offset, shift } from '@floating-ui/dom';

export interface TutorialStep {
	title: string;
	description: string;
	elementId?: string;
}

interface TutorialOptions {
	steps: TutorialStep[];
	onComplete?: () => void;
	onStepChange?: (step: number) => void;
}

export function tutorial(opts: TutorialOptions) {
	const { steps, onComplete, onStepChange } = opts;
	let currentStep = 0;
	let backdrop: HTMLDivElement | undefined;
	let popover: HTMLDivElement | undefined;
	let title: HTMLHeadingElement | undefined;
	let description: HTMLParagraphElement | undefined;
	let clone: HTMLElement | undefined;
	function next() {
		if (currentStep === steps.length - 1) {
			onComplete?.();
			return;
		}

		setupStep(steps[currentStep + 1]);
		currentStep++;
		onStepChange?.(currentStep);
	}

	function prev() {
		setupStep(steps[currentStep - 1]);
		currentStep--;
		onStepChange?.(currentStep);
	}

	async function setupStep(step: TutorialStep) {
		if (!backdrop || !popover || !title || !description) return;

		if (clone) {
			clone.remove();
		}

		Object.assign(popover.style, {
			left: 'auto',
			top: 'auto',
			transform: 'none'
		});

		// Update popover content
		if (title) {
			title.textContent = step.title;
		}

		if (description) {
			description.textContent = step.description;
		}

		// Clone the element and position it exactly where the original is
		const el = step.elementId ? document.getElementById(step.elementId) : undefined;
		if (!el) {
			// if no element id, position the popover in the center of the screen
			Object.assign(popover.style, {
				left: '50%',
				top: `50%`,
				transform: 'translate(-50%, -50%)',
				position: 'absolute'
			});

			popover.classList.remove('hidden');
			return;
		}

		clone = el.cloneNode(true) as HTMLElement;
		clone.style.position = 'absolute';
		clone.style.pointerEvents = 'none';
		clone.style.opacity = '1';
		clone.style.zIndex = '51';

		const rect = el.getBoundingClientRect();
		clone.style.left = `${rect.left}px`;
		clone.style.top = `${rect.top}px`;
		clone.style.width = `${rect.width}px`;
		clone.style.height = `${rect.height}px`;
		clone.classList.add('bg-white', 'dark:bg-black', 'px-2', 'rounded-md');

		backdrop.appendChild(clone);

		// Position popover using floating-ui
		const { x, y } = await computePosition(el, popover, {
			placement: 'right',
			middleware: [offset(10), flip(), shift({ padding: 5 })]
		});

		Object.assign(popover.style, {
			left: `${x}px`,
			top: `${y}px`,
			position: 'absolute'
		});

		popover.classList.remove('hidden');
	}

	function start() {
		if (!popover) return;
		if (backdrop) {
			backdrop.remove();
		}

		backdrop = document.createElement('div');
		backdrop.classList.add('fixed', 'inset-0', 'z-50', 'bg-black/65');
		backdrop.appendChild(popover);
		document.body.appendChild(backdrop);

		popover.classList.remove('hidden');
		const [firstStep] = steps;
		if (firstStep) {
			setupStep(firstStep);
		}
	}

	return {
		start,
		destroy: () => {
			backdrop?.remove();
		},
		popover: (node: HTMLDivElement) => {
			popover = node;
			return {
				destroy() {
					popover?.classList.add('hidden');
				}
			};
		},
		title: (node: HTMLHeadingElement) => {
			title = node;
			return {
				destroy() {
					title?.remove();
				}
			};
		},
		description: (node: HTMLParagraphElement) => {
			description = node;
			return {
				destroy() {
					description?.remove();
				}
			};
		},
		next,
		prev,
		currentStep
	};
}
