<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import Search from '$lib/components/Search.svelte';
	import { NanobotService } from '$lib/services/index.js';
	import type { OrgUser } from '$lib/services/admin/types';
	import { profile } from '$lib/stores/index.js';
	import { ExternalLink, LoaderCircle } from 'lucide-svelte';
	import { untrack } from 'svelte';

	let { data } = $props();
	let agents = $state(untrack(() => data.agents));
	let users = $state<OrgUser[]>(untrack(() => data.users));
	let query = $state('');
	let launchingAgentId = $state<string | null>(null);

	const userMap = $derived(new Map(users.map((u) => [u.id, u])));

	function getOwnerDisplay(userID: string): string {
		const user = userMap.get(userID);
		if (!user) return userID;
		return user.email || user.username || userID;
	}

	const tableData = $derived(
		agents
			.filter((agent) => agent.userID !== profile.current?.id)
			.map((agent) => ({
				...agent,
				ownerDisplay: getOwnerDisplay(agent.userID)
			}))
			.filter((agent) =>
				agent.ownerDisplay.toLowerCase().includes(query.toLowerCase())
			)
	);

	type TableItem = (typeof tableData)[0];

	async function handleConnect(agent: TableItem) {
		launchingAgentId = agent.id;
		try {
			await NanobotService.launchProjectV2Agent(agent.projectV2ID, agent.id);
			window.open(
				`/agent?projectId=${agent.projectV2ID}&agentId=${agent.id}`,
				'_blank'
			);
		} catch (error) {
			console.error('Failed to launch agent:', error);
		} finally {
			launchingAgentId = null;
		}
	}
</script>

<Layout title="User Impersonation">
	<div class="flex flex-col gap-4">
		<div class="flex items-center justify-between">
			<p class="text-sm text-gray-500">
				Browse and connect to agents across all users. Clicking "Connect" will open the agent's chat
				interface in a new tab.
			</p>
		</div>

		<Search
			value={query}
			onChange={(v) => (query = v)}
			placeholder="Search by owner..."
		/>

		<Table
			data={tableData}
			fields={['ownerDisplay', 'projectV2ID']}
			headers={[
				{ title: 'Owner', property: 'ownerDisplay' },
				{ title: 'Project ID', property: 'projectV2ID' }
			]}
			sortable={['ownerDisplay']}
			noDataMessage="No agents found."
		>
			{#snippet actions(agent)}
				<button
					class="button-primary flex items-center gap-1 text-sm"
					onclick={() => handleConnect(agent)}
					disabled={launchingAgentId === agent.id}
				>
					{#if launchingAgentId === agent.id}
						<LoaderCircle class="h-4 w-4 animate-spin" />
						Launching...
					{:else}
						<ExternalLink class="h-4 w-4" />
						Connect
					{/if}
				</button>
			{/snippet}
		</Table>
	</div>
</Layout>
