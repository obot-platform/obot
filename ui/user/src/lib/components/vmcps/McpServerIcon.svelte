<script lang="ts">
	import { Server } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		icon?: string;
		width?: number;
		height?: number;
		class?: string;
		classes?: {
			root?: string;
		};
	}

	let { icon, width = 20, height = 20, class: klass, classes }: Props = $props();
</script>

{#if icon}
	<div
		class={twMerge('relative bg-primary/10 text-primary rounded-md p-2 shrink-0', classes?.root)}
	>
		<img
			src={icon}
			alt=""
			{width}
			{height}
			loading="lazy"
			decoding="async"
			class={twMerge('size-5 rounded-md object-contain pointer-events-none shrink-0', klass)}
		/>
	</div>
{:else}
	<div class="bg-primary/10 text-primary rounded-md p-2 shrink-0">
		<Server class="size-5" />
	</div>
{/if}
