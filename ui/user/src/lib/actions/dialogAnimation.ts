import type { Action } from 'svelte/action';

type AnimationType = 'slide' | 'fade' | 'drawer';

interface DialogAnimationParams {
	type?: AnimationType | null;
}

// for <dialog> elements
export const dialogAnimation: Action<HTMLDialogElement, DialogAnimationParams> = (
	node,
	params = {}
) => {
	let { type } = params;

	if (type) {
		node.setAttribute('data-dialog-animated', 'true');
	}

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

	// Backdrop animations (always fade)
	const backdropFadeIn = [{ opacity: 0 }, { opacity: 1 }];
	const backdropFadeOut = [{ opacity: 1 }, { opacity: 0 }];

	const getAnimationOptions = (animationType: AnimationType): KeyframeAnimationOptions => ({
		duration: 200,
		easing: animationType === 'slide' || animationType === 'drawer' ? 'ease-out' : 'ease-in-out',
		fill: 'forwards' as const
	});

	const backdropAnimationOptions: KeyframeAnimationOptions = {
		duration: 200,
		easing: 'ease-in-out',
		fill: 'forwards' as const
	};

	const getContentElement = () => node.querySelector('.dialog-container') as HTMLElement | null;
	const getBackdropElement = () => node.querySelector('.dialog-backdrop') as HTMLElement | null;

	// Avoid stacking semi-transparent overlays when slide/drawer dialogs transition.
	const hasOtherActiveDialog = () => {
		return Array.from(
			document.querySelectorAll('dialog.dialog[open], dialog.dialog[closing]')
		).some((dialog) => dialog !== node);
	};

	const originalShowModal = node.showModal;
	const originalClose = node.close;

	node.showModal = function () {
		originalShowModal.call(node);

		const backdrop = getBackdropElement();
		if (!backdrop) return;

		// Suppress overlay before paint when another dialog already owns it
		if (hasOtherActiveDialog()) {
			backdrop.style.opacity = '0';
		} else {
			backdrop.style.removeProperty('opacity');
		}
	};

	// Override the dialog.close method
	node.close = function () {
		if (node.hasAttribute('closing')) return;
		node.setAttribute('closing', '');

		const content = getContentElement();
		const backdrop = getBackdropElement();

		if (!type || !content) {
			originalClose.call(node);
			node.removeAttribute('closing');
			return;
		}

		// Animate content (slide/fade/drawer)
		const contentAnimation = content.animate(
			type === 'drawer' ? drawerOut : type === 'slide' ? slideOut : fadeOut,
			getAnimationOptions(type)
		);

		// Keep backdrop visible when another dialog is taking over the overlay.
		// Defer one frame so close-then-open in the same tick still detects the incoming dialog.
		const animateBackdropOut = () => {
			if (hasOtherActiveDialog()) {
				backdrop?.animate([{ opacity: 1 }], { duration: 0, fill: 'forwards' });
			} else {
				backdrop?.animate(backdropFadeOut, backdropAnimationOptions);
			}
		};
		requestAnimationFrame(animateBackdropOut);

		// Wait for content animation to complete
		contentAnimation.addEventListener(
			'finish',
			() => {
				const handedOffToAnotherDialog = hasOtherActiveDialog();
				originalClose.call(node);
				node.removeAttribute('closing');
				getBackdropElement()?.style.removeProperty('opacity');

				// Keep overlay at full strength on the surviving dialog after a handoff
				if (handedOffToAnotherDialog) {
					const survivingBackdrop = document.querySelector(
						'dialog.dialog[open] .dialog-backdrop'
					) as HTMLElement | null;
					survivingBackdrop?.style.removeProperty('opacity');
					survivingBackdrop?.animate([{ opacity: 1 }], { duration: 0, fill: 'forwards' });
				}
			},
			{ once: true }
		);
	};

	const observer = new MutationObserver((mutations) => {
		mutations.forEach((mutation) => {
			if (mutation.attributeName === 'open') {
				if (node.hasAttribute('open')) {
					if (!type) return;

					const content = getContentElement();
					const backdrop = getBackdropElement();

					// Animate content (slide/fade/drawer)
					content?.animate(
						type === 'drawer' ? drawerIn : type === 'slide' ? slideIn : fadeIn,
						getAnimationOptions(type)
					);

					// Skip backdrop fade-in when another dialog already provides the overlay
					if (hasOtherActiveDialog()) {
						backdrop?.animate([{ opacity: 0 }], { duration: 0, fill: 'forwards' });
					} else {
						backdrop?.animate(backdropFadeIn, backdropAnimationOptions);
					}
				}
			}
		});
	});

	observer.observe(node, {
		attributes: true,
		attributeFilter: ['open']
	});

	// Adds drawer positioning styles
	const style = document.createElement('style');
	style.textContent = `
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

			if (newType) {
				node.setAttribute('data-dialog-animated', 'true');
			} else {
				node.removeAttribute('data-dialog-animated');
			}

			// Update data attribute for drawer styling
			if (newType === 'drawer') {
				node.setAttribute('data-drawer', 'true');
			} else {
				node.removeAttribute('data-drawer');
			}

			if (node.hasAttribute('open') && newType) {
				const content = getContentElement();
				content?.animate(
					newType === 'drawer' ? drawerIn : newType === 'slide' ? slideIn : fadeIn,
					getAnimationOptions(newType)
				);
			}
		},
		destroy() {
			observer.disconnect();
			node.showModal = originalShowModal;
			node.close = originalClose;
			node.removeAttribute('data-dialog-animated');
			node.removeAttribute('data-drawer');
			style.remove();
		}
	};
};
