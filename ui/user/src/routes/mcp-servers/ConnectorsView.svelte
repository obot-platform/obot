<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import ConnectToServer from '$lib/components/mcp/ConnectToServer.svelte';
	import McpSelectServerDeployment from '$lib/components/mcp/McpSelectServerDeployment.svelte';
	import { stripMarkdownToText } from '$lib/markdown';
	import {
		UserService,
		type MCPCatalog,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type OrgUser,
		type MCPServerInstance
	} from '$lib/services';
	import { MCP_CONNECTION_INVALID_LICENSE_MESSAGE } from '$lib/services/user/constants';
	import {
		convertEntriesAndServersToTableData,
		disconnectMcpServerUser,
		hasMissingSecretBindingConfig,
		isMultiUserServer,
		restartMcpServer,
		isMultiUserCatalogEntry,
		requiresUserUpdate
	} from '$lib/services/user/mcp';
	import { mcpServersAndEntries, profile, version } from '$lib/stores';
	import { openUrl } from '$lib/utils';
	import EditExistingDeployment from '../../lib/components/mcp/EditExistingDeployment.svelte';
	import { CircleFadingArrowUp, Server } from '@lucide/svelte';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	type ServerSelectMode =
		| 'connect'
		| 'rename'
		| 'edit'
		| 'disconnect'
		| 'server-details'
		| 'restart'
		| 'reauthenticate';

	interface Props {
		entity?: 'workspace' | 'catalog';
		id?: string;
		catalog?: MCPCatalog;
		noDataContent?: Snippet;
		usersMap?: Map<string, OrgUser>;
		query?: string;
	}

	let { entity, id, catalog = $bindable(), noDataContent, query, usersMap }: Props = $props();

	let connectToServerDialog = $state<ReturnType<typeof ConnectToServer>>();
	let editExistingDialog = $state<ReturnType<typeof EditExistingDeployment>>();

	let selectedEntry = $state<MCPCatalogEntry>();
	let selectServerDialog = $state<ReturnType<typeof McpSelectServerDeployment>>();
	let selectServerMode = $state<ServerSelectMode>('connect');

	let instancesMap = $derived(
		new Map(
			mcpServersAndEntries.current.userInstances.map((instance) => [instance.mcpServerID, instance])
		)
	);

	let entriesMap = $derived(
		new Map(mcpServersAndEntries.current.entries.map((entry) => [entry.id, entry]))
	);

	let tableData = $derived(
		convertEntriesAndServersToTableData(
			profile.current.hasAdminAccess?.()
				? mcpServersAndEntries.current.entries.filter((e) => e.canConnect)
				: mcpServersAndEntries.current.entries,
			profile.current.hasAdminAccess?.()
				? mcpServersAndEntries.current.servers.filter((s) => s.canConnect)
				: mcpServersAndEntries.current.servers,
			usersMap,
			mcpServersAndEntries.current.userConfiguredServers,
			mcpServersAndEntries.current.userInstances
		).filter((d) => {
			const requiresOauth =
				d.data.manifest?.runtime === 'remote' &&
				d.data.manifest?.remoteConfig &&
				'staticOAuthRequired' in d.data.manifest.remoteConfig &&
				d.data.manifest?.remoteConfig?.staticOAuthRequired;

			if ('isCatalogEntry' in d.data && isMultiUserCatalogEntry(d.data)) {
				return false;
			}

			return (
				!requiresOauth ||
				(requiresOauth && 'isCatalogEntry' in d.data && d.data.oauthCredentialConfigured)
			);
		})
	);

	let filteredTableData = $derived.by(() => {
		const sorted = [...tableData].sort((a, b) => {
			if (a.connected !== b.connected) {
				return a.connected ? -1 : 1;
			}
			return a.name.localeCompare(b.name);
		});

		return query
			? sorted.filter((d) => d.name.toLowerCase().includes(query.toLowerCase()))
			: sorted;
	});

	let hasLicenseEntitlementViolations = $derived(
		(version.current.licenseEntitlementViolations || []).length > 0
	);

	function getConfiguredServersForCatalogEntry(entry: MCPCatalogEntry): MCPCatalogServer[] {
		return mcpServersAndEntries.current.userConfiguredServers.filter(
			(server) => server.catalogEntryID === entry.id
		);
	}

	function getUsableConfiguredServersForCatalogEntry(entry: MCPCatalogEntry): MCPCatalogServer[] {
		return getConfiguredServersForCatalogEntry(entry).filter(
			(server) =>
				!hasMissingSecretBindingConfig(
					server.manifest,
					server.missingRequiredEnvVars,
					server.missingRequiredHeader
				)
		);
	}

	async function reauthenticateServer(server: MCPCatalogServer) {
		await UserService.clearMcpServerOAuth(server.id);
		await connectToServerDialog?.authenticate(
			server,
			server.catalogEntryID ? entriesMap.get(server.catalogEntryID) : undefined
		);
		await mcpServersAndEntries.refreshAll();
	}

	async function restartServer(server: MCPCatalogServer) {
		await restartMcpServer(server, catalog?.id);
	}

	async function disconnectCurrentUser(server: MCPCatalogServer) {
		await disconnectMcpServerUser(server);
	}

	function handleShowSelectServerDialog(
		entry: MCPCatalogEntry,
		mode: ServerSelectMode = 'connect'
	) {
		const allServers =
			mode === 'connect'
				? getUsableConfiguredServersForCatalogEntry(entry)
				: getConfiguredServersForCatalogEntry(entry);
		selectedEntry = entry;
		selectServerDialog?.open(allServers);
		selectServerMode = mode;
	}

	function handleConnectToServer({ instance }: { instance?: MCPServerInstance }) {
		if (instance) {
			mcpServersAndEntries.refreshUserInstances();
		} else {
			mcpServersAndEntries.refreshUserConfiguredServers();
		}
	}

	function handleSelect(data: MCPCatalogEntry | MCPCatalogServer, e: MouseEvent | KeyboardEvent) {
		const isTouchDevice = 'ontouchstart' in window || navigator.maxTouchPoints > 0;
		const isCtrlClick = isTouchDevice ? false : e.metaKey || e.ctrlKey;

		let url = '';
		if ('isCatalogEntry' in data && isMultiUserCatalogEntry(data)) {
			url = `/mcp-servers/c/${data.id}`;
		} else {
			const matchingServers =
				'isCatalogEntry' in data ? getConfiguredServersForCatalogEntry(data) : [];
			if (matchingServers.length === 1) {
				url = `/mcp-servers/c/${data.id}/instance/${matchingServers[0].id}`;
			} else if (matchingServers.length > 1) {
				handleShowSelectServerDialog(data as MCPCatalogEntry, 'server-details');
			} else {
				url = 'isCatalogEntry' in data ? `/mcp-servers/c/${data.id}` : `/mcp-servers/s/${data.id}`;
			}
		}

		openUrl(url, isCtrlClick);
	}

	function handleConnect(d: MCPCatalogEntry | MCPCatalogServer) {
		let instance: MCPServerInstance | undefined;
		let server: MCPCatalogServer | undefined;
		let entry: MCPCatalogEntry | undefined;

		if ('isCatalogEntry' in d) {
			if (isMultiUserCatalogEntry(d)) {
				const multiUserCatalogEntryServers = getConfiguredServersForCatalogEntry(d);
				if (multiUserCatalogEntryServers.length > 1) {
					handleShowSelectServerDialog(d);
					return;
				} else {
					server = multiUserCatalogEntryServers[0];
					instance = instancesMap.get(server.id);
				}
			}

			const matchingServers = getUsableConfiguredServersForCatalogEntry(d);
			if (matchingServers.length > 1) {
				handleShowSelectServerDialog(d);
				return;
			} else {
				entry = d;
				server = matchingServers[0];
			}
		} else {
			const matchingEntry = d.catalogEntryID ? entriesMap.get(d.catalogEntryID) : undefined;
			instance = instancesMap.get(d.id);
			server = d;
			entry = matchingEntry;
		}

		if (server && !server.configured) {
			editExistingDialog?.edit({
				server,
				entry
			});
		} else {
			connectToServerDialog?.open({
				entry,
				server,
				instance
			});
		}
	}
