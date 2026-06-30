<script lang="ts">
	import { dialogAnimation } from '$lib/actions/dialogAnimation';
	import {
		AdminService,
		UserService,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type MCPAllowedSecretBindingTarget,
		type MCPSubField,
		type MCPServerInstance
	} from '$lib/services';
	import { EventStreamService } from '$lib/services/admin/eventstream.svelte';
	import {
		convertCompositeLaunchFormDataToPayload,
		convertEnvHeadersToRecord,
		getSecretBindingEngineError,
		isMultiUserServer,
		isKubernetesRuntimeBackend,
		hasEditableConfiguration,
		getMCPDisplayName,
		hasSecretBinding,
		isDeprecatedMCPServer,
		supportsMCPBackendDetails
	} from '$lib/services/user/mcp';
	import { errors, mcpServersAndEntries, profile, version } from '$lib/stores';
	import { goto } from '$lib/url';
	import Confirm from '../Confirm.svelte';
	import CopyField from '../CopyField.svelte';
	import DotDotDot from '../DotDotDot.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import CatalogConfigureForm, {
		type CompositeLaunchFormData,
		type LaunchFormData
	} from './CatalogConfigureForm.svelte';
	import HowToConnect from './HowToConnect.svelte';
	import McpDeprecatedNotice from './McpDeprecatedNotice.svelte';
	import { Server, X, CircleAlert } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		catalogID?: string;
		workspaceID?: string;
		onConnect?: ({
			server,
			entry,
			instance
		}: {
			server?: MCPCatalogServer;
			entry?: MCPCatalogEntry;
			instance?: MCPServerInstance;
		}) => void;
		onClose?: () => void;
		skipConnectDialog?: boolean;
		renderIntroText?: ({
			entry,
			server
		}: {
			entry?: MCPCatalogEntry;
			server?: MCPCatalogServer;
		}) => string;
		introTitle?: string;
	}

	let {
		catalogID,
		workspaceID,
		onConnect,
		onClose,
		skipConnectDialog,
		renderIntroText,
		introTitle
	}: Props = $props();

	let server = $state<MCPCatalogServer>();
	let entry = $state<MCPCatalogEntry>();
	let instance = $state<MCPServerInstance>();
	let userConfiguredServers = $derived(mcpServersAndEntries.current.userConfiguredServers);

	let manifest = $derived(server?.manifest || entry?.manifest);
	let deprecated = $derived(isDeprecatedMCPServer(entry) || isDeprecatedMCPServer(server));
	let isConfigured = $derived(Boolean((entry && server) || (server && instance)));
	let isDeployingMultiUserCatalogEntry = $derived(
		Boolean(entry && !server && isMultiUserCatalogEntry(entry))
	);
	let canBindSecretsForCatalogEntry = $derived(
		Boolean(
			isDeployingMultiUserCatalogEntry &&
			catalogID &&
			!workspaceID &&
			isKubernetesRuntimeBackend(version.current.engine)
		)
	);
	let secretBindingEngineError = $derived(
		isKubernetesRuntimeBackend(version.current.engine)
			? undefined
			: getSecretBindingEngineError(manifest)
	);

	let showIntroDialog = $state(false);

	let connectDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let configDialog = $state<ReturnType<typeof CatalogConfigureForm>>();
	let configureForm = $state<LaunchFormData | CompositeLaunchFormData>();
	let configureFormTitle = $state<string>();
	let configureInstance = $state(false);
	let secretBindingTargets = $state<MCPAllowedSecretBindingTarget[]>([]);
	let secretBindingTargetsLoaded = $state(false);
	let loadingSecretBindingTargets = $state(false);

	let launchError = $state<string>();
	let launchProgress = $state<number>(0);
	let launchLogsEventStream = $state<EventStreamService<string>>();
	let launchLogs = $state<string[]>([]);
	let launchState = $state<'relaunching' | 'launching' | undefined>();
	let launchMissingSecretBinding = $state(false);
	let error = $state<string>();
	let saving = $state(false);

	let shouldShowAlias = $derived(
		isDeployingMultiUserCatalogEntry ||
			(isConfigured && !isMultiUserServer(server) && launchState !== 'relaunching')
	);

	let canModifyCatalogEntry = $derived(
		profile.current.isAdmin?.() || (entry && entry.powerUserID === profile.current.id)
	);

	async function loadSecretBindingTargets() {
		if (
			!canBindSecretsForCatalogEntry ||
			loadingSecretBindingTargets ||
			secretBindingTargetsLoaded
		) {
			return;
		}
		loadingSecretBindingTargets = true;
		try {
			secretBindingTargets = await AdminService.listMCPSecretBindingTargets({
				dontLogErrors: true
			});
		} catch (err) {
			errors.append(`Failed to load Kubernetes Secrets for binding: ${err}`);
			secretBindingTargets = [];
		} finally {
			secretBindingTargetsLoaded = true;
			loadingSecretBindingTargets = false;
		}
	}

	let oauthDialog = $state<HTMLDialogElement>();
	let oauthURL = $state<string>('');
	let oauthVerifying = $state(false);

	let existingServerNames = $derived(
		userConfiguredServers
			.flatMap((server) => [server.manifest?.name || '', server.alias || ''])
			.filter(Boolean)
			.map((name) => name.toLowerCase())
	);

	let howToConnect = $state<ReturnType<typeof HowToConnect>>();
	let connectionUrlField = $state<ReturnType<typeof CopyField>>();

	function handleOnClose() {
		howToConnect?.resetCopied();
		connectionUrlField?.clear();
		onClose?.();
	}

	function handleConnect(skipOnConnect?: boolean) {
		launchState = undefined;
		configDialog?.close();
		if (!skipConnectDialog) {
			connectDialog?.open();
		}

		if (onConnect && !skipOnConnect) {
			onConnect({ server, entry, instance });
		}
	}

	export async function authenticate(item: MCPCatalogServer, parentEntry?: MCPCatalogEntry) {
		server = item;
		entry = parentEntry;
		instance = undefined;
		oauthVerifying = false;
		oauthURL = await getOauthURL();
		if (oauthURL) {
			oauthDialog?.showModal();
		} else {
			handleConnect();
		}
	}

	function getUniqueAlias(serverName: string): string | undefined {
		const nameLower = serverName.toLowerCase();

		// Return undefined if no conflict
		if (!existingServerNames.includes(nameLower)) {
			return undefined;
		}

		// Generate unique alias with counter
		let counter = 1;
		let candidateAlias: string;
		do {
			candidateAlias = `${serverName} ${counter}`;
			counter++;
		} while (existingServerNames.includes(candidateAlias.toLowerCase()));

		return candidateAlias;
	}

	function initConfigureForm(item: MCPCatalogEntry) {
		configureFormTitle = undefined;
		configureForm = {
			name: '',
			envs: item.manifest?.env?.map((env) => ({
				...env,
				value: '',
				isStatic: env.value !== '',
				secretBindingReadonly: hasSecretBinding(env)
			})),
			headers: item.manifest?.remoteConfig?.headers?.map((header) => ({
				...header,
				value: '',
				isStatic: header.value !== '',
				secretBindingReadonly: hasSecretBinding(header)
			})),
			...(item.manifest?.remoteConfig?.hostname
				? { hostname: item.manifest.remoteConfig?.hostname, url: '' }
				: {})
		};
	}

	function secretBoundFields(fields?: MCPSubField[]) {
		// Whitelist the API manifest properties so UI-only runtime fields
		// (e.g. isStatic, secretBindingReadonly) don't leak into the request payload.
		return (fields ?? [])
			.filter((field) => hasSecretBinding(field))
			.map((field) => ({
				key: field.key,
				name: field.name,
				description: field.description,
				required: field.required,
				sensitive: field.sensitive,
				file: field.file,
				prefix: field.prefix,
				secretBinding: field.secretBinding,
				value: ''
			}));
	}

	type TemplateDeployManifest = {
		env?: MCPSubField[];
		remoteConfig?: {
			url?: string;
			headers?: MCPSubField[];
		};
	};

	function buildTemplateSecretBindingManifest(
		form?: LaunchFormData
	): TemplateDeployManifest | undefined {
		const env = secretBoundFields(form?.envs);
		const headers = secretBoundFields(form?.headers);
		const url = form?.url?.trim();
		const manifest: TemplateDeployManifest = {};
		if (env.length > 0) {
			manifest.env = env;
		}
		if (url || headers.length > 0) {
			manifest.remoteConfig = {
				...(url ? { url } : {}),
				...(headers.length > 0 ? { headers } : {})
			};
		}
		return Object.keys(manifest).length > 0 ? manifest : undefined;
	}

	async function initMultiUserInstanceForm(
		item: MCPCatalogServer,
		currentInstance?: MCPServerInstance
	) {
		configureFormTitle = 'User Specific Configuration';
		let values: Record<string, string> = {};
		if (currentInstance) {
			try {
				values = await UserService.revealMcpServerInstance(currentInstance.id, {
					dontLogErrors: true
				});
			} catch (_error) {
				values = {};
			}
		}
		configureForm = {
			headers: item.manifest?.multiUserConfig?.userDefinedHeaders?.map((header) => ({
				...header,
				value: values[header.key] ?? '',
				isStatic: false
			}))
		};
		configDialog?.open();
	}

	function hasMultiUserInstanceConfiguration(item?: MCPCatalogServer) {
		return (item?.manifest?.multiUserConfig?.userDefinedHeaders?.length ?? 0) > 0;
	}

	function isMultiUserCatalogEntry(item?: MCPCatalogEntry) {
		return item?.manifest?.serverUserType === 'multiUser';
	}

	function initCompositeForm(item: MCPCatalogEntry) {
		configureFormTitle = undefined;
		// For composite: open form first to collect per-component URLs before creating
		if (item.manifest.runtime === 'composite') {
			const components = item.manifest?.compositeConfig?.componentServers || [];
			const componentConfigs: Record<
				string,
				{
					name?: string;
					icon?: string;
					deprecated?: boolean;
					hostname?: string;
					url?: string;
					disabled?: boolean;
					isMultiUser?: boolean;
					envs?: Array<Record<string, unknown> & { key: string; value: string }>;
					headers?: Array<Record<string, unknown> & { key: string; value: string }>;
				}
			> = {};
			for (const c of components) {
				const id = c.catalogEntryID || c.mcpServerID;
				if (!id || !c.manifest) continue;
				const m = c.manifest;
				const isMultiUser = !!c.mcpServerID && !c.catalogEntryID;
				componentConfigs[id] = {
					name: m.name,
					icon: m.icon,
					deprecated: isDeprecatedMCPServer({ manifest: m }),
					hostname: isMultiUser ? undefined : m.remoteConfig?.hostname,
					url: isMultiUser ? undefined : (m.remoteConfig?.fixedURL ?? ''),
					disabled: false,
					isMultiUser,
					envs: isMultiUser
						? []
						: (m.env ?? []).map((e) => ({
								...(e as unknown as Record<string, unknown>),
								key: e.key,
								value: '',
								isStatic: e.value !== ''
							})),
					headers: isMultiUser
						? (m.multiUserConfig?.userDefinedHeaders ?? []).map((h) => ({
								...(h as unknown as Record<string, unknown>),
								key: h.key,
								value: '',
								isStatic: false
							}))
						: (m.remoteConfig?.headers ?? []).map((h) => ({
								...(h as unknown as Record<string, unknown>),
								key: h.key,
								value: '',
								isStatic: h.value !== ''
							}))
				};
			}
			configureForm = { componentConfigs } as CompositeLaunchFormData;
			configDialog?.open();
		}
	}

	function listLaunchLogs(mcpServerId: string) {
		launchLogsEventStream = new EventStreamService<string>();
		launchLogsEventStream.connect(`/api/mcp-servers/${mcpServerId}/logs`, {
			onMessage: (data) => {
				launchLogs = [...launchLogs, data];
			}
		});
	}

	function initUpdatingOrLaunchProgress(existing?: boolean) {
		if (launchLogsEventStream) {
			// reset launch logs
			launchLogsEventStream.disconnect();
			launchLogsEventStream = undefined;
			launchLogs = [];
		}

		launchError = undefined;
		launchMissingSecretBinding = false;
		launchProgress = 0;
		launchState = existing ? 'relaunching' : 'launching';

		let timeout1 = setTimeout(() => {
			launchProgress = 10;
		}, 100);

		let timeout2 = setTimeout(() => {
			launchProgress = 30;
		}, 3000);

		let timeout3 = setTimeout(() => {
			launchProgress = 80;
		}, 10000);

		return { timeout1, timeout2, timeout3 };
	}

	function missingSecretBindingConfigMessage(mcpServer: MCPCatalogServer) {
		if (mcpServer.manifest.runtime === 'composite') {
			const missing = [
				...(mcpServer.missingRequiredEnvVars ?? []),
				...(mcpServer.missingRequiredHeader ?? [])
			];
			return missing.length > 0
				? `Missing Kubernetes Secret required by this MCP server: ${missing.join(', ')}`
				: undefined;
		}

		const missingEnvKeys = new Set(mcpServer.missingRequiredEnvVars ?? []);
		const missingHeaderKeys = new Set(mcpServer.missingRequiredHeader ?? []);
		const missing = [
			...(mcpServer.manifest.env ?? [])
				.filter((env) => env.secretBinding && missingEnvKeys.has(env.key))
				.map((env) => env.key),
			...(mcpServer.manifest.remoteConfig?.headers ?? [])
				.filter((header) => header.secretBinding && missingHeaderKeys.has(header.key))
				.map((header) => header.key)
		];
		if (missing.length === 0) return undefined;

		return `Missing Kubernetes Secret required by this MCP server: ${missing.join(', ')}`;
	}

	async function getOauthURL() {
		if (!server) return '';
		// Multi-user server OAuth is admin-managed; per-user connect flow doesn't use it.
		if (isMultiUserServer(server)) return '';
		const oauthURL = await UserService.getMcpServerOauthURL(server.id);
		return oauthURL || '';
	}

	async function handleOauthVisibilityChange() {
		if (!oauthURL && !oauthVerifying) return;
		if (document.visibilityState === 'visible') {
			oauthURL = await getOauthURL();
			if (!oauthURL) {
				oauthDialog?.close();
				handleConnect();
			}
			oauthVerifying = false;
		}
	}

	function ensureOauthVisibilityListener() {
		document.removeEventListener('visibilitychange', handleOauthVisibilityChange);
		document.addEventListener('visibilitychange', handleOauthVisibilityChange);
	}

	async function verifyOauthOrConnect() {
		oauthVerifying = false; // reset
		oauthURL = await getOauthURL();
		launchProgress = 100;

		setTimeout(() => {
			launchState = undefined;
			launchProgress = 0;
			if (oauthURL) {
				configDialog?.close();
				oauthDialog?.showModal();
			} else {
				handleConnect();
			}
		}, 1000);
	}

	async function validateConfiguredServerAndConnect(configuredResponse: MCPCatalogServer) {
		server = configuredResponse;
		const missingConfigMessage = missingSecretBindingConfigMessage(configuredResponse);
		if (missingConfigMessage) {
			launchMissingSecretBinding = true;
			launchError = missingConfigMessage;
			launchProgress = 100;
			return;
		}

		const launchResponse = await UserService.validateSingleOrRemoteMcpServerLaunched(
			configuredResponse.id
		);
		if (!launchResponse.success) {
			launchError = launchResponse.message;
			if (supportsMCPBackendDetails(configuredResponse)) {
				listLaunchLogs(configuredResponse.id);
			}
		}

		if (!launchError) {
			verifyOauthOrConnect();
		}
	}

	async function handleRelaunchExistingServer() {
		if (!server || !entry || !configureForm) return;

		const { timeout1, timeout2, timeout3 } = initUpdatingOrLaunchProgress();
		try {
			let configuredResponse: MCPCatalogServer;
			if (entry.manifest?.runtime === 'composite') {
				const payload = convertCompositeLaunchFormDataToPayload(
					configureForm as CompositeLaunchFormData
				);
				configuredResponse = await UserService.configureCompositeMcpServer(server.id, payload);
			} else {
				await updateExistingRemoteOrSingleUser(configureForm as LaunchFormData);
				configuredResponse = server;
			}
			await validateConfiguredServerAndConnect(configuredResponse);
		} catch (err) {
			launchError = err instanceof Error ? err.message : 'An unknown error occurred';
		} finally {
			clearTimeout(timeout1);
			clearTimeout(timeout2);
			clearTimeout(timeout3);
		}
	}

	async function handleLaunchCatalogEntry() {
		if (!entry) return;

		if (!entry.manifest) {
			console.error('No server manifest found');
			return;
		}

		const { timeout1, timeout2, timeout3 } = initUpdatingOrLaunchProgress();
		const url =
			entry.manifest.runtime === 'remote'
				? (
						(configureForm as LaunchFormData | undefined)?.url ||
						entry.manifest.remoteConfig?.fixedURL
					)?.trim()
				: undefined;
		const serverName = entry.manifest.name || '';

		// Generate unique alias if there's a naming conflict
		const aliasToUse = configureForm?.name || getUniqueAlias(serverName);

		let response: MCPCatalogServer | undefined = undefined;
		try {
			response = await UserService.createSingleOrRemoteMcpServer({
				catalogEntryID: entry.id,
				manifest: url ? { remoteConfig: { url } } : {},
				alias: aliasToUse
			});
			server = response;
		} catch (err) {
			console.error('error: ', err);
			launchError = err instanceof Error ? err.message : 'An unknown error occurred';
		}

		if (response) {
			try {
				const lf = configureForm as LaunchFormData | undefined;
				const envs = convertEnvHeadersToRecord(lf?.envs, lf?.headers);
				const configuredResponse = await UserService.configureSingleOrRemoteMcpServer(
					response.id,
					envs
				);
				await validateConfiguredServerAndConnect(configuredResponse);
			} catch (err) {
				launchError = err instanceof Error ? err.message : 'An unknown error occurred';
			} finally {
				clearTimeout(timeout1);
				clearTimeout(timeout2);
				clearTimeout(timeout3);
			}
		}
	}

	async function handleLaunchCompositeServer() {
		if (!entry) return;

		// If no configureForm yet, initialize the composite form so user can enable/disable components.
		if (!configureForm || !('componentConfigs' in configureForm)) {
			initCompositeForm(entry);
			return;
		}

		if (!entry.manifest) {
			console.error('No server manifest found');
			return;
		}

		if (launchLogsEventStream) {
			// reset launch logs
			launchLogsEventStream.disconnect();
			launchLogsEventStream = undefined;
			launchLogs = [];
		}

		launchError = undefined;
		launchProgress = 0;
		launchState = 'launching';

		let timeout1 = setTimeout(() => {
			launchProgress = 10;
		}, 100);

		let timeout2 = setTimeout(() => {
			launchProgress = 30;
		}, 3000);

		let timeout3 = setTimeout(() => {
			launchProgress = 80;
		}, 10000);

		try {
			const aliasToUse =
				(configureForm as { name?: string } | undefined)?.name ||
				getUniqueAlias(entry.manifest.name || '');
			const componentServersForCreate: Array<{
				catalogEntryID: string;
				manifest: Record<string, unknown>;
				disabled?: boolean;
			}> = [];
			const payload: Record<
				string,
				{ config: Record<string, string>; url?: string; disabled?: boolean }
			> = {};
			for (const [id, comp] of Object.entries(configureForm.componentConfigs)) {
				const url = comp.url?.trim();
				componentServersForCreate.push({
					catalogEntryID: id,
					manifest: url
						? { remoteConfig: { url: url.startsWith('http') ? url : `https://${url}` } }
						: {},
					disabled: comp.disabled ?? false
				});
				const config: Record<string, string> = {};
				for (const f of [
					...(comp.envs ?? ([] as Array<{ key: string; value: string }>)),
					...(comp.headers ?? ([] as Array<{ key: string; value: string }>))
				]) {
					if (f.value) config[f.key] = f.value;
				}
				payload[id] = { config, url, disabled: comp.disabled ?? false };
			}

			const created = await UserService.createCompositeMcpServer({
				catalogEntryID: entry.id,
				alias: aliasToUse,
				manifest: {
					compositeConfig: { componentServers: componentServersForCreate }
				}
			});
			server = created;

			const configured = await UserService.configureCompositeMcpServer(created.id, payload);
			await validateConfiguredServerAndConnect(configured);
		} catch (err) {
			launchError = err instanceof Error ? err.message : 'An unknown error occurred';
		} finally {
			clearTimeout(timeout1);
			clearTimeout(timeout2);
			clearTimeout(timeout3);
		}
	}

	async function handleMultiUserServer() {
		if (!server) return;
		try {
			if (hasMultiUserInstanceConfiguration(server)) {
				await initMultiUserInstanceForm(server);
				return;
			}

			const response = await UserService.createMcpServerInstance(server.id);
			instance = response;
			await finishMultiUserServerConnect();
		} catch (err) {
			error = err instanceof Error ? err.message : 'An unknown error occurred';
		}
	}

	async function handleLaunchMultiUserCatalogEntry() {
		if (!entry) return;
		if (!catalogID && !workspaceID) {
			error = 'A catalog or workspace is required to deploy a multi-user catalog entry.';
			return;
		}

		let created: MCPCatalogServer | undefined;
		let launchHandedOff = false;
		const { timeout1, timeout2, timeout3 } = initUpdatingOrLaunchProgress();
		try {
			const lf = configureForm as LaunchFormData | undefined;
			const aliasToUse = lf?.name?.trim() || getUniqueAlias(entry.manifest.name || '');
			const manifest = canBindSecretsForCatalogEntry
				? buildTemplateSecretBindingManifest(lf)
				: lf?.url?.trim()
					? { remoteConfig: { url: lf.url.trim() } }
					: undefined;
			const serverPayload = {
				...(manifest ? { manifest } : {}),
				alias: aliasToUse
			};
			if (workspaceID) {
				created = await UserService.deployWorkspaceMultiUserCatalogEntry(
					workspaceID,
					entry.id,
					serverPayload
				);
			} else {
				created = await AdminService.deployMultiUserCatalogEntry(
					catalogID!,
					entry.id,
					serverPayload
				);
			}
			server = created;

			const staticEnvValues =
				entry.manifest.env?.reduce<Record<string, string>>((acc, env) => {
					if (env.value) {
						acc[env.key] = env.value;
					}
					return acc;
				}, {}) ?? {};
			const envs = convertEnvHeadersToRecord(lf?.envs, lf?.headers, staticEnvValues);
			server = workspaceID
				? await UserService.configureWorkspaceMCPCatalogServer(workspaceID, created.id, envs)
				: await AdminService.configureMCPCatalogServer(catalogID!, created.id, envs);

			instance = undefined;
			launchHandedOff = true;
			launchState = undefined;
			launchProgress = 100;

			await new Promise((resolve) => setTimeout(resolve, 1000));
			configDialog?.close();
			onConnect?.({ server, entry });
		} catch (err) {
			launchError = err instanceof Error ? err.message : 'An unknown error occurred';
			launchMissingSecretBinding = launchError.includes('secret binding');
			if (created && !launchHandedOff) {
				try {
					if (workspaceID) {
						await UserService.deleteWorkspaceMCPCatalogServer(workspaceID, created.id);
					} else if (catalogID) {
						await AdminService.deleteMCPCatalogServer(catalogID, created.id);
					}
					server = undefined;
					instance = undefined;
				} catch (cleanupErr) {
					console.error('Failed to clean up partially-created multi-user server', cleanupErr);
				}
			}
		} finally {
			clearTimeout(timeout1);
			clearTimeout(timeout2);
			clearTimeout(timeout3);
		}
	}

	async function finishMultiUserServerConnect() {
		oauthURL = await getOauthURL();
		if (oauthURL) {
			oauthDialog?.showModal();
		} else {
			handleConnect();
		}
	}

	async function handleLaunch() {
		error = undefined;
		saving = true;
		try {
			if (entry && isMultiUserCatalogEntry(entry) && !server) {
				await handleLaunchMultiUserCatalogEntry();
			} else if (entry && entry.manifest?.runtime === 'composite') {
				await handleLaunchCompositeServer();
			} else if (isMultiUserServer(server)) {
				// Deployed multi-user servers (including catalog entry deployments) always
				// create an MCPServerInstance, regardless of whether entry is also set.
				await handleMultiUserServer();
			} else if (entry) {
				await handleLaunchCatalogEntry();
			} else {
				await handleMultiUserServer();
			}
		} catch (error) {
			console.error('Error during launching', error);
		} finally {
			saving = false;
		}
	}

	async function deleteCatalogEntryServer() {
		if (server && entry) {
			if (isMultiUserServer(server)) {
				if (workspaceID) {
					await UserService.deleteWorkspaceMCPCatalogServer(workspaceID, server.id);
				} else if (catalogID) {
					await AdminService.deleteMCPCatalogServer(catalogID, server.id);
				}
			} else {
				await UserService.deleteSingleOrRemoteMcpServer(server.id);
			}
		}
	}

	async function handleCancelLaunch() {
		if (launchLogsEventStream) {
			launchLogsEventStream.disconnect();
		}
		await deleteCatalogEntryServer();

		launchState = undefined;
		launchError = undefined;
		launchMissingSecretBinding = false;

		configDialog?.close();
	}

	async function updateExistingRemoteOrSingleUser(lf: LaunchFormData) {
		if (!entry || !server) return;
		if (
			entry &&
			entry.manifest.runtime === 'remote' &&
			entry.manifest.remoteConfig?.hostname &&
			lf?.url
		) {
			await UserService.updateRemoteMcpServerUrl(server.id, lf.url.trim());
		}

		const envs = convertEnvHeadersToRecord(lf.envs, lf.headers);
		await UserService.configureSingleOrRemoteMcpServer(server.id, envs);

		server = await UserService.getSingleOrRemoteMcpServer(server.id);
	}

	async function updateExistingComposite(lf: CompositeLaunchFormData) {
		if (!server) return;
		// Composite flow using CatalogConfigureForm data
		if ('componentConfigs' in lf) {
			const payload = convertCompositeLaunchFormDataToPayload(lf);
			await UserService.configureCompositeMcpServer(server.id, payload);
		}
	}

	async function handleConfigureForm() {
		if (!configureForm) return;
		if (isMultiUserServer(server) && hasMultiUserInstanceConfiguration(server)) {
			try {
				if (!server) return;
				saving = true;
				const lf = configureForm as LaunchFormData;
				if (!instance) {
					instance = await UserService.createMcpServerInstance(server.id);
				}
				const configuredInstance = await UserService.configureMcpServerInstance(
					instance.id,
					convertEnvHeadersToRecord(undefined, lf.headers)
				);
				instance = configuredInstance;
				configDialog?.close();
				await finishMultiUserServerConnect();
			} catch (err) {
				error = err instanceof Error ? err.message : 'An unknown error occurred';
			} finally {
				saving = false;
			}
			return;
		}

		if (launchState === 'relaunching' && server && entry) {
			saving = true;
			try {
				await handleRelaunchExistingServer();
			} finally {
				saving = false;
			}
			return;
		}

		try {
			if (server?.id) {
				saving = true;
				const { timeout1, timeout2, timeout3 } = initUpdatingOrLaunchProgress(true);
				// updating existing
				if (entry?.id === 'composite') {
					const lf = configureForm as CompositeLaunchFormData;
					await updateExistingComposite(lf);
				} else {
					const lf = configureForm as LaunchFormData;
					await updateExistingRemoteOrSingleUser(lf);
				}
				launchProgress = 100;
				clearTimeout(timeout1);
				clearTimeout(timeout2);
				clearTimeout(timeout3);
				// onUpdate?.();

				await new Promise((resolve) => setTimeout(resolve, 1000));
				launchState = undefined;
				saving = false;
			} else {
				// launching new
				await new Promise((resolve) => setTimeout(resolve, 300));
				await handleLaunch();
			}
		} catch (_error) {
			console.error('Error during configuration:', _error);
			configDialog?.close();
		}
	}

	async function initCatalogEntry() {
		if (!entry) return;
		error = secretBindingEngineError;
		if (secretBindingEngineError && entry.manifest?.runtime === 'composite') {
			await loadSecretBindingTargets();
			initCompositeForm(entry);
			return;
		}
		if (secretBindingEngineError) {
			await loadSecretBindingTargets();
			initConfigureForm(entry);
			configDialog?.open();
			return;
		}
		if (hasEditableConfiguration(entry) && entry.manifest?.runtime === 'composite') {
			await loadSecretBindingTargets();
			initCompositeForm(entry);
		} else if (hasEditableConfiguration(entry) || isMultiUserCatalogEntry(entry)) {
			await loadSecretBindingTargets();
			initConfigureForm(entry);
			configDialog?.open();
		} else {
			configDialog?.open();
			handleLaunch();
		}
	}

	export function setupNewInstance(initEntry: MCPCatalogEntry) {
		entry = initEntry;
		server = undefined;
		instance = undefined;
		showIntroDialog = true;
	}

	function handleLaunchOrConfigure() {
		showIntroDialog = false;
		ensureOauthVisibilityListener();

		if (server && instance && configureInstance && hasMultiUserInstanceConfiguration(server)) {
			initMultiUserInstanceForm(server, instance);
		} else if (
			server &&
			instance &&
			!instance.configured &&
			hasMultiUserInstanceConfiguration(server)
		) {
			initMultiUserInstanceForm(server, instance);
		} else if (isMultiUserServer(server) && !instance) {
			handleLaunch();
		} else if ((entry && server) || (server && instance)) {
			handleConnect();
		} else {
			if (entry && !server) {
				initCatalogEntry();
			} else {
				handleLaunch();
			}
		}
	}

	export function open({
		server: initServer,
		entry: initEntry,
		instance: initInstance,
		configureInstance: initConfigureInstance
	}: {
		server?: MCPCatalogServer;
		entry?: MCPCatalogEntry;
		instance?: MCPServerInstance;
		configureInstance?: boolean;
	}) {
		server = initServer;
		entry = initEntry;
		instance = initInstance;
		configureInstance = initConfigureInstance ?? false;

		if (server && instance && configureInstance && hasMultiUserInstanceConfiguration(server)) {
			initMultiUserInstanceForm(server, instance);
		} else if (
			entry?.connectURL ||
			server?.connectURL ||
			(entry && server) ||
			(server && instance)
		) {
			connectDialog?.open();
		} else {
			showIntroDialog = true;
		}
	}

	function handleOauthClose() {
		oauthDialog?.close();
		oauthURL = '';
		handleConnect();
	}

	function generateIdFromName(name: string) {
		return name
			.toLowerCase()
			.replace(/ /g, '-')
			.replace(/[^a-z0-9-_]/g, '');
	}

	function isEditableCatalogEntry(entry?: MCPCatalogEntry) {
		return Boolean(
			entry &&
			'isCatalogEntry' in entry &&
			hasEditableConfiguration(entry) &&
			!launchMissingSecretBinding
		);
	}

	onMount(() => {
		ensureOauthVisibilityListener();
		return () => {
			document.removeEventListener('visibilitychange', handleOauthVisibilityChange);
		};
	});
