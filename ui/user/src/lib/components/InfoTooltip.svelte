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
	}

	let { text, class: klass, classes, placement }: Props = $props();
</script>

<div
	class={twMerge('size-3', klass)}
	use:tooltip={{
		text,
		disablePortal: true,
		classes: ['w-64', 'break-normal'],
		placement
	}}
>
	<CircleHelpIcon class={twMerge('text-gray size-3', classes?.icon)} />
</div>
