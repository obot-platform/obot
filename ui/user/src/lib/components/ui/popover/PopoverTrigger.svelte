<script lang="ts">
	import { twMerge } from 'tailwind-merge';
	import { PopoverController } from './controller.svelte';

	const controller = PopoverController.get();

	let { class: klass = '', children, ...restProps } = $props();

	const triggerProps = $derived({
		...controller?.setup.trigger.attrs(),
		...restProps
	});
</script>

<button {@attach controller?.setup?.trigger?.fn} class={twMerge('', klass)}  {...triggerProps}>
	{@render children?.({ popover: controller })}
</button>
