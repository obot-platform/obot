// Deep import so this file does not resolve to itself through the Vitest alias.
// @ts-expect-error Svelte ships this runtime entry without a declaration file.
import * as animateRuntime from '../../../node_modules/svelte/src/animate/index.js';
import type * as SvelteAnimate from 'svelte/animate';
import type { AnimationConfig } from 'svelte/animate';

const { flip: flipAnimation } = animateRuntime as typeof SvelteAnimate;

/** Test stand-in for `svelte/animate`. Zero duration skips the FLIP animation. */
function instant(config: AnimationConfig): AnimationConfig {
	return { ...config, delay: 0, duration: 0 };
}

export const flip: typeof flipAnimation = (node, fromTo, params) =>
	instant(flipAnimation(node, fromTo, params));
