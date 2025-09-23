<script lang="ts">
	import {
		AdminService,
		type MCPFilter,
		type MCPCatalogServer,
		ChatService,
		Group
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
	import CatalogConfigureForm, { type LaunchFormData } from '../mcp/CatalogConfigureForm.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import { setVirtualPageDisabled } from '../ui/virtual-page/context';
	import { profile } from '$lib/stores';
	import OverflowContainer from '../OverflowContainer.svelte';

	type MCPType = 'single' | 'multi' | 'remote';

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

	const tabs = $derived(
		entry
			? entity === 'workspace' && !profile.current?.isAdmin?.()
				? [
						{ label: 'Overview', view: 'overview' },
						{ label: 'Server Details', view: 'server-instances' },
						{ label: 'Tools', view: 'tools' },
						{ label: 'Configuration', view: 'configuration' },
						// TODO: support workspace usage and audit logs
						// { label: 'Usage', view: 'usage' },
						// { label: 'Audit Logs', view: 'audit-logs' },
						...(isAtLeastPowerUserPlus ? [{ label: 'Access Control', view: 'access-control' }] : [])
					]
				: [
						{ label: 'Overview', view: 'overview' },
						{ label: 'Server Details', view: 'server-instances' },
						{ label: 'Tools', view: 'tools' },
						{ label: 'Configuration', view: 'configuration' },
						{ label: 'Usage', view: 'usage' },
						{ label: 'Audit Logs', view: 'audit-logs' },
						{ label: 'Access Control', view: 'access-control' },
						{ label: 'Filters', view: 'filters' }
					]
			: []
	);

	let listAccessControlRules = $state<Promise<AccessControlRule[]>>();
	let listFilters = $state<Promise<MCPFilter[]>>();
	let users = $state<OrgUser[]>([]);
	let registry = $derived.by(() => {
		if (!entry) return undefined;
		const ownerUserId = 'isCatalogEntry' in entry ? entry.powerUserID : entry.userID;
		return getRegistryLabel(ownerUserId, profile.current?.id, users);
	});

	let deleteServer = $state(false);
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

	let configDialog = $state<ReturnType<typeof CatalogConfigureForm>>();
	let configureForm = $state<LaunchFormData>();
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

	onMount(() => {
		if (isAtLeastPowerUserPlus) {
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
		return {
			url: configureForm?.url,
			config: [...(configureForm?.headers ?? []), ...(configureForm?.envs ?? [])].reduce<
				Record<string, string>
			>((acc, curr) => {
				acc[curr.key] = curr.value;
				return acc;
			}, {})
		};
	}

	async function handleVisibilityChange() {
		if (!entry || !id) return;
		document.removeEventListener('visibilitychange', handleVisibilityChange);
		handleLaunchTemporaryInstance();
	}

	function handleTemporaryInstanceOauth(oauthUrlToUse: string) {
		if (!oauthUrlToUse) return;
		oauthURL = oauthUrlToUse;
		oauthDialog?.open();

		// add visibility change listener
		document.addEventListener('visibilitychange', handleVisibilityChange);
	}

	async function handleLaunchTemporaryInstance(showInlineError = false) {
		if (!entry || !id) return;

		error = undefined;
		showButtonInlineError = false;
		saving = true;
		const body = compileTemporaryInstanceBody();
		try {
			const generateToolsFn =
				entity === 'workspace'
					? ChatService.generateWorkspaceMCPCatalogEntryToolPreviews
					: AdminService.generateMcpCatalogEntryToolPreviews;
			await generateToolsFn(id, entry.id, body);
			window.location.reload();
		} catch (err) {
			const errMessage = err instanceof Error ? err.message : 'An unknown error occurred';
			if (errMessage.includes('MCP server requires OAuth authentication')) {
				const getOauthFn =
					entity === 'workspace'
						? ChatService.getWorkspaceMCPCatalogEntryToolPreviewsOauth
						: AdminService.getMcpCatalogToolPreviewsOauth;
				const oauthResponse = await getOauthFn(id, entry.id, body);
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
			type !== 'multi' && (needsEnvValue || needsHeaderValue || configureForm.hostname);
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
				{#if entry.manifest.icon}
					<img
						src={entry.manifest.icon}
						alt={entry.manifest.name}
						class="bg-surface1 size-10 rounded-md p-1 dark:bg-gray-600"
					/>
				{/if}
				<h1 class="text-2xl font-semibold capitalize">{entry.manifest.name || 'Unknown'}</h1>
				<div class="dark:bg-surface2 bg-surface3 rounded-full px-3 py-1 text-xs">
					{type === 'single' ? 'Single User' : type === 'multi' ? 'Multi-User' : 'Remote'}
				</div>
				{#if registry}
					<div class="dark:bg-surface2 bg-surface3 rounded-full px-3 py-1 text-xs">
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
			class="scrollbar-none flex w-full items-center gap-2 overflow-x-auto"
			style="scroll-behavior: smooth;"
			{@attach (node: HTMLDivElement) => (scrollContainer = node)}
		>
			{#snippet children({ x })}
				{#if tabs.length > 0}
					{#if x}
						<button
							disabled={!showLeftChevron}
							onclick={scrollLeft}
							class="bg-surface1 sticky left-0 flex aspect-square h-full items-center justify-center rounded-l-md p-2.5 opacity-100 transition-all duration-200 disabled:opacity-30 dark:bg-black"
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
										'dark:bg-surface1 dark:border-surface3 bg-white shadow-sm',
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
							class="bg-surface1 sticky right-0 flex aspect-square h-full items-center justify-center rounded-r-md p-2.5 opacity-100 transition-all duration-200 disabled:opacity-30 dark:bg-black"
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
							<Wrench class="size-24 text-gray-200 dark:text-gray-900" />
							{#if !entry || (entry && readonly)}
								<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">No tools</h4>
								<p class="text-sm font-light text-gray-400 dark:text-gray-600">
									Looks like this MCP server doesn't have any tools available.
								</p>
							{:else if !readonly}
								<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">No tools</h4>
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
									<p class="text-sm font-light text-gray-400 dark:text-gray-600">
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
							class="text-xs text-gray-500">({entry.sourceURL.split('/').pop()})</span
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
				onSelectRow={(d, isCtrlClick) => {
					if (!entry) return;
					setLastVisitedMcpServer();

					const isAdminRoute = window.location.pathname.includes('/admin/');

					let url = '';
					const from =
						entity === 'workspace' && !isAdminRoute
							? encodeURIComponent(`mcp-publisher/${entry.id}`)
							: encodeURIComponent(`mcp-servers/${entry.id}`);
					if (entity === 'workspace') {
						url = !isAdminRoute
							? `/mcp-publisher/access-control/${d.id}?from=${from}`
							: `/admin/access-control/w/${id}/r/${d.id}?from=${from}`;
					} else {
						url = `/admin/access-control/${d.id}?from=${from}`;
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
				<GlobeLock class="size-24 text-gray-200 dark:text-gray-900" />
				<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">
					No access control rules
				</h4>
				<p class="text-sm font-light text-gray-400 dark:text-gray-600">
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
		{@const isMultiUserServer = !!page.url.pathname.match(/\/mcp-servers\/s.*$/)?.[0]}
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
					catalogId={id}
				>
					{#snippet emptyContent()}
						<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
							<Users class="size-24 text-gray-200 dark:text-gray-900" />
							<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">
								No recent audit logs
							</h4>
							<p class="text-sm font-light text-gray-400 dark:text-gray-600">
								This server has not had any active usage in the last 7 days.
							</p>
							{#if entryId || mcpCatalogEntryId}
								{@const param = entryId ? 'mcpId=' + entryId : 'entryId=' + mcpCatalogEntryId}
								<p class="text-sm font-light text-gray-400 dark:text-gray-600">
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
					onSelectRow={(d, isCtrlClick) => {
						setLastVisitedMcpServer();
						const url = `/admin/filters/${d.id}?from=${encodeURIComponent(`mcp-servers/${entry?.id}`)}`;
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
					<ListFilter class="size-24 text-gray-200 dark:text-gray-900" />
					<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">
						No filters configured
					</h4>
					<p class="text-sm font-light text-gray-400 dark:text-gray-600">
						This server is not referenced by any filters.
					</p>
				</div>
			{/if}
		{/await}
	{:else}
		<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
			<ListFilter class="size-24 text-gray-200 dark:text-gray-900" />
			<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">No filters available</h4>
			<p class="text-sm font-light text-gray-400 dark:text-gray-600">
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
			await deleteServerFn(id, entry.id);
		} else {
			const deleteCatalogEntryFn =
				entity === 'workspace'
					? ChatService.deleteWorkspaceMCPCatalogEntry
					: AdminService.deleteMCPCatalogEntry;
			await deleteCatalogEntryFn(id, entry.id);
		}
		goto(entity === 'workspace' ? '/mcp-publisher/mcp-servers' : '/admin/mcp-servers');
	}}
	oncancel={() => (deleteServer = false)}
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

<ResponsiveDialog bind:this={oauthDialog} title="OAuthentication Required" class="w-md">
	{#if error}
		{@render errorSnippet()}
	{/if}
	{#if saving}
		<div class="flex w-full justify-center">
			<LoaderCircle class="size-6 animate-spin" />
		</div>
	{:else}
		<a href={oauthURL} target="_blank" class="button-primary text-center">Authenticate</a>
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
