<script lang="ts">
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import TabLayout from '$lib/components/TabLayout.svelte';
	import AuditLogsPageContent from '$lib/components/admin/audit-logs/AuditLogsPageContent.svelte';
	import LlmAuditLogsContent from '$lib/components/admin/audit-logs/LlmAuditLogsContent.svelte';
	import VirtualPageRoot from '$lib/components/ui/virtual-page/virtual-page-viewport.svelte';
	import { m } from '$lib/i18n';
	import { Group } from '$lib/services';
	import { profile } from '$lib/stores';
	import { goto } from '$lib/url';
	import { Plus, Settings } from '@lucide/svelte';
	import type { Component } from 'svelte';

	let mcpLogs = $state<ReturnType<typeof AuditLogsPageContent>>();
	let llmLogs = $state<ReturnType<typeof LlmAuditLogsContent>>();
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	let canManageMcpExports = $derived(
		profile.current.groups.includes(Group.ADMIN) || profile.current.groups.includes(Group.OWNER)
	);
</script>

<svelte:head>
	<title>{m.audit_usage_audit_logs_page_title()}</title>
</svelte:head>

<TabLayout
	title={m.nav_audit_logs()}
	defaultView="mcp"
	classes={{ childrenContainer: 'max-w-none' }}
	rightNavActions={navActions}
	main={{
		component: VirtualPageRoot as unknown as Component,
		props: { class: '', as: 'main', itemHeight: 56, overscan: 5 }
	}}
	views={profile.current.hasAdminAccess?.()
		? [
				{ label: 'MCP', value: 'mcp', content: mcp },
				{ label: m.audit_usage_audit_logs_model(), value: 'llm', content: llm }
			]
		: [{ label: 'MCP', value: 'mcp', content: mcp }]}
/>

{#snippet navActions(view: string)}
	{#if view === 'mcp' && canManageMcpExports}
		<button class="btn btn-secondary rounded-4xl" onclick={() => goto('/audit-logs/mcp/exports')}>
			<Settings class="size-4" />
			{m.audit_usage_audit_logs_manage_exports()}
		</button>
		<DotDotDot class="btn btn-block btn-primary w-fit text-sm" placement="bottom">
			{#snippet icon()}
				<span class="flex items-center justify-center gap-1">
					<Plus class="size-4" />
					{m.audit_usage_audit_logs_create_export()}
				</span>
			{/snippet}
			<button class="menu-button" onclick={() => mcpLogs?.handleExportRequest('export')}>
				{m.audit_usage_audit_logs_create_one_time_export()}
			</button>
			<button class="menu-button" onclick={() => mcpLogs?.handleExportRequest('scheduled')}>
				{m.audit_usage_audit_logs_create_export_schedule()}
			</button>
		</DotDotDot>
	{:else if view === 'llm' && !isAdminReadonly}
		<button class="btn btn-secondary rounded-4xl" onclick={() => goto('/audit-logs/llm/exports')}>
			<Settings class="size-4" />
			{m.audit_usage_audit_logs_manage_exports()}
		</button>
		<DotDotDot class="btn btn-block btn-primary w-fit text-sm" placement="bottom">
			{#snippet icon()}
				<span class="flex items-center justify-center gap-1">
					<Plus class="size-4" />
					{m.audit_usage_audit_logs_create_export()}
				</span>
			{/snippet}
			<button class="menu-button" onclick={() => llmLogs?.openExportForm('export')}>
				{m.audit_usage_audit_logs_create_one_time_export()}
			</button>
			<button class="menu-button" onclick={() => llmLogs?.openExportForm('scheduled')}>
				{m.audit_usage_audit_logs_create_export_schedule()}
			</button>
		</DotDotDot>
	{/if}
{/snippet}

{#snippet mcp()}
	<div class="flex min-h-full flex-col gap-8 pb-8">
		<AuditLogsPageContent bind:this={mcpLogs} />
	</div>
{/snippet}

{#snippet llm()}
	<div class="flex min-h-full flex-col gap-6">
		<LlmAuditLogsContent bind:this={llmLogs} />
	</div>
{/snippet}
