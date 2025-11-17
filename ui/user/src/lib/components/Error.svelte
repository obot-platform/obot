<script lang="ts">
	import { fade } from 'svelte/transition';

	interface Props {
		onClick?: (() => void) | null;
		error: Error;
	}

	let { onClick = null, error }: Props = $props();
</script>

<!--error component-->
<div in:fade class="flex items-center justify-center">
	<div class="text-center">
		<h3 class="text-on-background text-2xl font-semibold">Error</h3>
		<p class="text-gray dark:text-gray-400">{error.message}</p>
		{#if onClick}
			<button class="text-on-background mt-4 hover:underline" onclick={onClick}>Try again</button>
		{/if}
	</div>
</div>
