<script lang="ts">
	import { twMerge } from 'tailwind-merge';

	type Props = {
		class?: string;
		value: 'am' | 'pm';
		onchange?: (value: Props['value']) => void;
		onclick?: (ev: MouseEvent) => void;
	};

	let { class: klass, value = $bindable(), onchange, ...restProps }: Props = $props();
</script>

<div
	class={twMerge(
		'ampm-switch bg-surface1/0 border-surface2 flex h-10 gap-1 rounded-full border p-1 text-xs leading-1',
		klass
	)}
	{...restProps}
>
	<button
		class={twMerge(
			'switch-option bg-surface2 aspect-square h-full rounded-full p-1',
			value === 'am' && 'bg-blue/10 text-blue'
		)}
		onclick={() => {
			value = 'am';
			onchange?.(value);
		}}>AM</button
	>

	<button
		class={twMerge(
			'switch-option bg-surface2/50 aspect-square h-full rounded-full p-1',
			value === 'pm' && 'bg-blue/10 text-blue'
		)}
		onclick={() => {
			value = 'pm';
			onchange?.(value);
		}}>PM</button
	>
</div>
