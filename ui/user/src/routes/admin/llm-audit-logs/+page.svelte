<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import LlmAuditLogsContent from '$lib/components/admin/audit-logs/LlmAuditLogsContent.svelte';
	import VirtualPageRoot from '$lib/components/ui/virtual-page/virtual-page-viewport.svelte';
	import type { Component } from 'svelte';
	import { fade } from 'svelte/transition';
</script>

<svelte:head>
	<title>Obot | LLM Audit Logs</title>
</svelte:head>

<Layout
	title="Audit Logs"
	subtitle="LLM Gateway"
	classes={{ childrenContainer: 'max-w-none', container: '' }}
	main={{
		component: VirtualPageRoot as unknown as Component,
		props: { class: '', as: 'main', itemHeight: 56, overscan: 5 }
	}}
>
	<div class="flex-1" in:fade={{ duration: 100 }}>
		<div class="flex min-h-full flex-col gap-6 pb-8">
			<LlmAuditLogsContent />
		</div>
	</div>
</Layout>
