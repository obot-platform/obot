<script lang="ts">
	import { cubicOut } from 'svelte/easing';
	import { Tween } from 'svelte/motion';

	interface Props {
		targetWidth: number;
		y: number;
		height: number;
		fill: string;
		rx: number;
		class?: string;
		onpointerenter: (e: PointerEvent) => void;
		onpointerleave: (e: PointerEvent) => void;
	}

	let {
		targetWidth,
		y,
		height,
		fill,
		rx,
		class: klass = '',
		onpointerenter,
		onpointerleave
	}: Props = $props();

	const width = new Tween(0, { duration: 400, easing: cubicOut });

	$effect(() => {
		width.set(targetWidth);
	});
</script>

<rect
	x={0}
	{y}
	width={width.current}
	{height}
	{rx}
	ry={rx}
	{fill}
	class={klass}
	{onpointerenter}
	{onpointerleave}
	role="presentation"
/>
