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
			'hover:bg-gray-70 flex items-center gap-2 rounded-3xl p-3 px-4 capitalize dark:hover:bg-gray-900',
			kclass
		)}
	>
		{selected ? values[selected] : values[''] || ''}
		<ChevronDown />
	</button>
	<div use:tooltip class="min-w-[150px] rounded-3xl bg-white shadow dark:bg-gray-900">
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
		border-top-left-radius: 1.5rem;
		border-top-right-radius: 1.5rem;
		padding-top: 1rem;
	}
	li:last-child button {
		border-bottom-left-radius: 1.5rem;
		border-bottom-right-radius: 1.5rem;
		padding-bottom: 1rem;
	}
</style>
