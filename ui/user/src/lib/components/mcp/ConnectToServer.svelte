<script lang="ts">
	import { dialogAnimation } from '$lib/actions/dialogAnimation';
	import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
	import { m } from '$lib/i18n';
	import {
		AdminService,
		UserService,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type MCPAllowedSecretBindingTarget,
		type MCPSubField,
		type MCPServerInstance,
		Group
	} from '$lib/services';
	import { EventStreamService } from '$lib/services/admin/eventstream.svelte';
	import {
		convertEnvHeadersToRecord,
		getSecretBindingEngineError,
		isMultiUserServer,
		isKubernetesRuntimeBackend,
		hasEditableConfiguration,
		getMCPDisplayName,
		getManifestConfiguration,
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
	import SelectMcpAccessControlRules from '../admin/SelectMcpAccessControlRules.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import CatalogConfigureForm, { type LaunchFormData } from './CatalogConfigureForm.svelte';
	import HowToConnect from './HowToConnect.svelte';
	import McpDeprecatedNotice from './McpDeprecatedNotice.svelte';
	import { Server, X, CircleAlert } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	type ConnectionResult = {
		server?: MCPCatalogServer;
		entry?: MCPCatalogEntry;
		instance?: MCPServerInstance;
	};

	interface Props {
		catalogID?: string;
		workspaceID?: string;
		onConnect?: (result: ConnectionResult) => void;
		onEdit?: ({
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
		onEdit,
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
	let isLaunchable = $derived(
		(entry && !isMultiUserCatalogEntry(entry) && !server) ||
			(!instance && server && hasMultiUserInstanceConfiguration(server))
	);
	let isEditable = $derived(
		(entry && !isMultiUserCatalogEntry(entry) && hasEditableConfiguration(entry) && server) ||
			(server && instance && hasMultiUserInstanceConfiguration(server))
	);

	let showIntroDialog = $state(false);

	let connectDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let configDialog = $state<ReturnType<typeof CatalogConfigureForm>>();
	let configureForm = $state<LaunchFormData>();
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
	let connectCompletion: ((result: ConnectionResult) => void) | undefined;

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
			errors.append(m.mcps_deployments_secret_targets_load_failed({ error: String(err) }));
			secretBindingTargets = [];
		} finally {
			secretBindingTargetsLoaded = true;
			loadingSecretBindingTargets = false;
		}
	}

	let oauthDialog = $state<HTMLDialogElement>();
	let oauthURL = $state<string>('');
	let oauthVerifying = $state(false);

	let selectRulesDialog = $state<ReturnType<typeof SelectMcpAccessControlRules>>();

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

	function notifyConnected(skipOnConnect = false) {
		const result = { server, entry, instance };
		if (!skipOnConnect) onConnect?.(result);
		const completion = connectCompletion;
		connectCompletion = undefined;
		completion?.(result);
	}

	function handleConnect(skipOnConnect?: boolean) {
		launchState = undefined;
		configDialog?.close();
		if (!skipConnectDialog && !connectCompletion) {
			connectDialog?.open();
		}

		notifyConnected(skipOnConnect);
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
		const { env, headers } = getManifestConfiguration(item.manifest);
		configureFormTitle = undefined;
		configureForm = {
			name: '',
			envs: env.map((field) => ({
				...field,
				value: '',
				isStatic: Boolean(field.static) || field.value !== '',
				secretBindingReadonly: hasSecretBinding(field)
			})),
			headers: headers.map((field) => ({
				...field,
				value: '',
				isStatic: Boolean(field.static) || field.value !== '',
				secretBindingReadonly: hasSecretBinding(field)
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
				dynamicFile: field.dynamicFile,
				interpolated: field.interpolated,
				prefix: field.prefix,
				secretBinding: field.secretBinding,
				value: ''
			}));
	}

	type TemplateDeployManifest = {
		config?: (MCPSubField & { usage: string })[];
		remoteConfig?: {
			url?: string;
		};
	};

	function buildTemplateSecretBindingManifest(
		form?: LaunchFormData
	): TemplateDeployManifest | undefined {
		const env = secretBoundFields(form?.envs);
		const headers = secretBoundFields(form?.headers);
		const url = form?.url?.trim();
		const manifest: TemplateDeployManifest = {};
		if (env.length > 0 || headers.length > 0) {
			manifest.config = [
				...env.map(({ file, dynamicFile, interpolated, ...field }) => ({
					...field,
					usage: interpolated
						? 'interpolated'
						: file
							? dynamicFile
								? 'dynamicFile'
								: 'file'
							: 'env'
				})),
				...headers.map((field) => ({ ...field, usage: 'header' }))
			];
		}
		if (url) {
			manifest.remoteConfig = {
				url
			};
		}
		return Object.keys(manifest).length > 0 ? manifest : undefined;
	}

	async function initMultiUserInstanceForm(
		item: MCPCatalogServer,
		currentInstance?: MCPServerInstance
	) {
		configureFormTitle = m.mcps_connect_user_specific_configuration();
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
			headers: (item.manifest?.config ?? [])
				.filter((field) => field.usage === 'header' && field.userAllowed)
				?.map((header) => ({
					...header,
					value: values[header.key] ?? '',
					isStatic: false
				}))
		};
		configDialog?.open();
	}

	function hasMultiUserInstanceConfiguration(item?: MCPCatalogServer) {
		return (
			((item?.manifest?.config ?? []).filter(
				(field) => field.usage === 'header' && field.userAllowed
			)?.length ?? 0) > 0
		);
	}

	function isMultiUserCatalogEntry(_item?: MCPCatalogEntry) {
		return false;
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
		const missingEnvKeys = new Set(mcpServer.missingRequiredEnvVars ?? []);
		const missingHeaderKeys = new Set(mcpServer.missingRequiredHeader ?? []);
		const missing = [
			...getManifestConfiguration(mcpServer.manifest)
				.env.filter((env) => env.secretBinding && missingEnvKeys.has(env.key))
				.map((env) => env.key),
			...getManifestConfiguration(mcpServer.manifest)
				.headers.filter((header) => header.secretBinding && missingHeaderKeys.has(header.key))
				.map((header) => header.key)
		];
		if (missing.length === 0) return undefined;

		return m.mcps_connect_missing_secret({ keys: missing.join(', ') });
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
			await updateExistingRemoteOrSingleUser(configureForm);
			const configuredResponse = server;
			await validateConfiguredServerAndConnect(configuredResponse);
		} catch (err) {
			launchError = err instanceof Error ? err.message : m.mcps_unknown_error();
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
			launchError = err instanceof Error ? err.message : m.mcps_unknown_error();
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
				launchError = err instanceof Error ? err.message : m.mcps_unknown_error();
			} finally {
				clearTimeout(timeout1);
				clearTimeout(timeout2);
				clearTimeout(timeout3);
			}
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
			error = err instanceof Error ? err.message : m.mcps_unknown_error();
		}
	}

	async function handleLaunchMultiUserCatalogEntry() {
		if (!entry) return;
		if (!catalogID && !workspaceID) {
			error = m.mcps_connect_catalog_required();
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
				getManifestConfiguration(entry.manifest).env.reduce<Record<string, string>>((acc, env) => {
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

			const isAtLeastPowerUserPlus = profile.current?.groups.includes(Group.POWERUSER_PLUS);
			if (isMultiUserCatalogEntry(entry) && isAtLeastPowerUserPlus) {
				const existingRules = isAtLeastPowerUserPlus
					? workspaceID
						? await UserService.listWorkspaceAccessControlRules(workspaceID)
						: await AdminService.listAccessControlRules()
					: [];
				const hasEverythingEveryoneRule = existingRules.some(
					(rule) =>
						rule.subjects?.some((s) => s.id === '*') && rule.resources?.some((r) => r.id === '*')
				);
				const showSetAccessPolicy = isAtLeastPowerUserPlus && !hasEverythingEveryoneRule;
				if (showSetAccessPolicy) {
					selectRulesDialog?.open();
				} else {
					notifyConnected();
				}
			} else {
				notifyConnected();
			}
		} catch (err) {
			launchError = err instanceof Error ? err.message : m.mcps_unknown_error();
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
				error = err instanceof Error ? err.message : m.mcps_unknown_error();
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
				await updateExistingRemoteOrSingleUser(configureForm);
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
		if (secretBindingEngineError) {
			await loadSecretBindingTargets();
			initConfigureForm(entry);
			configDialog?.open();
			return;
		}
		if (hasEditableConfiguration(entry) || isMultiUserCatalogEntry(entry)) {
			await loadSecretBindingTargets();
			initConfigureForm(entry);
			configDialog?.open();
		} else {
			configDialog?.open();
			handleLaunch();
		}
	}

	export function setupNewInstance(
		initEntry: MCPCatalogEntry,
		onComplete?: (result: ConnectionResult) => void
	) {
		entry = initEntry;
		server = undefined;
		instance = undefined;
		connectCompletion = onComplete;
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
		connectCompletion = undefined;
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

<ResponsiveDialog
	bind:this={connectDialog}
	animate="slide"
	onClose={handleOnClose}
	id="connect-to-server-dialog"
>
	{#snippet titleContent()}
		{@render dialogTitle(server || entry)}
		<McpDeprecatedNotice {deprecated} />
	{/snippet}

	{#if entry?.connectURL || server?.connectURL || instance?.connectURL}
		{@const url = instance?.connectURL || server?.connectURL || entry?.connectURL}
		{@const displayName = getMCPDisplayName(server, entry?.manifest?.name ?? '')}
		{#if url}
			<div id="connection-url-container" class="flex flex-col gap-3 md:p-0 pb-0 p-4">
				<McpDeprecatedNotice {deprecated} variant="notification" />
				<CopyField
					bind:this={connectionUrlField}
					value={url}
					id="connectURL"
					label={m.mcps_servers_connection_url()}
				/>
			</div>
			<HowToConnect
				bind:this={howToConnect}
				{url}
				id={generateIdFromName(displayName)}
				{displayName}
				onLaunch={isLaunchable
					? () => {
							connectDialog?.close();
							if (server && !instance && hasMultiUserInstanceConfiguration(server)) {
								showIntroDialog = true;
							} else if (entry) {
								setupNewInstance(entry);
							}
						}
					: undefined}
				onEdit={onEdit && isEditable
					? () => {
							connectDialog?.close();
							onEdit({ entry, server, instance });
						}
					: undefined}
			/>
		{/if}
	{/if}
</ResponsiveDialog>

<Confirm
	show={showIntroDialog}
	onsuccess={handleLaunchOrConfigure}
	submitText={m.core_continue()}
	type="info"
	title={introTitle ??
		(isMultiUserCatalogEntry(entry) ? m.mcps_servers_launch_server() : m.mcps_connect_to_server())}
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
				{m.mcps_connect_intro_launch()}
			{:else}
				{m.mcps_connect_intro_setup()}
			{/if}
			{#if (entry && hasEditableConfiguration(entry)) || (server && hasMultiUserInstanceConfiguration(server))}
				{m.mcps_connect_intro_additional_config()}
			{:else}
				<br />{m.mcps_connect_intro_click_below()}
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
		? m.mcps_servers_actions_create_server()
		: isConfigured
			? m.core_update()
			: m.mcps_catalog_launch()}
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
							<h4 class="text-md font-medium">{m.mcps_deployments_launch_failed_title()}</h4>
						</div>

						<div class="text-xs mt-2">
							{m.mcps_connect_launch_issue()}

							<ul class="list-disc px-4 py-1 space-y-1">
								{#if isEditableCatalogEntry(entry)}
									<li>{m.mcps_connect_verify_launch_config()}</li>
								{/if}
								{#if canModifyCatalogEntry}
									<li>
										{m.mcps_connect_verify_entry_config()}
									</li>
								{/if}
								<li>{m.mcps_connect_contact_support()}</li>
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
									{m.mcps_connect_cancel_and_delete()}
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
									{m.core_close()}
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
										{m.mcps_deployments_launch_update_and_retry()}
									</button>
									<button
										class="menu-button"
										onclick={async () => {
											await deleteCatalogEntryServer();
											const url = `/mcp-servers/c/${entry?.id}${workspaceID ? `?wid=${workspaceID}` : ''}`;
											goto(url);
											toggle(false);
										}}
									>
										{m.mcps_connect_go_to_entry()}
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
									{m.mcps_deployments_launch_update_and_retry()}
								</button>
							{/if}
							{#if server}
								<button
									class="btn btn-secondary w-full md:w-1/2 md:flex-1"
									onclick={handleCancelLaunch}
								>
									{m.mcps_connect_cancel_and_delete()}
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
									{m.core_close()}
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
						<p class="text-xs font-light">{m.mcps_deployments_launching_server()}</p>
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
					{m.mcps_connect_oauth_required({ name: getMCPDisplayName(server) })}
				</p>

				<p>{m.mcps_connect_oauth_click_link()}</p>

				<a
					href={oauthURL}
					rel="external noopener noreferrer"
					target="_blank"
					class="btn btn-primary text-center text-sm outline-none"
					onclick={() => {
						oauthVerifying = true;
					}}
				>
					{#if oauthVerifying}
						{m.mcps_oauth_authenticating()}
					{:else}
						{m.mcps_oauth_authenticate()}
					{/if}
				</a>
			{/if}
		</div>
	</div>
	<form class="dialog-backdrop">
		<button type="button" aria-label={m.common_close_dialog()} onclick={handleOauthClose}
			>{m.common_close()}</button
		>
	</form>
</dialog>

<SelectMcpAccessControlRules
	bind:this={selectRulesDialog}
	entry={isMultiUserCatalogEntry(entry) ? server : entry}
	entity={workspaceID ? 'workspace' : 'catalog'}
	id={workspaceID ?? DEFAULT_MCP_CATALOG_ID}
	onSubmit={() => {
		notifyConnected();
	}}
/>
