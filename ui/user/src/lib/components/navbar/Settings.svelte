<script lang="ts">
	import { Pencil } from 'lucide-svelte';
	import { getLayout } from '$lib/context/layout.svelte';

	const layout = getLayout();
</script>

<button
	class="icon-button hidden md:flex"
	onclick={() => (layout.projectEditorOpen = !layout.projectEditorOpen)}
>
	<Pencil class="icon-default" />
</button>
