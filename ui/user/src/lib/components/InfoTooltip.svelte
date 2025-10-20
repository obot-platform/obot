<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import type { Placement } from '@floating-ui/dom';
	import { CircleHelpIcon } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		text: string;
		class?: string;
		classes?: {
			icon?: string;
		};
		placement?: Placement;
		popoverWidth?: 'sm' | 'md' | 'lg';
	}

	let { text, class: klass, classes, placement, popoverWidth = 'md' }: Props = $props();

	function getPopoverWidth() {
		switch (popoverWidth) {
			case 'sm':
				return 'w-48';
			case 'md':
				return 'w-64';
			case 'lg':
				return 'w-96';
			default:
				return 'w-64';
		}
	}
</script>

<div
	class={twMerge('size-3', klass)}
	use:tooltip={{
		text,
		disablePortal: true,
		classes: [getPopoverWidth(), 'break-normal'],
		placement
	}}
>
	<CircleHelpIcon class={twMerge('text-gray size-3', classes?.icon)} />
</div>
