<script lang="ts">
	import { type MCPCatalogEntry, type MCPCatalogServer } from '$lib/services';
	import { isMultiUserCatalogEntry } from '$lib/services/user/mcp';
	import { mcpServersAndEntries } from '$lib/stores';
	import { profile } from '$lib/stores';
	import Select from '../Select.svelte';
	import DebugOauthFlow from '../mcp/oauth/DebugOauthFlow.svelte';
	import { slide } from 'svelte/transition';

	interface Props {
		entry?: MCPCatalogEntry | MCPCatalogServer;
		server?: MCPCatalogServer;
		servers?: MCPCatalogServer[];
	}

	let { entry, server: restrictedSingleDeployment, servers }: Props = $props();

	let selectedDebugOauthDeployment = $state<MCPCatalogServer | undefined>();

	let selectableDeployments = $derived(
		servers
			? servers
			: mcpServersAndEntries.current.userConfiguredServers.filter((deployment) => {
					if (isMultiUserCatalogEntry(entry)) {
						return deployment.catalogEntryID === entry?.id;
					}

					// single-tenant entry: just get deployments tied to the user
					return (
						deployment.catalogEntryID === entry?.id && deployment.userID === profile.current?.id
					);
				})
	);
	let deploymentOptions = $derived(
		selectableDeployments.map((server) => ({
			id: server.id,
			label: `${server.id}: ${server.alias || server.manifest.name}`
		}))
	);
</script>

{#if entry && 'isCatalogEntry' in entry && !restrictedSingleDeployment}
	{#if entry?.manifest.runtime === 'remote'}
		<div class="paper">
			<h1 class="text-lg font-semibold">Debug OAuth Flow</h1>

			<div class="flex flex-col gap-2">
				<label for="debug-oauth-deployment-selector" class="text-sm font-light">Deployment</label>
				<Select
					id="debug-oauth-deployment-selector"
					classes={{
						root: 'w-full'
					}}
					class="bg-base-200 dark:bg-base-100"
					options={deploymentOptions}
					selected={selectedDebugOauthDeployment?.id}
					onSelect={(option) => {
						const match = selectableDeployments.find((server) => server.id === option.id);
						if (match) {
							selectedDebugOauthDeployment = match;
						}
					}}
					placeholder="Select Deployment"
				/>
			</div>

			{#if deploymentOptions.length === 0}
				<div class="notification-info flex items-center gap-2">
					<p class="text-xs">Deploy this MCP server to begin debugging the OAuth flow.</p>
				</div>
			{/if}

			{#if selectedDebugOauthDeployment}
				<div
					in:slide={{ axis: 'y', duration: 150 }}
					class="bg-base-200 dark:bg-base-100 shadow-inner p-2 rounded-md"
				>
					<div class="flex flex-col bg-base-100 dark:bg-base-300 rounded-md pt-4">
						<DebugOauthFlow mcpServer={selectedDebugOauthDeployment} />
					</div>
				</div>
			{/if}
		</div>
	{/if}
{:else if (entry && !('isCatalogEntry' in entry)) || restrictedSingleDeployment}
	{@const mcpServer = restrictedSingleDeployment ?? (entry as MCPCatalogServer)}
	{#if mcpServer?.manifest.runtime === 'remote'}
		<div class="flex flex-col bg-base-100 dark:bg-base-300 rounded-md pt-4">
			<h1 class="text-lg font-semibold px-4 pb-2">Debug OAuth Flow</h1>
			<DebugOauthFlow {mcpServer} />
		</div>
	{/if}
{/if}
