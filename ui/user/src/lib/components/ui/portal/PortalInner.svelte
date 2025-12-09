<script lang="ts">
	import { twMerge } from 'tailwind-merge';
	import { PortalController } from './portalController.svelte';

	const controller = PortalController.get();

	let { class: klass = '', children = undefined, ...restProps } = $props();

	const innerProps = $derived({
		...controller.innerProps(),
		...restProps
	});
</script>

<div class={twMerge('relative flex-1', klass)} {...innerProps}>
	{@render children?.()}
</div>
