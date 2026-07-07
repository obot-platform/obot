<script lang="ts">
	import arrowIdleSvg from './assets/obot-icon-blue-arrow-idle.svg?raw';
	import arrowSvg from './assets/obot-icon-blue-arrow.svg?raw';
	import idleSvg from './assets/obot-icon-blue-idle.svg?raw';
	import { untrack } from 'svelte';
	import { fade } from 'svelte/transition';

	export type ObotAnimation = 'enter' | 'idle' | 'arrow' | 'arrow_idle';

	interface ObotAsset {
		markup: string;
	}

	const OBOT_ASSETS: Record<ObotAnimation, ObotAsset> = {
		enter: { markup: idleSvg },
		idle: { markup: idleSvg },
		arrow: { markup: arrowSvg },
		arrow_idle: { markup: arrowIdleSvg }
	};

	/** Auto-advance to the next item in a sequence. Looping animations omit this. */
	const ADVANCE_MS: Partial<Record<ObotAnimation, number>> = {
		enter: 400,
		arrow: 2000
	};

	function mountSvg(container: HTMLElement, asset: ObotAsset) {
		container.innerHTML = asset.markup;
		const svg = container.querySelector('svg');
		if (!svg) return;

		svg.setAttribute('width', '100%');
		svg.setAttribute('height', '100%');
		svg.setAttribute('aria-hidden', 'true');
		svg.setAttribute('focusable', 'false');
		svg.style.display = 'block';

		svg.querySelector(':scope > g')?.classList.add('guide-obot__body');
		const paths = svg.querySelectorAll(':scope > path');
		paths.item(0)?.classList.add('guide-obot__foot');
		paths.item(1)?.classList.add('guide-obot__foot');
		paths.item(2)?.classList.add('guide-obot__arrow');
	}

	interface Props {
		animation?: ObotAnimation | ObotAnimation[];
		size?: number;
		class?: string;
	}

	let { animation = 'idle', size = 96, class: klass = '' }: Props = $props();

	function normalizeSequence(value: ObotAnimation | ObotAnimation[]): ObotAnimation[] {
		const sequence = Array.isArray(value) ? value : [value];
		return sequence.length > 0 ? sequence : ['idle'];
	}

	function sequencesEqual(a: ObotAnimation[], b: ObotAnimation[]): boolean {
		return a.length === b.length && a.every((name, i) => name === b[i]);
	}

	function advance() {
		if (pendingSequence !== undefined) {
			sequence = pendingSequence;
			pendingSequence = undefined;
			sequenceIndex = 0;
		} else if (sequenceIndex < sequence.length - 1) {
			sequenceIndex += 1;
		}
	}

	// Capture initial sequence once; later prop changes queue via pendingSequence.
	let sequence = $state<ObotAnimation[]>(untrack(() => normalizeSequence(animation)));
	let sequenceIndex = $state(0);
	let pendingSequence = $state<ObotAnimation[] | undefined>();
	let host = $state<HTMLDivElement | undefined>();

	const activeAnimation = $derived(sequence[sequenceIndex] ?? 'idle');
	const asset = $derived(OBOT_ASSETS[activeAnimation]);
	const fadeIn = untrack(() => normalizeSequence(animation)[0] === 'enter');

	$effect(() => {
		const next = normalizeSequence(animation);
		if (!sequencesEqual(next, sequence)) {
			pendingSequence = next;
		} else {
			pendingSequence = undefined;
		}
	});

	$effect(() => {
		const ms = ADVANCE_MS[activeAnimation];
		if (ms === undefined) {
			// Looping animation — apply a queued sequence immediately.
			if (pendingSequence !== undefined) {
				advance();
			}
			return;
		}

		const id = setTimeout(() => {
			advance();
		}, ms);
		return () => clearTimeout(id);
	});

	$effect(() => {
		const el = host;
		const nextAsset = asset;
		if (!el) return;
		mountSvg(el, nextAsset);
	});
</script>

<div
	bind:this={host}
	in:fade={{ duration: fadeIn ? 400 : 0 }}
	out:fade={{ duration: 100 }}
	class="guide-obot guide-obot--{activeAnimation} {klass}"
	style:width="{size}px"
	style:height="{size}px"
	role="img"
	aria-label="Obot {activeAnimation}"
></div>

