<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import type { ClassValue } from 'svelte/elements';
	import { twMerge } from 'tailwind-merge';

	type Props = {
		tooltipText?: string;
		text: string;
		class?: ClassValue;
		classes?: { tooltip?: string };
		disabled?: boolean;
	};
	let { text, class: className, classes = {}, tooltipText, disabled }: Props = $props();
	let anchorRef = $state<HTMLElement>();
	let truncated = $state(false);

	$effect(() => {
		if (!anchorRef) return;

		truncated =
			anchorRef.scrollWidth > anchorRef.clientWidth ||
			anchorRef.scrollHeight > anchorRef.clientHeight;
	});

	export { truncated };
</script>

<span
	bind:this={anchorRef}
	use:tooltip={{
		text: tooltipText || text,
		className: twMerge('tooltip', classes?.tooltip),
		disabled
	}}
	class={['line-clamp-1 break-all text-start', className]}
>
	{text}
</span>
