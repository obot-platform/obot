<script lang="ts">
	import UsageGraphs from '$lib/components/admin/usage/UsageGraphs.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { fade } from 'svelte/transition';

	const duration = PAGE_TRANSITION_DURATION;
</script>

<div class="mb-4 flex h-dvh min-h-full flex-col gap-8 pb-8" in:fade={{ duration }}>
	<UsageGraphs />
</div>