<style>
	:global(.guide-obot--enter .guide-obot__body),
	:global(.guide-obot--idle .guide-obot__body),
	:global(.guide-obot--arrow_idle .guide-obot__body) {
		animation: guide-obot-body-bounce 1s ease-in-out infinite;
	}

	:global(.guide-obot--enter .guide-obot__foot),
	:global(.guide-obot--idle .guide-obot__foot),
	:global(.guide-obot--arrow_idle .guide-obot__foot) {
		animation: guide-obot-foot-bounce 1s ease-in-out infinite;
	}

	:global(.guide-obot--arrow .guide-obot__body) {
		animation: guide-obot-body-bounce 1s ease-in-out both;
	}

	:global(.guide-obot--arrow .guide-obot__foot) {
		animation: guide-obot-foot-bounce 1s ease-in-out both;
	}

	:global(.guide-obot__arrow) {
		d: path(
			'M 169.93532 8.17274 L 169.93532 33.7791 L 80.03123 33.7791 L 80.03123 40.52496 L 49.72265 20.7733 L 80.03123 1.02164 L 80.03123 8.17274 L 169.93532 8.17274 Z'
		);
		opacity: 1;
	}

	:global(.guide-obot--arrow .guide-obot__arrow) {
		animation: guide-obot-arrow-grow 1s ease-in-out both;
	}

	:global(.guide-obot--arrow_idle .guide-obot__arrow) {
		animation: guide-obot-arrow-bounce 1s ease-in-out infinite;
	}

	@keyframes guide-obot-body-bounce {
		0% {
			transform: translate(0.79138px, 7.518117px);
		}
		50% {
			transform: translate(0.79138px, 17.298522px);
		}
		100% {
			transform: translate(0.798364px, 9.474199px);
		}
	}

	@keyframes guide-obot-foot-bounce {
		0% {
			d: path(
				'M 58.93396 157.12873 L 75.36388 157.12873 L 75.35727 157.11771 L 79.69756 157.08599 C 79.31226 157.63464 78.18323 162.21992 78.41188 161.31385 L 78.41188 180.99717 C 89.92194 184.88694 98.30589 195.62574 98.77796 208.39914 L 38.97702 208.39914 C 39.44348 195.77759 47.63483 185.14254 58.93396 181.13878 L 58.93396 157.12873 Z'
			);
		}
		50% {
			d: path(
				'M 58.93396 169.426302 L 75.36388 169.426302 L 75.35727 169.415282 L 79.69756 169.383562 C 79.31226 169.932212 78.18323 174.517492 78.41188 173.611422 L 78.41188 180.99717 C 89.92194 184.88694 98.30589 195.62574 98.77796 208.39914 L 38.97702 208.39914 C 39.44348 195.77759 47.63483 185.14254 58.93396 181.13878 L 58.93396 169.426302 Z'
			);
		}
		100% {
			d: path(
				'M 58.93396 159.645897 L 75.36388 159.645897 L 75.35727 159.634877 L 79.69756 159.603157 C 79.31226 160.151807 78.18323 164.737087 78.41188 163.831017 L 78.41188 180.99717 C 89.92194 184.88694 98.30589 195.62574 98.77796 208.39914 L 38.97702 208.39914 C 39.44348 195.77759 47.63483 185.14254 58.93396 181.13878 L 58.93396 159.645897 Z'
			);
		}
	}

	@keyframes guide-obot-arrow-bounce {
		0% {
			transform: translate(-0.340546px, -1.232358px);
		}
		50% {
			transform: translate(-0.340546px, 8.43249px);
		}
		100% {
			transform: translate(-0.340546px, 2.102297px);
		}
	}

	@keyframes guide-obot-arrow-grow {
		0% {
			d: path(
				'M 120.791111 10.549544 L 120.79111 33.7791 L 109.318177 33.779101 L 109.318177 40.524961 L 109.318177 19.751659 L 109.318177 1.02164 L 109.318178 10.549544 L 120.791111 10.549544 Z'
			);
			opacity: 0;
			transform: translate(-0.340546px, -1.232358px);
		}
		25% {
			d: path(
				'M 120.791111 10.549544 L 120.79111 33.7791 L 109.318177 33.779101 L 109.318177 40.524961 L 109.318177 19.751659 L 109.318177 1.02164 L 109.318178 10.549544 L 120.791111 10.549544 Z'
			);
			opacity: 1;
		}
		50% {
			transform: translate(-0.340546px, 8.43249px);
		}
		100% {
			d: path(
				'M 169.93532 8.17274 L 169.93532 33.7791 L 80.03123 33.7791 L 80.03123 40.52496 L 49.72265 20.7733 L 80.03123 1.02164 L 80.03123 8.17274 L 169.93532 8.17274 Z'
			);
			opacity: 1;
			transform: translate(-0.340546px, 2.102297px);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		:global(.guide-obot__body),
		:global(.guide-obot__foot),
		:global(.guide-obot__arrow) {
			animation: none;
		}
	}
</style>
