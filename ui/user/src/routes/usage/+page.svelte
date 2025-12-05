<script lang="ts">
	import { fade } from 'svelte/transition';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import UsageGraphs from '$lib/components/admin/usage/UsageGraphs.svelte';

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout classes={{ container: '', childrenContainer: 'max-w-none' }} showUserLinks title="Usage">
	<div class="mb-4 flex h-dvh min-h-full flex-col gap-8 pb-8" in:fade={{ duration }}>
		<UsageGraphs mcpId={null} mcpServerCatalogEntryName={null} mcpServerDisplayName={null} />
	</div>
</Layout>
