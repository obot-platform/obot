<script lang="ts">
	import { createTooltip } from '$lib/actions/tooltip.svelte';
	import type { ClassValue } from 'svelte/elements';

	type Props = {
		tooltipText?: string;
		text: string;
		class?: ClassValue;
		classes?: { tooltip?: ClassValue };
		disabled?: boolean;
	};
	let { text, class: className, classes = {}, tooltipText, disabled }: Props = $props();
	let anchorRef = $state<HTMLElement>();
	let truncated = $state(false);

	const tooltip = createTooltip({
		disabled: () => disabled || !truncated,
		delay: 200,
		placement: 'top'
	});

	$effect(() => {
		if (!anchorRef) return;

		truncated =
			anchorRef.scrollWidth > anchorRef.clientWidth ||
			anchorRef.scrollHeight > anchorRef.clientHeight;
	});

	export { truncated };
</script>

<p
	use:tooltip.content
	class={[
		'bg-blue max-w-md rounded-lg px-2 py-1 text-sm break-words text-white dark:text-black',
		classes.tooltip
	]}
>
	{tooltipText || text}
</p>

<span
	bind:this={anchorRef}
	use:tooltip.anchor
	class={['line-clamp-1 text-start break-words', className]}
>
	{text}
</span>
