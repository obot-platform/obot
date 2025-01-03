<script lang="ts">
	import { term } from '$lib/stores';
	import { Terminal } from 'lucide-svelte';
</script>

<button
	class="icon-button z-20"
	onclick={() => {
		term.open = !term.open;
	}}
	type="button"
>
	<Terminal class="h-5 w-5" />
</button>
