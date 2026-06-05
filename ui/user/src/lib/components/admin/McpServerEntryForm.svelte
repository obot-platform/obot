<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { ADMIN_SESSION_STORAGE } from '$lib/constants';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		type MCPFilter,
		type MCPCatalogServer,
		UserService,
		Group,
		MCPCompositeDeletionDependencyError,
		type LaunchServerType,
		type MCPServerOAuthCredentialStatus,
		type AccessControlRule,
		type MCPCatalogEntry,
		type OrgUser
	} from '$lib/services';
	import { getMCPDisplayName, getServerTypeLabel, getSource } from '$lib/services/user/mcp';
	import { profile } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { goto } from '$lib/url';
	import { openUrl, isOwnSingleUserServer } from '$lib/utils';
	import Confirm from '../Confirm.svelte';
	import OverflowContainer from '../OverflowContainer.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import CatalogConfigureForm, {
		type LaunchFormData,
		type CompositeLaunchFormData,
		type ComponentLaunchFormData
	} from '../mcp/CatalogConfigureForm.svelte';
	import McpMultiDeleteBlockedDialog from '../mcp/McpMultiDeleteBlockedDialog.svelte';
	import McpServerDetails from '../mcp/McpServerDetails.svelte';
	import McpServerInfo from '../mcp/McpServerInfo.svelte';
	import McpServerTools from '../mcp/McpServerTools.svelte';
	import StaticOAuthConfigureModal from '../mcp/StaticOAuthConfigureModal.svelte';
	import DebugOauthFlow from '../mcp/oauth/DebugOauthFlow.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import Table from '../table/Table.svelte';
	import { setVirtualPageDisabled } from '../ui/virtual-page/context';
	import CatalogServerForm from './CatalogServerForm.svelte';
	import McpServerInstances from './McpServerInstances.svelte';
	import AuditLogsPageContent from './audit-logs/AuditLogsPageContent.svelte';
	import UsageGraphs from './usage/UsageGraphs.svelte';
	import {
		CircleAlert,
		ChevronLeft,
		ChevronRight,
		CircleFadingArrowUp,
		GlobeLock,
		Info,
		ListFilter,
		Server,
		Settings,
		Trash2,
		Users,
		Wrench,
		ExternalLink
	} from 'lucide-svelte';
	import { onMount, untrack } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		id?: string;
		entity?: 'workspace' | 'catalog';
		entry?: MCPCatalogEntry | MCPCatalogServer;
		server?: MCPCatalogServer;
		type?: LaunchServerType;
		readonly?: boolean;
		onCancel?: () => void;
		onSubmit?: (id: string, isMultiUserEntry: boolean, message?: string) => void;
		hasExistingConfigured?: boolean;
		isDialogView?: boolean;
		limitViews?: string[];
		excludeViews?: string[];
		configuredServers?: MCPCatalogServer[];
		allowMultiUserServerConfigurationEdit?: boolean;
	}

	let {
		entry: initialEntry,
		server,
		id,
		entity = 'catalog',
		type,
		readonly,
		onCancel,
		onSubmit,
		hasExistingConfigured,
		isDialogView,
		limitViews,
		excludeViews,
		configuredServers,
		allowMultiUserServerConfigurationEdit
	}: Props = $props();

	let entry = $state(untrack(() => initialEntry));
	let lastSyncedInitialEntry: MCPCatalogEntry | MCPCatalogServer | undefined = undefined;
	let prefix = $derived(profile.current.hasAdminAccess?.() ? '/admin' : '');

	$effect(() => {
		const next = initialEntry;
		if (next !== lastSyncedInitialEntry) {
			lastSyncedInitialEntry = next;
			entry = next;
		}
	});

	let isAtLeastPowerUserPlus = $derived(profile.current?.groups.includes(Group.POWERUSER_PLUS));

	// True owner: admin, workspace owner, or power user who created the server
	let trueOwner = $derived(
		(entity === 'workspace' && entry?.powerUserWorkspaceID && entry.powerUserWorkspaceID === id) ||
			profile.current?.hasAdminAccess?.() ||
			// Basic users own their single-user servers
			(entry && isOwnSingleUserServer(entry, profile.current?.id))
	);

	// Basic user who just connected to a catalog entry
	let basicUserConnected = $derived(!trueOwner && entry && !server && hasExistingConfigured);

	// Combined: has any access
	let belongsToUser = $derived(trueOwner || basicUserConnected);

	let listAccessControlRules = $state<Promise<AccessControlRule[]>>();
	let listFilters = $state<Promise<MCPFilter[]>>();
	let users = $state<OrgUser[]>([]);
	let usersMap = $derived(new Map(users.map((user) => [user.id, user])));
	let source = $derived(entry ? getSource(entry, usersMap) : undefined);

	let deleteServer = $state(false);
	let deleteConflictError = $state<MCPCompositeDeletionDependencyError | undefined>();
	let deleteResourceFromRule = $state<{
		rule: AccessControlRule;
		resourceId: string;
	}>();
	let selected = $derived.by(() => {
		const searchParams = page.url.searchParams;
		const tab = searchParams.get('view');
		const fallback = entry && !excludeViews?.includes('overview') ? 'overview' : 'configuration';
		if (!tab) return fallback;
		return tabs.some((t) => t.view === tab) ? tab : fallback;
	});
	let configurationReadonly = $derived(readonly || isCatalogEntryDeployedMultiUserServer(entry));
	let showLeftChevron = $state(false);
	let showRightChevron = $state(false);
	let scrollContainer = $state<HTMLDivElement>();

	let oauthDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let oauthURL = $state<string>();
	let oauthURLs = $state<Record<string, string>>();
	let authenticatedComponents = $state<Set<string>>(new Set());

	let staticOauthConfigModal = $state<ReturnType<typeof StaticOAuthConfigureModal>>();
	let staticOauthStatus = $state<MCPServerOAuthCredentialStatus>();

	let configDialog = $state<ReturnType<typeof CatalogConfigureForm>>();
	let configureForm = $state<LaunchFormData | CompositeLaunchFormData>();
	let saving = $state(false);
	let error = $state<string>();
	let showButtonInlineError = $state(false);
	let showUpdateExistingDeploymentsConfirm = $state(false);

	let showRegenerateToolsButton = $derived(
		entry &&
			!server &&
			entry.manifest?.toolPreview &&
			'toolPreviewsLastGenerated' in entry &&
			'lastUpdated' in entry &&
			entry.toolPreviewsLastGenerated &&
			entry.lastUpdated &&
			new Date(entry.toolPreviewsLastGenerated) < new Date(entry.lastUpdated)
	);
	let requiresStaticOauth = $derived(
		staticOauthStatus
			? !staticOauthStatus.configured
			: entry &&
					'isCatalogEntry' in entry &&
					entry.manifest?.runtime === 'remote' &&
					entry.manifest?.remoteConfig?.staticOAuthRequired &&
					!entry.oauthCredentialConfigured
	);

	const tabs = $derived.by(() => {
		const availableTabs =
			entry && !server
				? [
						{ label: 'Overview', view: 'overview' },
						...(trueOwner &&
						(!isCatalogEntryDeployedMultiUserServer(entry) || allowMultiUserServerConfigurationEdit)
							? [{ label: 'Configuration', view: 'configuration' }]
							: []),
						...(belongsToUser ? [{ label: 'Server Details', view: 'server-instances' }] : []),
						{ label: 'Tools', view: 'tools' },
						// Basic users who just connected don't see Configuration.
						// Catalog entry-deployed multi-user servers also hide it: the configuration is
						// owned by the upstream catalog entry, not the deployment.
						...(belongsToUser
							? [
									{ label: 'Audit Logs', view: 'audit-logs' },
									{ label: 'Usage', view: 'usage' }
								]
							: []),
						...(isAtLeastPowerUserPlus && trueOwner
							? [{ label: 'Access Policies', view: 'access-control' }]
							: []),
						...(profile.current?.hasAdminAccess?.() ? [{ label: 'Filters', view: 'filters' }] : [])
					]
				: [
						{ label: 'Overview', view: 'overview' },
						...(belongsToUser ? [{ label: 'Server Details', view: 'server-instances' }] : []),
						{ label: 'Tools', view: 'tools' },
						...(profile.current?.hasAdminAccess?.() && entry?.manifest?.runtime === 'remote'
							? [{ label: 'Troubleshooting', view: 'troubleshooting' }]
							: [])
					];
		return limitViews
			? availableTabs.filter((tab) => limitViews.includes(tab.view))
			: availableTabs.filter((tab) => !excludeViews?.includes(tab.view));
	});

	$effect(() => {
		if (selected === 'access-control') {
			listAccessControlRules =
				entity === 'workspace' && id
					? UserService.listWorkspaceAccessControlRules(id)
					: AdminService.listAccessControlRules();
		} else if (selected === 'filters' && entity !== 'workspace') {
			// add filters back in for workspace once supported for workspace
			listFilters = AdminService.listMCPFilters();
		}
	});

	$effect(() => {
		if (page.url.searchParams.get('view')) {
			setVirtualPageDisabled(false);
		} else {
			setVirtualPageDisabled(true);
		}
	});

	// Auto-close OAuth dialog when all components are authenticated
	$effect(() => {
		if (oauthURLs !== undefined && Object.keys(oauthURLs).length === 0) {
			oauthDialog?.close();
		}
	});

	onMount(() => {
		UserService.listUsersIncludeDeleted().then((data) => {
			users = data;
		});

		checkScrollPosition();
		scrollContainer?.addEventListener('scroll', checkScrollPosition);
		window.addEventListener('resize', checkScrollPosition);

		return () => {
			scrollContainer?.removeEventListener('scroll', checkScrollPosition);
			window.removeEventListener('resize', checkScrollPosition);
			document.removeEventListener('visibilitychange', handleVisibilityChange);
		};
	});

	function filterRulesByEntry(rules?: AccessControlRule[]) {
		if (!entry || !rules) return [];
		return rules.filter((r) =>
			r.resources?.find(
				(resource) => (entry?.id && resource.id === entry.id) || resource.id === '*'
			)
		);
	}

	function filterFiltersByEntry(filters?: MCPFilter[]) {
		if (!entry || !filters) return [];
		return filters.filter((f) =>
			f.resources?.find(
				(resource) => (entry?.id && resource.id === entry.id) || resource.id === '*'
			)
		);
	}

	function setLastVisitedMcpServer() {
		if (isDialogView) return;
		if (!entry) return;
		const name = getMCPDisplayName(entry);
		sessionStorage.setItem(
			ADMIN_SESSION_STORAGE.LAST_VISITED_MCP_SERVER,
			JSON.stringify({ id: entry.id, name, type, entity, entityId: id })
		);
	}

	function isCatalogEntryDeployedMultiUserServer(item?: MCPCatalogEntry | MCPCatalogServer) {
		return (
			!!item &&
			!('isCatalogEntry' in item) &&
			item.serverUserType === 'multiUser' &&
			!!item.catalogEntryID
		);
	}

	function checkScrollPosition() {
		if (!scrollContainer) return;

		const { scrollLeft, scrollWidth, clientWidth } = scrollContainer;
		showLeftChevron = scrollLeft > 0;
		showRightChevron = scrollLeft < scrollWidth - clientWidth - 1; // -1 for rounding errors
	}

	function scrollLeft() {
		if (scrollContainer) {
			scrollContainer.scrollBy({ left: -200, behavior: 'smooth' });
		}
	}

	function scrollRight() {
		if (scrollContainer) {
			scrollContainer.scrollBy({ left: 200, behavior: 'smooth' });
		}
	}

	function handleSelectionChange(newSelection: string) {
		if (isDialogView) {
			selected = newSelection;
			return;
		}
		if (newSelection !== selected) {
			const url = new URL(window.location.href);
			url.searchParams.set('view', newSelection);
			goto(url, { replaceState: true });
		}
	}

	function compileTemporaryInstanceBody() {
		function isCompositeForm(
			f: LaunchFormData | CompositeLaunchFormData | undefined
		): f is CompositeLaunchFormData {
			return Boolean(f && typeof f === 'object' && 'componentConfigs' in f);
		}

		if (isCompositeForm(configureForm)) {
			const body: {
				componentConfigs: Record<
					string,
					{ config: Record<string, string>; url: string; disabled: boolean }
				>;
			} = { componentConfigs: {} };
			const composite = configureForm;
			for (const [compId, comp] of Object.entries(composite.componentConfigs)) {
				const cfg: Record<string, string> = {};
				for (const f of comp.envs || []) if (f.value) cfg[f.key] = f.value;
				for (const f of comp.headers || []) if (f.value) cfg[f.key] = f.value;
				body.componentConfigs[compId] = {
					config: cfg,
					url: comp.url || '',
					disabled: !!comp.disabled
				};
			}
			return body;
		}
		return {
			url: (configureForm as LaunchFormData)?.url,
			config: [
				...((configureForm as LaunchFormData)?.headers ?? []),
				...((configureForm as LaunchFormData)?.envs ?? [])
			].reduce<Record<string, string>>((acc, curr) => {
				acc[curr.key] = curr.value;
				return acc;
			}, {})
		};
	}

	async function handleVisibilityChange() {
		if (!entry || !id) return;
		if (document.visibilityState !== 'visible') return;

		// Composite OAuth case: check if all components have been clicked
		if (oauthURLs && Object.keys(oauthURLs).length > 0) {
			const pendingComponents = Object.keys(oauthURLs).filter(
				(componentId) => !authenticatedComponents.has(componentId)
			);

			// If there are still components that haven't been clicked, keep waiting
			if (pendingComponents.length > 0) {
				return;
			}

			// All components have been clicked; stop listening and regenerate tool previews
			document.removeEventListener('visibilitychange', handleVisibilityChange);
			handleLaunchTemporaryInstance();
			return;
		}

		// Single-server OAuth (string oauthURL) or non-composite case
		document.removeEventListener('visibilitychange', handleVisibilityChange);
		handleLaunchTemporaryInstance();
	}

	function handleTemporaryInstanceOauth(oauthUrlToUse: string | Record<string, string>) {
		if (!oauthUrlToUse) return;

		// Check if it's a single OAuth URL (string) or multiple (map)
		if (typeof oauthUrlToUse === 'string') {
			oauthURL = oauthUrlToUse;
			oauthURLs = undefined;
		} else {
			// It's a map of component IDs to OAuth URLs
			oauthURLs = oauthUrlToUse;
			oauthURL = undefined;
		}

		oauthDialog?.open();

		// add visibility change listener
		document.addEventListener('visibilitychange', handleVisibilityChange);
	}

	function markComponentAuthenticated(componentId: string) {
		// Create new Set to trigger reactivity in Svelte 5
		authenticatedComponents = new Set([...authenticatedComponents, componentId]);
	}

	async function handleLaunchTemporaryInstance(showInlineError = false) {
		if (!entry || !id) return;

		// For MCPServers (multi-user deployments), use the catalogEntryID for tool preview generation
		// and dryRun mode since the underlying catalog entry is not editable.
		const isMCPServer = entry.type === 'mcpserver';
		const entryID = isMCPServer ? (entry as MCPCatalogServer).catalogEntryID : entry.id;
		if (!entryID) return;

		error = undefined;
		showButtonInlineError = false;
		saving = true;
		const body = compileTemporaryInstanceBody();
		try {
			const result =
				entity === 'workspace'
					? await UserService.generateWorkspaceMCPCatalogEntryToolPreviews(
							id,
							entryID,
							body as unknown as { config?: Record<string, string>; url?: string },
							{ dryRun: isMCPServer }
						)
					: await AdminService.generateMcpCatalogEntryToolPreviews(
							id,
							entryID,
							body as unknown as { config?: Record<string, string>; url?: string },
							{ dryRun: isMCPServer }
						);

			if (isMCPServer && result && entry) {
				// In dryRun mode, the previews are returned but not persisted.
				// Update the entry's tool preview in-place.
				(entry as MCPCatalogServer).manifest.toolPreview = result.manifest?.toolPreview ?? [];
			}
		} catch (err) {
			const errMessage = err instanceof Error ? err.message : 'An unknown error occurred';
			if (errMessage.includes('MCP server requires OAuth authentication')) {
				const oauthResponse =
					entity === 'workspace'
						? await UserService.getWorkspaceMCPCatalogEntryToolPreviewsOauth(
								id,
								entryID,
								body as unknown as { config?: Record<string, string>; url?: string }
							)
						: await AdminService.getMcpCatalogToolPreviewsOauth(
								id,
								entryID,
								body as unknown as { config?: Record<string, string>; url?: string }
							);
				if (oauthResponse) {
					configDialog?.close();
					handleTemporaryInstanceOauth(oauthResponse);
				}
			} else {
				error = err instanceof Error ? err.message : 'An unknown error occurred';
				showButtonInlineError = showInlineError;
			}
		} finally {
			saving = false;
		}
	}

	function handleInitTemporaryInstance() {
		if (!entry) return;

		if (entry.manifest?.runtime === 'composite') {
			const comps = entry.manifest?.compositeConfig?.componentServers || [];
			const componentConfigs: Record<string, ComponentLaunchFormData> = {};
			for (const c of comps) {
				// Use catalogEntryID when present (catalog-based component), otherwise fall
				// back to mcpServerID (multi-user server component). Skip only if we have
				// neither identifier.
				const id = c.catalogEntryID || c.mcpServerID;
				if (!id) continue;

				const rc = c.manifest?.remoteConfig as Record<string, unknown> | undefined;
				const hasHostname = Boolean(rc && 'hostname' in rc && rc.hostname);
				const isMultiUser = Boolean(c.mcpServerID && !c.catalogEntryID);
				componentConfigs[id] = isMultiUser
					? {
							// Multi-user server components are configured at the org/admin level;
							// for composite previews we only expose the enable/disable toggle.
							name: c.manifest?.name || id,
							icon: c.manifest?.icon,
							disabled: false,
							isMultiUser: true
						}
					: {
							envs: (c.manifest?.env || []).map((e) => ({ ...e, value: '' })),
							headers: (c.manifest?.remoteConfig?.headers || []).map((h) => ({ ...h, value: '' })),
							...(hasHostname
								? { hostname: (rc as Record<string, unknown>).hostname as string, url: '' }
								: {}),
							name: c.manifest?.name || id,
							icon: c.manifest?.icon,
							disabled: false
						};
			}
			configureForm = { componentConfigs } as CompositeLaunchFormData;

			// Always open the composite configuration dialog so the user can
			// enable/disable individual components before generating previews,
			// even if no component has required config fields.
			configDialog?.open();
			return;
		}

		const hostname =
			entry?.manifest?.remoteConfig &&
			'hostname' in entry.manifest.remoteConfig &&
			entry.manifest.remoteConfig.hostname;

		configureForm = {
			name: '',
			envs: entry.manifest?.env?.map((env) => ({
				...env,
				value: ''
			})),
			headers: entry.manifest?.remoteConfig?.headers?.map((header) => ({
				...header,
				value: ''
			})),
			...(hostname ? { hostname, url: '' } : {})
		};

		const needsEnvValue = configureForm.envs?.some((env) => !env.value);
		const needsHeaderValue = configureForm.headers?.some((header) => !header.value);
		const hasConfigFields =
			type !== 'multi' &&
			(needsEnvValue || needsHeaderValue || (configureForm as LaunchFormData).hostname);
		if (hasConfigFields) {
			configDialog?.open();
		} else {
			handleLaunchTemporaryInstance(true);
		}
	}

	async function handleConfigureOAuth() {
		if (!entry || !id) return;
		try {
			staticOauthStatus =
				entity === 'workspace'
					? await UserService.getWorkspaceMCPCatalogEntryOAuthCredentials(id, entry.id)
					: await AdminService.getMCPCatalogEntryOAuthCredentials(id, entry.id);
		} catch {
			staticOauthStatus = { configured: false };
		}
		staticOauthConfigModal?.open();
	}

	function handleCancel() {
		if (onCancel) {
			onCancel();
		} else {
			goto(`${prefix}/mcp-catalog`);
		}
	}

	function handleSubmit(updatedEntry: MCPCatalogEntry | MCPCatalogServer, message?: string) {
		if (onSubmit) {
			const isMultiUserEntry =
				'isCatalogEntry' in updatedEntry
					? updatedEntry.manifest?.serverUserType === 'multiUser'
					: true;
			onSubmit(updatedEntry.id, isMultiUserEntry, message);
		} else {
			entry = updatedEntry;

			if ('isCatalogEntry' in updatedEntry && id) {
				const listInstances =
					entity === 'workspace'
						? UserService.getWorkspaceCatalogEntryServers
						: AdminService.listMCPServersForEntry;
				listInstances(id, updatedEntry.id).then((response) => {
					console.log(response);
					if (response.length > 0 && response.some((instance) => instance)) {
						showUpdateExistingDeploymentsConfirm = true;
					}
				});
			}
			if (message) {
				success.add(message);
			}
		}
	}