</script>

{#snippet dialogTitle(item?: MCPCatalogServer | MCPCatalogEntry)}
	{#if item}
		{@const name = getMCPDisplayName(item)}
		{@const icon = item.manifest.icon ?? ''}

		<div class="bg-base-200 rounded-sm p-1 dark:bg-base-300">
			{#if icon}
				<img src={icon} alt={name} class="size-8" />
			{:else}
				<Server class="size-8" />
			{/if}
		</div>
		{name}
	{/if}
{/snippet}

<ResponsiveDialog bind:this={connectDialog} animate="slide" onClose={handleOnClose}>
	{#snippet titleContent()}
		{@render dialogTitle(server || entry)}
		<McpDeprecatedNotice {deprecated} />
	{/snippet}

	{#if entry?.connectURL || server?.connectURL || instance?.connectURL}
		{@const url = instance?.connectURL || server?.connectURL || entry?.connectURL}
		{@const displayName = getMCPDisplayName(server, entry?.manifest?.name ?? '')}
		{#if url}
			<div class="flex flex-col gap-3 md:p-0 pb-0 p-4">
				<McpDeprecatedNotice {deprecated} variant="notification" />
				<CopyField
					bind:this={connectionUrlField}
					value={url}
					id="connectURL"
					label="Connection URL"
				/>
			</div>
			<HowToConnect
				bind:this={howToConnect}
				{url}
				id={generateIdFromName(displayName)}
				{displayName}
			/>
		{/if}
	{/if}
</ResponsiveDialog>

<Confirm
	show={showIntroDialog}
	onsuccess={handleLaunchOrConfigure}
	submitText="Continue"
	type="info"
	title={introTitle ?? (isMultiUserCatalogEntry(entry) ? 'Launch Server' : 'Connect To Server')}
	oncancel={() => (showIntroDialog = false)}
	hideCancelButton
>
	{#snippet msgContent()}
		<div class="flex items-center gap-2 text-lg font-semibold mb-2">
			{@render dialogTitle(entry || server)}
			<McpDeprecatedNotice {deprecated} />
		</div>
	{/snippet}
	{#snippet note()}
		{#if deprecated}
			<div class="mb-3">
				<McpDeprecatedNotice {deprecated} variant="notification" />
			</div>
		{/if}
		<p>
			{#if renderIntroText}
				{renderIntroText({ entry, server })}
			{:else if isMultiUserCatalogEntry(entry)}
				You are about to launch a new server.
			{:else}
				This will begin the initial setup process for this server.
			{/if}
			{#if (entry && hasEditableConfiguration(entry)) || (server && hasMultiUserInstanceConfiguration(server))}
				Additional configuration details may also be required before the server can be used.
			{:else}
				<br />Click below to begin.
			{/if}
		</p>
	{/snippet}
</Confirm>

<CatalogConfigureForm
	bind:this={configDialog}
	bind:form={configureForm}
	{error}
	icon={manifest?.icon}
	name={getMCPDisplayName(server, entry?.manifest?.name ?? '')}
	onSave={handleConfigureForm}
	submitText={isDeployingMultiUserCatalogEntry
		? 'Create Server'
		: isConfigured
			? 'Update'
			: 'Launch'}
	loading={saving || launchState === 'launching'}
	disableSave={!!secretBindingEngineError}
	isNew={!isConfigured}
	showAlias={shouldShowAlias}
	configurationTitle={configureFormTitle}
	secretBindingTargets={canBindSecretsForCatalogEntry ? secretBindingTargets : undefined}
	disableEnvSecretBindings={manifest?.runtime === 'remote'}
	{deprecated}
>
	{#snippet loadingContent()}
		<div in:fade class="h-full w-full flex items-center justify-center">
			{#if launchError}
				<div class="flex flex-col gap-2 w-full h-full" in:fade>
					<div class="notification-error">
						<div class="flex items-center gap-2">
							<CircleAlert class="size-5 text-error" />
							<h4 class="text-md font-medium">MCP Server Launch Failed</h4>
						</div>

						<div class="text-xs mt-2">
							There was an issue launching the MCP server. Launch logs, if available, will be
							provided below.

							<ul class="list-disc px-4 py-1 space-y-1">
								{#if isEditableCatalogEntry(entry)}
									<li>Verify your configurations provided at launch are correct and try again.</li>
								{/if}
								{#if canModifyCatalogEntry}
									<li>
										Verify your catalog entry configurations consist of all necessary information to
										properly configure the server.
									</li>
								{/if}
								<li>If the issue persists, please contact support.</li>
							</ul>
						</div>
					</div>
					{#if launchLogs.length > 0}
						<div
							class="default-scrollbar-thin bg-base-200 max-h-[50vh] w-full overflow-y-auto rounded-lg p-4 shadow-inner"
						>
							{#each launchLogs as log, i (i)}
								<div class="font-mono text-sm">
									<span class="text-muted-content">{log}</span>
								</div>
							{/each}
						</div>
					{:else}
						<p class="text-sm self-start">{launchError}</p>
					{/if}
					{#if canModifyCatalogEntry}
						<div class="flex w-full items-center gap-0.5">
							{#if server}
								<button
									onclick={handleCancelLaunch}
									class="flex grow items-center justify-center btn btn-secondary rounded-r-none!"
								>
									Cancel and Delete Server
								</button>
							{:else}
								<button
									class="flex grow items-center justify-center btn btn-secondary rounded-r-none!"
									onclick={() => {
										launchState = undefined;
										launchError = undefined;
										launchMissingSecretBinding = false;
										configDialog?.close();
									}}
								>
									Close
								</button>
							{/if}
							<DotDotDot
								class="btn btn-secondary btn-block w-14 rounded-l-none! p-0!"
								disablePortal
							>
								{#snippet children({ toggle })}
									<button
										class="menu-button"
										onclick={() => {
											launchState = 'relaunching';
											launchError = undefined;
											launchProgress = 0;
											launchLogs = [];
											saving = false;
											toggle(false);
										}}
									>
										Update Configuration and Try Again
									</button>
									<button
										class="menu-button"
										onclick={async () => {
											await deleteCatalogEntryServer();
											const url = profile.current.isAdmin?.()
												? `/admin/mcp-catalog/c/${entry?.id}`
												: `/mcp-catalog/c/${entry?.id}`;
											goto(url);
											toggle(false);
										}}
									>
										Go to Catalog Entry
									</button>
								{/snippet}
							</DotDotDot>
						</div>
					{:else}
						<div class="flex w-full flex-col items-center gap-2 md:flex-row mt-2">
							{#if isEditableCatalogEntry(entry)}
								<button
									class="btn btn-primary w-full md:w-1/2 md:flex-1"
									onclick={() => {
										launchState = 'relaunching';
										launchError = undefined;
										launchProgress = 0;
										launchLogs = [];
										saving = false;
									}}
								>
									Update Configuration and Try Again
								</button>
							{/if}
							{#if server}
								<button
									class="btn btn-secondary w-full md:w-1/2 md:flex-1"
									onclick={handleCancelLaunch}
								>
									Cancel and Delete Server
								</button>
							{:else}
								<button
									class="btn btn-secondary w-full md:w-1/2 md:flex-1"
									onclick={() => {
										launchState = undefined;
										launchError = undefined;
										launchMissingSecretBinding = false;
										configDialog?.close();
									}}
								>
									Close
								</button>
							{/if}
						</div>
					{/if}
				</div>
			{:else}
				<div class="flex flex-col gap-1 mb-4">
					<div class="w-full text-xl font-extralight text-center">
						{Math.round(launchProgress ?? 0)}%
					</div>

					<div class="bg-base-400 h-3 w-full overflow-hidden rounded-full">
						<div
							class={twMerge('bg-primary h-full rounded-full transition-all duration-500 ease-out')}
							style="width: {launchProgress ?? 0}%"
						></div>
					</div>

					<div class="flex w-md flex-col justify-center gap-2 text-center">
						<p class="text-xs font-light">Launching MCP server...</p>
					</div>
				</div>
			{/if}
		</div>
	{/snippet}
</CatalogConfigureForm>

<dialog bind:this={oauthDialog} class="dialog" use:dialogAnimation={{ type: 'slide' }}>
	<div class="dialog-container md:w-sm">
		<div class="flex flex-col gap-4 p-4">
			{#if oauthURL}
				<div class="absolute top-2 right-2">
					<IconButton onclick={handleOauthClose}>
						<X class="size-4" />
					</IconButton>
				</div>
				<div class="flex items-center gap-2">
					<div class="h-fit shrink-0 self-start rounded-md bg-base-200 p-1 dark:bg-base-300">
						{#if server?.manifest.icon}
							<img src={server?.manifest.icon} alt={getMCPDisplayName(server)} class="size-6" />
						{:else}
							<Server class="size-6" />
						{/if}
					</div>
					<h3 class="text-lg leading-5.5 font-semibold">
						{getMCPDisplayName(server)}
					</h3>
				</div>

				<p>
					In order to use {getMCPDisplayName(server)}, authentication with the MCP server is
					required.
				</p>

				<p>Click the link below to authenticate.</p>

				<!-- eslint-disable svelte/no-navigation-without-resolve -- external OAuth URL -->
				<a
					href={oauthURL}
					rel="external"
					target="_blank"
					class="btn btn-primary text-center text-sm outline-none"
					onclick={() => {
						oauthVerifying = true;
					}}
				>
					{#if oauthVerifying}
						Authenticating...
					{:else}
						Authenticate
					{/if}
				</a>
			{/if}
		</div>
	</div>
	<form class="dialog-backdrop">
		<button type="button" aria-label="Close dialog" onclick={handleOauthClose}>close</button>
	</form>
</dialog>
