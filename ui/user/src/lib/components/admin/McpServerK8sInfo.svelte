<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		ChatService,
		Group,
		type K8sServerDetail,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type MCPSecretBinding,
		type OrgUser,
		type ServerK8sSettings
	} from '$lib/services';
	import { EventStreamService } from '$lib/services/admin/eventstream.svelte';
	import { profile } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { isOwnSingleUserServer } from '$lib/utils';
	import Confirm from '../Confirm.svelte';
	import SensitiveInput from '../SensitiveInput.svelte';
	import Table from '../table/Table.svelte';
	import DeploymentLogs from './DeploymentLogs.svelte';
	import { TriangleAlert, Info, RotateCcw, RefreshCw, CircleFadingArrowUp } from 'lucide-svelte';
	import { onDestroy, onMount } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		id?: string;
		entity?: 'workspace' | 'catalog' | 'agent' | 'webhook-validation';
		mcpServerId: string;
		name: string;
		mcpServerInstanceId?: string;
		connectedUsers: (OrgUser & { mcpInstanceId?: string; mcpInstanceConfigured?: boolean })[];
		title?: string;
		classes?: {
			title?: string;
		};
		catalogEntry?: MCPCatalogEntry;
		mcpServer?: MCPCatalogServer;
		readonly?: boolean;
		compositeParentName?: string;
	}

	const {
		id: entityId,
		mcpServerId,
		mcpServerInstanceId,
		name,
		connectedUsers,
		title,
		classes,
		catalogEntry,
		mcpServer,
		compositeParentName,
		entity = 'catalog',
		readonly
	}: Props = $props();

	let listK8sInfo = $state<Promise<K8sServerDetail | undefined>>();
	let listK8sSettingsStatus = $state<Promise<ServerK8sSettings | undefined>>();
	let revealServerValues = $state<Promise<Record<string, string>>>();
	let messages = $state<string[]>([]);
	let error = $state<string>();
	let showRestartConfirm = $state(false);
	let restarting = $state(false);
	let refreshingEvents = $state(false);
	let refreshingLogs = $state(false);
	let showUpdateK8sSettingsConfirm = $state(false);
	let updatingK8sSettings = $state(false);
	let isAdminUrl = $derived(page.url.pathname.includes('/admin'));

	let logsUrl = $derived.by(() => {
		if (entity === 'workspace') {
			return catalogEntry?.id
				? `/api/workspaces/${entityId}/entries/${catalogEntry.id}/servers/${mcpServerId}/logs`
				: `/api/workspaces/${entityId}/servers/${mcpServerId}/logs`;
		}

		if (entity === 'webhook-validation') {
			return `/api/mcp-webhook-validations/${mcpServerId}/logs`;
		}

		return `/api/mcp-servers/${mcpServerId}/logs`;
	});

	let deploymentLogsInstance = $state<ReturnType<typeof DeploymentLogs>>();
	const hasAdminAccess = $derived(profile.current?.hasAdminAccess?.() ?? false);

	const eventStream = new EventStreamService<string>();
	const dontLogErrors = true;

	function handleScroll() {
		deploymentLogsInstance?.scroll();
	}

	function getK8sInfo() {
		if (!hasAdminAccess) return Promise.resolve<K8sServerDetail | undefined>(undefined);
		return entity === 'workspace' && entityId
			? catalogEntry?.id
				? ChatService.getWorkspaceCatalogEntryServerK8sDetails(
						entityId,
						catalogEntry.id,
						mcpServerId,
						{ dontLogErrors }
					)
				: ChatService.getWorkspaceK8sServerDetail(entityId, mcpServerId, { dontLogErrors })
			: entity === 'webhook-validation'
				? AdminService.getMCPFilterDetails(mcpServerId, { dontLogErrors })
				: AdminService.getK8sServerDetail(mcpServerId, { dontLogErrors });
	}

	function getK8sSettingsStatus() {
		if (!hasAdminAccess || entity === 'webhook-validation')
			return Promise.resolve<ServerK8sSettings | undefined>(undefined);
		return entity === 'workspace' && entityId
			? catalogEntry?.id
				? ChatService.getWorkspaceCatalogEntryServerK8sSettingsStatus(
						entityId,
						catalogEntry.id,
						mcpServerId,
						{
							dontLogErrors
						}
					)
				: ChatService.getWorkspaceK8sServerStatus(entityId, mcpServerId, {
						dontLogErrors
					})
			: catalogEntry?.id
				? AdminService.getMCPCatalogEntryServerK8sSettingsStatus(catalogEntry.id, mcpServerId, {
						dontLogErrors
					})
				: AdminService.getMcpCatalogServerK8sSettingsStatus(mcpServerId, {
						dontLogErrors
					});
	}

	onMount(() => {
		// Only load sensitive server values and k8s info if the user has admin access
		revealServerValues = profile.current.isAdmin?.()
			? entity === 'webhook-validation'
				? AdminService.revealMCPFilter(mcpServerId, {
						dontLogErrors: true
					})
				: ChatService.revealSingleOrRemoteMcpServer(mcpServerId, {
						dontLogErrors: true
					})
			: Promise.resolve<Record<string, string>>({});
		listK8sInfo = getK8sInfo();
		listK8sSettingsStatus = getK8sSettingsStatus();

		if (logsUrl) {
			eventStream.connect(logsUrl, {
				onMessage: (data) => {
					messages = [...messages, data];
					// Trigger auto-scroll after adding new message
					handleScroll();
				},
				onOpen: () => {
					console.debug(`${mcpServerId} event stream opened`);
					error = undefined;
				},
				onError: () => {
					error = 'Connection failed';
				},
				onClose: () => {
					console.debug(`${mcpServerId} event stream closed`);
				}
			});
		}
	});

	onDestroy(() => {
		eventStream.disconnect();
	});

	async function handleRestart() {
		restarting = true;
		try {
			await (entity === 'workspace' && entityId
				? catalogEntry?.id
					? ChatService.restartWorkspaceCatalogEntryServerDeployment(
							entityId,
							catalogEntry.id,
							mcpServerId
						)
					: ChatService.restartWorkspaceK8sServerDeployment(entityId, mcpServerId)
				: entity === 'webhook-validation'
					? AdminService.restartMCPFilter(mcpServerId)
					: AdminService.restartK8sDeployment(mcpServerId));
			// Refresh the k8s info after restart
			listK8sInfo = getK8sInfo();
		} catch (err) {
			console.error('Failed to restart deployment:', err);
		} finally {
			restarting = false;
			showRestartConfirm = false;
		}
	}

	async function handleRefreshEvents() {
		refreshingEvents = true;
		try {
			listK8sInfo = getK8sInfo();
		} catch (err) {
			console.error('Failed to refresh events:', err);
		} finally {
			refreshingEvents = false;
		}
	}

	async function handleRefreshLogs() {
		refreshingLogs = true;
		try {
			// Clear existing messages and reconnect to get fresh logs
			messages = [];
			eventStream.disconnect();
			if (logsUrl) {
				eventStream.connect(logsUrl, {
					onMessage: (data) => {
						messages = [...messages, data];
						// Trigger auto-scroll after adding new message
						handleScroll();
					},
					onOpen: () => {
						console.debug(`${mcpServerId} event stream opened`);
						error = undefined;
					},
					onError: () => {
						error = 'Connection failed';
					},
					onClose: () => {
						console.debug(`${mcpServerId} event stream closed`);
					}
				});
			}
		} catch (err) {
			console.error('Failed to refresh logs:', err);
		} finally {
			refreshingLogs = false;
		}
	}

	function compileK8sInfo(info?: K8sServerDetail) {
		if (!info) return [];
		const details = [
			{
				id: 'kubernetes_deployments',
				label: 'Deployment',
				value: `${info.namespace}/${info.deploymentName}`
			},
			{
				id: 'last_restart',
				label: 'Last Restart',
				value: formatTimeAgo(info.lastRestart).relativeTime
			},
			{
				id: 'status',
				label: 'Status',
				value: info.isAvailable ? 'Healthy' : 'Unhealthy'
			}
		];
		return details;
	}

	async function handleRedeployWithK8sSettings() {
		updatingK8sSettings = true;
		try {
			await (entity === 'workspace' && entityId
				? catalogEntry?.id
					? ChatService.redeployWorkspaceCatalogEntryServerWithK8sSettings(
							entityId,
							catalogEntry.id,
							mcpServerId
						)
					: ChatService.redeployWorkspaceK8sServerWithK8sSettings(entityId, mcpServerId)
				: catalogEntry?.id
					? AdminService.redeployMCPCatalogServerWithK8sSettings(catalogEntry.id, mcpServerId)
					: AdminService.redeployWithK8sSettings(
							mcpServerId,
							mcpServer?.mcpCatalogID ?? DEFAULT_MCP_CATALOG_ID
						));
			listK8sSettingsStatus = getK8sSettingsStatus();
		} catch (err) {
			console.error('Failed to update Kubernetes settings:', err);
		} finally {
			updatingK8sSettings = false;
			showUpdateK8sSettingsConfirm = false;
		}
	}

	type ConfigRow = {
		id: string;
		label: string;
		value: string;
		sensitive: boolean;
		file?: boolean;
		dynamicFile?: boolean;
		secretBinding?: MCPSecretBinding;
	};

	function compileRevealedValues(
		revealedValues?: Record<string, string>,
		catalogEntry?: MCPCatalogEntry
	) {
		if (!catalogEntry) {
			return {
				headers: [] as ConfigRow[],
				envs: [] as ConfigRow[]
			};
		}

		const envMap = new Map(catalogEntry.manifest.env?.map((env) => [env.key, env]));
		const headerMap = new Map(
			catalogEntry.manifest.remoteConfig?.headers?.map((header) => [header.key, header])
		);

		const envs: ConfigRow[] = [];
		const headers: ConfigRow[] = [];

		for (const key in revealedValues ?? {}) {
			if (envMap.has(key)) {
				const env = envMap.get(key);
				envs.push({
					id: key,
					label: env?.name ?? 'Unknown',
					value: env?.prefix ? env.prefix + revealedValues![key] : (revealedValues![key] ?? ''),
					sensitive: env?.sensitive || false,
					file: env?.file,
					dynamicFile: env?.dynamicFile
				});
			} else if (headerMap.has(key)) {
				const header = headerMap.get(key);
				headers.push({
					id: key,
					label: header?.name ?? 'Unknown',
					value: header?.prefix
						? header.prefix + revealedValues![key]
						: (revealedValues![key] ?? ''),
					sensitive: header?.sensitive || false
				});
			}
		}

		// Include secret-bound fields — their values are not stored by Obot so they
		// won't appear in revealedValues, but we can still show the binding reference.
		for (const env of catalogEntry.manifest.env ?? []) {
			if (env.secretBinding && !revealedValues?.[env.key]) {
				envs.push({
					id: env.key,
					label: env.name ?? env.key,
					value: '',
					sensitive: false,
					file: env.file,
					dynamicFile: env.dynamicFile,
					secretBinding: env.secretBinding
				});
			}
		}
		for (const header of catalogEntry.manifest.remoteConfig?.headers ?? []) {
			if (header.secretBinding && !revealedValues?.[header.key]) {
				headers.push({
					id: header.key,
					label: header.name ?? header.key,
					value: '',
					sensitive: false,
					secretBinding: header.secretBinding
				});
			}
		}

		return {
			envs,
			headers
		};
	}

	const missingSecretBindings = $derived(getMissingSecretBindings());

	function getMissingSecretBindings() {
		const missingEnvKeys = new Set(mcpServer?.missingRequiredEnvVars ?? []);
		const missingHeaderKeys = new Set(mcpServer?.missingRequiredHeaders ?? []);
		const results: { label: string; secretName: string; secretKey: string }[] = [];

		for (const env of catalogEntry?.manifest.env ?? []) {
			if (env.secretBinding && missingEnvKeys.has(env.key)) {
				results.push({
					label: env.name ?? env.key,
					secretName: env.secretBinding.name,
					secretKey: env.secretBinding.key
				});
			}
		}
		for (const header of catalogEntry?.manifest.remoteConfig?.headers ?? []) {
			if (header.secretBinding && missingHeaderKeys.has(header.key)) {
				results.push({
					label: header.name ?? header.key,
					secretName: header.secretBinding.name,
					secretKey: header.secretBinding.key
				});
			}
		}
		return results;
	}

	function getAuditLogUrl(d: (typeof connectedUsers)[number]) {
		const id = mcpServerId || mcpServerInstanceId;

		// can agents have audit logs?
		if (compositeParentName || entity === 'agent') return null;

		if (isAdminUrl) {
			if (!hasAdminAccess) return null;
			return entity === 'workspace'
				? catalogEntry?.id
					? `/admin/mcp-servers/w/${entityId}/c/${catalogEntry.id}?view=audit-logs&user_id=${d.id}`
					: `/admin/mcp-servers/w/${entityId}/s/${encodeURIComponent(id ?? '')}?view=audit-logs&user_id=${d.id}`
				: catalogEntry?.id
					? `/admin/mcp-servers/c/${catalogEntry.id}?view=audit-logs&user_id=${d.id}`
					: `/admin/mcp-servers/s/${encodeURIComponent(id ?? '')}?view=audit-logs&user_id=${d.id}`;
		}

		// Basic users can access audit logs for their own single-user servers
		let isOwnServer = mcpServer && isOwnSingleUserServer(mcpServer, profile.current?.id);
		if (!isOwnServer && !profile.current?.groups.includes(Group.POWERUSER)) return null;
		return catalogEntry?.id
			? `/mcp-servers/c/${catalogEntry.id}?view=audit-logs&user_id=${d.id}`
			: `/mcp-servers/s/${encodeURIComponent(id ?? '')}?view=audit-logs&user_id=${d.id}`;
	}
