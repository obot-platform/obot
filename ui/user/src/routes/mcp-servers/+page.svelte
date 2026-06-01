<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { Group } from '$lib/services';
	import { mcpServersAndEntries, profile } from '$lib/stores/index';
	import { setUrlParamAndUpdateUrl } from '$lib/url';
	import ConnectorsView from './ConnectorsView.svelte';
	import { debounce } from 'es-toolkit';
	import { Server } from 'lucide-svelte';
	import { fade, fly } from 'svelte/transition';

	let { data } = $props();

	let workspaceId = $derived(data.workspace?.id);
	let isAtLeastPowerUser = $derived(profile.current.groups.includes(Group.POWERUSER));

	let query = $derived(page.url.searchParams.get('query') || '');

	const updateSearchQuery = debounce((value: string) => {
		setUrlParamAndUpdateUrl(page.url, 'query', value);
	}, 100);

	const duration = PAGE_TRANSITION_DURATION;
	let title = 'MCP Servers';
</script>

<Layout classes={{ navbar: 'bg-base-200', container: 'pt-0' }} {title}>
	<div class="flex min-h-full flex-col gap-8" in:fade>
		{@render mainContent()}
	</div>
</Layout>

{#snippet mainContent()}
	<div
		class="flex flex-col"
		in:fly={{ x: 100, delay: duration, duration }}
		out:fly={{ x: -100, duration }}
	>
		<div class="bg-base-200 dark:bg-base-100 sticky top-16 left-0 z-20 w-full py-1">
			<div class="mb-2">
				<Search
					class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
					value={query}
					onChange={updateSearchQuery}
					placeholder="Search servers..."
				/>
			</div>
		</div>
		<ConnectorsView
			id={workspaceId}
			entity="workspace"
			{query}
			onConnect={({ instance }) => {
				if (instance) {
					mcpServersAndEntries.refreshUserInstances();
				} else {
					mcpServersAndEntries.refreshUserConfiguredServers();
				}
			}}
		>
			{#snippet noDataContent()}
				<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
					<Server class="text-base-content/80 size-24 opacity-25" />
					<h4 class="text-muted-content text-lg font-semibold">No created MCP servers</h4>
					<p class="text-muted-content text-sm font-light">
						{#if isAtLeastPowerUser}
							Looks like you don't have any servers created yet. <br />
							Go to
							<a
								href={resolve(
									`/${profile.current.hasAdminAccess?.() ? 'admin' : 'manage'}/mcp-servers`
								)}
								class="text-link">Manage ▸ MCP Servers</a
							> to get started.
						{:else}
							There are no servers available to connect to yet. <br />
							Please check back later or contact your administrator.
						{/if}
					</p>
				</div>
			{/snippet}
		</ConnectorsView>
	</div>
{/snippet}

<svelte:head>
	<title>Obot | MCP Servers</title>
</svelte:head>
