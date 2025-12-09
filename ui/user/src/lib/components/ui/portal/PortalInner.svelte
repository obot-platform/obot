<script lang="ts">
	import { twMerge } from 'tailwind-merge';
	import { PortalController } from './portalController.svelte';

	const controller = PortalController.get();

	let { class: klass = '', children, ...restProps } = $props();

	const innerProps = $derived({
		...controller.setup.inner.attrs(),
		...restProps
	});
</script>

<div class={twMerge('relative flex-1', klass)} {...innerProps}>
	{@render children?.()}
</div>
