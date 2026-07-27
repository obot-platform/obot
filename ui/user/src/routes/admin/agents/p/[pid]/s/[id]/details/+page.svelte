<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte.js';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import McpServerDetails from '$lib/components/mcp/McpServerDetails.svelte';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import Loading from '$lib/icons/Loading.svelte';
	import { NanobotService, UserService, type OrgUser } from '$lib/services';
	import { profile } from '$lib/stores';
	import { getUserDisplayName } from '$lib/utils';
	import { HatGlasses } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let { mcpServer, agent } = $derived(data);
	let loading = $state(false);
	let users = $state<OrgUser[]>([]);
	let confirmImpersonate = $state(false);
	let launchingAgentId = $state<string | null>(null);

	let usersMap = $derived(new Map(users.map((u) => [u.id, u])));

	onMount(async () => {
		if (!mcpServer) return;
		loading = true;
		users = await UserService.listUsersIncludeDeleted();
		loading = false;
	});
	let user = $derived(mcpServer?.userID ? usersMap.get(mcpServer.userID) : undefined);
	let userDisplayName = $derived(getUserDisplayName(usersMap, mcpServer?.userID ?? ''));
	let title = $derived(userDisplayName ? `${userDisplayName}'s Agent` : 'Agent Details');

	async function impersonate() {
		if (!agent) return;
		launchingAgentId = agent.id;
		try {
			await NanobotService.launchProjectAgent(agent.projectID, agent.id);
			window.open(`/agent?projectId=${agent.projectID}&agentId=${agent.id}`, '_blank');
		} catch (error) {
			console.error('Failed to launch agent:', error);
		} finally {
			launchingAgentId = null;
			confirmImpersonate = false;
		}
	}
</script>

<Layout {title} showBackButton>
	{#snippet rightNavActions()}
		<button
			class="btn btn-primary flex items-center gap-1 text-sm"
			onclick={() => (confirmImpersonate = true)}
			use:tooltip={profile.current.canImpersonate?.() && agent?.userID !== profile.current.id
				? undefined
				: agent?.userID === profile.current.id
					? { text: 'You cannot impersonate yourself.', disablePortal: true }
					: { text: 'You do not have permission to impersonate other users.', disablePortal: true }}
			disabled={!profile.current.canImpersonate?.() || agent?.userID === profile.current.id}
		>
			<HatGlasses class="size-4" /> Connect as User
		</button>
	{/snippet}

	<div class="flex flex-col gap-6 pb-8" in:fly={{ x: 100, delay: PAGE_TRANSITION_DURATION }}>
		{#if loading}
			<div class="flex w-full justify-center">
				<Loading class="size-6" />
			</div>
		{:else}
			<div class="flex flex-col gap-6">
				{#if mcpServer}
					<McpServerDetails
						entity="agent"
						entityId={DEFAULT_MCP_CATALOG_ID}
						server={mcpServer}
						connectedUsers={user ? [user] : []}
						readonly={profile.current.isAdminReadonly?.()}
						k8sOverrides={{
							title: 'Details',
							classes: {
								title: 'text-lg font-semibold'
							}
						}}
					/>
				{/if}
			</div>
		{/if}
	</div>
</Layout>

<Confirm
	show={confirmImpersonate}
	oncancel={() => (confirmImpersonate = false)}
	onsuccess={impersonate}
	type="info"
	title="Confirm Agent Connection"
	msg={`Connect as ${userDisplayName}?`}
	loading={Boolean(launchingAgentId)}
>
	{#snippet note()}
		<p>
			This will allow you to connect to the agent, impersonating as <b class="font-semibold"
				>{userDisplayName}</b
			>. Any actions you take will be attributed to this user. Are you sure you wish to continue?
		</p>

		<p class="text-muted-content mt-4 text-sm">Note: This will open in a new window.</p>
	{/snippet}
</Confirm>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