</script>

<div class="flex flex-col gap-1 @container">
	{#if mcpServersAndEntries.current.loading}
		{#each Array.from({ length: 4 }) as _, i (i)}
			<div class="skeleton h-23 w-full"></div>
		{/each}
	{:else if tableData.length === 0 && noDataContent}
		{@render noDataContent?.()}
	{:else}
		{#each filteredTableData as d (d.id)}
			{@const isCatalogEntry = 'isCatalogEntry' in d.data}
			{@const catalogEntry = isCatalogEntry ? (d.data as MCPCatalogEntry) : undefined}
			{@const userConfiguredServers = catalogEntry
				? getConfiguredServersForCatalogEntry(catalogEntry)
				: []}
			{@const instance = instancesMap.get(d.id)}
			{@const requiresUserAttention = instance
				? instance.configured === false
				: userConfiguredServers.some(requiresUserUpdate)}
			<div
				class={twMerge(
					'flex items-center justify-between gap-8 rounded-md p-3 bg-base-100 dark:bg-base-300 shadow-xs hover:bg-base-300 dark:hover:bg-base-400/75 cursor-pointer'
				)}
				role="button"
				tabindex="0"
				onkeydown={(e) => {
					if (e.key === 'Enter' || e.key === ' ') {
						e.preventDefault();
						e.stopPropagation();
						handleSelect(d.data, e);
					}
				}}
				onclick={(e) => handleSelect(d.data, e)}
			>
				<div class="text-sm font-light line-clamp-2">
					<div class="flex items-center gap-2">
						<div class="icon">
							{#if d.icon}
								<img src={d.icon} alt={d.name} class="size-6" />
							{:else}
								<Server class="size-6" />
							{/if}
						</div>
						<div class="flex items-center gap-2">
							<p>{d.name}</p>
							{#if requiresUserAttention}
								<span
									use:tooltip={{
										classes: ['border-primary', 'bg-primary/10', 'dark:bg-primary/50'],
										text: 'Configuration requires your attention'
									}}
								>
									<CircleFadingArrowUp class="text-primary size-4" />
								</span>
							{:else if d.connected}
								<div class="badge badge-xs badge-secondary gap-1">
									<span class="status status-primary"></span>
									Connected
								</div>
							{/if}
						</div>
					</div>
					<p class="text-xs text-muted-content min-h-8 mt-2">
						{stripMarkdownToText(d.data.manifest.description ?? '')}
					</p>
				</div>
				<div
					use:tooltip={{
						text: hasLicenseEntitlementViolations
							? MCP_CONNECTION_INVALID_LICENSE_MESSAGE
							: undefined
					}}
				>
					<button
						class="btn btn-sm btn-primary border-none"
						disabled={hasLicenseEntitlementViolations}
						onclick={(e) => {
							e.stopPropagation();
							handleConnect(d.data);
						}}
					>
						Connect
					</button>
				</div>
			</div>
		{/each}
	{/if}
</div>

<ConnectToServer
	bind:this={connectToServerDialog}
	catalogID={catalog?.id}
	workspaceID={entity === 'workspace' ? id : undefined}
	onConnect={handleConnectToServer}
/>

<McpSelectServerDeployment
	bind:this={selectServerDialog}
	onSelectServer={async (d) => {
		selectServerDialog?.close();
		switch (selectServerMode) {
			case 'server-details': {
				goto(
					resolve(
						isMultiUserServer(d)
							? `/mcp-servers/s/${d.id}`
							: `/mcp-servers/c/${d.catalogEntryID}/instance/${d.id}`
					)
				);
				break;
			}
			case 'rename': {
				editExistingDialog?.rename({
					server: d,
					entry: d.catalogEntryID ? entriesMap.get(d.catalogEntryID) : undefined
				});
				break;
			}
			case 'edit': {
				editExistingDialog?.edit({
					server: d,
					entry: d.catalogEntryID ? entriesMap.get(d.catalogEntryID) : undefined
				});
				break;
			}
			case 'disconnect': {
				await disconnectCurrentUser(d);
				await mcpServersAndEntries.refreshAll();
				break;
			}
			case 'restart': {
				await restartServer(d);
				await mcpServersAndEntries.refreshAll();
				break;
			}
			case 'reauthenticate': {
				await reauthenticateServer(d);
				break;
			}
			default:
				connectToServerDialog?.open({
					entry: selectedEntry,
					server: d,
					instance: isMultiUserServer(d) ? instancesMap.get(d.id) : undefined
				});
				break;
		}
	}}
/>

<EditExistingDeployment
	bind:this={editExistingDialog}
	onUpdateConfigure={() => {
		mcpServersAndEntries.refreshUserConfiguredServers();
	}}
/>
