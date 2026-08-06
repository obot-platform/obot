<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import McpDeprecatedNotice from '$lib/components/mcp/McpDeprecatedNotice.svelte';
	import McpServerActions from '$lib/components/mcp/McpServerActions.svelte';
	import { VirtualPageViewport } from '$lib/components/ui/virtual-page';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { UserService } from '$lib/services';
	import { isDeprecatedMCPServer } from '$lib/services/user/mcp';
	import { mcpServersAndEntries } from '$lib/stores';
	import { type Component } from 'svelte';
	import { fly } from 'svelte/transition';

	const duration = PAGE_TRANSITION_DURATION;

	let { data } = $props();
	let { workspaceId, catalogEntry } = $derived(data);
	let title = $derived(catalogEntry?.manifest?.name ?? 'MCP Server');
	const hasExistingConfigured = $derived(
		Boolean(
			catalogEntry &&
			mcpServersAndEntries.current.userConfiguredServers.some(
				(server) => server.catalogEntryID === catalogEntry?.id
			)
		)
	);
	const configuredServers = $derived(
		catalogEntry
			? mcpServersAndEntries.current.userConfiguredServers.filter(
					(server) => server.catalogEntryID === catalogEntry?.id
				)
			: []
	);
	let promptOAuthConfig = $derived(page.url.searchParams.get('configure-oauth') === 'true');
	let deprecated = $derived(isDeprecatedMCPServer(catalogEntry));
</script>

<Layout
	main={{
		component: VirtualPageViewport as unknown as Component,
		props: { class: '', as: 'main', itemHeight: 56, overscan: 5, disabled: true }
	}}
	{title}
	showBackButton
>
	{#snippet rightNavActions()}
		<McpServerActions
			entry={catalogEntry}
			workspaceID={workspaceId}
			{promptOAuthConfig}
			onOAuthConfigured={() => {
				if (!catalogEntry) return;
				UserService.getMCP(catalogEntry.id).then((entry) => {
					catalogEntry = entry;
				});
			}}
		/>
	{/snippet}
	<div class="flex h-full flex-col gap-6" in:fly={{ x: 100, delay: duration, duration }}>
		<McpDeprecatedNotice {deprecated} variant="notification" />

		{#if catalogEntry}
			<McpServerEntryForm
				entry={catalogEntry}
				type={catalogEntry?.manifest.runtime === 'composite'
					? 'composite'
					: catalogEntry?.manifest.runtime === 'remote'
						? 'remote'
						: 'hosted'}
				readonly={catalogEntry && 'sourceURL' in catalogEntry && !!catalogEntry.sourceURL}
				id={workspaceId}
				entity="workspace"
				{hasExistingConfigured}
				{configuredServers}
				limitViews={['overview', 'tools']}
				connectOnly
			/>
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
