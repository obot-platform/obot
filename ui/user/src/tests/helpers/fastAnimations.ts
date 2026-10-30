const TEST_MOTION_MS = 1;

const nativeAnimate = Element.prototype.animate;

/**
 * Finish finite Web Animations on the next frame.
 * Svelte 5 transitions are already skipped (duration 0). Dialogs still call
 * `element.animate()` with `fill: 'forwards'`. Rewriting that duration to 1ms
 * can commit the first keyframe and leave a slide off-screen. `finish()` runs
 * after `animate()` returns, so listeners attached by the caller still run.
 */
export function useFastAnimations(maxMs = TEST_MOTION_MS) {
	Element.prototype.animate = function (
		keyframes: Keyframe[] | PropertyIndexedKeyframes | null,
		options?: number | KeyframeAnimationOptions
	) {
		const animation = nativeAnimate.call(this, keyframes, options);
		const timing = animation.effect?.getComputedTiming();
		const iterations = timing?.iterations ?? 1;
		const endTime = typeof timing?.endTime === 'number' ? timing.endTime : 0;
		if (!Number.isFinite(iterations) || endTime <= maxMs) return animation;

		requestAnimationFrame(() => {
			if (animation.playState === 'idle') return;
			try {
				animation.finish();
			} catch {
				// The animation cannot be finished.
			}
		});
		return animation;
	};

	return () => {
		Element.prototype.animate = nativeAnimate;
	};
}