</script>

<div class="flex items-center gap-3">
	<h1 class={twMerge('text-2xl font-semibold', classes?.title)}>
		{#if title}
			{title}
		{:else if mcpServerInstanceId}
			{name} | {mcpServerInstanceId}
		{:else}
			{name}
		{/if}
	</h1>
	{#if hasAdminAccess}
		<button
			onclick={handleRefreshEvents}
			class="rounded-md p-1 text-muted-content hover:bg-base-200 hover:text-base-content disabled:opacity-50 dark:text-muted-content dark:hover:bg-base-200 dark:hover:text-base-content"
			disabled={refreshingEvents}
		>
			<RefreshCw class="size-4 {refreshingEvents ? 'animate-spin' : ''}" />
		</button>
	{/if}
</div>

{#if mcpServerInstanceId}
	<div class="notification-info p-3 text-sm font-light">
		<div class="flex items-center gap-3">
			<Info class="size-6" />
			<p>
				This is a multi-user server instance. The server information displayed here is the root
				server that is shared between all server instances.
			</p>
		</div>
	</div>
{/if}

{#if missingSecretBindings.length > 0 && hasAdminAccess}
	<div class="notification-alert">
		<div class="flex grow flex-col gap-2">
			<div class="flex items-center gap-2">
				<TriangleAlert class="size-6 shrink-0 self-start text-warning" />
				<p class="my-0.5 flex flex-col text-sm font-semibold">
					Missing Kubernetes Secret{missingSecretBindings.length > 1 ? 's' : ''}
				</p>
			</div>
			<div class="text-sm font-light">
				The following Kubernetes Secrets referenced by this server could not be found:
				<ul class="mt-1 list-disc pl-5">
					{#each missingSecretBindings as binding (binding.secretName + '/' + binding.secretKey)}
						<li>
							<code class="font-mono">{binding.secretName}/{binding.secretKey}</code> (for
							<strong>{binding.label}</strong>)
						</li>
					{/each}
				</ul>
				<p class="mt-2">Server details and logs are temporarily unavailable as a result.</p>
			</div>
		</div>
	</div>
{/if}

{#await listK8sInfo}
	{#if hasAdminAccess}
		<div class="flex w-full justify-center">
			<Loading class="size-6" />
		</div>
	{/if}
{:then info}
	{@const k8sInfo = compileK8sInfo(info)}
	{#if hasAdminAccess && k8sInfo}
		<div class="flex flex-col gap-2">
			{#each k8sInfo as detail (detail.id)}
				{@render detailRow(detail.label, detail.value, detail.id)}
			{/each}
			{#if catalogEntry?.manifest.runtime === 'remote' && mcpServer?.manifest.remoteConfig?.url}
				{@render configurationRow('URL', mcpServer?.manifest.remoteConfig?.url)}
			{/if}
		</div>

		{#if hasAdminAccess}
			{#await revealServerValues}
				<div class="flex w-full justify-center">
					<Loading class="size-6" />
				</div>
			{:then revealedValues}
				{@const { headers, envs } = compileRevealedValues(revealedValues, catalogEntry)}
				{#if catalogEntry?.manifest.runtime === 'remote'}
					<div>
						<h2 class="mb-2 text-lg font-semibold">Headers</h2>
						{#if headers.length > 0}
							<div class="flex flex-col gap-2">
								{#each headers as h (h.id)}
									{@render configurationRow(h.label, h.value, h.sensitive, h.secretBinding)}
								{/each}
							</div>
						{:else}
							<span class="text-muted-content text-sm font-light">No configured headers.</span>
						{/if}
					</div>
				{/if}

				<div>
					<h2 class="mb-2 text-lg font-semibold">Configuration</h2>
					{#if envs.length > 0}
						<div class="flex flex-col gap-2">
							{#each envs as env (env.id)}
								{@render configurationRow(
									env.label,
									env.value,
									env.sensitive,
									env.secretBinding,
									env.file,
									env.dynamicFile
								)}
							{/each}
						</div>
					{:else}
						<span class="text-muted-content text-sm font-light"
							>No configured environment or file variables set.</span
						>
					{/if}
				</div>
			{/await}
		{/if}

		<div>
			<h2 class="mb-2 text-lg font-semibold">Recent Events</h2>
			{#if info?.events && info.events.length > 0}
				{@const tableData = info.events.map((event, index) => ({
					id: `${event.time}-${index}`,
					...event
				}))}
				<Table
					data={tableData}
					fields={['time', 'eventType', 'message']}
					headers={[{ title: 'Event Type', property: 'eventType' }]}
				>
					{#snippet onRenderColumn(property, d)}
						{#if property === 'time'}
							{formatTimeAgo(d.time).fullDate}
						{:else}
							{d[property as keyof typeof d]}
						{/if}
					{/snippet}
				</Table>
			{:else}
				<span class="text-muted-content text-sm font-light">No events.</span>
			{/if}
		</div>
	{/if}
{:catch error}
	{@const isPending = error instanceof Error && error.message.includes('ContainerCreating')}
	{@const needsUpdate = error instanceof Error && error.message.includes('missing required config')}

	{#if needsUpdate && hasAdminAccess && missingSecretBindings.length === 0}
		<div class="notification-alert">
			<div class="flex grow flex-col gap-2">
				<div class="flex items-center gap-2">
					<TriangleAlert class="size-6 shrink-0 self-start text-warning" />
					<p class="my-0.5 flex flex-col text-sm font-semibold">
						User Configuration Update Required
					</p>
				</div>
				<span class="text-sm font-light break-all">
					The server was recently updated and requires the user to update their configuration.
					Server details and logs are temporarily unavailable as a result.
				</span>
			</div>
		</div>
	{/if}

	{#if hasAdminAccess}
		<div class="flex flex-col gap-2">
			<div
				class="dark:bg-base-200 dark:border-base-400 bg-base-100 flex flex-col rounded-lg border border-transparent p-4 shadow-sm"
			>
				<div class="grid grid-cols-2 gap-4">
					<p class="text-sm font-semibold">Status</p>
					<p class="text-sm font-light">
						{isPending ? 'Pending' : needsUpdate ? 'Update Required' : 'Error'}
					</p>
				</div>
			</div>
		</div>
	{/if}
{/await}

<DeploymentLogs
	bind:this={deploymentLogsInstance}
	bind:messages
	{error}
	refreshing={refreshingLogs}
	onRefresh={handleRefreshLogs}
	onClear={() => (messages = [])}
/>

{#if hasAdminAccess && entity !== 'webhook-validation'}
	<div>
		<h2 class="mb-2 text-lg font-semibold">Connected Users</h2>
		<Table
			data={connectedUsers ?? []}
			fields={['name', 'updateStatus']}
			headers={[{ title: 'Config Status', property: 'updateStatus' }]}
		>
			{#snippet onRenderColumn(property, d)}
				{#if property === 'name'}
					{d.email || d.username || 'Unknown'}
				{:else if property === 'updateStatus'}
					{d.mcpInstanceConfigured === false ? 'Not Configured' : 'Up to date'}
				{:else}
					{d[property as keyof typeof d]}
				{/if}
			{/snippet}

			{#snippet actions(d)}
				{@const auditLogsUrl = getAuditLogUrl(d)}
				{#if auditLogsUrl}
					<a href={resolve(auditLogsUrl as `/${string}`)} class="btn btn-link"> View Audit Logs </a>
				{/if}
			{/snippet}
		</Table>
	</div>
{/if}

{#snippet detailRow(label: string, value: string, id: string)}
	<div
		class="dark:bg-base-200 dark:border-base-400 bg-base-100 flex flex-col rounded-lg border border-transparent p-4 shadow-sm"
	>
		<div class="grid grid-cols-12 gap-4">
			<p class="col-span-4 text-sm font-semibold">{label}</p>
			<div class="col-span-8 flex items-center justify-between">
				<p class="truncate text-sm font-light">{value}</p>
				{#if id === 'status' && !readonly}
					<button
						onclick={() => (showRestartConfirm = true)}
						class="btn btn-primary flex items-center gap-2 rounded-md px-3 py-1.5 text-xs font-medium text-white disabled:opacity-50"
						disabled={restarting}
					>
						<RotateCcw class="size-3" />
						Restart
					</button>
				{:else if id === 'kubernetes_deployments' && !readonly}
					{#await listK8sSettingsStatus}
						<div class="flex w-full justify-center">
							<Loading class="size-6" />
						</div>
					{:then k8sSettingsStatus}
						{#if k8sSettingsStatus?.needsK8sUpdate || mcpServer?.needsK8sUpdate}
							<button
								class="flex items-center gap-2 rounded-md bg-warning/75 px-3 py-1.5 text-xs font-medium text-warning-content hover:bg-warning disabled:opacity-50"
								disabled={readonly}
								onclick={() => (showUpdateK8sSettingsConfirm = true)}
							>
								<CircleFadingArrowUp class="size-3" />
								Redeploy with Latest Settings
							</button>
						{/if}
					{/await}
				{/if}
			</div>
		</div>
	</div>
{/snippet}

{#snippet configurationRow(
	label: string,
	value: string,
	sensitive?: boolean,
	secretBinding?: MCPSecretBinding,
	file?: boolean,
	dynamicFile?: boolean
)}
	<div
		class="dark:bg-base-200 dark:border-base-400 bg-base-100 flex flex-col rounded-lg border border-transparent px-4 py-1.5 shadow-sm"
	>
		<div class="grid grid-cols-12 items-center gap-4">
			<p class="col-span-4 text-sm font-semibold">{label}</p>
			<div class="col-span-8 flex items-center justify-between">
				{#if secretBinding}
					<span class="text-muted-content flex flex-wrap items-center gap-2 text-sm">
						<span>
							Kubernetes Secret: <code class="font-mono">{secretBinding.name}</code> /
							<code class="font-mono">{secretBinding.key}</code>
						</span>
						{#if file}
							<span
								class="rounded bg-blue-100 px-1.5 py-0.5 text-xs font-medium text-blue-700 dark:bg-blue-900/40 dark:text-blue-300"
								title="Secret value is mounted as a file; the env var contains the file path"
							>
								file
							</span>
						{/if}
						{#if dynamicFile}
							<span
								class="rounded bg-blue-100 px-1.5 py-0.5 text-xs font-medium text-purple-700 dark:bg-purple-900/40 dark:text-purple-300"
								title="File updates in-place when the Secret changes — no pod restart needed"
							>
								dynamic
							</span>
						{/if}
					</span>
				{:else if sensitive}
					<SensitiveInput {value} disabled name={label} />
				{:else}
					<input type="text" {value} class="text-input-filled" disabled />
				{/if}
			</div>
		</div>
	</div>
{/snippet}

<Confirm
	show={showRestartConfirm}
	msg={`Restart ${title || name}?`}
	onsuccess={handleRestart}
	oncancel={() => (showRestartConfirm = false)}
	loading={restarting}
	title="Confirm Restart"
	type="info"
>
	{#snippet note()}
		Are you sure you want to restart this deployment? This will cause a brief service interruption.
	{/snippet}
</Confirm>
<Confirm
	show={showUpdateK8sSettingsConfirm}
	msg={`Redeploy ${title || name}?`}
	onsuccess={handleRedeployWithK8sSettings}
	oncancel={() => (showUpdateK8sSettingsConfirm = false)}
	loading={updatingK8sSettings}
	title="Confirm Redeploy"
	type="info"
>
	{#snippet note()}
		Are you sure you want to redeploy this server with the latest Kubernetes settings? This will
		cause a brief service interruption.
	{/snippet}
</Confirm>
