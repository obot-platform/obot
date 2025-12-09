<script lang="ts">
	import { twMerge } from 'tailwind-merge';
	import { PopoverController } from './controller.svelte';
	import { ChevronDown } from 'lucide-svelte';

	const controller = PopoverController.get();

	let { class: klass } = $props();

	const isOpen = $derived(controller.props.open ?? false);
</script>

<div
	class={twMerge(
		'flex size-5 items-center justify-center transition-transform duration-200',
		klass
	)}
>
	<ChevronDown class={twMerge('size-full', isOpen ? 'rotate-180' : 'rotate-0')} />
</div>
