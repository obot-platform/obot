<script lang="ts">
	import { twMerge } from 'tailwind-merge';

	interface Props {
		type: 'table' | 'card' | 'list';
		class?: string;
		classes?: {
			header?: string;
			body?: string;
		};
		count?: number;
	}

	let { type, class: klass, classes, count = 5 }: Props = $props();
</script>

{#if type === 'table'}
	<div class={twMerge('flex flex-col gap-0.5', klass)}>
		<div class={twMerge('skeleton h-9 w-full rounded-md', classes?.header)}></div>
		{#each Array.from({ length: count }) as _, i (i)}
			<div class={twMerge('skeleton h-14 w-full rounded-md', classes?.body)}></div>
		{/each}
	</div>
{:else if type === 'list'}
	<div class={twMerge('pt-2 flex flex-col gap-4', klass)}>
		{#each Array.from({ length: count }) as _, i (i)}
			<div class="flex gap-2 items-center w-full">
				<div class="size-8 rounded-md skeleton shrink-0"></div>
				<div class="flex flex-col gap-2 flex-1">
					<div class="h-4 w-full rounded-md skeleton"></div>
					<div class="h-3 w-full rounded-md skeleton"></div>
				</div>
			</div>
		{/each}
	</div>
{:else}
	<div class={twMerge('skeleton rounded-md', klass)}></div>
{/if}
