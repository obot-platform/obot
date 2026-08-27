<script lang="ts">
	import { Server } from '@lucide/svelte';

	interface Props {
		icon?: string;
	}

	let { icon }: Props = $props();
</script>

{#if icon}
	<div class="relative bg-primary/10 text-primary rounded-md p-2 shrink-0">
		<img
			src={icon}
			alt=""
			width="20"
			height="20"
			loading="lazy"
			decoding="async"
			class="size-5 rounded-md object-contain pointer-events-none shrink-0"
		/>
	</div>
{:else}
	<div class="bg-primary/10 text-primary rounded-md p-2 shrink-0">
		<Server class="size-5" />
	</div>
{/if}
