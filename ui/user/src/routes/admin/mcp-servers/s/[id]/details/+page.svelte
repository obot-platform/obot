<script lang="ts">
	import McpServerK8sInfo from '$lib/components/admin/McpServerK8sInfo.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import McpServerActions from '$lib/components/mcp/McpServerActions.svelte';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { AdminService, type MCPServerInstance, type OrgUser } from '$lib/services';
	import { profile } from '$lib/stores/index.js';
	import { LoaderCircle } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let { mcpServer } = data;
	let loading = $state(false);
	let users = $state<OrgUser[]>([]);
	let instances = $state<MCPServerInstance[]>([]);
	let usersMap = $derived(new Map(users.map((u) => [u.id, u])));

	onMount(async () => {
		if (!mcpServer) return;
		loading = true;
		instances = await AdminService.listMcpCatalogServerInstances(
			DEFAULT_MCP_CATALOG_ID,
			mcpServer.id
		);
		users = await AdminService.listUsersIncludeDeleted();
		loading = false;
	});
	let title = $derived(mcpServer?.manifest.name);
	let instance = $derived(instances.find((instance) => instance.userID === profile.current.id));
</script>

<Layout {title} showBackButton>
	{#snippet rightNavActions()}
		<McpServerActions
			server={mcpServer}
			{instance}
			{loading}
			onConnect={({ instance: connectedInstance }) => (instance = connectedInstance)}
		/>
	{/snippet}

	<div class="flex flex-col gap-6 pb-8" in:fly={{ x: 100, delay: PAGE_TRANSITION_DURATION }}>
		{#if loading}
			<div class="flex w-full justify-center">
				<LoaderCircle class="size-6 animate-spin" />
			</div>
		{:else}
			<div class="flex flex-col gap-6">
				{#if mcpServer}
					<McpServerK8sInfo
						id={DEFAULT_MCP_CATALOG_ID}
						entity="catalog"
						mcpServerId={mcpServer.id}
						name={mcpServer.manifest.name || ''}
						connectedUsers={(instances ?? []).map((instance) => {
							const user = usersMap.get(instance.userID)!;
							return {
								...user,
								mcpInstanceId: instance.id
							};
						})}
						title={mcpServer.manifest.name}
						readonly={profile.current.isAdminReadonly?.()}
					/>
				{/if}
			</div>
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
