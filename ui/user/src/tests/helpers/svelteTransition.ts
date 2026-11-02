// Deep import so this file does not resolve to itself through the Vitest alias.
// @ts-expect-error Svelte ships this runtime entry without a declaration file.
import * as transitionRuntime from '../../../node_modules/svelte/src/transition/index.js';
import type * as SvelteTransition from 'svelte/transition';
import type { TransitionConfig } from 'svelte/transition';

const {
	blur: blurTransition,
	crossfade: crossfadeTransition,
	draw: drawTransition,
	fade: fadeTransition,
	fly: flyTransition,
	scale: scaleTransition,
	slide: slideTransition
} = transitionRuntime as typeof SvelteTransition;

/**
 * Test stand-in for `svelte/transition`.
 * `in:`, `out:`, `transition:`, and `|global` all call these functions, and
 * templates pass an explicit duration, so changing the default is not enough.
 * Duration and delay of 0 make Svelte finish the transition synchronously
 * instead of driving a Web Animation.
 */
function instant(config: TransitionConfig | undefined): TransitionConfig {
	if (!config) return { delay: 0, duration: 0 };
	return { ...config, delay: 0, duration: 0 };
}

function wrap<Args extends unknown[]>(
	transition: (...args: Args) => TransitionConfig
): (...args: Args) => TransitionConfig {
	return (...args) => instant(transition(...args));
}

export const blur = wrap(blurTransition);
export const fade = wrap(fadeTransition);
export const fly = wrap(flyTransition);
export const slide = wrap(slideTransition);
export const scale = wrap(scaleTransition);
export const draw = wrap(drawTransition);

export const crossfade: typeof crossfadeTransition = (params) => {
	const [send, receive] = crossfadeTransition(params);
	const zero =
		(direction: typeof send): typeof send =>
		(node, transitionParams) => {
			const deferred = direction(node, transitionParams);
			return () => instant(deferred());
		};
	return [zero(send), zero(receive)];
};
