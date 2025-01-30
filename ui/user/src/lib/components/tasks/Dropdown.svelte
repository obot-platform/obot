<script lang="ts">
	import { ChevronDown } from 'lucide-svelte/icons';
	import { popover } from '$lib/actions';
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

	async function select(value: string) {
		await onSelected?.(value);
		toggle();
	}
</script>

{#if disabled}
	<span class="flex items-center gap-2 rounded-3xl p-3 px-4 capitalize">
		{selected ? values[selected] : values[''] || ''}
	</span>
{:else}
	<button
		use:ref
		onclick={() => {
			toggle();
		}}
		class={twMerge(
			'flex items-center gap-2 rounded-3xl p-3 px-4 capitalize hover:bg-gray-70 dark:hover:bg-gray-900',
			kclass
		)}
	>
		{selected ? values[selected] : values[''] || ''}
		<ChevronDown />
	</button>
	<div use:tooltip class="z-30 min-w-[150px] rounded-3xl bg-white shadow dark:bg-gray-900">
		<ul>
			{#each Object.keys(values) as key}
				{@const value = values[key]}
				<li>
					<button
						class:bg-gray-70={selected === key}
						class:dark:bg-gray-800={selected === key}
						class="w-full px-6 py-2.5 text-start capitalize hover:bg-gray-100 dark:hover:bg-gray-800"
						onclick={() => select(key)}
					>
						{value}
					</button>
				</li>
			{/each}
		</ul>
	</div>
{/if}

<style lang="postcss">
	li:first-child button {
		@apply rounded-t-3xl pt-4;
	}
	li:last-child button {
		@apply rounded-b-3xl pb-4;
	}
</style>
