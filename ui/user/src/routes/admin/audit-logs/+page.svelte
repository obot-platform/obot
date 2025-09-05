<script lang="ts">
	import type { Component } from 'svelte';
	import { fade } from 'svelte/transition';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import AuditLogsPageContent from '$lib/components/admin/audit-logs/AuditLogsPageContent.svelte';
	import VirtualPageRoot from '$lib/components/ui/virtual-page/virtual-page-viewport.svelte';

	const duration = PAGE_TRANSITION_DURATION;
</script>

<svelte:head>
	<title>Obot | Audit Logs</title>
</svelte:head>

<Layout
	classes={{ childrenContainer: 'max-w-none', container: '' }}
	main={{
		component: VirtualPageRoot as unknown as Component,
		props: { class: '', as: 'main', itemHeight: 56, overscan: 5 }
	}}
>
	<div class="flex-1" in:fade={{ duration }} out:fade={{ duration }}>
		<div class="flex min-h-full flex-col gap-8 pb-8">
			<div class="flex items-center justify-between gap-4">
				<h1 class="text-2xl font-semibold">Audit Logs</h1>
			</div>

			<AuditLogsPageContent />
		</div>
	</div>
</Layout>
