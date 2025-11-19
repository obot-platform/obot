<script lang="ts">
	import {
		AdminService,
		type MCPFilter,
		type MCPCatalogServer,
		ChatService,
		Group,
		MCPCompositeDeletionDependencyError
	} from '$lib/services';
	import type { AccessControlRule, MCPCatalogEntry, OrgUser } from '$lib/services/admin/types';
	import { twMerge } from 'tailwind-merge';
	import McpServerInfo from '../mcp/McpServerInfo.svelte';
	import CatalogServerForm from './CatalogServerForm.svelte';
	import Table from '../table/Table.svelte';
	import {
		AlertCircle,
		ChevronLeft,
		ChevronRight,
		GlobeLock,
		ListFilter,
		LoaderCircle,
		Server,
		Trash2,
		Users,
		Wrench
	} from 'lucide-svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { goto } from '$app/navigation';
	import Confirm from '../Confirm.svelte';
	import { ADMIN_SESSION_STORAGE } from '$lib/constants';
	import { onMount } from 'svelte';
	import UsageGraphs from './usage/UsageGraphs.svelte';
	import McpServerInstances from './McpServerInstances.svelte';
	import McpServerTools from '../mcp/McpServerTools.svelte';
	import AuditLogsPageContent from './audit-logs/AuditLogsPageContent.svelte';
	import { page } from '$app/state';
	import { getRegistryLabel, openUrl } from '$lib/utils';
	import CatalogConfigureForm, {
		type LaunchFormData,
		type CompositeLaunchFormData,
		type ComponentLaunchFormData
	} from '../mcp/CatalogConfigureForm.svelte';
	import McpMultiDeleteBlockedDialog from '../mcp/McpMultiDeleteBlockedDialog.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import { setVirtualPageDisabled } from '../ui/virtual-page/context';
	import { profile } from '$lib/stores';
	import OverflowContainer from '../OverflowContainer.svelte';
	import { getServerTypeLabel } from '$lib/services/chat/mcp';

	type MCPType = 'single' | 'multi' | 'remote' | 'composite';

	interface Props {
		id?: string;
		entity?: 'workspace' | 'catalog';
		entry?: MCPCatalogEntry | MCPCatalogServer;
		type?: MCPType;
		readonly?: boolean;
		onCancel?: () => void;
		onSubmit?: (id: string, type: MCPType) => void;
	}

	let { entry, id, entity = 'catalog', type, readonly, onCancel, onSubmit }: Props = $props();
	let isAtLeastPowerUserPlus = $derived(profile.current?.groups.includes(Group.POWERUSER_PLUS));
	let isAuditor = $derived(profile.current?.groups.includes(Group.AUDITOR));

	const tabs = $derived(
		entry
			? [
					{ label: 'Overview', view: 'overview' },
					...(profile.current?.groups.includes(Group.POWERUSER)
						? [{ label: 'Server Details', view: 'server-instances' }]
						: []),
					{ label: 'Tools', view: 'tools' },
					...(profile.current?.groups.includes(Group.POWERUSER)
						? [
								{ label: 'Configuration', view: 'configuration' },
								{ label: 'Audit Logs', view: 'audit-logs' },
								{ label: 'Usage', view: 'usage' }
							]
						: []),
					...(isAtLeastPowerUserPlus ? [{ label: 'Access Control', view: 'access-control' }] : []),
					...(profile.current?.hasAdminAccess?.() ? [{ label: 'Filters', view: 'filters' }] : [])
				]
			: []
	);

	let listAccessControlRules = $state<Promise<AccessControlRule[]>>();
	let listFilters = $state<Promise<MCPFilter[]>>();
	let users = $state<OrgUser[]>([]);
	let registry = $derived.by(() => {
		if (!entry) return undefined;
		const ownerUserId =
			'isCatalogEntry' in entry
				? entry.powerUserID
				: entry.powerUserWorkspaceID
					? entry.userID
					: undefined;
		return getRegistryLabel(ownerUserId, profile.current?.id, users);
	});

	let deleteServer = $state(false);
	let deleteConflictError = $state<MCPCompositeDeletionDependencyError | undefined>();
	let deleteResourceFromRule = $state<{
		rule: AccessControlRule;
		resourceId: string;
	}>();
	let selected = $derived.by(() => {
		const searchParams = page.url.searchParams;

		const tab = searchParams.get('view');

		return tab ?? (entry ? 'overview' : 'configuration');
	});
	let showLeftChevron = $state(false);
	let showRightChevron = $state(false);
	let scrollContainer = $state<HTMLDivElement>();

	let oauthDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let oauthURL = $state<string>();
	let oauthURLs = $state<Record<string, string>>();
	let authenticatedComponents = $state<Set<string>>(new Set());

	let configDialog = $state<ReturnType<typeof CatalogConfigureForm>>();
	let configureForm = $state<LaunchFormData | CompositeLaunchFormData>();
	let saving = $state(false);
	let error = $state<string>();
	let showButtonInlineError = $state(false);

	let showRegenerateToolsButton = $derived(
		entry &&
			entry.manifest?.toolPreview &&
			'toolPreviewsLastGenerated' in entry &&
			'lastUpdated' in entry &&
			entry.toolPreviewsLastGenerated &&
			entry.lastUpdated &&
			new Date(entry.toolPreviewsLastGenerated) < new Date(entry.lastUpdated)
	);

	$effect(() => {
		if (selected === 'access-control') {
			listAccessControlRules =
				entity === 'workspace' && id
					? ChatService.listWorkspaceAccessControlRules(id)
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
		if (isAtLeastPowerUserPlus || isAuditor) {
			AdminService.listUsersIncludeDeleted().then((data) => {
				users = data;
			});
		}

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
			r.resources?.find((resource) => resource.id === entry.id || resource.id === '*')
		);
	}

	function filterFiltersByEntry(filters?: MCPFilter[]) {
		if (!entry || !filters) return [];
		return filters.filter((f) =>
			f.resources?.find((resource) => resource.id === entry.id || resource.id === '*')
		);
	}

	function setLastVisitedMcpServer() {
		if (!entry) return;
		const name = entry.manifest.name;
		sessionStorage.setItem(
			ADMIN_SESSION_STORAGE.LAST_VISITED_MCP_SERVER,
			JSON.stringify({ id: entry.id, name, type, entity, entityId: id })
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
		if (newSelection !== selected) {
			const url = new URL(window.location.href);
			url.searchParams.set('view', newSelection);
			goto(url.toString(), { replaceState: true });
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

		error = undefined;
		showButtonInlineError = false;
		saving = true;
		const body = compileTemporaryInstanceBody();
		try {
			if (entity === 'workspace') {
				await ChatService.generateWorkspaceMCPCatalogEntryToolPreviews(
					id,
					entry.id,
					body as unknown as { config?: Record<string, string>; url?: string }
				);
			} else {
				await AdminService.generateMcpCatalogEntryToolPreviews(
					id,
					entry.id,
					body as unknown as { config?: Record<string, string>; url?: string }
				);
			}
			window.location.reload();
		} catch (err) {
			const errMessage = err instanceof Error ? err.message : 'An unknown error occurred';
			if (errMessage.includes('MCP server requires OAuth authentication')) {
				const oauthResponse =
					entity === 'workspace'
						? await ChatService.getWorkspaceMCPCatalogEntryToolPreviewsOauth(
								id,
								entry.id,
								body as unknown as { config?: Record<string, string>; url?: string }
							)
						: await AdminService.getMcpCatalogToolPreviewsOauth(
								id,
								entry.id,
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
						<img
							src={entry.manifest.icon}
							alt={entry.manifest.name}
							class="size-10 flex-shrink-0"
						/>
					{:else}
						<Server class="size-10" />
					{/if}
				</div>
				<h1 class="text-2xl font-semibold capitalize">{entry.manifest.name || 'Unknown'}</h1>
				<div class="pill-rounded">
					{getServerTypeLabel(entry)}
				</div>
				{#if registry}
					<div class="pill-rounded">
						{registry}
					</div>
				{/if}
			</div>
			{#if !readonly}
				<button
					class="button-destructive flex items-center gap-1 text-xs font-normal"
					use:tooltip={'Delete Server'}
					onclick={() => {
						deleteServer = true;
					}}
				>
					<Trash2 class="size-4" />
				</button>
			{/if}
		</div>
	{/if}
	<div class="flex grow flex-col gap-2">
		<OverflowContainer
			class="scrollbar-none flex min-h-fit w-full items-center gap-2 overflow-x-auto"
			style="scroll-behavior: smooth;"
			{@attach (node: HTMLDivElement) => (scrollContainer = node)}
		>
			{#snippet children({ x })}
				{#if tabs.length > 0}
					{#if x}
						<button
							disabled={!showLeftChevron}
							onclick={scrollLeft}
							class="bg-surface1 dark:bg-background sticky left-0 flex aspect-square h-full items-center justify-center rounded-l-md p-2.5 opacity-100 transition-all duration-200 disabled:opacity-30"
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
									'w-48 flex-shrink-0 rounded-md border border-transparent px-4 py-2 text-center transition-colors duration-300',
									selected === tab.view &&
										'dark:bg-surface1 dark:border-surface3 bg-background shadow-sm',
									selected !== tab.view && 'hover:bg-surface3'
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
							class="bg-surface1 dark:bg-background sticky right-0 flex aspect-square h-full items-center justify-center rounded-r-md p-2.5 opacity-100 transition-all duration-200 disabled:opacity-30"
						>
							<ChevronRight class="size-full" />
						</button>
					{/if}
				{/if}
			{/snippet}
		</OverflowContainer>

		{#if selected === 'overview' && entry}
			<div class="pb-8">
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
					<button class="button-primary mb-4 text-sm" onclick={handleInitTemporaryInstance}>
						Regenerate Tools & Capabilities
					</button>
				{/if}
				<McpServerTools {entry}>
					{#snippet noToolsContent()}
						<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
							<Wrench class="text-on-surface1 size-24 opacity-50" />
							{#if !entry || (entry && readonly)}
								<h4 class="text-on-surface1 text-lg font-semibold">No tools</h4>
								<p class="text-on-surface1 text-sm font-light">
									Looks like this MCP server doesn't have any tools available.
								</p>
							{:else if !readonly}
								<h4 class="text-on-surface1 text-lg font-semibold">No tools</h4>
								<button
									class="button-primary flex items-center gap-1 text-sm"
									onclick={handleInitTemporaryInstance}
									disabled={saving}
								>
									{#if saving}
										<LoaderCircle class="size-4 animate-spin" />
									{:else}
										Populate Tool Preview
									{/if}
								</button>
								{#if !error}
									<p class="text-on-surface1 text-sm font-light">
										{#if type === 'remote'}
											Click above to connect to the remote MCP server to populate capabilities and
											tools.
										{:else}
											Click above to set up a temporary instance that will populate capabilities and
											tools. Otherwise, tools will populate when the user first launches this
											server.
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
			<McpServerInstances {id} {entity} {entry} {users} {type} />
		{:else if selected === 'filters'}
			{@render filtersView()}
		{/if}
	</div>
</div>

{#snippet configurationView()}
	<div class="flex flex-col gap-8">
		<CatalogServerForm
			{entry}
			{type}
			{readonly}
			{id}
			{entity}
			{onCancel}
			{onSubmit}
			hideTitle={Boolean(entry)}
		>
			{#snippet readonlyMessage()}
				{#if entry && 'sourceURL' in entry && !!entry.sourceURL}
					<p>
						This MCP Server comes from an external Git Source URL <span
							class="text-no-surface1 text-xs">({entry.sourceURL.split('/').pop()})</span
						> and cannot be edited.
					</p>
				{:else}
					<p>This MCP server is non-editable.</p>
				{/if}
			{/snippet}
		</CatalogServerForm>
	</div>
{/snippet}

{#snippet accessControlView()}
	{#await listAccessControlRules}
		<div class="flex w-full justify-center">
			<LoaderCircle class="size-6 animate-spin" />
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
							? `/access-control/${d.id}`
							: `/admin/access-control/w/${id}/r/${d.id}`;
					} else {
						url = `/admin/access-control/${d.id}`;
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
				<GlobeLock class="text-on-surface1 size-24 opacity-50" />
				<h4 class="text-on-surface1 text-lg font-semibold">No access control rules</h4>
				<p class="text-on-surface1 text-sm font-light">
					This server is not tied to any access control rules.
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
		{@const isMultiUserServer = 'catalogEntryID' in entry ? !entry.catalogEntryID : false}
		{@const isSingleUserServer =
			!isMultiUserServer && ['npx', 'uvx', 'containerized'].includes(entry.manifest.runtime)}
		{@const isRemoteServer = !isMultiUserServer && entry.manifest.runtime === 'remote'}

		{@const mcpServerDisplayName = entry.manifest?.name ?? null}
		{@const entryId = entry.id ?? null}
		{@const mcpCatalogEntryId = 'catalogEntryID' in entry ? entry?.catalogEntryID : null}

		<div class="mt-4 flex flex-1 flex-col gap-8 pb-8">
			<!-- temporary filter mcp server by name and catalog entry id-->
			{#if id}
				<AuditLogsPageContent
					mcpId={isMultiUserServer ? entryId : null}
					mcpServerCatalogEntryName={isSingleUserServer || isRemoteServer ? entryId : null}
					{mcpServerDisplayName}
					{id}
					{entity}
				>
					{#snippet emptyContent()}
						<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
							<Users class="text-on-surface1 size-24 opacity-50" />
							<h4 class="text-on-surface1 text-lg font-semibold">No recent audit logs</h4>
							<p class="text-on-surface1 text-sm font-light">
								This server has not had any active usage in the last 7 days.
							</p>
							{#if entryId || mcpCatalogEntryId}
								{@const param = entryId ? 'mcpId=' + entryId : 'entryId=' + mcpCatalogEntryId}
								<p class="text-on-surface1 text-sm font-light">
									See more usage details in the server's <a
										href={`/admin/audit-logs?${param}`}
										class="text-link"
									>
										Audit Logs
									</a>.
								</p>
							{/if}
						</div>
					{/snippet}
				</AuditLogsPageContent>
			{/if}
		</div>
	{/if}
{/snippet}

{#snippet filtersView()}
	{#if listFilters}
		{#await listFilters}
			<div class="flex w-full justify-center">
				<LoaderCircle class="size-6 animate-spin" />
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
					<ListFilter class="text-on-surface1 size-24 opacity-50" />
					<h4 class="text-on-surface1 text-lg font-semibold">No filters configured</h4>
					<p class="text-on-surface1 text-sm font-light">
						This server is not referenced by any filters.
					</p>
				</div>
			{/if}
		{/await}
	{:else}
		<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
			<ListFilter class="text-on-surface1 size-24 opacity-50" />
			<h4 class="text-on-surface1 text-lg font-semibold">No filters available</h4>
			<p class="text-on-surface1 text-sm font-light">
				No filters have been configured in the system.
			</p>
		</div>
	{/if}
{/snippet}
<Confirm
	msg="Are you sure you want to delete this server?"
	show={deleteServer}
	onsuccess={async () => {
		if (!id || !entry) return;
		if (!('isCatalogEntry' in entry)) {
			const deleteServerFn =
				entity === 'workspace'
					? ChatService.deleteWorkspaceMCPCatalogServer
					: AdminService.deleteMCPCatalogServer;
			try {
				await deleteServerFn(id, entry.id);
			} catch (error) {
				if (error instanceof MCPCompositeDeletionDependencyError) {
					deleteConflictError = error;
					return;
				}
				throw error;
			}
		} else {
			const deleteCatalogEntryFn =
				entity === 'workspace'
					? ChatService.deleteWorkspaceMCPCatalogEntry
					: AdminService.deleteMCPCatalogEntry;
			await deleteCatalogEntryFn(id, entry.id);
			goto(entity === 'workspace' ? '/mcp-servers' : '/admin/mcp-servers');
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
		? 'Are you sure you want to remove Everything from this rule?'
		: 'Are you sure you want to remove this MCP server from this rule?'}
	show={Boolean(deleteResourceFromRule)}
	onsuccess={async () => {
		if (!deleteResourceFromRule) {
			return;
		}

		const updateAccessControlRuleFn =
			entity === 'workspace' && id
				? ChatService.updateWorkspaceAccessControlRule(
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
				? ChatService.listWorkspaceAccessControlRules(id)
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
			<LoaderCircle class="size-6 animate-spin" />
		</div>
	{:else if oauthURL}
		<!-- Single server OAuth -->
		<a href={oauthURL} target="_blank" class="button-primary text-center">Authenticate</a>
	{:else if oauthURLs && Object.keys(oauthURLs).length > 0}
		<!-- Composite server OAuth - multiple components -->
		<div class="flex flex-col gap-3">
			<p class="text-on-surface1 text-sm">
				Multiple components require authentication. Please authenticate each component below:
			</p>
			{#each Object.entries(oauthURLs).filter(([id]) => !authenticatedComponents.has(id)) as [componentId, url] (componentId)}
				{@const component = entry?.manifest?.compositeConfig?.componentServers?.find(
					(c) => c.catalogEntryID === componentId || c.mcpServerID === componentId
				)}
				{@const componentName = component?.manifest?.name || componentId}
				<div
					class="flex items-center justify-between gap-2 rounded border border-gray-200 p-3 dark:border-gray-700"
				>
					<div class="flex items-center gap-2">
						{#if component?.manifest?.icon}
							<img src={component.manifest.icon} alt={componentName} class="size-6 flex-shrink-0" />
						{/if}
						<span class="text-sm font-medium">{componentName}</span>
					</div>
					<button
						type="button"
						class="button-primary text-sm"
						onclick={() => {
							markComponentAuthenticated(componentId);
							window.open(url, '_blank');
						}}
					>
						Authenticate
					</button>
				</div>
			{/each}
		</div>
	{/if}
</ResponsiveDialog>

{#snippet errorSnippet()}
	<div class="notification-error flex items-center gap-2">
		<AlertCircle class="size-6 flex-shrink-0 text-red-500" />
		<p class="flex flex-col text-left text-sm font-light">
			<span class="font-semibold">Error with launching temporary instance:</span>
			<span>
				{error}
			</span>
		</p>
	</div>
{/snippet}
