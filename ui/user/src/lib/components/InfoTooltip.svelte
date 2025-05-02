<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { CircleHelpIcon } from 'lucide-svelte';

	let { text }: { text: string } = $props();
</script>

<div class="size-3" use:tooltip={text}>
	<CircleHelpIcon class="text-gray size-3" />
</div>