</script>

<div
	class="flex h-full w-full flex-col gap-4"
	class:mb-8={selected !== 'configuration' &&
		selected !== 'server-instances' &&
		selected === 'configuration' &&
		readonly}
>
	{#if entry}
		<div class="flex items-center justify-between gap-4">
			<div class="flex items-center gap-2">
				<div class="icon">
					{#if entry.manifest.icon}
						<img src={entry.manifest.icon} alt={entry.manifest.name} class="size-10 shrink-0" />
					{:else}
						<Server class="size-10" />
					{/if}
				</div>
				<h1 class="text-2xl font-semibold capitalize">
					{#if entry}
						{getMCPDisplayName(entry)}
					{/if}
				</h1>
				<div class="pill-rounded">
					{getServerTypeLabel(entry)}
				</div>
				{#if source}
					{#if source.type === 'git'}
						<a
							href={source.url}
							target="_blank"
							rel="external noopener noreferrer"
							class="btn btn-xs btn-primary px-3!"
						>
							{source.url?.split('/').pop()}
							<ExternalLink class="size-3" />
						</a>
					{:else}
						<div class="pill-rounded">
							{source.name}
						</div>
					{/if}
				{/if}
			</div>
			{#if belongsToUser && !readonly}
				<IconButton
					variant="danger2"
					tooltip={{ text: 'Delete Server' }}
					onclick={() => {
						deleteServer = true;
					}}
				>
					<Trash2 class="size-4" />
				</IconButton>
			{/if}
		</div>
	{/if}

	{#if requiresStaticOauth}
		<div class="flex items-center gap-3 rounded-lg border border-warning bg-warning/10 p-4">
			<Info class="size-5 shrink-0 text-warning" />
			<div class="flex-1">
				<p class="text-sm font-medium">Requires Oauth Config</p>
				<p class="text-muted-foreground mt-1 text-xs">
					This MCP server is missing static client ID and secret credentials. Click the button to
					get started.
				</p>
			</div>
			<button
				class="btn btn-secondary flex items-center gap-1.5 font-normal"
				onclick={handleConfigureOAuth}
			>
				<Settings class="size-4" />
				Configure OAuth Credentials
			</button>
		</div>
	{/if}

	<div class="flex grow flex-col gap-2">
		{#if tabs.length > 0 && (entry?.id || server?.id)}
			<OverflowContainer
				class="scrollbar-none flex min-h-12 w-full items-center gap-2 overflow-x-auto"
				style="scroll-behavior: smooth;"
				{@attach (node: HTMLDivElement) => (scrollContainer = node)}
			>
				{#snippet children({ x })}
					{#if tabs.length > 0 && (entry?.id || server?.id)}
						{#if x}
							<button
								disabled={!showLeftChevron}
								onclick={scrollLeft}
								class="bg-base-200 dark:bg-base-100 sticky left-0 flex aspect-square h-full items-center justify-center rounded-l-md p-2.5 opacity-100 transition-all duration-200 disabled:opacity-30"
							>
								<ChevronLeft class="size-full" />
							</button>
						{/if}

						<div class="flex flex-1 gap-2 py-1 text-sm font-light">
							{#each tabs as tab (tab.view)}
								<button
									onclick={() => {
										handleSelectionChange(tab.view);
									}}
									class={twMerge(
										'min-w-fit flex-1 rounded-md border border-transparent px-3 py-2 text-center whitespace-nowrap transition-colors duration-300',
										selected === tab.view &&
											'dark:bg-base-200 dark:border-base-400 bg-base-100 shadow-sm',
										selected !== tab.view && 'hover:bg-base-400',
										tab.view === 'troubleshooting' && 'flex items-center justify-center gap-1'
									)}
								>
									{tab.label}
								</button>
							{/each}
						</div>

						{#if x}
							<button
								disabled={!showRightChevron}
								onclick={scrollRight}
								class="bg-base-200 dark:bg-base-100 sticky right-0 flex aspect-square h-full items-center justify-center rounded-r-md p-2.5 opacity-100 transition-all duration-200 disabled:opacity-30"
							>
								<ChevronRight class="size-full" />
							</button>
						{/if}
					{/if}
				{/snippet}
			</OverflowContainer>
		{/if}

		{#if selected === 'overview' && entry}
			<div class="pb-8">
				{#if !('isCatalogEntry' in entry) && entry.needsUpdate}
					<div class="notification-info mb-3 p-3 text-sm font-light">
						<div class="flex items-center gap-3">
							<CircleFadingArrowUp class="size-6" />
							<p>
								The configuration for this server's catalog entry has changed and can be applied to
								this server.
							</p>
						</div>
					</div>
				{/if}
				{#if hasExistingConfigured}
					<div class="notification-info mb-3 p-3 text-sm font-light">
						<div class="flex items-center gap-3">
							<Info class="size-6" />
							<p>
								It looks like you already have an existing server instance available. It is
								recommended to only create another one if you need to instantiate another one with
								different configurations.
							</p>
						</div>
					</div>
				{/if}
				<McpServerInfo
					{entry}
					descriptionPlaceholder="Add a description for this MCP server in the Configuration tab"
				/>
			</div>
		{:else if selected === 'configuration'}
			{@render configurationView()}
		{:else if selected === 'tools' && entry}
			<div class="pb-8">
				{#if showRegenerateToolsButton}
					<button class="btn btn-primary mb-4 text-sm" onclick={handleInitTemporaryInstance}>
						Regenerate Tools & Capabilities
					</button>
				{/if}
				<McpServerTools
					entry={'isCatalogEntry' in entry && server ? server : entry}
					showToolNameIssues={entry.manifest?.runtime === 'composite'}
				>
					{#snippet noToolsContent()}
						<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
							<Wrench class="text-muted-content size-24 opacity-50" />
							{#if !entry || (entry && (readonly || server))}
								<h4 class="text-muted-content text-lg font-semibold">No tools</h4>
								<p class="text-muted-content text-sm font-light">
									Looks like this MCP server doesn't have any tools available currently.
								</p>
							{:else if !readonly}
								<h4 class="text-muted-content text-lg font-semibold">No tools</h4>
								<button
									class="btn btn-primary flex items-center gap-1 text-sm"
									onclick={handleInitTemporaryInstance}
									disabled={saving}
								>
									{#if saving}
										<Loading class="size-4" />
									{:else}
										Populate Tool Preview
									{/if}
								</button>
								{#if !error}
									<p class="text-muted-content text-sm font-light">
										{#if type === 'remote'}
											Click above to connect to the remote MCP server to populate capabilities and
											tools.
										{:else}
											Click above to set up a temporary instance that will populate capabilities and
											tools. Otherwise, tools will populate when the user first deploys a server for
											the catalog entry.
										{/if}
									</p>
								{/if}
							{/if}
						</div>
						{#if error && showButtonInlineError}
							<div class="mt-4 w-full">
								{@render errorSnippet()}
							</div>
						{/if}
					{/snippet}
				</McpServerTools>
			</div>
		{:else if selected === 'access-control'}
			{@render accessControlView()}
		{:else if selected === 'usage'}
			{@render usageView()}
		{:else if selected === 'audit-logs'}
			{@render auditLogsView()}
		{:else if selected === 'server-instances'}
			{#if entry && 'isCatalogEntry' in entry && server}
				<McpServerDetails catalogEntry={entry} {server} />
			{:else}
				<McpServerInstances
					{id}
					{entity}
					entry={entry && 'isCatalogEntry' in entry && server ? server : entry}
					catalogEntry={entry && 'isCatalogEntry' in entry ? entry : undefined}
					{users}
					{type}
					{configuredServers}
				/>
			{/if}
		{:else if selected === 'filters'}
			{@render filtersView()}
		{:else if selected === 'troubleshooting'}
			{@render troubleshootingView()}
		{/if}
	</div>
</div>

{#snippet configurationView()}
	<div class="flex flex-col gap-8">
		<CatalogServerForm
			{entry}
			{type}
			readonly={configurationReadonly}
			{id}
			{entity}
			onCancel={handleCancel}
			onSubmit={handleSubmit}
			hideTitle={Boolean(entry)}
			onConfigureOAuth={handleConfigureOAuth}
		>
			{#snippet readonlyMessage()}
				{#if entry && 'sourceURL' in entry && !!entry.sourceURL}
					<p>
						This catalog entry comes from an external Git Source URL <span
							class="text-muted-content text-xs">({entry.sourceURL.split('/').pop()})</span
						> and cannot be edited.
					</p>
				{:else}
					<p>This catalog entry is non-editable.</p>
				{/if}
			{/snippet}
		</CatalogServerForm>
	</div>
{/snippet}

{#snippet accessControlView()}
	{#await listAccessControlRules}
		<div class="flex w-full justify-center">
			<Loading class="size-6" />
		</div>
	{:then rules}
		{@const serverRules = entry ? filterRulesByEntry(rules) : []}
		{#if serverRules && serverRules.length > 0}
			<Table
				data={serverRules}
				fields={['displayName', 'resources']}
				headers={[
					{ title: 'Rule', property: 'displayName' },
					{ title: 'Accessible To', property: 'resources' }
				]}
				onClickRow={(d, isCtrlClick) => {
					if (!entry) return;
					setLastVisitedMcpServer();

					const isAdminRoute = window.location.pathname.includes('/admin/');

					let url = '';
					if (entity === 'workspace') {
						url = !isAdminRoute
							? `/mcp-access-policies/${d.id}`
							: `/admin/mcp-access-policies/w/${id}/r/${d.id}`;
					} else {
						url = `/admin/mcp-access-policies/${d.id}`;
					}
					openUrl(url, isCtrlClick);
				}}
			>
				{#snippet onRenderColumn(property, d)}
					{#if property === 'resources'}
						{@const hasEveryone = d.subjects?.find((s) => s.id === '*')}
						{@const { totalUsers, totalGroups } = d.subjects?.reduce(
							(acc, s) => {
								if (s.type === 'user') {
									acc.totalUsers++;
								} else {
									acc.totalGroups++;
								}
								return acc;
							},
							{ totalUsers: 0, totalGroups: 0 }
						) ?? { totalUsers: 0, totalGroups: 0 }}
						{#if hasEveryone}
							Everyone
						{:else}
							{@const userCount = `${totalUsers} user${totalUsers === 1 ? '' : 's'}`}
							{@const groupCount = `${totalGroups} group${totalGroups === 1 ? '' : 's'}`}
							{#if totalUsers > 0 && totalGroups > 0}
								{userCount}, {groupCount}
							{:else if totalUsers > 0}
								{userCount}
							{:else if totalGroups > 0}
								{groupCount}
							{/if}
						{/if}
					{:else}
						{d[property as keyof typeof d]}
					{/if}
				{/snippet}
			</Table>
		{:else}
			<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
				<GlobeLock class="text-muted-content size-24 opacity-50" />
				<h4 class="text-muted-content text-lg font-semibold">No MCP access policies</h4>
				<p class="text-muted-content text-sm font-light">
					This server is not tied to any access policies.
				</p>
			</div>
		{/if}
	{/await}
{/snippet}

{#snippet usageView()}
	{#if entry}
		{@const isMultiUserServer = !!page.url.pathname.match(/\/mcp-servers\/s.*$/)?.[0]}
		{@const isSingleUserServer =
			!isMultiUserServer && ['npx', 'uvx', 'containerized'].includes(entry.manifest.runtime)}
		{@const isRemoteServer = !isMultiUserServer && entry.manifest.runtime === 'remote'}

		{@const mcpServerDisplayName = entry.manifest?.name ?? null}
		{@const entryId = entry.id ?? null}

		<div class="mt-4 flex min-h-full flex-col gap-8 pb-8">
			<UsageGraphs
				mcpId={isMultiUserServer ? entryId : null}
				mcpServerCatalogEntryName={isSingleUserServer || isRemoteServer ? entryId : null}
				{mcpServerDisplayName}
			/>
		</div>
	{/if}
{/snippet}

{#snippet auditLogsView()}
	{#if entry}
		{@const isMultiUserServer = 'serverUserType' in entry && entry.serverUserType === 'multiUser'}
		{@const isSingleUserServer =
			!isMultiUserServer && ['npx', 'uvx', 'containerized'].includes(entry.manifest.runtime)}
		{@const isRemoteServer = !isMultiUserServer && entry.manifest.runtime === 'remote'}

		{@const mcpServerDisplayName = entry.manifest?.name ?? null}
		{@const entryId = entry.id ?? null}
		{@const mcpCatalogEntryId = 'catalogEntryID' in entry ? entry?.catalogEntryID : null}
		{@const mcpServerCatalogEntryName =
			isMultiUserServer && mcpCatalogEntryId
				? mcpCatalogEntryId
				: isSingleUserServer || isRemoteServer
					? entryId
					: null}
		<div class="mt-4 flex flex-1 flex-col gap-8 pb-8">
			<!-- temporary filter mcp server by name and catalog entry id-->
			<AuditLogsPageContent
				mcpId={isMultiUserServer ? entryId : server ? server.id : null}
				{mcpServerCatalogEntryName}
				{mcpServerDisplayName}
				{id}
				{entity}
			>
				{#snippet emptyContent()}
					<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
						<Users class="text-muted-content size-24 opacity-50" />
						<h4 class="text-muted-content text-lg font-semibold">No recent audit logs</h4>
						<p class="text-muted-content text-sm font-light">
							This server has not had any active usage in the last 7 days.
						</p>
						{#if entryId || mcpCatalogEntryId}
							{@const param = entryId ? 'mcpId=' + entryId : 'entryId=' + mcpCatalogEntryId}
							<p class="text-muted-content text-sm font-light">
								See more usage details in the server's <a
									href={resolve(`/admin/audit-logs?${param}`)}
									class="text-link"
								>
									Audit Logs
								</a>.
							</p>
						{/if}
					</div>
				{/snippet}
			</AuditLogsPageContent>
		</div>
	{/if}
{/snippet}

{#snippet filtersView()}
	{#if listFilters}
		{#await listFilters}
			<div class="flex w-full justify-center">
				<Loading class="size-6" />
			</div>
		{:then filters}
			{@const serverFilters = entry ? filterFiltersByEntry(filters) : []}
			{#if serverFilters && serverFilters.length > 0}
				<Table
					data={serverFilters}
					fields={['name', 'url', 'selectors']}
					headers={[
						{ title: 'Name', property: 'name' },
						{ title: 'Webhook URL', property: 'url' },
						{ title: 'Selectors', property: 'selectors' }
					]}
					onClickRow={(d, isCtrlClick) => {
						setLastVisitedMcpServer();
						const url = `/admin/filters/${d.id}`;
						openUrl(url, isCtrlClick);
					}}
				>
					{#snippet onRenderColumn(property, d)}
						{#if property === 'name'}
							{d.name || '-'}
						{:else if property === 'url'}
							{d.url || '-'}
						{:else if property === 'selectors'}
							{@const count = d.selectors?.length || 0}
							{count > 0 ? `${count} selector${count > 1 ? 's' : ''}` : '-'}
						{:else}
							{d[property as keyof typeof d]}
						{/if}
					{/snippet}
				</Table>
			{:else}
				<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
					<ListFilter class="text-muted-content size-24 opacity-50" />
					<h4 class="text-muted-content text-lg font-semibold">No filters configured</h4>
					<p class="text-muted-content text-sm font-light">
						This server is not referenced by any filters.
					</p>
				</div>
			{/if}
		{/await}
	{:else}
		<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
			<ListFilter class="text-muted-content size-24 opacity-50" />
			<h4 class="text-muted-content text-lg font-semibold">No filters available</h4>
			<p class="text-muted-content text-sm font-light">
				No filters have been configured in the system.
			</p>
		</div>
	{/if}
{/snippet}

{#snippet troubleshootingView()}
	{#if server}
		<div class="flex flex-col bg-base-100 dark:bg-base-300 rounded-md pt-4">
			<h1 class="text-lg font-semibold px-4 pb-2">Debug OAuth Flow</h1>
			<DebugOauthFlow mcpServer={server} />
		</div>
	{/if}
{/snippet}

<Confirm
	msg={`Delete ${entry?.manifest?.name || 'this server'}?`}
	show={deleteServer}
	onsuccess={async () => {
		if (!id || !entry) return;
		const url = `${prefix}/mcp-catalog` as `/${string}`;

		if (!('isCatalogEntry' in entry)) {
			const workspaceID = entry.powerUserWorkspaceID || (entity === 'workspace' ? id : undefined);
			const deleteServerFn = workspaceID
				? UserService.deleteWorkspaceMCPCatalogServer
				: AdminService.deleteMCPCatalogServer;
			try {
				await deleteServerFn(workspaceID || id, entry.id);
			} catch (error) {
				if (error instanceof MCPCompositeDeletionDependencyError) {
					deleteConflictError = error;
					return;
				}
				throw error;
			}
			goto(url);
		} else {
			const deleteCatalogEntryFn =
				entity === 'workspace'
					? UserService.deleteWorkspaceMCPCatalogEntry
					: AdminService.deleteMCPCatalogEntry;
			await deleteCatalogEntryFn(id, entry.id);
			goto(url);
		}
	}}
	oncancel={() => (deleteServer = false)}
/>

<McpMultiDeleteBlockedDialog
	show={!!deleteConflictError}
	error={deleteConflictError}
	onClose={() => {
		deleteConflictError = undefined;
	}}
/>

<Confirm
	msg={deleteResourceFromRule?.resourceId === '*'
		? 'Remove Everything from this rule?'
		: 'Remove this MCP server from this rule?'}
	show={Boolean(deleteResourceFromRule)}
	onsuccess={async () => {
		if (!deleteResourceFromRule) {
			return;
		}

		const updateAccessControlRuleFn =
			entity === 'workspace' && id
				? UserService.updateWorkspaceAccessControlRule(
						id,
						deleteResourceFromRule.rule.id,
						deleteResourceFromRule.rule
					)
				: AdminService.updateAccessControlRule(
						deleteResourceFromRule.rule.id,
						deleteResourceFromRule.rule
					);
		await updateAccessControlRuleFn;

		listAccessControlRules =
			entity === 'workspace' && id
				? UserService.listWorkspaceAccessControlRules(id)
				: AdminService.listAccessControlRules();
		deleteResourceFromRule = undefined;
	}}
	oncancel={() => (deleteResourceFromRule = undefined)}
/>

<CatalogConfigureForm
	bind:this={configDialog}
	bind:form={configureForm}
	{error}
	icon={entry?.manifest?.icon}
	name={entry?.manifest?.name}
	onSave={handleLaunchTemporaryInstance}
	submitText="Launch"
	loading={saving}
	isNew={false}
/>

<ResponsiveDialog
	bind:this={oauthDialog}
	title="Authentication Required"
	class="w-md"
	onClose={() => {
		// Clean up when dialog closes
		document.removeEventListener('visibilitychange', handleVisibilityChange);
	}}
>
	{#if error}
		{@render errorSnippet()}
	{/if}
	{#if saving}
		<div class="flex w-full justify-center">
			<Loading class="size-6" />
		</div>
	{:else if oauthURL}
		<!-- Single server OAuth -->
		<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external OAuth URL -->
		<a href={oauthURL} rel="external" target="_blank" class="btn btn-primary text-center"
			>Authenticate</a
		>
	{:else if oauthURLs && Object.keys(oauthURLs).length > 0}
		<!-- Composite server OAuth - multiple components -->
		<div class="flex flex-col gap-3">
			<p class="text-muted-content text-sm">
				Multiple components require authentication. Please authenticate each component below:
			</p>
			{#each Object.entries(oauthURLs).filter(([id]) => !authenticatedComponents.has(id)) as [componentId, url] (componentId)}
				{@const component = entry?.manifest?.compositeConfig?.componentServers?.find(
					(c) => c.catalogEntryID === componentId || c.mcpServerID === componentId
				)}
				{@const componentName = component?.manifest?.name || componentId}
				<div class="flex items-center justify-between gap-2 rounded border border-base-400 p-3">
					<div class="flex items-center gap-2">
						{#if component?.manifest?.icon}
							<img src={component.manifest.icon} alt={componentName} class="size-6 shrink-0" />
						{/if}
						<span class="text-sm font-medium">{componentName}</span>
					</div>
					<button
						type="button"
						class="btn btn-primary text-sm"
						onclick={() => {
							markComponentAuthenticated(componentId);
							const newWindow = window.open(url, '_blank', 'noopener,noreferrer');
							if (newWindow) {
								newWindow.opener = null;
							}
						}}
					>
						Authenticate
					</button>
				</div>
			{/each}
		</div>
	{/if}
</ResponsiveDialog>

<StaticOAuthConfigureModal
	bind:this={staticOauthConfigModal}
	oauthStatus={staticOauthStatus}
	onSave={async (credentials) => {
		if (!entry || !id) return;
		if (entity === 'workspace') {
			await UserService.setWorkspaceMCPCatalogEntryOAuthCredentials(id, entry.id, credentials);
		} else {
			await AdminService.setMCPCatalogEntryOAuthCredentials(id, entry.id, credentials);
		}
		staticOauthStatus = {
			...staticOauthStatus,
			configured: true
		};
	}}
	onDelete={async () => {
		if (!entry || !id) return;
		if (entity === 'workspace') {
			await UserService.deleteWorkspaceMCPCatalogEntryOAuthCredentials(id, entry.id);
		} else {
			await AdminService.deleteMCPCatalogEntryOAuthCredentials(id, entry.id);
		}
		staticOauthStatus = {
			...staticOauthStatus,
			configured: false
		};
	}}
/>

{#snippet errorSnippet()}
	<div class="notification-error flex items-center gap-2">
		<CircleAlert class="size-6 shrink-0 text-error" />
		<p class="flex flex-col text-left text-sm font-light">
			<span class="font-semibold">Error with launching temporary instance:</span>
			<span>
				{error}
			</span>
		</p>
	</div>
{/snippet}

<Confirm
	title="Update Deployments"
	msg="Update existing deployments now?"
	show={showUpdateExistingDeploymentsConfirm}
	onsuccess={() => {
		showUpdateExistingDeploymentsConfirm = false;
		handleSelectionChange('server-instances');
	}}
	oncancel={() => {
		showUpdateExistingDeploymentsConfirm = false;
	}}
	cancelText="Skip"
	submitText="Go to Server Details"
	type="info"
>
	{#snippet note()}
		<p class="text-sm font-light">
			There are existing deployment(s) of this MCP server that need to be updated. Would you like to
			take care of this now?
		</p>

		{#if profile.current.hasAdminAccess?.()}
			<p class="text-xs font-light mt-2 text-muted-content">
				Deployments can also be updated at a later time through the "Server Details" tab or through
				the MCP Management "Deployments" page.
			</p>
		{/if}
	{/snippet}
</Confirm>
