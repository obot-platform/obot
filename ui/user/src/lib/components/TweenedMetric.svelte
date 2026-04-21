<script lang="ts">
	import { cubicOut } from 'svelte/easing';
	import { Tween } from 'svelte/motion';

	interface Props {
		target: number;
		/** When true, the displayed value stays at 0 (e.g. while loading). */
		holdAtZero?: boolean;
		duration?: number;
		/** Format the animated value for display; defaults to rounded integer string. */
		format?: (n: number) => string;
	}

	let {
		target,
		holdAtZero = false,
		duration = 650,
		format = (n) => String(Math.round(n))
	}: Props = $props();

	const tween = new Tween(0, { easing: cubicOut });

	$effect(() => {
		if (holdAtZero) {
			void tween.set(0, { duration: 0 });
			return;
		}
		void tween.set(target, { duration });
	});
</script>

{format(tween.current)}
