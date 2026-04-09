<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import UsageGraphs from '$lib/components/admin/usage/UsageGraphs.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { fade } from 'svelte/transition';

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout classes={{ container: '', childrenContainer: 'max-w-none' }} title="Usage">
	<div class="mb-4 flex h-dvh min-h-full flex-col gap-8 pb-8" in:fade={{ duration }}>
		<UsageGraphs mcpId={null} mcpServerCatalogEntryName={null} mcpServerDisplayName={null} />
	</div>
</Layout>
