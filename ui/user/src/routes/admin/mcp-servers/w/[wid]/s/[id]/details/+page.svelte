<script lang="ts">
	import { page } from '$app/state';
	import McpServerK8sInfo from '$lib/components/admin/McpServerK8sInfo.svelte';
	import BackLink from '$lib/components/BackLink.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { AdminService, ChatService, type MCPServerInstance, type OrgUser } from '$lib/services';
	import { LoaderCircle } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let { mcpServer, workspaceId } = data;
	let loading = $state(false);
	let users = $state<OrgUser[]>([]);
	let instances = $state<MCPServerInstance[]>([]);
	let usersMap = $derived(new Map(users.map((u) => [u.id, u])));

	onMount(async () => {
		if (!mcpServer) return;
		loading = true;
		instances = await ChatService.listWorkspaceMcpCatalogServerInstances(workspaceId, mcpServer.id);
		users = await AdminService.listUsersIncludeDeleted();
		loading = false;
	});
</script>

<Layout>
	<div class="mt-6 flex flex-col gap-6 pb-8" in:fly={{ x: 100, delay: PAGE_TRANSITION_DURATION }}>
		{#if mcpServer?.id}
			{@const currentLabel = mcpServer?.manifest.name ?? 'Server'}
			{@const from = page.url.searchParams.get('from') ?? `/deployed-servers`}
			<BackLink fromURL={from} {currentLabel} />
		{/if}

		{#if loading}
			<div class="flex w-full justify-center">
				<LoaderCircle class="size-6 animate-spin" />
			</div>
		{:else}
			<div class="flex flex-col gap-6">
				{#if mcpServer}
					<McpServerK8sInfo
						id={workspaceId}
						entity="workspace"
						mcpServerId={mcpServer.id}
						name={mcpServer.manifest.name || ''}
						connectedUsers={(instances ?? []).map((instance) => {
							const user = usersMap.get(instance.userID)!;
							return {
								...user,
								mcpInstanceId: instance.id
							};
						})}
						title="Details"
						classes={{
							title: 'text-lg font-semibold'
						}}
					/>
				{/if}
			</div>
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {mcpServer?.manifest.name}</title>
</svelte:head>
