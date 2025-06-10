<script lang="ts">
	import { twMerge } from 'tailwind-merge';

	type Props = {
		class?: string;
		value: 'am' | 'pm';
		onchange?: (value: Props['value']) => void;
		onclick?: (ev: MouseEvent) => void;
	};

	let { class: klass, value = $bindable(), onchange, ...restProps }: Props = $props();

	const items: { id: typeof value; text: string }[] = [
		{ id: 'am', text: 'AM' },
		{ id: 'pm', text: 'PM' }
	];
</script>

<div
	class={twMerge(
		'ampm-switch bg-surface1/0 border-surface2 leading-1 flex h-10 gap-1 rounded-full border p-1 text-xs',
		klass
	)}
	{...restProps}
>
	{#each items as { id, text } (id)}
		<button
			class={twMerge(
				'switch-option bg-surface2 aspect-square h-full rounded-md p-1',
				value === id && 'bg-blue/10 text-blue'
			)}
			onclick={() => {
				value = id;
				onchange?.(value);
			}}>{text}</button
		>
	{/each}
</div>
