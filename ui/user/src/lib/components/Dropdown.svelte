<script lang="ts">
	import { popover } from '$lib/actions';
	import type { Snippet } from 'svelte';

	interface Props {
		keys: string[];
		selected?: string;
		onSelected?: (value: string) => void | Promise<void>;
		display: Snippet<[string]>;
		option: Snippet<[string, { selected?: boolean; first?: boolean; last?: boolean }]>;
		setWidth?: boolean;
	}

	const { ref, tooltip, toggle } = popover({
		placement: 'bottom-start'
	});
	let { keys, selected, onSelected, display, option, setWidth = false }: Props = $props();
	let tt: HTMLDivElement;
	let button: HTMLButtonElement;

	async function select(value: string) {
		await onSelected?.(value);
		toggle();
	}

	function resize() {
		if (setWidth) {
			tt.style.width = button.getBoundingClientRect().width + 'px';
		}
	}
</script>

<button
	bind:this={button}
	use:ref
	onclick={() => {
		resize();
		toggle();
	}}
>
	{@render display(selected ?? '')}
</button>
<div use:tooltip bind:this={tt} class="max-h-full overflow-auto">
	<ul class="w-full">
		{#each keys as key, i}
			<li class="w-full">
				<button class="w-full" onclick={() => select(key)}>
					{@render option(key, {
						selected: selected === key,
						first: i === 0,
						last: i === keys.length - 1
					})}
				</button>
			</li>
		{/each}
	</ul>
</div>

<style lang="postcss">
</style>
