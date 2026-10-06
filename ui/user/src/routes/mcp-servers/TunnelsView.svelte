<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import MCPTunnelConnectionStatus from '$lib/components/admin/MCPTunnelConnectionStatus.svelte';
	import MCPTunnelForm from '$lib/components/admin/MCPTunnelForm.svelte';
	import MCPTunnelSecretRevealDialog from '$lib/components/admin/MCPTunnelSecretRevealDialog.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { m } from '$lib/i18n';
	import { AdminService, type MCPTunnel, type TunnelConnection } from '$lib/services';
	import { mcpTunnelConnections, profile } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { goto } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import { Cable, Plus, Trash2 } from '@lucide/svelte';
	import { untrack } from 'svelte';

	interface Props {
		mcpTunnels?: MCPTunnel[];
		tunnelConnections?: TunnelConnection[];
	}

	let { mcpTunnels = $bindable([]), tunnelConnections }: Props = $props();

	let localTunnels = $state<MCPTunnel[]>(untrack(() => mcpTunnels));
	let tunnelToDelete = $state<MCPTunnel>();
	let createdTunnel = $state<MCPTunnel>();
	let deleting = $state(false);

	let connections = $derived(mcpTunnelConnections.current.connections ?? tunnelConnections);

	let showCreateTunnel = $derived(page.url.searchParams.has('new'));
	let isReadonly = $derived(profile.current.isAdminReadonly?.());
	let tableData = $derived.by(() => {
		const connectionsByName = new Map(
			(connections ?? []).map((connection) => [connection.name, connection])
		);

		return localTunnels.map((tunnel) => {
			const connection = connectionsByName.get(tunnel.id);
			return {
				...tunnel,
				allowedURLs: tunnel.manifest.allowedURLs?.join(', ') || '-',
				connection,
				displayName: tunnel.manifest.displayName?.trim() || tunnel.id,
				status:
					connections === undefined
						? m.core_unknown()
						: connection
							? m.core_mcp_value_connected()
							: m.mcps_tunnels_disconnected()
			};
		});
	});

	function createUrl() {
		return `${page.url.pathname}?view=tunnels&new=true`;
	}

	function createTunnel() {
		goto(createUrl());
	}

	function onTunnelCreated(tunnel: MCPTunnel) {
		createdTunnel = tunnel;
	}

	function closeCreatedTunnelDialog() {
		const tunnelID = createdTunnel?.id;
		createdTunnel = undefined;
		if (tunnelID) {
			goto(`/mcp-servers/tunnels/${tunnelID}`, { replaceState: true });
		}
	}
</script>

{#if showCreateTunnel}
	<MCPTunnelForm onCreate={onTunnelCreated} readonly={isReadonly} />
{:else if localTunnels.length === 0}
	<div class="mx-auto mt-12 flex w-md max-w-full flex-col items-center gap-4 text-center">
		<Cable class="text-muted-content size-24 opacity-25" />
		<h2 class="text-muted-content text-lg font-semibold">{m.mcps_tunnels_no_mcp_tunnels()}</h2>
		<p class="text-muted-content text-sm font-light">
			{m.mcps_tunnels_description_prefix()} <code class="font-mono">obot tunnel</code>
			{m.mcps_tunnels_description_suffix()}
		</p>
		{#if !isReadonly}
			<button class="btn btn-primary flex items-center gap-1 text-sm" onclick={createTunnel}>
				<Plus class="size-4" />
				{m.mcps_tunnels_create_mcp_tunnel()}
			</button>
		{/if}
	</div>
{:else}
	<div class="flex flex-col gap-6">
		<p class="text-muted-content text-sm">
			{m.mcps_tunnels_description_prefix()} <code class="font-mono">obot tunnel</code>
			{m.mcps_tunnels_description_suffix()}
		</p>
		<Table
			data={tableData}
			fields={['displayName', 'status', 'allowedURLs']}
			headers={[
				{ title: m.core_name(), property: 'displayName' },
				{ title: m.core_status(), property: 'status' },
				{ title: m.mcps_tunnels_allowed_urls(), property: 'allowedURLs' }
			]}
			filterable={['displayName', 'status']}
			sortable={['displayName', 'status']}
			noAutoHideFields={['displayName', 'status']}
			onClickRow={(tunnel, isCtrlClick) =>
				openUrl(`/mcp-servers/tunnels/${tunnel.id}`, isCtrlClick)}
		>
			{#snippet actions(tunnel)}
				{#if !isReadonly}
					<IconButton
						variant="danger"
						onclick={(event) => {
							event.stopPropagation();
							tunnelToDelete = tunnel;
						}}
						tooltip={{ text: m.mcps_tunnels_delete_tunnel() }}
					>
						<Trash2 class="size-4" />
					</IconButton>
				{/if}
			{/snippet}
			{#snippet onRenderColumn(property, tunnel)}
				{#if property === 'status'}
					<MCPTunnelConnectionStatus
						connection={tunnel.connection}
						known={connections !== undefined}
					/>
				{:else if property === 'allowedURLs'}
					<span class="line-clamp-2 break-all" title={tunnel.allowedURLs}>
						{tunnel.allowedURLs}
					</span>
				{:else}
					{String(tunnel[property as keyof typeof tunnel])}
				{/if}
			{/snippet}
		</Table>
	</div>
{/if}

<Confirm
	msg={tunnelToDelete?.manifest.displayName || tunnelToDelete?.id
		? m.mcps_delete_named({
				name: tunnelToDelete.manifest.displayName || tunnelToDelete.id
			})
		: m.mcps_tunnels_delete_this_tunnel()}
	note={m.mcps_tunnels_delete_tunnel_note()}
	show={Boolean(tunnelToDelete)}
	loading={deleting}
	onsuccess={async () => {
		if (!tunnelToDelete) return;
		deleting = true;
		try {
			await AdminService.deleteMCPTunnel(tunnelToDelete.id);
			localTunnels = localTunnels.filter((tunnel) => tunnel.id !== tunnelToDelete?.id);
			mcpTunnels = localTunnels;
			success.add(m.mcps_tunnels_tunnel_deleted());
			tunnelToDelete = undefined;
		} finally {
			deleting = false;
		}
	}}
	oncancel={() => (tunnelToDelete = undefined)}
/>

<MCPTunnelSecretRevealDialog
	tunnel={createdTunnel}
	action="created"
	onClose={closeCreatedTunnelDialog}
/>
