<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		ChatService,
		AdminService,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type MCPServerInstance
	} from '$lib/services';
	import { hasEditableConfiguration, requiresUserUpdate } from '$lib/services/chat/mcp';
	import { mcpServersAndEntries, profile, userDeviceSettings, version } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { goto } from '$lib/url';
	import DotDotDot from '../DotDotDot.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import Table from '../table/Table.svelte';
	import ConnectToServer from './ConnectToServer.svelte';
	import EditExistingDeployment from './EditExistingDeployment.svelte';
	import StaticOAuthConfigureModal from './StaticOAuthConfigureModal.svelte';
	import DebugOauthDialog from './oauth/DebugOauthDialog.svelte';
	import {
		KeyRound,
		MessageCircle,
		PencilLine,
		ReceiptText,
		RefreshCw,
		Server,
		ServerCog,
		StepForward,
		Trash2,
		Unplug,
		Bug
	} from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	type ServerSelectMode =
		| 'connect'
		| 'rename'
		| 'edit'
		| 'disconnect'
		| 'chat'
		| 'server-details'
		| 'restart';

	interface Props {
		server?: MCPCatalogServer;
		entry?: MCPCatalogEntry;
		loading?: boolean;
		instance?: MCPServerInstance;
		skipConnectDialog?: boolean;
		onConnect?: ({ server, entry }: { server?: MCPCatalogServer; entry?: MCPCatalogEntry }) => void;
		onOAuthConfigured?: () => void;
		promptInitialLaunch?: boolean;
		promptOAuthConfig?: boolean;
		connectOnly?: boolean;
		isProjectMcp?: boolean;
		readonly?: boolean;
		allowMultiUserServerConfigurationEdit?: boolean;
	}

	let {
		server,
		entry,
		instance: instanceProp,
		loading,
		skipConnectDialog,
		onConnect,
		onOAuthConfigured,
		promptInitialLaunch,
		isProjectMcp,
		promptOAuthConfig,
		connectOnly,
		readonly,
		allowMultiUserServerConfigurationEdit
	}: Props = $props();
	let connectToServerDialog = $state<ReturnType<typeof ConnectToServer>>();
	let editExistingDialog = $state<ReturnType<typeof EditExistingDeployment>>();
	let selectServerDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let selectServerMode = $state<ServerSelectMode>('connect');
	let launchDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let launchPromptHandled = $state(false);

	let oauthConfigModal = $state<ReturnType<typeof StaticOAuthConfigureModal>>();
	let oauthConfigPromptHandled = $state(false);
	let isInitialOAuthConfig = $state(false);
	let oauthConfiguredOverride = $state<boolean | undefined>(undefined);
	let debugOauthDialog = $state<ReturnType<typeof DebugOauthDialog>>();

	let disconnecting = $state(false);
	let restarting = $state(false);

	let instance = $derived(
		instanceProp ??
			(server && !server.catalogEntryID
				? mcpServersAndEntries.current.userInstances.find(
						(instance) => instance.mcpServerID === server.id
					)
				: undefined)
	);
	let configuredServers = $derived(
		entry
			? mcpServersAndEntries.current.userConfiguredServers.filter(
					(server) => server.catalogEntryID === entry.id
				)
			: []
	);
	let requiresUpdate = $derived(server && requiresUserUpdate(server));
	let canReauthenticate = $derived(
		server?.manifest.runtime === 'remote' && Object.keys(server.oauthMetadata ?? {}).length > 0
	);
	let canDebugOauth = $derived(canReauthenticate && userDeviceSettings.developerMode);
	let canConfigure = $derived(
		entry && (entry.manifest.runtime === 'composite' || hasEditableConfiguration(entry))
	);
	let belongsToComposite = $derived(Boolean(server && server.compositeName));
	let canEditMultiUserServerConfiguration = $derived(
		Boolean(
			server &&
			!server.catalogEntryID &&
			!readonly &&
			allowMultiUserServerConfigurationEdit &&
			instance &&
			(server.manifest.multiUserConfig?.userDefinedHeaders?.length ?? 0) > 0
		)
	);
	let showServerDetails = $derived(entry && !server && configuredServers.length > 0);
	let hasActions = $derived.by(() => {
		if (isProjectMcp) {
			return server && entry && hasEditableConfiguration(entry);
		}
		return Boolean(
			(entry && server) ||
			showServerDetails ||
			(server && instance) ||
			canEditMultiUserServerConfiguration
		);
	});
	let showDisconnectUser = $derived(
		entry && server && profile.current.isAdmin?.() && server.userID !== profile.current.id
	);
	// Look up canConnect from the store if not set on props (e.g., when entry/server loaded directly via API)
	let canConnect = $derived.by(() => {
		const entryCanConnect =
			entry?.canConnect ??
			mcpServersAndEntries.current.entries.find((e) => e.id === entry?.id)?.canConnect ??
			true;
		const serverCanConnect =
			server?.canConnect ??
			mcpServersAndEntries.current.servers.find((s) => s.id === server?.id)?.canConnect ??
			true;
		return entryCanConnect && serverCanConnect;
	});

	let requiresStaticOAuth = $derived(
		entry?.manifest?.runtime === 'remote' && entry?.manifest?.remoteConfig?.staticOAuthRequired
	);
	// Use entry's oauthCredentialConfigured status (from backend controller) by default,
	// with an override for when we update credentials locally
	let oauthConfigured = $derived(
		oauthConfiguredOverride !== undefined
			? oauthConfiguredOverride
			: !requiresStaticOAuth || entry?.oauthCredentialConfigured
	);

	function refresh() {
		if (entry) {
			mcpServersAndEntries.refreshUserConfiguredServers();
		} else if (!server?.catalogEntryID) {
			mcpServersAndEntries.refreshUserInstances();
		}
	}

	export function connect() {
		connectToServerDialog?.open({
			entry,
			server,
			instance
		});
	}

	$effect(() => {
		if (promptInitialLaunch && !launchPromptHandled) {
			launchPromptHandled = true;
			launchDialog?.open();

			// clear out the launch param
			const url = new URL(page.url);
			url.searchParams.delete('launch');
			goto(url, { replaceState: true });
		}
	});

	$effect(() => {
		if (promptOAuthConfig && !oauthConfigPromptHandled) {
			oauthConfigPromptHandled = true;
			isInitialOAuthConfig = true;
			oauthConfigModal?.open();

			// clear out the configure-oauth param
			const url = new URL(page.url);
			url.searchParams.delete('configure-oauth');
			goto(url, { replaceState: true });
		}
	});

	function handleShowSelectServerDialog(mode: ServerSelectMode = 'connect') {
		selectServerDialog?.open();
		selectServerMode = mode;
	}

	async function reauthenticateServer(item: MCPCatalogServer) {
		await ChatService.clearMcpServerOAuth(item.id);
		await connectToServerDialog?.authenticate(item, entry);
		refresh();
	}
