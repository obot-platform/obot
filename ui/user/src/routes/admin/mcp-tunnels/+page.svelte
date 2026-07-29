<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import MCPTunnelConnectionStatus from '$lib/components/admin/MCPTunnelConnectionStatus.svelte';
	import MCPTunnelForm from '$lib/components/admin/MCPTunnelForm.svelte';
	import MCPTunnelSecretRevealDialog from '$lib/components/admin/MCPTunnelSecretRevealDialog.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { AdminService, type MCPTunnel, type TunnelConnection } from '$lib/services';
	import { profile } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { goto } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import { Cable, Plus, Trash2 } from '@lucide/svelte';
	import { onMount, untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	let {
		data
	}: {
		data: { mcpTunnels: MCPTunnel[]; tunnelConnections?: TunnelConnection[] };
	} = $props();

	let mcpTunnels = $state<MCPTunnel[]>(untrack(() => data.mcpTunnels));
	let tunnelConnections = $state<TunnelConnection[] | undefined>(
		untrack(() => data.tunnelConnections)
	);
	let tunnelToDelete = $state<MCPTunnel>();
	let createdTunnel = $state<MCPTunnel>();
	let deleting = $state(false);
	let refreshingConnections = false;

	let showCreateTunnel = $derived(page.url.searchParams.has('new'));
	let isReadonly = $derived(profile.current.isAdminReadonly?.());
	let title = $derived(showCreateTunnel ? 'Create MCP Tunnel' : 'MCP Tunnels');
	let tableData = $derived.by(() => {
		const connectionsByName = new Map(
			(tunnelConnections ?? []).map((connection) => [connection.name, connection])
		);

		return mcpTunnels.map((tunnel) => {
			const connection = connectionsByName.get(tunnel.id);
			return {
				...tunnel,
				allowedURLs: tunnel.manifest.allowedURLs?.join(', ') || '-',
				connection,
				displayName: tunnel.manifest.displayName?.trim() || tunnel.id,
				status:
					tunnelConnections === undefined ? 'Unknown' : connection ? 'Connected' : 'Disconnected'
			};
		});
	});
	const duration = PAGE_TRANSITION_DURATION;
	const CONNECTION_POLL_INTERVAL_MS = 5000;

	async function refreshTunnelConnections() {
		if (refreshingConnections) return;

		refreshingConnections = true;
		try {
			tunnelConnections = await AdminService.listTunnelConnections({ dontLogErrors: true });
		} catch {
			tunnelConnections = undefined;
		} finally {
			refreshingConnections = false;
		}
	}

	onMount(() => {
		const shouldRefresh = () =>
			document.visibilityState === 'visible' && mcpTunnels.length > 0 && !showCreateTunnel;
		if (shouldRefresh()) {
			void refreshTunnelConnections();
		}
		const interval = window.setInterval(() => {
			if (shouldRefresh()) {
				void refreshTunnelConnections();
			}
		}, CONNECTION_POLL_INTERVAL_MS);
		const handleVisibilityChange = () => {
			if (shouldRefresh()) {
				void refreshTunnelConnections();
			}
		};
		document.addEventListener('visibilitychange', handleVisibilityChange);

		return () => {
			window.clearInterval(interval);
			document.removeEventListener('visibilitychange', handleVisibilityChange);
		};
	});

	function createTunnel() {
		goto('/admin/mcp-tunnels?new=true');
	}

	function closeCreateForm() {
		goto('/admin/mcp-tunnels', { replaceState: true });
	}

	function onTunnelCreated(tunnel: MCPTunnel) {
		createdTunnel = tunnel;
	}

	function closeCreatedTunnelDialog() {
		const tunnelID = createdTunnel?.id;
		createdTunnel = undefined;
		if (tunnelID) {
			goto(`/admin/mcp-tunnels/${tunnelID}`, { replaceState: true });
		}
	}
</script>

<Layout
	{title}
	showBackButton={showCreateTunnel}
	onBackButtonClick={showCreateTunnel ? closeCreateForm : undefined}
>
	<div
		class="h-full w-full"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if showCreateTunnel}
			<MCPTunnelForm onCreate={onTunnelCreated} readonly={isReadonly} />
		{:else if mcpTunnels.length === 0}
			<div class="mx-auto mt-12 flex w-md max-w-full flex-col items-center gap-4 text-center">
				<Cable class="text-muted-content size-24 opacity-25" />
				<h2 class="text-muted-content text-lg font-semibold">No MCP tunnels</h2>
				<p class="text-muted-content text-sm font-light">
					MCP tunnels let Obot securely connect to private MCP servers that are not directly
					reachable from Obot's network. You run a lightweight <code class="font-mono"
						>obot tunnel</code
					>
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
				<p class="text-base-content/80 text-base">
					MCP tunnels let Obot securely connect to private MCP servers that are not directly
					reachable from Obot's network. You run a lightweight <code class="font-mono"
						>obot tunnel</code
					>
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
						openUrl(`/admin/mcp-tunnels/${tunnel.id}`, isCtrlClick)}
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
								known={tunnelConnections !== undefined}
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
	</div>

	{#snippet rightNavActions()}
		{#if !showCreateTunnel && !isReadonly}
			<button class="btn btn-primary flex items-center gap-1 text-sm" onclick={createTunnel}>
				<Plus class="size-4" />
				Create MCP Tunnel
			</button>
		{/if}
	{/snippet}
</Layout>

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
			mcpTunnels = mcpTunnels.filter((tunnel) => tunnel.id !== tunnelToDelete?.id);
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

<svelte:head>
	<title>Obot | MCP Tunnels</title>
</svelte:head>
