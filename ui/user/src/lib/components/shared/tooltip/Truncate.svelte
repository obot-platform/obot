<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import type { ClassValue } from 'svelte/elements';

	type Props = {
		tooltipText?: string;
		text: string;
		class?: ClassValue;
		classes?: { tooltip?: ClassValue };
		disabled?: boolean;
	};
	let { text, class: className, classes = {}, tooltipText, disabled }: Props = $props();
	let anchor = $state<HTMLElement>();
	let truncated = $state(false);

	$effect(() => {
		if (!anchor) return;

		truncated =
			anchor.scrollWidth > anchor.clientWidth || anchor.scrollHeight > anchor.clientHeight;
	});

	export { truncated };
</script>

<p
	use:tooltip={{
		anchor,
		placement: 'top',
		delay: 200,
		get disabled() {
			return disabled || !truncated;
		}
	}}
	class={[
		'max-w-md break-words rounded-lg bg-blue-500 px-2 py-1 text-sm text-white dark:text-black',
		classes.tooltip
	]}
>
	{tooltipText || text}
</p>

<span bind:this={anchor} class={['line-clamp-1 break-words text-start', className]}>
	{text}
</span>
