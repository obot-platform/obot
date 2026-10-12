<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		UserService,
		AdminService,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type MCPServerInstance
	} from '$lib/services';
	import type { MCPServerOAuthCredentialStatus } from '$lib/services/admin/types';
	import { MCP_CONNECTION_INVALID_LICENSE_MESSAGE } from '$lib/services/user/constants';
	import {
		deleteMcpServerDeployment,
		disconnectMcpServerUser,
		hasEditableConfiguration,
		isDeprecatedMCPServer,
		isMultiUserCatalogEntry,
		isMultiUserServer,
		requiresUserUpdate,
		restartMcpServer,
		supportsMCPBackendDetails
	} from '$lib/services/user/mcp';
	import { mcpServersAndEntries, profile, version } from '$lib/stores';
	import { goto } from '$lib/url';
	import CopyField from '../CopyField.svelte';
	import DotDotDot from '../DotDotDot.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import ConnectToServer from './ConnectToServer.svelte';
	import EditExistingDeployment from './EditExistingDeployment.svelte';
	import McpDeprecatedNotice from './McpDeprecatedNotice.svelte';
	import McpSelectServerDeployment from './McpSelectServerDeployment.svelte';
	import StaticOAuthConfigureModal from './StaticOAuthConfigureModal.svelte';
	import DebugOauthDialog from './oauth/DebugOauthDialog.svelte';
	import {
		PencilLine,
		Plus,
		RefreshCw,
		Server,
		ServerCog,
		Trash2,
		Unplug,
		Bug
	} from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	type ServerSelectMode =
		| 'connect'
		| 'rename'
		| 'edit'
		| 'disconnect'
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
		readonly?: boolean;
		allowMultiUserServerConfigurationEdit?: boolean;
		catalogID?: string;
		workspaceID?: string;
		hideActions?: boolean;
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
		promptOAuthConfig,
		readonly,
		allowMultiUserServerConfigurationEdit,
		catalogID,
		workspaceID,
		hideActions
	}: Props = $props();
	let connectToServerDialog = $state<ReturnType<typeof ConnectToServer>>();
	let editExistingDialog = $state<ReturnType<typeof EditExistingDeployment>>();
	let selectServerDialog = $state<ReturnType<typeof McpSelectServerDeployment>>();
	let selectServerMode = $state<ServerSelectMode>('connect');
	let launchDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let launchPromptHandled = $state(false);

	let oauthConfigModal = $state<ReturnType<typeof StaticOAuthConfigureModal>>();
	let oauthConfigPromptHandled = $state(false);
	let oauthConfiguredOverride = $state<boolean | undefined>(undefined);
	let oauthStatus = $state<MCPServerOAuthCredentialStatus>();
	let debugOauthDialog = $state<ReturnType<typeof DebugOauthDialog>>();

	let disconnecting = $state(false);
	let restarting = $state(false);

	let instance = $derived(
		instanceProp ??
			(server && isMultiUserServer(server)
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
	let restartableConfiguredServers = $derived(
		configuredServers.filter((server) => supportsMCPBackendDetails(server))
	);
	// Connecting from a multi-user catalog entry row always starts a new shared server deployment.
	let isMultiUserCatalogEntryRow = $derived(isMultiUserCatalogEntry(entry) && !server);
	let requiresUpdate = $derived(server && requiresUserUpdate(server));
	let canDebugOauth = $derived(
		server?.manifest.runtime === 'remote' &&
			Object.keys(server.oauthMetadata ?? {}).length > 0 &&
			!hasEditableConfiguration(server) &&
			profile.current?.hasAdminAccess?.()
	);
	let deprecated = $derived(isDeprecatedMCPServer(entry) || isDeprecatedMCPServer(server));
	let configurableItem = $derived(server ?? entry);
	// True when the user can manage the server deployment (restart, rename, edit config).
	// For multi-user servers, only admins or the workspace owner who deployed it.
	let isServerOwner = $derived(
		!isMultiUserServer(server) ||
			profile.current.isAdmin?.() ||
			(server?.powerUserWorkspaceID && server?.userID === profile.current.id)
	);
	let canConfigure = $derived(
		configurableItem && hasEditableConfiguration(configurableItem) && isServerOwner
	);
	let canEditMultiUserServerConfiguration = $derived(
		Boolean(
			server &&
			isMultiUserServer(server) &&
			!readonly &&
			allowMultiUserServerConfigurationEdit &&
			instance &&
			((server.manifest.config ?? []).filter(
				(field) => field.usage === 'header' && field.userAllowed
			)?.length ?? 0) > 0
		)
	);
	let canDeleteMultiUserServer = $derived(
		Boolean(
			server &&
			isMultiUserServer(server) &&
			!readonly &&
			(profile.current.isAdmin?.() ||
				(server.powerUserWorkspaceID && server.userID === profile.current.id))
		)
	);
	let canManageServerDeployment = $derived(Boolean(server && isServerOwner));
	let canRestartServerDeployment = $derived(
		Boolean(server && canManageServerDeployment && supportsMCPBackendDetails(server))
	);
	let showServerDetails = $derived(
		entry &&
			!server &&
			supportsMCPBackendDetails(entry) &&
			configuredServers.some((s) => supportsMCPBackendDetails(s))
	);
	let hasMultiUserServerNotOwned = $derived(
		configuredServers.some(
			(s) =>
				(isMultiUserServer(s) || isMultiUserCatalogEntry(entry)) &&
				!profile.current.isAdmin?.() &&
				!(s.powerUserWorkspaceID && s.userID === profile.current.id)
		)
	);
	let hasConfiguredServerUserInstance = $derived(
		configuredServers.some((s) =>
			mcpServersAndEntries.current.userInstances.some((i) => i.mcpServerID === s.id)
		)
	);
	let showConnectionActions = $derived(
		!hasMultiUserServerNotOwned || hasConfiguredServerUserInstance
	);
	let canCreateAnotherMultiUserServer = $derived(
		isMultiUserCatalogEntry(entry) && (!!catalogID || !!workspaceID) && configuredServers.length > 0
	);
	let hasLicenseEntitlementViolations = $derived(
		(version.current.licenseEntitlementViolations || []).length > 0
	);
	let hasActions = $derived.by(() => {
		return Boolean(
			(entry && server && isServerOwner) ||
			(server && instance) ||
			(showServerDetails && showConnectionActions) ||
			canEditMultiUserServerConfiguration ||
			canCreateAnotherMultiUserServer
		);
	});
	let showDisconnectUser = $derived(
		entry &&
			server &&
			!isMultiUserServer(server) &&
			!isMultiUserCatalogEntry(entry) &&
			profile.current.isAdmin?.() &&
			server.userID !== profile.current.id
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
	let showConnectButton = $derived(
		!hideActions &&
			Boolean(
				(entry && !server) ||
				(server &&
					(isMultiUserServer(server) ||
						!server.catalogEntryID ||
						(server.catalogEntryID && server.userID === profile.current.id)))
			)
	);
	let connectionDisabled = $derived(
		Boolean(
			loading ||
			hasLicenseEntitlementViolations ||
			(isMultiUserCatalogEntryRow && !catalogID && !workspaceID) ||
			!canConnect ||
			(requiresStaticOAuth && oauthConfigured === false)
		)
	);

	function refresh() {
		if (entry) {
			mcpServersAndEntries.refreshAll();
		} else if (isMultiUserServer(server)) {
			mcpServersAndEntries.refreshUserInstances();
		}
	}

	async function restartServer(server: MCPCatalogServer) {
		await restartMcpServer(server, catalogID);
	}

	async function deleteServerDeployment(server: MCPCatalogServer) {
		await deleteMcpServerDeployment(server, catalogID);
	}

	async function disconnectCurrentUser(server: MCPCatalogServer) {
		await disconnectMcpServerUser(server);
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
			void openOAuthConfig();

			// clear out the configure-oauth param
			const url = new URL(page.url);
			url.searchParams.delete('configure-oauth');
			goto(url, { replaceState: true });
		}
	});

	async function openOAuthConfig() {
		if (!entry) return;
		try {
			oauthStatus = entry.powerUserWorkspaceID
				? await UserService.getWorkspaceMCPCatalogEntryOAuthCredentials(
						entry.powerUserWorkspaceID,
						entry.id
					)
				: await AdminService.getMCPCatalogEntryOAuthCredentials('default', entry.id);
		} catch {
			oauthStatus = { configured: Boolean(entry.oauthCredentialConfigured) };
		}
		oauthConfigModal?.open();
	}

	function handleShowSelectServerDialog(
		mode: ServerSelectMode = 'connect',
		servers: MCPCatalogServer[] = []
	) {
		if (!entry) return;
		selectServerMode = mode;
		selectServerDialog?.open(servers);
	}
</script>

<!-- Use class:hidden to avoid Svelte 5 production build with conditional DOM cleanup -->
<div class="contents" class:hidden={hideActions}>
	<button
		class="btn btn-primary flex w-full items-center gap-1 text-sm disabled:cursor-not-allowed disabled:opacity-50 md:w-fit"
		class:hidden={!showConnectButton}
		use:tooltip={{
			text: hasLicenseEntitlementViolations
				? MCP_CONNECTION_INVALID_LICENSE_MESSAGE
				: isMultiUserCatalogEntryRow && !catalogID && !workspaceID
					? m.mcps_servers_actions_multi_user_entry_tooltip()
					: canConnect
						? ''
						: m.mcps_servers_actions_no_access_tooltip()
		}}
		onclick={async () => {
			if (isMultiUserCatalogEntryRow) {
				if (configuredServers.length === 1) {
					const targetServer = configuredServers[0];
					connectToServerDialog?.open({
						entry,
						server: targetServer,
						instance: mcpServersAndEntries.current.userInstances.find(
							(i) => i.mcpServerID === targetServer.id
						)
					});
				} else if (configuredServers.length > 1) {
					handleShowSelectServerDialog('connect', configuredServers);
				} else {
					connectToServerDialog?.open({ entry });
				}
			} else if (entry && !server && configuredServers.length > 0) {
				if (configuredServers.length === 1) {
					connectToServerDialog?.open({
						entry,
						server: configuredServers[0]
					});
				} else {
					handleShowSelectServerDialog('connect', configuredServers);
				}
			} else {
				connectToServerDialog?.open({
					entry,
					server,
					instance
				});
			}
		}}
		disabled={connectionDisabled}
	>
		{#if loading}
			<Loading class="size-4" />
		{:else if isMultiUserCatalogEntryRow && configuredServers.length === 0}
			{m.mcps_servers_actions_create_server()}
		{:else}
			{m.mcps_servers_actions_connect()}
		{/if}
	</button>

	<div class:hidden={loading || !hasActions}>
		<DotDotDot classes={{ menu: 'min-w-48 p-0', popover: 'z-60' }}>
			{#snippet children({ toggle })}
				{@render serverActions(toggle)}
			{/snippet}
		</DotDotDot>
	</div>
</div>

<ConnectToServer
	bind:this={connectToServerDialog}
	{catalogID}
	{workspaceID}
	onConnect={(data) => {
		onConnect?.(data);
		refresh();
	}}
	{skipConnectDialog}
	onEdit={({ entry, server, instance }) => {
		if (server && instance && isMultiUserServer(server)) {
			connectToServerDialog?.open({ server, instance, configureInstance: true });
		} else if (entry && server) {
			editExistingDialog?.edit({
				server,
				entry
			});
		}
	}}
/>

<EditExistingDeployment bind:this={editExistingDialog} onUpdateConfigure={refresh} />

<McpSelectServerDeployment
	bind:this={selectServerDialog}
	contextEntry={entry}
	onSelectServer={async (d) => {
		selectServerDialog?.close();
		switch (selectServerMode) {
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
				if (supportsMCPBackendDetails(d)) {
					await restartServer(d);
					await mcpServersAndEntries.refreshAll();
				}
				break;
			}
			case 'disconnect': {
				await disconnectCurrentUser(d);
				await mcpServersAndEntries.refreshAll();
				break;
			}
			default:
				connectToServerDialog?.open({
					entry,
					server: d,
					instance: isMultiUserServer(d)
						? mcpServersAndEntries.current.userInstances.find((i) => i.mcpServerID === d.id)
						: undefined
				});
				break;
		}
	}}
/>

<ResponsiveDialog
	bind:this={launchDialog}
	animate="slide"
	class={isMultiUserCatalogEntry(entry) ? 'md:max-w-sm' : ''}
>
	{#snippet titleContent()}
		{#if entry || server}
			{@const name = entry?.manifest.name ?? server?.manifest.name ?? m.mcps_server_fallback_name()}
			{@const imageUrl = entry?.manifest.icon || server?.manifest.icon}
			<div class="icon">
				{#if imageUrl}
					<img
						src={imageUrl}
						alt={entry?.manifest.name ?? server?.manifest.name ?? m.mcps_server_fallback_name()}
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
		{#if isMultiUserCatalogEntry(entry) || isMultiUserServer(server)}
			<p class="text-center">
				{#if entry}
					{m.mcps_servers_actions_mcp_server_configured()}
				{:else}
					{m.mcps_servers_actions_server_configured()}
				{/if}
			</p>
		{/if}
		{#if hasLicenseEntitlementViolations}
			<p class="mb-2 text-center text-muted-content">
				{m.mcps_servers_actions_license_disabled()}
			</p>
		{:else if isMultiUserCatalogEntry(entry)}
			<p class="mb-2 text-center">{m.mcps_servers_actions_launch_now()}</p>
		{:else if !entry && isMultiUserServer(server)}
			<p class="mb-2 text-center">{m.mcps_servers_actions_connect_now()}</p>
		{:else}
			<div class="mt-4 flex flex-col gap-3">
				<McpDeprecatedNotice {deprecated} variant="notification" />
				<CopyField
					id="server-action-connection-url"
					label={m.mcps_servers_connection_url()}
					value={entry?.connectURL ?? server?.connectURL ?? ''}
				/>
			</div>
		{/if}
		<div class="flex grow"></div>
		{#if isMultiUserCatalogEntry(entry) || (!entry && isMultiUserServer(server))}
			<div class="flex flex-col gap-2">
				<button class="btn btn-secondary" onclick={() => launchDialog?.close()}
					>{m.mcps_skip()}</button
				>
				<button
					class="btn btn-primary"
					onclick={() => {
						launchDialog?.close();
						connectToServerDialog?.open({
							entry,
							server
						});
					}}
					disabled={hasLicenseEntitlementViolations}
				>
					{#if isMultiUserCatalogEntry(entry)}
						{m.mcps_servers_launch_server()}
					{:else}
						{m.mcps_servers_actions_connect()}
					{/if}
				</button>
			</div>
		{/if}
	</div>
</ResponsiveDialog>

{#snippet serverActions(toggle: (value: boolean) => void)}
	{#if server && (server.userID === profile.current.id || instance || canEditMultiUserServerConfiguration || canDeleteMultiUserServer)}
		<div
			class="bg-base-100 dark:bg-base-300 rounded-t-xl pt-2 pb-1 pl-4 text-[11px] font-semibold uppercase"
		>
			{m.mcps_servers_actions_my_connection()}
		</div>
		<div class="flex flex-col gap-1 p-2 bg-base-200">
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
					<ServerCog class="size-4" />
					{m.mcps_servers_actions_edit_my_connection()}
				</button>
			{/if}
			{#if entry && isServerOwner}
				<button
					class="menu-button"
					onclick={() => {
						editExistingDialog?.rename({
							server,
							entry
						});
					}}
				>
					<PencilLine class="size-4" />
					{m.mcps_servers_actions_rename()}
				</button>
				{#if server && canDebugOauth}
					<button
						class="menu-button"
						onclick={async (e) => {
							e.stopPropagation();
							debugOauthDialog?.open(server);
							toggle(false);
						}}
						disabled={profile.current?.isAdminReadonly?.()}
					>
						<Bug class="size-4" />
						{m.mcps_servers_actions_debug_oauth()}
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
						<ServerCog class="size-4" />
						{m.mcps_actions_edit_configuration()}
					</button>
				{/if}
			{/if}
			{#if server && canRestartServerDeployment}
				<button
					class="menu-button"
					disabled={restarting}
					onclick={async (e) => {
						e.stopPropagation();
						restarting = true;
						try {
							await restartServer(server);
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
					{/if}
					{m.mcps_actions_restart()}
				</button>
			{/if}
			{#if server && instance}
				<button
					class="menu-button"
					disabled={disconnecting}
					onclick={async (e) => {
						e.stopPropagation();
						disconnecting = true;
						await UserService.deleteMcpServerInstance(instance.id);
						await mcpServersAndEntries.refreshAll();
						toggle(false);
						disconnecting = false;
					}}
				>
					{#if disconnecting}
						<Loading class="size-4" />
					{:else}
						<Unplug class="size-4" />
					{/if}
					{m.mcps_servers_actions_disconnect()}
				</button>
			{:else if entry && server && isServerOwner && !canDeleteMultiUserServer && !isMultiUserCatalogEntry(entry)}
				<button
					class="menu-button"
					disabled={disconnecting}
					onclick={async (e) => {
						e.stopPropagation();
						disconnecting = true;
						await deleteServerDeployment(server);
						await mcpServersAndEntries.refreshAll();
						toggle(false);
						goto(resolve(`/mcp-servers/c/${entry.id}`), { replaceState: true });
						disconnecting = false;
					}}
				>
					{#if disconnecting}
						<Loading class="size-4" />
					{:else}
						<Trash2 class="size-4" />
					{/if}
					{m.mcps_servers_actions_disconnect()}
				</button>
			{/if}
		</div>
	{:else if entry && configuredServers.length > 0}
		{#if showConnectionActions}
			<div
				class="bg-base-100 dark:bg-base-300 rounded-t-xl pt-2 pb-1 pl-4 text-[11px] font-semibold uppercase"
			>
				{m.mcps_servers_actions_my_connections()}
			</div>
			<div class="bg-base-200 flex flex-col gap-1 p-2">
				{#if entry && !hasMultiUserServerNotOwned}
					<button
						class="menu-button"
						onclick={() => {
							if (configuredServers.length === 1) {
								editExistingDialog?.rename({
									server: configuredServers[0],
									entry
								});
							} else {
								handleShowSelectServerDialog('rename', configuredServers);
							}
						}}
					>
						<PencilLine class="size-4" />
						{m.mcps_servers_actions_rename()}
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
									handleShowSelectServerDialog('edit', configuredServers);
								}
							}}
						>
							<ServerCog class="size-4" />
							{m.mcps_actions_edit_configuration()}
						</button>
					{/if}
				{/if}
				{#if restartableConfiguredServers.length > 0 && !hasMultiUserServerNotOwned}
					<button
						class="menu-button"
						disabled={restarting}
						onclick={async (e) => {
							e.stopPropagation();
							if (restartableConfiguredServers.length === 1) {
								restarting = true;
								try {
									await restartServer(restartableConfiguredServers[0]);
									refresh();
								} finally {
									restarting = false;
									toggle(false);
								}
							} else {
								handleShowSelectServerDialog('restart', configuredServers);
							}
						}}
					>
						{#if restarting}
							<Loading class="size-4" />
						{:else}
							<RefreshCw class="size-4" />
						{/if}
						{m.mcps_actions_restart()}
					</button>
				{/if}
				{#if !isMultiUserCatalogEntry(entry)}
					<button
						class="menu-button"
						onclick={async (e) => {
							e.stopPropagation();
							if (configuredServers.length === 1) {
								await disconnectCurrentUser(configuredServers[0]);
								await mcpServersAndEntries.refreshAll();
							} else {
								handleShowSelectServerDialog('disconnect', configuredServers);
							}
							toggle(false);
						}}
					>
						<Unplug class="size-4" />
						{m.mcps_servers_actions_disconnect()}
					</button>
				{/if}
			</div>
		{/if}
		{#if isMultiUserCatalogEntry(entry) && (catalogID || workspaceID)}
			<div class="flex flex-col gap-1 p-2">
				<button
					class="menu-button"
					onclick={(e) => {
						e.stopPropagation();
						connectToServerDialog?.open({ entry });
						toggle(false);
					}}
				>
					<Plus class="size-4" />
					{m.mcps_servers_actions_create_server()}
				</button>
			</div>
		{/if}
	{/if}

	{#if (showDisconnectUser && server) || (entry && configuredServers.length > 0)}
		<div class="flex flex-col gap-2 p-2 pt-1">
			{#if entry && !isMultiUserCatalogEntry(entry) && configuredServers.length > 0}
				<button
					class="menu-button"
					onclick={(e) => {
						e.stopPropagation();
						connectToServerDialog?.setupNewInstance(entry);
						toggle(false);
					}}
				>
					<Plus class="size-4" />
					{m.mcps_servers_actions_create_new_connection()}
				</button>
			{/if}
			{#if showDisconnectUser && server}
				<button
					class="menu-button text-error"
					onclick={async (e) => {
						e.stopPropagation();
						await deleteServerDeployment(server);
						await mcpServersAndEntries.refreshAll();
						toggle(false);
					}}
				>
					<Trash2 class="size-4" />
					{m.mcps_servers_actions_disconnect_user()}
				</button>
			{/if}
		</div>
	{/if}
{/snippet}

<StaticOAuthConfigureModal
	bind:this={oauthConfigModal}
	{deprecated}
	{oauthStatus}
	onSave={async (credentials) => {
		if (!entry) return;
		oauthStatus = entry.powerUserWorkspaceID
			? await UserService.setWorkspaceMCPCatalogEntryOAuthCredentials(
					entry.powerUserWorkspaceID,
					entry.id,
					credentials
				)
			: await AdminService.setMCPCatalogEntryOAuthCredentials('default', entry.id, credentials);
		oauthConfiguredOverride = oauthStatus.configured;
		onOAuthConfigured?.();
	}}
	onDelete={async () => {
		if (!entry) return;
		if (entry.powerUserWorkspaceID) {
			await UserService.deleteWorkspaceMCPCatalogEntryOAuthCredentials(
				entry.powerUserWorkspaceID,
				entry.id
			);
		} else {
			await AdminService.deleteMCPCatalogEntryOAuthCredentials('default', entry.id);
		}
		oauthConfiguredOverride = false;
		oauthStatus = { configured: false };
		onOAuthConfigured?.();
	}}
/>

<DebugOauthDialog bind:this={debugOauthDialog} />
