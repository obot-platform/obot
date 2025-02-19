<script lang="ts">
	import { Settings } from 'lucide-svelte';
	import { context } from '$lib/stores';
</script>

<button class="icon-button hidden md:flex" onclick={() => (context.editMode = !context.editMode)}>
	<Settings class="icon-default" />
</button>
