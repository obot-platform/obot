<script lang="ts">
	import {
		getProjectMCPs,
		validateOauthProjectMcps,
		type ProjectMcpItem
	} from '$lib/context/projectMcps.svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import { Server, X, AlertTriangle } from 'lucide-svelte';
	import { dialogAnimation } from '$lib/actions/dialogAnimation';
	import { onMount, tick } from 'svelte';
	import { getLayout } from '$lib/context/chatLayout.svelte';
	import CatalogConfigureForm, {
		type CompositeLaunchFormData,
		type LaunchFormData
	} from '$lib/components/mcp/CatalogConfigureForm.svelte';
	import { ChatService, type MCPCatalogEntry, type MCPCatalogServer } from '$lib/services';
	import {
		convertCompositeInfoToLaunchFormData,
		convertCompositeLaunchFormDataToPayload,
		convertEnvHeadersToRecord,
		requiresUserConfiguration,
		requiresAdminOAuthConfig
	} from '$lib/services/chat/mcp';

	interface Props {
		assistantId: string;
		projectId: string;
	}

	const { assistantId, projectId }: Props = $props();

	const projectMcps = getProjectMCPs();
	let closed = new SvelteSet<string>();
	let authenticating = new SvelteSet<string>();
	let currentOauthId = $state<string | null>(null);

	type Requirement =
		| { type: 'oauth'; id: string; name: string; icon?: string; oauthURL: string }
		| { type: 'config'; id: string; mcpID: string }
		| { type: 'admin-oauth'; id: string; name: string; icon?: string };

	type OauthRequirement = Extract<Requirement, { type: 'oauth' }>;
	type AdminOauthRequirement = Extract<Requirement, { type: 'admin-oauth' }>;

	type RequirementKey = 'config' | 'oauth' | 'admin-oauth' | null;

	function getRequirementKey(mcp: ProjectMcpItem): RequirementKey {
		if (requiresUserConfiguration(mcp)) return 'config';
		if (!mcp.authenticated && mcp.oauthURL) return 'oauth';
		if (requiresAdminOAuthConfig(mcp) && !requiresUserConfiguration(mcp)) return 'admin-oauth';
		return null;
	}

	// Track previous requirement key to detect changes and clear closed set
	let previousRequirementKeys = $state<Map<string, RequirementKey>>(new Map());

	$effect(() => {
		for (const mcp of projectMcps.items) {
			const prevKey = previousRequirementKeys.get(mcp.id!);
			const currentKey = getRequirementKey(mcp);
			// If the requirement type changed, clear from closed so new prompts appear
			if (prevKey !== undefined && prevKey !== currentKey) {
				closed.delete(mcp.id!);
			}
			previousRequirementKeys.set(mcp.id!, currentKey);
		}
	});
	let requirements = $derived([
		// User configuration requirements (env vars, headers, URL - NOT OAuth-only)
		...projectMcps.items
			.filter((m) => requiresUserConfiguration(m) && !closed.has(m.id!))
			.map((m) => ({ type: 'config', id: m.id!, mcpID: m.mcpID! }) as Requirement),

		// User OAuth requirements (per-user authentication via OAuth flow)
		...projectMcps.items
			.filter((m) => !m.authenticated && m.oauthURL && !closed.has(m.id!))
			.map(
				(m) =>
					({
						type: 'oauth',
						id: m.id!,
						name: m.name!,
						icon: m.icon,
						oauthURL: m.oauthURL!
					}) as Requirement
			),

		// Admin OAuth requirements (show AFTER user config is done)
		...projectMcps.items
			.filter((m) => {
				if (closed.has(m.id!)) return false;
				// Only show when there are no user config issues remaining
				return requiresAdminOAuthConfig(m) && !requiresUserConfiguration(m);
			})
			.map(
				(m) =>
					({
						type: 'admin-oauth',
						id: m.id!,
						name: m.name!,
						icon: m.icon
					}) as Requirement
			)
	]);

	let oauthDialog = $state<HTMLDialogElement>();
	const layout = getLayout();
	const isInMcp = $derived(
		layout.sidebarConfig === 'mcp-server-tools' || layout.sidebarConfig === 'mcp-server'
	);

	// Config dialog state
	let configDialog = $state<ReturnType<typeof CatalogConfigureForm>>();
	let currentConfigReq = $state<(Requirement & { type: 'config' }) | null>(null);
	let configureForm = $state<LaunchFormData>();
	let configuring = $state(false);
	let configError = $state<string>();
	let configName = $state<string>('');
	let configIcon = $state<string>('');
	let configServerId = $state<string>('');

	let userServers = $state<MCPCatalogServer[]>([]);
	let entries = $state<MCPCatalogEntry[]>([]);

	async function ensureServersLoaded() {
		if (userServers.length > 0 && entries.length > 0) return;
		const [servers, entriesResult] = await Promise.all([
			ChatService.listSingleOrRemoteMcpServers(),
			ChatService.listMCPs()
		]);
		userServers = servers;
		entries = entriesResult;
	}

	function findServerAndParentByMcpId(mcpID: string): {
		server?: MCPCatalogServer;
		parent?: MCPCatalogEntry;
	} {
		const byId = new Map(userServers.map((s) => [s.id, s]));
		const entriesById = new Map(entries.map((e) => [e.id, e]));
		const server = byId.get(mcpID);
		const parent = server?.catalogEntryID ? entriesById.get(server.catalogEntryID) : undefined;
		return { server, parent };
	}

	$effect(() => {
		if (isInMcp) return;
		if (currentOauthId) {
			const stillNeedsOauth = requirements.some(
				(r) => r.type === 'oauth' && r.id === currentOauthId
			);
			if (!stillNeedsOauth) {
				if (oauthDialog?.open) oauthDialog.close();
				authenticating.delete(currentOauthId);
				currentOauthId = null;
			}
		}

		if (oauthDialog?.open || currentConfigReq) return;
		if (requirements.length === 0) return;

		const req = requirements[0];
		if (!req) return;
		if (req.type === 'oauth' || req.type === 'admin-oauth') {
			if (!oauthDialog?.open) {
				oauthDialog?.showModal();
			}
			return;
		}
		if (req.type === 'config') {
			openConfigForRequirement(req);
		}
	});

	onMount(() => {
		const handleVisibilityChange = () => {
			if (isInMcp) return;
			if (document.visibilityState === 'visible') {
				checkMcpOauths();
			}
		};

		document.addEventListener('visibilitychange', handleVisibilityChange);

		return () => {
			document.removeEventListener('visibilitychange', handleVisibilityChange);
		};
	});

	onMount(() => {
		checkMcpOauths();
	});

	async function checkMcpOauths() {
		const updatedMcps = await validateOauthProjectMcps(
			assistantId,
			projectId,
			projectMcps.items,
			true
		);
		if (updatedMcps.length > 0) {
			projectMcps.items = updatedMcps;
		}
	}

	function dismissCurrent() {
		const req = requirements[0];
		if (!req) return;
		if (req.type === 'oauth' || req.type === 'admin-oauth') {
			closed.add(req.id);
			if (currentOauthId === req.id) {
				authenticating.delete(currentOauthId);
				currentOauthId = null;
			}
		}
		if (oauthDialog?.open) oauthDialog.close();
	}

	async function openConfigForRequirement(req: Requirement & { type: 'config' }) {
		await ensureServersLoaded();
		if (oauthDialog?.open) return;

		const { server, parent } = findServerAndParentByMcpId(req.mcpID);
		if (!server) return;

		if (server.manifest?.runtime === 'composite') {
			await prepareCompositeConfigureForm(server);
		} else {
			await prepareConfigureForm(server, parent);
		}
		await tick();

		currentConfigReq = req;
		configDialog?.open();
	}

	async function prepareCompositeConfigureForm(server: MCPCatalogServer) {
		configError = '';
		configuring = false;
		configName = server.alias || server.manifest?.name || '';
		configIcon = server.manifest?.icon || '';
		configServerId = server.id;
		configureForm = await convertCompositeInfoToLaunchFormData(server);
	}

	async function prepareConfigureForm(server: MCPCatalogServer, parent?: MCPCatalogEntry) {
		configError = '';
		configuring = false;
		configName = server.alias || server.manifest?.name || '';
		configIcon = server.manifest?.icon || '';
		configServerId = server.id;

		let values: Record<string, string> = {};
		try {
			values = await ChatService.revealSingleOrRemoteMcpServer(server.id, { dontLogErrors: true });
		} catch (error) {
			if (error instanceof Error && !error.message.includes('404')) {
				console.error('Failed to reveal user server values:', error);
			}
			values = {};
		}

		configureForm = {
			envs: server.manifest.env?.map((env) => ({ ...env, value: values[env.key] ?? '' })),
			headers: server.manifest.remoteConfig?.headers?.map((header) => ({
				...header,
				value: values[header.key] ?? '',
				isStatic: header.value !== ''
			})),
			url: server.manifest.remoteConfig?.url,
			hostname: parent?.manifest.remoteConfig?.hostname
		};
	}

	async function handleSaveConfig() {
		const req = currentConfigReq;
		if (!req || req.type !== 'config' || !configureForm) return;
		configuring = true;
		configError = '';
		await ensureServersLoaded();
		const { server, parent } = findServerAndParentByMcpId(req.mcpID);
		if (!server) return;
		try {
			if (
				parent?.manifest.runtime === 'remote' &&
				configureForm.url &&
				parent.manifest.remoteConfig?.urlTemplate === undefined &&
				server.manifest.remoteConfig?.fixedURL !== undefined
			) {
				await ChatService.updateRemoteMcpServerUrl(server.id, configureForm.url.trim());
			}

			if (server.manifest.runtime === 'composite') {
				const payload = convertCompositeLaunchFormDataToPayload(
					configureForm as CompositeLaunchFormData
				);
				await ChatService.configureCompositeMcpServer(server.id, payload);
				try {
					const refreshed = await ChatService.listProjectMCPs(assistantId, projectId);
					projectMcps.items = await validateOauthProjectMcps(
						assistantId,
						projectId,
						refreshed,
						true
					);
				} catch {
					// ignore refresh errors
				}
				currentConfigReq = null;
				configDialog?.close();
			} else {
				const secretValues = convertEnvHeadersToRecord(configureForm.envs, configureForm.headers);
				await ChatService.configureSingleOrRemoteMcpServer(server.id, secretValues);
				try {
					const refreshed = await ChatService.listProjectMCPs(assistantId, projectId);
					projectMcps.items = await validateOauthProjectMcps(
						assistantId,
						projectId,
						refreshed,
						true
					);
				} catch {
					// ignore refresh errors
				}
				currentConfigReq = null;
				configDialog?.close();
			}
		} catch (error) {
			configError = error instanceof Error ? error.message : 'Unknown error';
		} finally {
			configuring = false;
		}
	}
