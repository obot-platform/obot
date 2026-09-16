<script lang="ts">
	import { resolve } from '$app/paths';
	import Table from '$lib/components/table/Table.svelte';
	import type { MCPCatalogEntry, OrgUser, VMCP, VMCPInstance } from '$lib/services';
	import { isDeprecatedMCPServer } from '$lib/services/user/mcp';
	import { vmcpComponentId, vmcpInstanceAuditLogsPath } from '$lib/services/vmcps/utils';
	import { mcpServersAndEntries, profile } from '$lib/stores';
	import { getUserDisplayName, openUrl } from '$lib/utils';
	import McpDeprecatedNotice from '../mcp/McpDeprecatedNotice.svelte';
	import { ChevronRight, CircleAlert, Server } from '@lucide/svelte';

	interface Props {
		vmcp: VMCP;
		instance: VMCPInstance;
		usersMap?: Map<string, OrgUser>;
		hideTitle?: boolean;
	}

	let { vmcp, instance, usersMap = new Map(), hideTitle }: Props = $props();

	let entriesMap = $derived(
		new Map(mcpServersAndEntries.current.entries.map((entry) => [entry.id, entry] as const))
	);
	let entriesReady = $derived(mcpServersAndEntries.current.isInitialized);
	let associatedUser = $derived(usersMap.get(instance.userID));
	let associatedUsers = $derived(associatedUser ? [associatedUser] : []);
	let hasAdminAccess = $derived(profile.current.hasAdminAccess?.() ?? false);

	function liveEntry(catalogEntryID: string): MCPCatalogEntry | undefined {
		return entriesMap.get(catalogEntryID);
	}

	function componentExists(catalogEntryID: string) {
		return Boolean(liveEntry(catalogEntryID));
	}

	function componentUsesSharedDeployment(component: VMCP['components'][number]) {
		if (component.forceSingleUser) return false;
		const remote = component.catalogEntry?.manifest.remoteConfig;
		if (remote && !remote.fixedURL && remote.hostname) return false;

		return !(component.configuration ?? []).some((policy) => {
			if (policy.policy !== 'userAllowed') return false;
			return !component.catalogEntry?.manifest.config?.some(
				(field) => field.key === policy.key && field.usage === 'header'
			);
		});
	}

	function openComponent(component: VMCP['components'][number], isCtrlClick: boolean) {
		const componentID = vmcpComponentId(component);
		if (!componentID) return;
		const shared = componentUsesSharedDeployment(component);
		const deploymentOwnerID = shared ? vmcp.id : instance.id;
		const deploymentID = `ms1${deploymentOwnerID}-${componentID}`;
		const path = shared
			? `/mcp-servers/s/${encodeURIComponent(deploymentID)}/details`
			: `/mcp-servers/c/${encodeURIComponent(component.mcpServerCatalogEntryID)}/instance/${encodeURIComponent(deploymentID)}/details`;
		const workspaceID =
			!shared && liveEntry(component.mcpServerCatalogEntryID)?.powerUserWorkspaceID;
		openUrl(workspaceID ? `${path}?wid=${encodeURIComponent(workspaceID)}` : path, isCtrlClick);
	}
</script>

{#if !hideTitle}
	<div class="flex items-center gap-3">
		<h1 class="text-2xl font-semibold">
			{vmcp.displayName}
		</h1>
		{#if instance.id}
			<span class="text-muted-content text-sm">({instance.id})</span>
		{/if}
	</div>
{/if}

{#if vmcp.components?.length}
	<div>
		<h2 class="mb-2 text-lg font-semibold">MCP Servers</h2>
		<div class="flex flex-col gap-2">
			{#each vmcp.components as component (vmcpComponentId(component) || component.mcpServerCatalogEntryID)}
				{@const catalogEntryID = component.mcpServerCatalogEntryID}
				{@const entry = liveEntry(catalogEntryID)}
				{@const exists = componentExists(catalogEntryID)}
				{@const deprecated = isDeprecatedMCPServer(component.catalogEntry)}
				{@const name = component.name || entry?.manifest.name || catalogEntryID}
				{@const icon = component.catalogEntry?.manifest?.icon || entry?.manifest.icon}

				{#if exists}
					<button
						onclick={(e) => {
							openComponent(component, e.metaKey || e.ctrlKey);
						}}
						class="group dark:bg-base-200 dark:border-base-400 dark:hover:bg-base-200 bg-base-100 flex items-center justify-between gap-2 rounded-lg border border-transparent p-2 pl-4 shadow-sm hover:bg-gray-50"
					>
						<div class="flex items-center gap-2">
							<div class="icon">
								{#if icon}
									<img src={icon} alt={name} class="size-6" />
								{:else}
									<Server class="size-6" />
								{/if}
							</div>
							<p class="text-sm">{name}</p>
							<McpDeprecatedNotice {deprecated} child />
							{#if catalogEntryID}
								<span class="text-muted-content text-sm">({catalogEntryID})</span>
							{/if}
						</div>
						<div
							class="size-10 shrink-0 flex items-center justify-center text-muted-content group-hover:text-base-content"
						>
							<ChevronRight class="size-6" />
						</div>
					</button>
				{:else}
					<div
						class="dark:bg-base-200 dark:border-base-400 bg-base-100 flex items-center justify-between gap-2 rounded-lg border border-transparent p-2 pl-4 opacity-60 shadow-sm"
					>
						<div class="flex items-center gap-2">
							<div class="icon">
								{#if icon}
									<img src={icon} alt={name} class="size-6" />
								{:else}
									<Server class="size-6" />
								{/if}
							</div>
							<p class="text-sm">{name}</p>
							<McpDeprecatedNotice {deprecated} child />
							{#if !entriesReady}
								<span class="text-muted-content text-xs">Loading...</span>
							{:else}
								<span
									class="text-muted-content flex items-center gap-1 text-xs"
									title="This component server no longer exists"
								>
									<CircleAlert class="size-4" />
									<span>Deleted</span>
								</span>
							{/if}
						</div>
						<div class="size-10 shrink-0"></div>
					</div>
				{/if}
			{/each}
		</div>
	</div>
{/if}

{#if associatedUsers.length > 0}
	<div>
		<h2 class="mb-2 text-lg font-semibold">Associated User</h2>
		<Table data={associatedUsers} fields={['name']}>
			{#snippet onRenderColumn(property: string, d: OrgUser)}
				{#if property === 'name'}
					{getUserDisplayName(usersMap, d.id)}
				{:else}
					{d[property as keyof typeof d]}
				{/if}
			{/snippet}

			{#snippet actions(d)}
				{#if hasAdminAccess}
					<a
						href={resolve(vmcpInstanceAuditLogsPath(vmcp.id, d.id) as `/${string}`)}
						class="btn btn-link"
					>
						View Audit Logs
					</a>
				{/if}
			{/snippet}
		</Table>
	</div>
{/if}
