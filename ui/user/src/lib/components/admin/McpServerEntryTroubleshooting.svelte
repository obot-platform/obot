<script lang="ts">
	import Loading from '$lib/icons/Loading.svelte';
	import { UserService, type MCPCatalogEntry, type MCPCatalogServer } from '$lib/services';
	import { isMultiUserCatalogEntry } from '$lib/services/user/mcp';
	import { mcpServersAndEntries } from '$lib/stores';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import Select from '../Select.svelte';
	import ConnectToServer from '../mcp/ConnectToServer.svelte';
	import McpServerTools from '../mcp/McpServerTools.svelte';
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

	// for launch testing
	let connectToServerDialog = $state<ReturnType<typeof ConnectToServer>>();
	let launchError = $state('');
	let launchedServer = $state<MCPCatalogServer | undefined>(undefined);
	let testLaunchSuccessDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let pending = $state<'deleting' | 'refreshing' | undefined>(undefined);

	let selectedDebugOauthDeployment = $state<MCPCatalogServer | undefined>(undefined);
	let selectedDebugToolsDeployment = $state<MCPCatalogServer | undefined>(undefined);

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
		testLaunchSuccessDialog?.close();
		launchedServer = undefined;
		launchError = '';
		pending = undefined;
	}
</script>

{#if entry && 'isCatalogEntry' in entry}
	<div class="paper gap-2">
		<h1 class="text-lg font-semibold">Test Launch</h1>
		<p class="text-sm text-muted-content">
			This is to troubleshoot & verify the MCP catalog entry can be launched. Each launch will
			create a new deployment for this catalog entry.
		</p>
		<button
			class="btn btn-primary w-full"
			onclick={() => {
				connectToServerDialog?.setupNewInstance(entry!);
			}}
		>
			Test Launch
		</button>
	</div>

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

	<div class="paper">
		<h1 class="text-lg font-semibold">Debug Tools</h1>
		<div class="flex flex-col gap-2">
			<label for="debug-tools-deployment-selector" class="text-sm font-light">Deployment</label>
			<Select
				id="debug-tools-deployment-selector"
				classes={{
					root: 'w-full'
				}}
				class="bg-base-200 dark:bg-base-100"
				options={deploymentOptions}
				selected={selectedDebugToolsDeployment?.id}
				onSelect={(option) => {
					const match = selectableDeployments.find((server) => server.id === option.id);
					if (match) {
						selectedDebugToolsDeployment = match;
					}
				}}
				placeholder="Select Deployment"
			/>
		</div>
		{#if selectedDebugToolsDeployment}
			{#key selectedDebugToolsDeployment.id}
				<div class="bg-base-200 dark:bg-base-100 shadow-inner p-2 rounded-md">
					<McpServerTools
						{entry}
						server={selectedDebugToolsDeployment}
						showToolNameIssues={entry.manifest?.runtime === 'composite'}
						debugMode
					>
						{#snippet noToolsContent()}
							<h4 class="text-muted-content text-lg font-semibold">No tools</h4>
							<p class="text-muted-content text-sm font-light">
								This deployment does not have any tools available.
							</p>
						{/snippet}
					</McpServerTools>
				</div>
			{/key}
		{/if}
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
		testLaunchSuccessDialog?.open();
	}}
	skipConnectDialog
	renderIntroText={({ entry }) => {
		if (isMultiUserCatalogEntry(entry)) {
			return 'You are about to launch a new server.';
		}
		return 'You are about to launch a new connection.';
	}}
	introTitle="Test Launch"
/>

<ResponsiveDialog
	title="Test Launch Successful"
	bind:this={testLaunchSuccessDialog}
	class="max-w-sm"
	hideClose
	disableClickOutside
>
	<div class="flex flex-col gap-2">
		{#if launchError}
			<div class="alert alert-error">
				<CircleAlert class="size-6 text-error" />
				<h4 class="text-md font-medium">MCP Server Launch Failed</h4>
				<p>{launchError}</p>
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
				testLaunchSuccessDialog?.close();
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
