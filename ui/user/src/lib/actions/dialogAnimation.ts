import type { Action } from 'svelte/action';

type AnimationType = 'slide' | 'fade' | 'drawer';

interface DialogAnimationParams {
	type?: AnimationType;
}

// for <dialog> elements
export const dialogAnimation: Action<HTMLDialogElement, DialogAnimationParams> = (
	node,
	params = {}
) => {
	let { type } = params;

	// Set data attribute for drawer styling
	if (type === 'drawer') {
		node.setAttribute('data-drawer', 'true');
	}

	const slideIn = [
		{ transform: 'translateX(200%)', opacity: 0 },
		{ transform: 'translateX(0)', opacity: 1 }
	];

	const slideOut = [
		{ transform: 'translateX(0)', opacity: 1 },
		{ transform: 'translateX(-200%)', opacity: 0 }
	];

	const drawerIn = [
		{ transform: 'translateX(100%)', opacity: 0 },
		{ transform: 'translateX(0)', opacity: 1 }
	];
	const drawerOut = [
		{ transform: 'translateX(0)', opacity: 1 },
		{ transform: 'translateX(100%)', opacity: 0 }
	];

	const fadeIn = [{ opacity: 0 }, { opacity: 1 }];

	const fadeOut = [{ opacity: 1 }, { opacity: 0 }];

	const getAnimationOptions = (animationType: AnimationType): KeyframeAnimationOptions => ({
		duration: 200,
		easing: animationType === 'slide' || animationType === 'drawer' ? 'ease-out' : 'ease-in-out',
		fill: 'forwards' as const
	});

	const originalClose = node.close;

	// Override the dialog.close method
	node.close = function () {
		if (node.hasAttribute('closing')) return;
		node.setAttribute('closing', '');

		if (!type) {
			originalClose.call(node);
			node.removeAttribute('closing');
			return;
		}

		const dialogAnimation = node.animate(
			type === 'drawer' ? drawerOut : type === 'slide' ? slideOut : fadeOut,
			getAnimationOptions(type)
		);

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
					if (!type) return;

					node.animate(
						type === 'drawer' ? drawerIn : type === 'slide' ? slideIn : fadeIn,
						getAnimationOptions(type)
					);
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
		dialog[data-drawer="true"] {
			position: fixed !important;
			top: 0 !important;
			right: 0 !important;
			left: auto !important;
			bottom: 0 !important;
			margin: 0 !important;
			width: auto !important;
			max-width: none !important;
		}
	`;
	document.head.appendChild(style);

	return {
		update(newParams: DialogAnimationParams) {
			const { type: newType } = newParams;
			type = newType;

			// Update data attribute for drawer styling
			if (newType === 'drawer') {
				node.setAttribute('data-drawer', 'true');
			} else {
				node.removeAttribute('data-drawer');
			}

			if (node.hasAttribute('open') && newType) {
				node.animate(newType === 'slide' ? slideIn : fadeIn, getAnimationOptions(newType));
			}
		},
		destroy() {
			observer.disconnect();
			node.close = originalClose;
			node.removeAttribute('data-drawer');
			style.remove();
		}
	};
};