</script>

{#key requirements[0]?.id}
	{#if requirements[0]?.type === 'oauth'}
		{@const oauth = requirements[0] as OauthRequirement}
		<dialog bind:this={oauthDialog} class="dialog" use:dialogAnimation={{ type: 'fade' }}>
			<div class="dialog-container relative flex w-full flex-col gap-4 p-4 md:w-sm">
				<div class="absolute top-2 right-2">
					<button class="icon-button" onclick={dismissCurrent}>
						<X class="size-4" />
					</button>
				</div>
				<div class="flex items-center gap-2">
					<div class="h-fit flex-shrink-0 self-start rounded-md bg-gray-50 p-1 dark:bg-gray-600">
						{#if oauth.icon}
							<img src={oauth.icon} alt={oauth.name} class="size-6" />
						{:else}
							<Server class="size-6" />
						{/if}
					</div>
					<h3 class="text-lg leading-5.5 font-semibold">
						{oauth.name}
					</h3>
				</div>

				<p>
					In order to use {oauth.name}, authentication with the MCP server is required.
				</p>

				<p>Click the link below to authenticate.</p>

				<!-- eslint-disable svelte/no-navigation-without-resolve -- external OAuth URL -->
				<a
					href={oauth.oauthURL}
					rel="external"
					target="_blank"
					class="button-primary text-center text-sm outline-none"
					onclick={() => {
						if (currentOauthId) return;
						currentOauthId = oauth.id;
					}}
				>
					{#if currentOauthId === oauth.id}
						Authenticating...
					{:else}
						Authenticate
					{/if}
				</a>
			</div>
			<form class="dialog-backdrop">
				<button type="button" aria-label="Close dialog" onclick={dismissCurrent}>close</button>
			</form>
		</dialog>
	{:else if requirements[0]?.type === 'admin-oauth'}
		{@const adminOauth = requirements[0] as AdminOauthRequirement}
		<dialog bind:this={oauthDialog} class="dialog" use:dialogAnimation={{ type: 'fade' }}>
			<div class="dialog-container relative flex w-full flex-col gap-4 p-4 md:w-sm">
				<div class="absolute top-2 right-2">
					<button class="icon-button" onclick={dismissCurrent}>
						<X class="size-4" />
					</button>
				</div>
				<div class="flex items-center gap-2">
					<div class="h-fit flex-shrink-0 self-start rounded-md bg-gray-50 p-1 dark:bg-gray-600">
						{#if adminOauth.icon}
							<img src={adminOauth.icon} alt={adminOauth.name} class="size-6" />
						{:else}
							<Server class="size-6" />
						{/if}
					</div>
					<h3 class="text-lg leading-5.5 font-semibold">
						{adminOauth.name}
					</h3>
				</div>

				<div class="notification-warning flex items-start gap-2 p-3">
					<AlertTriangle class="size-5 flex-shrink-0 text-yellow-500" />
					<p class="text-sm">
						This MCP server requires OAuth credentials to be configured by an administrator.
					</p>
				</div>

				<p class="text-sm font-light">
					Please contact your administrator to configure the OAuth credentials for this server.
				</p>

				<button class="button" onclick={dismissCurrent}> Dismiss </button>
			</div>
			<form class="dialog-backdrop">
				<button type="button" aria-label="Close dialog" onclick={dismissCurrent}>close</button>
			</form>
		</dialog>
	{:else if requirements[0]?.type === 'config'}
		<CatalogConfigureForm
			bind:this={configDialog}
			bind:form={configureForm}
			name={configName}
			icon={configIcon}
			serverId={configServerId}
			submitText="Save"
			loading={configuring}
			error={configError}
			onSave={handleSaveConfig}
			onClose={() => {
				if (currentConfigReq) {
					closed.add(currentConfigReq.id);
					currentConfigReq = null;
				}
			}}
		/>
	{/if}
{/key}
