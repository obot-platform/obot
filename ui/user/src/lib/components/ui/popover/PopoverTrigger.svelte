<script lang="ts">
	import { twMerge } from 'tailwind-merge';
	import { PopoverController } from './controller.svelte';

	const controller = PopoverController.get();

	let { class: klass = '', children, ...restProps } = $props();

	const triggerProps = $derived({
		...controller.triggerProps(),
		...restProps
	});
</script>

<button class={twMerge('', klass)} {...triggerProps}>
	{@render children?.({ popover: controller })}
</button>
