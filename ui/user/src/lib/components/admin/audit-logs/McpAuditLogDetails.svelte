<script lang="ts">
	import { type McpAuditLog } from '$lib/services';
	import { profile } from '$lib/stores';
	import AuditLogDetails from './AuditLogDetails.svelte';

	interface Props {
		auditLog: McpAuditLog & {
			user: string;
		};
		onClose: () => void;
	}

	let { auditLog, onClose }: Props = $props();

	const mcp = $derived(auditLog.mcpFields);
	const shouldShowPayload = $derived(
		!!mcp &&
			(profile?.current?.hasAdminAccess?.() ||
				(auditLog.userID === profile.current.id && !mcp.powerUserWorkspaceID))
	);

	const fields = [
		'callType',
		'sessionID',
		'mcpID',
		'mcpServerDisplayName',
		'mcpServerCatalogEntryName'
	] as const;
	const titles = {
		callType: 'Call Type',
		sessionID: 'Session ID',
		mcpID: 'MCP ID',
		mcpServerDisplayName: 'MCP Server Display Name',
		mcpServerCatalogEntryName: 'MCP Server Catalog Entry Name'
	};
</script>

<AuditLogDetails
	auditLog={{
		...mcp,
		...auditLog
	}}
	{shouldShowPayload}
	{onClose}
>
	{#snippet preRequestContent(data)}
		<div class="flex flex-wrap gap-2 p-4 pl-5">
			{#each fields as field (field)}
				{#if data[field as keyof typeof data]}
					<div class="bg-base-400 rounded-full px-3 py-1 text-[11px] font-light">
						<span class="font-medium">{titles[field as keyof typeof titles]}:</span>
						{data[field as keyof typeof data]}
					</div>
				{/if}
			{/each}
		</div>
	{/snippet}

	{#snippet additRequestContent(data)}
		{#if data?.apiKey}
			<p>
				<span class="font-medium">API Key</span>: {data.apiKey}***
				<span class="text-muted-content text-xs italic">(redacted)</span>
			</p>
		{/if}
		{#if data?.userAgent}
			<p><span class="font-medium">User Agent</span>: {data.userAgent}</p>
		{/if}
		{#if data?.client}
			<p>
				<span class="font-medium">Client</span>: {data.client.name}/{data.client.version}
			</p>
		{/if}
		{#if data.clientIP}
			<p><span class="font-medium">Client IP</span>: {auditLog.clientIP}</p>
		{/if}
	{/snippet}
</AuditLogDetails>
