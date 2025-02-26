import type { Action } from 'svelte/action';

export type StickToBottomControls = {
	stickToBottom: () => void;
};

type StickToBottomOptions = {
	contentEl?: HTMLElement;
	setControls?: (controls: StickToBottomControls) => void;
};

export const sticktobottom: Action<HTMLElement, StickToBottomOptions | undefined> = (
	node,
	options = {}
) => {
	let shouldStick = true;
	let resizeObserver: ResizeObserver | null = null;

	function scrollToBottom() {
		node.scrollTop = node.scrollHeight - node.clientHeight;
	}

	function isAtBottom() {
		return node.scrollHeight - node.scrollTop - node.clientHeight <= 40;
	}

	$effect(() => {
		if (!options.contentEl) return;

		resizeObserver = new ResizeObserver(() => {
			if (shouldStick) {
				scrollToBottom();
			}
		});

		resizeObserver.observe(options.contentEl, { box: 'device-pixel-content-box' });

		return () => {
			if (resizeObserver) {
				resizeObserver.disconnect();
				resizeObserver = null;
			}
		};
	});

	// Handle wheel events to determine user scrolling intention
	$effect(() => {
		const handleWheel = (e: WheelEvent) => {
			// If user scrolls up, disable auto-scrolling
			if (e.deltaY < 0) {
				shouldStick = false;
			} else {
				// If user scrolls down to the bottom, re-enable auto-scrolling
				shouldStick = isAtBottom();
			}
		};

		node.addEventListener('wheel', handleWheel, { passive: true });

		// Clean up event listener when the effect is destroyed
		return () => node.removeEventListener('wheel', handleWheel);
	});

	$effect(() => {
		options.setControls?.({ stickToBottom: () => (shouldStick = true) });
	});

	// Return the action API
	return {
		update(newOptions) {
			options = { ...options, ...newOptions };
		}
	};
};
