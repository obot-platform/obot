<script lang="ts">
	import Loading from '$lib/icons/Loading.svelte';
	import { UserService, type MCPCatalogEntry, type MCPCatalogServer } from '$lib/services';
	import {
		MCP_MULTI_TENANT_LAUNCH_TEXT,
		MCP_SINGLE_TENANT_LAUNCH_TEXT
	} from '$lib/services/admin/constants';
	import { isMultiUserCatalogEntry } from '$lib/services/user/mcp';
	import { mcpServersAndEntries } from '$lib/stores';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import Select from '../Select.svelte';
	import ConnectToServer from '../mcp/ConnectToServer.svelte';
	import DebugOauthFlow from '../mcp/oauth/DebugOauthFlow.svelte';
	import { CircleAlert } from '@lucide/svelte';
	import { slide } from 'svelte/transition';

	interface Props {
		catalogID?: string;
		entry?: MCPCatalogEntry | MCPCatalogServer;
		server?: MCPCatalogServer;
		onCreateServerForEntry?: (server: MCPCatalogServer) => void;
	}

	let { catalogID, entry, server, onCreateServerForEntry }: Props = $props();

	let connectToServerDialog = $state<ReturnType<typeof ConnectToServer>>();
	let launchError = $state('');
	let launchedServer = $state<MCPCatalogServer | undefined>(undefined);
	let launchSuccessDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let pending = $state<'deleting' | 'refreshing' | undefined>(undefined);

	let selectedDebugOauthDeployment = $state<MCPCatalogServer | undefined>(undefined);

	let selectableDeployments = $derived(
		mcpServersAndEntries.current.userConfiguredServers.filter(
			(server) => server.catalogEntryID === entry?.id
		)
	);
	let deploymentOptions = $derived(
		selectableDeployments.map((server) => ({
			id: server.id,
			label: `${server.id}: ${server.alias || server.manifest.name}`
		}))
	);

	function resetLaunchStates() {
		launchSuccessDialog?.close();
		launchedServer = undefined;
		launchError = '';
		pending = undefined;
	}
</script>

{#if entry && 'isCatalogEntry' in entry}
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

	<div class="paper gap-2">
		<h1 class="text-lg font-semibold">Launch Server</h1>
		<p class="text-sm text-muted-content">
			Each launch will create a new deployment for this catalog entry.
		</p>
		<button
			class="btn btn-primary w-full"
			onclick={() => {
				connectToServerDialog?.setupNewInstance(entry!);
			}}
		>
			Launch Server
		</button>
	</div>
{:else if (entry && !('isCatalogEntry' in entry)) || server}
	{@const mcpServer = entry || server}
	{#if mcpServer?.manifest.runtime === 'remote'}
		<div class="flex flex-col bg-base-100 dark:bg-base-300 rounded-md pt-4">
			<h1 class="text-lg font-semibold px-4 pb-2">Debug OAuth Flow</h1>
			<DebugOauthFlow {mcpServer} />
		</div>
	{/if}
{/if}

<ConnectToServer
	bind:this={connectToServerDialog}
	{catalogID}
	onConnect={({ server }) => {
		if (!server) {
			launchError = 'No server was launched';
			return;
		}

		launchError = '';
		launchedServer = server;
		launchSuccessDialog?.open();
	}}
	skipConnectDialog
	renderIntroText={({ entry }) =>
		isMultiUserCatalogEntry(entry) ? MCP_MULTI_TENANT_LAUNCH_TEXT : MCP_SINGLE_TENANT_LAUNCH_TEXT}
	introTitle="Launch Server"
/>

<ResponsiveDialog
	title={launchError ? 'Launch Failed' : 'Launch Successful'}
	bind:this={launchSuccessDialog}
	class="max-w-sm"
	hideClose
	disableClickOutside
>
	<div class="flex flex-col gap-2">
		{#if launchError}
			<div class="alert alert-error alert-soft">
				<CircleAlert class="size-4 text-error" />
				<span>{launchError}</span>
			</div>
		{:else}
			<p>The server was launched successfully.</p>
		{/if}

		<p class="mb-4">
			Feel free to delete the created deployment below. {entry?.manifest.runtime === 'remote' &&
				'Or use the existing deployment to test & debug the OAuth flow.'}
		</p>

		<button
			class="btn btn-error"
			disabled={!!pending}
			onclick={() => {
				if (launchedServer) {
					pending = 'deleting';
					try {
						UserService.deleteSingleOrRemoteMcpServer(launchedServer.id);
					} catch (_err) {
						// built-in error will display a toast
					}
					resetLaunchStates();
				}
			}}
		>
			{#if pending === 'deleting'}
				<Loading class="size-4" />
			{:else}
				Delete Deployment
			{/if}
		</button>
		<button
			class="btn btn-secondary"
			disabled={!!pending}
			onclick={async () => {
				launchSuccessDialog?.close();
				pending = 'refreshing';
				await mcpServersAndEntries.refreshUserConfiguredServers();
				if (launchedServer) {
					onCreateServerForEntry?.(launchedServer);
				}
				resetLaunchStates();
			}}
		>
			{#if pending === 'refreshing'}
				<Loading class="size-4" />
			{:else}
				Skip
			{/if}
		</button>
	</div>
</ResponsiveDialog>
