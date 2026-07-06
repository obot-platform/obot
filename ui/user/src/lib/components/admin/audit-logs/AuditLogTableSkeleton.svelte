<script lang="ts">
	import { fade } from 'svelte/transition';
</script>

<div in:fade={{ duration: 100 }}>
	<div class="flex flex-col gap-0.5">
		<div class="min-h-10 skeleton rounded-md"></div>
		{#each Array.from({ length: 5 }) as _, i (i)}
			<div class="min-h-14 skeleton rounded-md"></div>
		{/each}
	</div>
</div>
