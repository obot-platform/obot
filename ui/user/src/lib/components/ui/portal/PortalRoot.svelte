<script lang="ts">
	import { twMerge } from 'tailwind-merge';
	import { PortalController } from './portalController.svelte';

	let { class: klass = '', id, children, ...restProps } = $props();

	const controller = new PortalController(() => ({ id })).share();

	const rootProps = $derived({
		...controller.rootProps(),
		...restProps
	});
</script>

<div
	class={twMerge('pointer-events-none absolute inset-0 z-10 flex', klass)}
	{...rootProps}
	data-id={id}
>
	{@render children?.()}
</div>
