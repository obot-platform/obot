<script lang="ts">
	import { Settings } from 'lucide-svelte';
	import { getLayout } from '$lib/context/layout.svelte';

	const layout = getLayout();
</script>

<button
	class="icon-button hidden md:flex"
	onclick={() => (layout.projectEditorOpen = !layout.projectEditorOpen)}
>
	<Settings class="icon-default" />
</button>
