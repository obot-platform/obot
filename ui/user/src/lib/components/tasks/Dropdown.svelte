<script lang="ts">
	import { popover } from '$lib/actions';
	import { ChevronDown } from 'lucide-svelte/icons';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		class?: string;
		values: Record<string, string>;
		selected?: string;
		disabled?: boolean;
		onSelected?: (value: string) => void | Promise<void>;
	}

	const { ref, tooltip, toggle } = popover({
		placement: 'bottom-start'
	});
	let { values, selected, disabled = false, onSelected, class: kclass = '' }: Props = $props();
	let button = $state<HTMLButtonElement>();

	async function select(value: string) {
		await onSelected?.(value);
		toggle();
	}
</script>

{#if disabled}
	<span
		class={twMerge(
			'text-muted-content bg-base-100 flex items-center justify-between gap-2 p-3 px-4 capitalize',
			kclass
		)}
	>
		{selected ? values[selected] : values[''] || ''}
		<ChevronDown class="text-muted-content" />
	</span>
{:else}
	<button
		bind:this={button}
		use:ref
		type="button"
		onclick={() => {
			toggle();
		}}
		class={twMerge(
			'flex items-center justify-between gap-2 rounded-sm p-3 px-4 capitalize',
			kclass
		)}
	>
		{selected ? values[selected] : values[''] || ''}
		<ChevronDown />
	</button>
	<div
		use:tooltip
		class="bg-base-100 min-w-[150px] rounded-sm shadow"
		style="width: {button?.getBoundingClientRect().width || 150}px;"
	>
		<ul>
			{#each Object.keys(values) as key (key)}
				{@const value = values[key]}
				<li>
					<button
						class:bg-base-200={selected === key}
						class="w-full px-6 py-2.5 text-start capitalize hover:bg-base-300"
						onclick={() => select(key)}
					>
						{value}
					</button>
				</li>
			{/each}
		</ul>
	</div>
{/if}
