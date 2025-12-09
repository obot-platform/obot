<script lang="ts">
	import type { Snippet } from 'svelte';
	import { PopoverController, type PopoverControllerProps } from './controller.svelte';
	import type { Placement } from '@floating-ui/dom';

	type Props = {
		open?: boolean;
		placement?: Placement;
		placements?: Placement[];
		offset?: number;
		children?: Snippet<[{ popover: PopoverController }]>;
	};
	let {
		open = $bindable(false),
		placement = 'bottom-end',
		placements = ['top', 'top-start', 'top-end', 'bottom', 'bottom-start', 'bottom-end'],
		offset = 1,
		children
	}: Props = $props();

	const proxy: PopoverControllerProps = {
		get open() {
			return open;
		},
		set open(value: boolean) {
			open = value;
		},
		get placement() {
			return placement;
		},
		get placements() {
			return placements;
		},
		get offset() {
			return offset;
		}
	};

	const controller = new PopoverController(() => proxy).share();
</script>

{@render children?.({ popover: controller })}
