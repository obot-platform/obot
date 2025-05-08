import type { Action } from 'svelte/action';

type AnimationType = 'slide' | 'fade';

interface DialogAnimationParams {
	type?: AnimationType;
}

// for <dialog> elements
export const dialogAnimation: Action<HTMLDialogElement, DialogAnimationParams> = (
	node,
	params = {}
) => {
	const { type = 'slide' } = params;

	const slideIn = [
		{ transform: 'translateX(200%)', opacity: 0 },
		{ transform: 'translateX(0)', opacity: 1 }
	];

	const slideOut = [
		{ transform: 'translateX(0)', opacity: 1 },
		{ transform: 'translateX(-200%)', opacity: 0 }
	];

	const fadeIn = [{ opacity: 0 }, { opacity: 1 }];

	const fadeOut = [{ opacity: 1 }, { opacity: 0 }];

	const animationOptions: KeyframeAnimationOptions = {
		duration: 200,
		easing: type === 'slide' ? 'ease-out' : 'ease-in-out',
		fill: 'forwards' as const
	};

	const originalClose = node.close;

	// Override the dialog.close method
	node.close = function () {
		if (node.hasAttribute('closing')) return;
		node.setAttribute('closing', '');

		const dialogAnimation = node.animate(type === 'slide' ? slideOut : fadeOut, animationOptions);

		// Wait for animation to complete
		dialogAnimation.addEventListener(
			'finish',
			() => {
				originalClose.call(node);
				node.removeAttribute('closing');
			},
			{ once: true }
		);
	};

	const observer = new MutationObserver((mutations) => {
		mutations.forEach((mutation) => {
			if (mutation.attributeName === 'open') {
				if (node.hasAttribute('open')) {
					node.animate(type === 'slide' ? slideIn : fadeIn, animationOptions);
				}
			}
		});
	});

	observer.observe(node, {
		attributes: true,
		attributeFilter: ['open']
	});

	// Adds backdrop animation styles
	const style = document.createElement('style');
	style.textContent = `
		dialog::backdrop {
			background-color: rgba(0, 0, 0, 0.5);
			transition: opacity 200ms ease-in-out;
		}
		dialog[closing]::backdrop {
			opacity: 0;
		}
	`;
	document.head.appendChild(style);

	return {
		update(newParams: DialogAnimationParams) {
			const { type: newType = 'slide' } = newParams;
			if (newType !== type) {
				if (node.hasAttribute('open')) {
					node.animate(newType === 'slide' ? slideIn : fadeIn, animationOptions);
				}
			}
		},
		destroy() {
			observer.disconnect();
			node.close = originalClose;
			style.remove();
		}
	};
};
