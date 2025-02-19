<script lang="ts">
	import { CircleHelpIcon } from 'lucide-svelte';
	import type { Snippet } from 'svelte';
	import { popover } from '$lib/actions';

	let { children }: { children: Snippet } = $props();
	let { ref, tooltip } = popover({
		placement: 'right',
		offset: 0,
		hover: true
	});
</script>

<div use:ref class="w-4">
	<CircleHelpIcon class="h-4 w-4 text-gray" />
</div>

<div use:tooltip class="m-4">
	<p
		class="rounded-lg bg-gray-800 p-2 text-sm text-gray-50 shadow dark:bg-gray-100 dark:text-black"
	>
		{@render children()}
	</p>
</div>
