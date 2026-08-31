<script lang="ts">
	import TabLayout from '$lib/components/TabLayout.svelte';
	import AuditLogsPageContent from '$lib/components/admin/audit-logs/AuditLogsPageContent.svelte';
	import LlmAuditLogsContent from '$lib/components/admin/audit-logs/LlmAuditLogsContent.svelte';
	import VirtualPageRoot from '$lib/components/ui/virtual-page/virtual-page-viewport.svelte';
	import type { Component } from 'svelte';
</script>

<svelte:head>
	<title>Obot | Audit Logs</title>
</svelte:head>

<TabLayout
	title="Audit Logs"
	defaultView="mcp"
	classes={{ childrenContainer: 'max-w-none' }}
	main={{
		component: VirtualPageRoot as unknown as Component,
		props: { class: '', as: 'main', itemHeight: 56, overscan: 5 }
	}}
	views={[
		{ label: 'MCP', value: 'mcp', content: mcp },
		{ label: 'LLM', value: 'llm', content: llm },
		{ label: 'Chat', value: 'chat', content: chat }
	]}
/>

{#snippet mcp()}
	<div class="flex min-h-full flex-col gap-8 pb-8">
		<AuditLogsPageContent />
	</div>
{/snippet}

{#snippet llm()}
	<div class="flex min-h-full flex-col gap-6">
		<LlmAuditLogsContent />
	</div>
{/snippet}

{#snippet chat()}
	<!-- Chat audit logs -->
{/snippet}
