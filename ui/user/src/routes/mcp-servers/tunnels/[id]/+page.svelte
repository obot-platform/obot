<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import MCPTunnelConnectionStatus from '$lib/components/admin/MCPTunnelConnectionStatus.svelte';
	import MCPTunnelForm from '$lib/components/admin/MCPTunnelForm.svelte';
	import MCPTunnelSecretRevealDialog from '$lib/components/admin/MCPTunnelSecretRevealDialog.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { m } from '$lib/i18n';
	import { AdminService, type MCPTunnel, type TunnelConnection } from '$lib/services';
	import { profile } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { goto } from '$lib/url';
	import { onMount, untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	let {
		data
	}: {
		data: { mcpTunnel?: MCPTunnel; tunnelConnections?: TunnelConnection[] };
	} = $props();

	let mcpTunnel = $state<MCPTunnel | undefined>(untrack(() => data.mcpTunnel));
	let tunnelConnections = $state<TunnelConnection[] | undefined>(
		untrack(() => data.tunnelConnections)
	);
	let revealedTunnel = $state<MCPTunnel>();
	let deleting = $state(false);
	let rotating = $state(false);
	let showDeleteConfirm = $state(false);
	let showRotateConfirm = $state(false);
	let refreshingConnections = false;

	let isReadonly = $derived(profile.current.isAdminReadonly?.());
	let title = $derived(
		mcpTunnel?.manifest.displayName?.trim() || mcpTunnel?.id || m.mcps_tunnels_mcp_tunnel()
	);
	let tunnelConnection = $derived(
		tunnelConnections?.find((connection) => connection.name === mcpTunnel?.id)
	);
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
		const shouldRefresh = () => document.visibilityState === 'visible' && Boolean(mcpTunnel);
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

	async function rotateSecret() {
		if (!mcpTunnel) return;

		rotating = true;
		try {
			revealedTunnel = await AdminService.rotateMCPTunnelSecret(mcpTunnel.id);
			showRotateConfirm = false;
			tunnelConnections = tunnelConnections?.filter(
				(connection) => connection.name !== mcpTunnel?.id
			);
			mcpTunnel = await AdminService.getMCPTunnel(mcpTunnel.id);
		} finally {
			rotating = false;
		}
	}

	async function deleteTunnel() {
		if (!mcpTunnel) return;

		deleting = true;
		try {
			await AdminService.deleteMCPTunnel(mcpTunnel.id);
			success.add(m.mcps_tunnels_tunnel_deleted());
			goto('/mcp-servers?view=tunnels');
		} finally {
			deleting = false;
		}
	}
</script>

<Layout {title} showBackButton>
	<div class="h-full w-full" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		{#if mcpTunnel}
			<div class="flex flex-col gap-6">
				<MCPTunnelConnectionStatus
					connection={tunnelConnection}
					known={tunnelConnections !== undefined}
					detailed
				/>
				<MCPTunnelForm
					tunnel={mcpTunnel}
					readonly={isReadonly}
					onUpdate={(updated) => {
						mcpTunnel = updated;
						success.add(m.mcps_tunnels_tunnel_updated());
					}}
					onDelete={() => {
						showDeleteConfirm = true;
					}}
					onRotateSecret={() => {
						showRotateConfirm = true;
					}}
				/>
			</div>
		{/if}
	</div>
</Layout>

<Confirm
	msg={m.mcps_delete_named({ name: title })}
	note={m.mcps_tunnels_delete_tunnel_note()}
	show={showDeleteConfirm}
	loading={deleting}
	onsuccess={deleteTunnel}
	oncancel={() => (showDeleteConfirm = false)}
/>

<Confirm
	title={m.mcps_tunnels_rotate_tunnel_secret_title()}
	type="info"
	msg={m.mcps_tunnels_rotate_secret_for({ name: title })}
	note={m.mcps_tunnels_rotate_secret_note()}
	show={showRotateConfirm}
	loading={rotating}
	submitText={m.mcps_tunnels_rotate_secret()}
	onsuccess={rotateSecret}
	oncancel={() => (showRotateConfirm = false)}
/>

<MCPTunnelSecretRevealDialog
	tunnel={revealedTunnel}
	action="rotated"
	onClose={() => {
		revealedTunnel = undefined;
	}}
/>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