</script>

<!-- Use class:hidden to avoid Svelte 5 production build with conditional DOM cleanup -->
<div class="contents" class:hidden={belongsToComposite}>
	<button
		class="btn btn-primary flex w-full items-center gap-1 text-sm disabled:cursor-not-allowed disabled:opacity-50 md:w-fit"
		class:hidden={!(
			(entry && !server) ||
			(server &&
				(!server.catalogEntryID || (server.catalogEntryID && server.userID === profile.current.id)))
		)}
		use:tooltip={{
			text: canConnect ? '' : 'See MCP Registries to grant connect access to this server'
		}}
		onclick={() => {
			if (entry && !server && configuredServers.length > 0) {
				if (configuredServers.length === 1) {
					connectToServerDialog?.open({
						entry,
						server: configuredServers[0]
					});
				} else {
					handleShowSelectServerDialog();
				}
			} else {
				connectToServerDialog?.open({
					entry,
					server,
					instance
				});
			}
		}}
		disabled={loading || !canConnect || (requiresStaticOAuth && oauthConfigured === false)}
	>
		{#if loading}
			<Loading class="size-4" />
		{:else}
			Connect To Server
		{/if}
	</button>

	<div class:hidden={loading || !hasActions}>
		<DotDotDot
			class={!connectOnly ? 'hover:bg-base-200 dark:hover:bg-base-300' : ''}
			disablePortal={connectOnly}
			classes={{ menu: 'min-w-48 p-0', popover: 'z-60' }}
		>
			{#snippet children({ toggle })}
				{@render serverActions(toggle)}
			{/snippet}
		</DotDotDot>
	</div>
</div>

<ConnectToServer
	bind:this={connectToServerDialog}
	userConfiguredServers={mcpServersAndEntries.current.userConfiguredServers}
	onConnect={(data) => {
		onConnect?.(data);
		refresh();
	}}
	{skipConnectDialog}
	hideActions={isProjectMcp}
/>

<EditExistingDeployment bind:this={editExistingDialog} onUpdateConfigure={refresh} />

<ResponsiveDialog
	class="bg-base-200 dark:bg-base-100"
	bind:this={selectServerDialog}
	title="Select Your Server"
>
	<Table
		data={configuredServers || []}
		fields={['name', 'created']}
		onClickRow={(d) => {
			selectServerDialog?.close();
			switch (selectServerMode) {
				case 'chat': {
					connectToServerDialog?.handleSetupChat(d);
					break;
				}
				case 'server-details': {
					if (profile.current?.hasAdminAccess?.()) {
						goto(
							resolve(
								entry?.powerUserWorkspaceID
									? `/admin/mcp-servers/w/${d.powerUserWorkspaceID}/c/${d.catalogEntryID}/instance/${d.id}`
									: `/admin/mcp-servers/c/${d.catalogEntryID}/instance/${d.id}`
							),
							{ replaceState: true }
						);
					} else {
						goto(resolve(`/mcp-servers/c/${d.catalogEntryID}/instance/${d.id}`));
					}
					break;
				}
				case 'rename': {
					editExistingDialog?.rename({
						server: d,
						entry
					});
					break;
				}
				case 'edit': {
					editExistingDialog?.edit({
						server: d,
						entry
					});
					break;
				}
				case 'restart': {
					ChatService.restartMcpServer(d.id);
					mcpServersAndEntries.refreshUserConfiguredServers();
					break;
				}
				case 'disconnect': {
					ChatService.deleteSingleOrRemoteMcpServer(d.id);
					mcpServersAndEntries.refreshUserConfiguredServers();
					break;
				}
				default:
					connectToServerDialog?.open({
						entry,
						server: d
					});
					break;
			}
		}}
		disablePortal
	>
		{#snippet onRenderColumn(property, d)}
			{#if property === 'name'}
				<div class="flex shrink-0 items-center gap-2">
					<div class="icon">
						{#if d.manifest.icon}
							<img src={d.manifest.icon} alt={d.manifest.name} class="size-6" />
						{:else}
							<Server class="size-6" />
						{/if}
					</div>
					<p class="flex items-center gap-2">
						{d.alias || d.manifest.name}
					</p>
				</div>
			{:else if property === 'created'}
				{formatTimeAgo(d.created).relativeTime}
			{/if}
		{/snippet}
		{#snippet actions()}
			<IconButton class="hover:dark:bg-base-100/50">
				<StepForward class="size-4" />
			</IconButton>
		{/snippet}
	</Table>
</ResponsiveDialog>

<ResponsiveDialog bind:this={launchDialog} animate="slide" class="md:max-w-sm">
	{#snippet titleContent()}
		{#if entry || server}
			{@const name = entry?.manifest.name ?? server?.manifest.name ?? 'MCP Server'}
			{@const imageUrl = entry?.manifest.icon || server?.manifest.icon}
			<div class="icon">
				{#if imageUrl}
					<img
						src={imageUrl}
						alt={entry?.manifest.name ?? server?.manifest.name ?? 'MCP Server'}
						class="size-6"
					/>
				{:else}
					<Server class="size-6" />
				{/if}
			</div>
			{name}
		{/if}
	{/snippet}
	<div class="flex grow flex-col gap-2 p-4 pt-0 md:p-0">
		<p class="text-center">
			{#if entry && entry.manifest.runtime === 'remote'}
				Your remote server details have been configured.
			{:else if entry}
				Your server details have been configured.
			{:else}
				Your server has been configured.
			{/if}
		</p>
		<p class="mb-2 text-center">Would you like to connect now?</p>
		<div class="flex grow"></div>
		<div class="flex flex-col gap-2">
			<button class="btn btn-secondary" onclick={() => launchDialog?.close()}>Skip</button>
			<button
				class="btn btn-primary"
				onclick={() => {
					launchDialog?.close();
					connectToServerDialog?.open({
						entry,
						server
					});
				}}>Connect To Server</button
			>
		</div>
	</div>
</ResponsiveDialog>

{#snippet serverActions(toggle: (value: boolean) => void)}
	{#if server && (server.userID === profile.current.id || canEditMultiUserServerConfiguration)}
		<div
			class="flex flex-col gap-1 p-2 {!isProjectMcp && 'bg-base-200'} {!isProjectMcp &&
				'rounded-t-xl'}"
		>
			{#if canEditMultiUserServerConfiguration}
				<button
					class="menu-button"
					onclick={() => {
						connectToServerDialog?.open({
							server,
							instance,
							configureInstance: true
						});
						toggle(false);
					}}
				>
					<ServerCog class="size-4" /> Edit Configuration
				</button>
			{/if}
			{#if !isProjectMcp && !connectOnly && version.current.disableLegacyChat !== true}
				<button
					class="menu-button"
					onclick={async () => {
						connectToServerDialog?.handleSetupChat(server, instance);
					}}
				>
					<MessageCircle class="size-4" /> Chat
				</button>
			{/if}
			{#if entry}
				{#if !isProjectMcp}
					<button
						class="menu-button"
						onclick={() => {
							editExistingDialog?.rename({
								server,
								entry
							});
						}}
					>
						<PencilLine class="size-4" /> Rename
					</button>
				{/if}
				{#if server && canReauthenticate}
					<button
						class="menu-button"
						onclick={async (e) => {
							e.stopPropagation();
							toggle(false);
							await reauthenticateServer(server);
						}}
					>
						<KeyRound class="size-4" /> Reauthenticate
					</button>
				{/if}
				{#if server && canDebugOauth}
					<button
						class="menu-button bg-yellow-500/10 text-yellow-500 hover:bg-yellow-500/30"
						onclick={async (e) => {
							e.stopPropagation();
							debugOauthDialog?.open(server);
							toggle(false);
						}}
					>
						<Bug class="size-4" /> Debug OAuth
					</button>
				{/if}
				{#if canConfigure}
					<button
						class={twMerge(
							'menu-button',
							requiresUpdate && 'bg-warning/10 text-warning hover:bg-warning/30'
						)}
						onclick={() => {
							editExistingDialog?.edit({
								server,
								entry
							});
						}}
					>
						<ServerCog class="size-4" /> Edit Configuration
					</button>
				{/if}
			{/if}
			{#if server}
				<button
					class="menu-button"
					disabled={restarting}
					onclick={async (e) => {
						e.stopPropagation();
						restarting = true;
						try {
							await ChatService.restartMcpServer(server.id);
							refresh();
						} finally {
							restarting = false;
							toggle(false);
						}
					}}
				>
					{#if restarting}
						<Loading class="size-4" />
					{:else}
						<RefreshCw class="size-4" />
					{/if} Restart
				</button>
			{/if}
			{#if server && instance}
				<button
					class="menu-button"
					disabled={disconnecting}
					onclick={async (e) => {
						e.stopPropagation();
						disconnecting = true;
						await ChatService.deleteMcpServerInstance(instance.id);
						mcpServersAndEntries.refreshUserInstances();
						toggle(false);

						if (profile.current.hasAdminAccess?.()) {
							goto(
								resolve(
									entry?.powerUserWorkspaceID
										? `/admin/mcp-servers/w/${server.powerUserWorkspaceID}/s/${server.id}`
										: `/admin/mcp-servers/s/${server.id}`
								),
								{ replaceState: true }
							);
						} else {
							goto(resolve(`/mcp-servers/c/${server.id}`), { replaceState: true });
						}
						disconnecting = false;
					}}
				>
					{#if disconnecting}
						<Loading class="size-4" />
					{:else}
						<Unplug class="size-4" />
					{/if} Disconnect
				</button>
			{:else if entry && server && !isProjectMcp}
				<button
					class="menu-button"
					disabled={disconnecting}
					onclick={async (e) => {
						e.stopPropagation();
						disconnecting = true;
						await ChatService.deleteSingleOrRemoteMcpServer(server.id);
						mcpServersAndEntries.refreshUserConfiguredServers();
						toggle(false);

						if (profile.current.hasAdminAccess?.()) {
							goto(
								resolve(
									entry?.powerUserWorkspaceID
										? `/admin/mcp-servers/w/${entry.powerUserWorkspaceID}/c/${entry.id}`
										: `/admin/mcp-servers/c/${entry.id}`
								),
								{ replaceState: true }
							);
						} else {
							goto(resolve(`/mcp-servers/c/${entry.id}`), { replaceState: true });
						}
						disconnecting = false;
					}}
				>
					{#if disconnecting}
						<Loading class="size-4" />
					{:else}
						<Trash2 class="size-4" />
					{/if} Disconnect
				</button>
			{/if}
		</div>
	{:else if entry && configuredServers.length > 0}
		<div
			class="bg-base-100 dark:bg-base-300 rounded-t-xl p-2 pl-4 text-[11px] font-semibold uppercase"
		>
			My Connection(s)
		</div>
		<div class="bg-base-200 flex flex-col gap-1 p-2">
			{#if !connectOnly && version.current.disableLegacyChat !== true}
				<button
					class="menu-button"
					onclick={() => {
						if (configuredServers.length === 1) {
							connectToServerDialog?.handleSetupChat(configuredServers[0]);
						} else {
							handleShowSelectServerDialog('chat');
						}
					}}
				>
					<MessageCircle class="size-4" /> Chat
				</button>
			{/if}
			{#if entry}
				<button
					class="menu-button"
					onclick={() => {
						if (configuredServers.length === 1) {
							editExistingDialog?.rename({
								server: configuredServers[0],
								entry
							});
						} else {
							handleShowSelectServerDialog('rename');
						}
					}}
				>
					<PencilLine class="size-4" /> Rename
				</button>
				{#if canConfigure}
					<button
						class={twMerge(
							'menu-button',
							requiresUpdate && 'bg-warning/10 text-warning hover:bg-warning/30'
						)}
						onclick={() => {
							if (configuredServers.length === 1) {
								editExistingDialog?.edit({
									server: configuredServers[0],
									entry
								});
							} else {
								handleShowSelectServerDialog('edit');
							}
						}}
					>
						<ServerCog class="size-4" /> Edit Configuration
					</button>
				{/if}
			{/if}
			{#if configuredServers.length > 0}
				<button
					class="menu-button"
					disabled={restarting}
					onclick={async (e) => {
						e.stopPropagation();
						if (configuredServers.length === 1) {
							restarting = true;
							try {
								await ChatService.restartMcpServer(configuredServers[0].id);
								refresh();
							} finally {
								restarting = false;
								toggle(false);
							}
						} else {
							handleShowSelectServerDialog('restart');
						}
					}}
				>
					{#if restarting}
						<Loading class="size-4" />
					{:else}
						<RefreshCw class="size-4" />
					{/if} Restart
				</button>
			{/if}
			<button
				class="menu-button"
				onclick={() => {
					if (configuredServers.length === 1) {
						if (profile.current.hasAdminAccess?.()) {
							goto(
								resolve(
									entry?.powerUserWorkspaceID
										? `/admin/mcp-servers/w/${entry.powerUserWorkspaceID}/c/${entry.id}/instance/${configuredServers[0].id}`
										: `/admin/mcp-servers/c/${entry.id}/instance/${configuredServers[0].id}`
								),
								{ replaceState: true }
							);
						} else {
							goto(resolve(`/mcp-servers/c/${entry.id}/instance/${configuredServers[0].id}`), {
								replaceState: true
							});
						}
					} else {
						handleShowSelectServerDialog('server-details');
					}
				}}
			>
				<ReceiptText class="size-4" /> Server Details
			</button>
			<button
				class="menu-button"
				onclick={async (e) => {
					e.stopPropagation();
					if (configuredServers.length === 1) {
						await ChatService.deleteSingleOrRemoteMcpServer(configuredServers[0].id);
						mcpServersAndEntries.refreshUserConfiguredServers();
					} else {
						handleShowSelectServerDialog('disconnect');
					}
					toggle(false);
				}}
			>
				<Unplug class="size-4" /> Disconnect
			</button>
		</div>
	{/if}
	{#if showDisconnectUser && server}
		<div class="flex flex-col gap-2 p-2">
			<button
				class="menu-button text-error"
				onclick={async (e) => {
					e.stopPropagation();
					await ChatService.deleteSingleOrRemoteMcpServer(server.id);
					mcpServersAndEntries.refreshUserConfiguredServers();
					toggle(false);
				}}
			>
				<Trash2 class="size-4" /> Disconnect User
			</button>
		</div>
	{/if}
{/snippet}

<StaticOAuthConfigureModal
	bind:this={oauthConfigModal}
	onSave={async (credentials) => {
		if (!entry) return;
		if (entry.powerUserWorkspaceID) {
			await ChatService.setWorkspaceMCPCatalogEntryOAuthCredentials(
				entry.powerUserWorkspaceID,
				entry.id,
				credentials
			);
		} else {
			await AdminService.setMCPCatalogEntryOAuthCredentials('default', entry.id, credentials);
		}
		oauthConfiguredOverride = true;
		onOAuthConfigured?.();

		// Show the connect dialog if this was part of the initial creation flow
		if (isInitialOAuthConfig) {
			isInitialOAuthConfig = false;
			launchDialog?.open();
		}
	}}
/>

<DebugOauthDialog bind:this={debugOauthDialog} />
