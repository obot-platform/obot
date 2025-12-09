<script lang="ts">
	import { PopoverController } from './controller.svelte';
	import Teleport from '../teleport/Teleport.svelte';
	import { PortalController } from '../portal';
	import { twMerge } from 'tailwind-merge';

	const controller = PopoverController.get();
	const portal = PortalController.get();

	let { class: klass = '', children, ...restProps } = $props();

	const isOpen = $derived(controller.props.open ?? false);
	const position = $derived(controller.position);
	const offset = $derived(controller.props.offset);

	function animate(node: HTMLElement) {
		const direction = position?.placement.split('-')[0];
		const offsetValue = isOpen ? 0 : (offset ?? 0);

		requestAnimationFrame(() => {
			const transformOriginMap: Record<string, string> = {
				top: 'bottom',
				bottom: 'top',
				left: 'right',
				right: 'left'
			};
			node.style.transform = '';

			const sign = ['top', 'left'].includes(direction!) ? 1 : -1;
			const translate = ['top', 'bottom'].includes(direction!) ? 'translateY' : 'translateX';
			const scale = ['top', 'bottom'].includes(direction!) ? 'scaleY' : 'scaleX';

			node.style.transform = `${translate}(${sign * offsetValue}px) ${scale}( ${isOpen ? 1 : 0.98} )`;
			node.style.transformOrigin = transformOriginMap[direction!] ?? 'center';
			node.style.opacity = +isOpen + '';
		});
	}
</script>

<Teleport
	class={'absolute left-0 top-0 flex h-min w-fit'}
	target={portal.target}
	{...controller.setup.content.attrs()}
	{@attach (node: HTMLElement) => {
		if (!position) return;

		requestAnimationFrame(() => {
			node.style.left = position.x + 'px';
			node.style.top = position.y + 'px';
		});
	}}
>
	<div
		class={twMerge(
			'duration-50 flex flex-col transition-all',
			isOpen ? 'pointer-events-auto' : '',
			klass
		)}
		{...restProps}
		{@attach animate}
	>
		{@render children?.({ popover: controller })}
	</div>
</Teleport>
