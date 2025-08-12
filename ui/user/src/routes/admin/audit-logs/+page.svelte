<script lang="ts">
	import { fade } from 'svelte/transition';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import AuditLogsPageContent from '$lib/components/admin/audit-logs/AuditLogsPageContent.svelte';

	const duration = PAGE_TRANSITION_DURATION;
</script>

<svelte:head>
	<title>Obot | Audit Logs</title>
</svelte:head>

<Layout classes={{ childrenContainer: 'max-w-none' }}>
	<div class="my-4 h-dvh" in:fade={{ duration }} out:fade={{ duration }}>
		<div class="flex min-h-full flex-col gap-8 pb-8">
			<div class="flex items-center justify-between gap-4">
				<h1 class="text-2xl font-semibold">Audit Logs</h1>
			</div>

			<AuditLogsPageContent />
		</div>
	</div>
</Layout>
