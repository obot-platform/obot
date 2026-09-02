<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import MCPTunnelConnectionStatus from '$lib/components/admin/MCPTunnelConnectionStatus.svelte';
	import MCPTunnelForm from '$lib/components/admin/MCPTunnelForm.svelte';
	import MCPTunnelSecretRevealDialog from '$lib/components/admin/MCPTunnelSecretRevealDialog.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
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
				status: connections === undefined ? 'Unknown' : connection ? 'Connected' : 'Disconnected'
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
		<h2 class="text-muted-content text-lg font-semibold">No MCP tunnels</h2>
		<p class="text-muted-content text-sm font-light">
			MCP tunnels let Obot securely connect to private MCP servers that are not directly reachable
			from Obot's network. You run a lightweight <code class="font-mono">obot tunnel</code>
			process on the private network and it opens an authenticated outbound connection to Obot.
		</p>
		{#if !isReadonly}
			<button class="btn btn-primary flex items-center gap-1 text-sm" onclick={createTunnel}>
				<Plus class="size-4" />
				Create MCP Tunnel
			</button>
		{/if}
	</div>
{:else}
	<div class="flex flex-col gap-6">
		<p class="text-muted-content text-sm">
			MCP tunnels let Obot securely connect to private MCP servers that are not directly reachable
			from Obot's network. You run a lightweight <code class="font-mono">obot tunnel</code>
			process on the private network and it opens an authenticated outbound connection to Obot.
		</p>
		<Table
			data={tableData}
			fields={['displayName', 'status', 'allowedURLs']}
			headers={[
				{ title: 'Name', property: 'displayName' },
				{ title: 'Status', property: 'status' },
				{ title: 'Allowed URLs', property: 'allowedURLs' }
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
						tooltip={{ text: 'Delete Tunnel' }}
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
	msg={`Delete ${tunnelToDelete?.manifest.displayName || tunnelToDelete?.id || 'this tunnel'}?`}
	note="The tunnel cannot be deleted while any MCP catalog entries use it. If deleted, its active connection will be disconnected."
	show={Boolean(tunnelToDelete)}
	loading={deleting}
	onsuccess={async () => {
		if (!tunnelToDelete) return;
		deleting = true;
		try {
			await AdminService.deleteMCPTunnel(tunnelToDelete.id);
			localTunnels = localTunnels.filter((tunnel) => tunnel.id !== tunnelToDelete?.id);
			mcpTunnels = localTunnels;
			success.add('MCP tunnel deleted successfully.');
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
