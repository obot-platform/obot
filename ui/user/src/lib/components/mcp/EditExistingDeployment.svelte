<script lang="ts">
	import { HttpError } from '$lib/errors';
	import {
		AdminService,
		UserService,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type MCPAllowedSecretBindingTarget,
		type MCPCatalogServerManifest,
		type MCPSubField,
		type CatalogUpgradePlan
	} from '$lib/services';
	import type { EventStreamService } from '$lib/services/admin/eventstream.svelte';
	import {
		convertCompositeInfoToLaunchFormData,
		convertCompositeLaunchFormDataToPayload,
		convertEnvHeadersToRecord,
		getMCPDisplayName,
		getSecretBindingEngineError,
		hasSecretBinding,
		isDeprecatedMCPServer,
		isKubernetesRuntimeBackend
	} from '$lib/services/user/mcp';
	import { errors, version } from '$lib/stores';
	import CatalogConfigureForm, {
		type CompositeLaunchFormData,
		type LaunchFormData
	} from './CatalogConfigureForm.svelte';
	import CatalogEditAliasForm from './CatalogEditAliasForm.svelte';
	import {
		catalogUpgradeConfiguration,
		catalogUpgradeForm,
		catalogUpgradeNeedsConfiguration,
		getCatalogUpgradeBlockers
	} from './catalogUpgrade';
	import { CircleAlert } from '@lucide/svelte';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		onUpdateConfigure?: () => void | Promise<void>;
	}
	let { onUpdateConfigure }: Props = $props();

	let configDialog = $state<ReturnType<typeof CatalogConfigureForm>>();
	let configureForm = $state<LaunchFormData | CompositeLaunchFormData>();
	let editAliasDialog = $state<ReturnType<typeof CatalogEditAliasForm>>();

	let entry = $state<MCPCatalogEntry>();
	let server = $state<MCPCatalogServer>();
	let mode = $state<'edit' | 'catalog-update'>('edit');
	let upgradePlan = $state<CatalogUpgradePlan>();

	let editingError = $state<string>();
	let editingManifest = $derived(upgradePlan?.targetManifest ?? server?.manifest);
	let deprecated = $derived(isDeprecatedMCPServer(entry) || isDeprecatedMCPServer(server));
	let secretBindingEngineError = $derived(
		isKubernetesRuntimeBackend(version.current.engine)
			? undefined
			: getSecretBindingEngineError(editingManifest)
	);
	let editing = $state(false);
	let launchError = $state<string>();
	let launchProgress = $state<number>(0);
	let launchLogsEventStream = $state<EventStreamService<string>>();
	let launchLogs = $state<string[]>([]);
	let secretBindingTargets = $state<MCPAllowedSecretBindingTarget[]>([]);

	const editableSecretBindingTargets = $derived(
		isKubernetesRuntimeBackend(version.current.engine) &&
			server?.mcpCatalogID &&
			isMultiUserServer(server)
			? secretBindingTargets
			: undefined
	);

	function isMultiUserServer(server?: MCPCatalogServer) {
		return (server as { serverUserType?: string } | undefined)?.serverUserType === 'multiUser';
	}

	function sameSecretBinding(
		a?: { name?: string; key?: string },
		b?: { name?: string; key?: string }
	) {
		if (!a?.name || !a?.key || !b?.name || !b?.key) return false;
		return a.name === b.name && a.key === b.key;
	}

	function templateBindingByKey(fields?: MCPSubField[]) {
		return new Map(
			(fields ?? [])
				.filter((field) => hasSecretBinding(field))
				.map((field) => [field.key, field.secretBinding])
		);
	}

	function markPinnedSecretBinding(
		field: MCPSubField,
		templateBindings: Map<string, MCPSubField['secretBinding']>
	) {
		return {
			...field,
			secretBindingReadonly: sameSecretBinding(field.secretBinding, templateBindings.get(field.key))
		};
	}

	function applyFormBindingsToManifest(
		manifest: MCPCatalogServer['manifest'],
		form: LaunchFormData
	): MCPCatalogServerManifest['manifest'] {
		const envByKey = new Map((form.envs ?? []).map((field) => [field.key, field]));
		const headerByKey = new Map((form.headers ?? []).map((field) => [field.key, field]));
		const withoutSecretBinding = <T extends MCPSubField>(field: T) => {
			const result = { ...field };
			delete result.secretBinding;
			return result;
		};
		const updated = {
			...manifest,
			env: manifest.env?.map((field) => {
				const formField = envByKey.get(field.key);
				if (!formField?.secretBinding) return withoutSecretBinding(field);
				return { ...field, value: '', secretBinding: formField.secretBinding };
			})
		};
		if (manifest.remoteConfig) {
			updated.remoteConfig = {
				...manifest.remoteConfig,
				...(form.url?.trim() ? { url: form.url.trim() } : {}),
				headers: manifest.remoteConfig.headers?.map((field) => {
					const formField = headerByKey.get(field.key);
					if (!formField?.secretBinding) return withoutSecretBinding(field);
					return { ...field, value: '', secretBinding: formField.secretBinding };
				})
			};
		}
		return updated as unknown as MCPCatalogServerManifest['manifest'];
	}

	export async function edit({
		server: initServer,
		entry: initEntry
	}: {
		server: MCPCatalogServer;
		entry?: MCPCatalogEntry;
	}) {
		server = initServer;
		entry = initEntry;
		mode = 'edit';
		upgradePlan = undefined;
		editingError = isKubernetesRuntimeBackend(version.current.engine)
			? undefined
			: getSecretBindingEngineError(initServer.manifest);

		if (entry?.manifest.runtime === 'composite') {
			configureForm = await convertCompositeInfoToLaunchFormData(server);
			configDialog?.open();
			return;
		}
		if (
			isKubernetesRuntimeBackend(version.current.engine) &&
			initServer.mcpCatalogID &&
			isMultiUserServer(initServer)
		) {
			try {
				secretBindingTargets = await AdminService.listMCPSecretBindingTargets({
					dontLogErrors: true
				});
			} catch (err) {
				errors.append(`Failed to load Kubernetes Secrets for binding: ${err}`);
				secretBindingTargets = [];
			}
		} else {
			secretBindingTargets = [];
		}

		let values: Record<string, string>;
		try {
			values = await revealServerValues(server);
		} catch (error) {
			if (!(error instanceof HttpError) || error.statusCode !== 404) {
				console.error('Failed to reveal server values due to unexpected error', error);
			}
			values = {};
		}
		const templateEnvBindings = templateBindingByKey(entry?.manifest.env);
		const templateHeaderBindings = templateBindingByKey(entry?.manifest.remoteConfig?.headers);
		configureForm = {
			name: server.alias || '',
			envs: server.manifest.env?.map((env) => ({
				...markPinnedSecretBinding(env, templateEnvBindings),
				value: values[env.key] ?? ''
			})),
			headers: server.manifest.remoteConfig?.headers?.map((header) => ({
				...markPinnedSecretBinding(header, templateHeaderBindings),
				value: values[header.key] ?? '',
				isStatic: header.value !== ''
			})),
			url: server.manifest.remoteConfig?.url,
			hostname: entry?.manifest.remoteConfig?.hostname
		};
		configDialog?.open();
	}

	export async function updateFromCatalogEntry({
		server: initServer,
		entry: initEntry,
		plan
	}: {
		server: MCPCatalogServer;
		entry: MCPCatalogEntry;
		plan?: CatalogUpgradePlan;
	}): Promise<boolean> {
		server = initServer;
		entry = initEntry;
		mode = 'catalog-update';
		upgradePlan = plan ?? (await previewCatalogUpdate(initServer));
		const blockers = getCatalogUpgradeBlockers(upgradePlan);
		if (blockers.length > 0) {
			throw new Error(blockers.join(' '));
		}
		editingError = isKubernetesRuntimeBackend(version.current.engine)
			? undefined
			: getSecretBindingEngineError(upgradePlan.targetManifest);

		// Load secret binding targets so an admin can re-select a secret during the update flow
		if (
			isKubernetesRuntimeBackend(version.current.engine) &&
			initServer.mcpCatalogID &&
			isMultiUserServer(initServer)
		) {
			try {
				secretBindingTargets = await AdminService.listMCPSecretBindingTargets({
					dontLogErrors: true
				});
			} catch (err) {
				errors.append(`Failed to load Kubernetes Secrets for binding: ${err}`);
				secretBindingTargets = [];
			}
		} else {
			secretBindingTargets = [];
		}

		if (!catalogUpgradeNeedsConfiguration(upgradePlan)) {
			await applyCatalogUpdate(initServer, upgradePlan, {});
			await onUpdateConfigure?.();
			return false;
		}

		configureForm = catalogUpgradeForm(upgradePlan);
		configDialog?.open();
		return true;
	}

	async function revealServerValues(server: MCPCatalogServer) {
		if (server.powerUserWorkspaceID) {
			return UserService.revealWorkspaceMCPCatalogServer(server.powerUserWorkspaceID, server.id, {
				dontLogErrors: true
			});
		}

		if (server.mcpCatalogID) {
			return AdminService.revealMcpCatalogServer(server.mcpCatalogID, server.id, {
				dontLogErrors: true
			});
		}

		return UserService.revealSingleOrRemoteMcpServer(server.id, {
			dontLogErrors: true
		});
	}

	export function rename({
		server: initServer,
		entry: initEntry
	}: {
		server: MCPCatalogServer;
		entry?: MCPCatalogEntry;
	}) {
		server = initServer;
		entry = initEntry;
		mode = 'edit';
		upgradePlan = undefined;

		editAliasDialog?.open();
	}

	function initUpdatingOrLaunchProgress() {
		if (launchLogsEventStream) {
			// reset launch logs
			launchLogsEventStream.disconnect();
			launchLogsEventStream = undefined;
			launchLogs = [];
		}

		launchError = undefined;
		launchProgress = 0;

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

	async function updateExistingRemoteOrSingleUser(lf: LaunchFormData) {
		if (!server) return;

		if (
			entry &&
			entry.manifest.runtime === 'remote' &&
			// The update-url endpoint should only be called for remote servers that have a hostname set. For ones that have a fixedURL or urlTemplate the update-url endpoint is not supported.
			entry.manifest.remoteConfig?.hostname &&
			lf?.url
		) {
			await UserService.updateRemoteMcpServerUrl(server.id, lf.url.trim());
		}

		const envs = convertEnvHeadersToRecord(lf.envs, lf.headers);
		if (server.mcpCatalogID) {
			if (isMultiUserServer(server)) {
				await AdminService.updateMCPCatalogServer(
					server.mcpCatalogID,
					server.id,
					applyFormBindingsToManifest(server.manifest, lf)
				);
			}
			await AdminService.configureMCPCatalogServer(server.mcpCatalogID, server.id, envs);
		} else if (server.powerUserWorkspaceID) {
			await UserService.configureWorkspaceMCPCatalogServer(
				server.powerUserWorkspaceID,
				server.id,
				envs
			);
		} else {
			await UserService.configureSingleOrRemoteMcpServer(server.id, envs);
		}
		await updateServerAlias(lf.name?.trim() ?? '');
	}

	async function updateServerAlias(alias: string) {
		if (!server || alias === (server.alias || '')) return;

		if (server.powerUserWorkspaceID) {
			await UserService.updateWorkspaceMCPCatalogServerAlias(
				server.powerUserWorkspaceID,
				server.id,
				alias
			);
		} else if (server.mcpCatalogID) {
			await AdminService.updateMCPCatalogServerAlias(server.mcpCatalogID, server.id, alias);
		} else {
			await UserService.updateSingleOrRemoteMcpServerAlias(server.id, alias);
		}

		server = { ...server, alias };
	}

	async function previewCatalogUpdate(server: MCPCatalogServer): Promise<CatalogUpgradePlan> {
		if (server.powerUserWorkspaceID && server.catalogEntryID) {
			return UserService.previewWorkspaceMcpServerCatalogUpgrade(
				server.powerUserWorkspaceID,
				server.catalogEntryID,
				server.id
			);
		}
		if (server.mcpCatalogID) {
			return AdminService.previewMCPCatalogServerUpgrade(server.mcpCatalogID, server.id);
		}
		return UserService.previewMcpServerCatalogUpgrade(server.id);
	}

	async function applyCatalogUpdate(
		server: MCPCatalogServer,
		plan: CatalogUpgradePlan,
		configuration: Record<string, string>,
		url?: string
	) {
		if (plan.oauthReauthorizationRequired) {
			await clearCatalogUpdateOAuth(server);
		}
		const request = {
			planID: plan.id,
			configuration,
			url: url?.trim() || undefined,
			confirmOAuthReauthorization: plan.oauthReauthorizationRequired || undefined
		};
		if (server.powerUserWorkspaceID && server.catalogEntryID) {
			return UserService.applyWorkspaceMcpServerCatalogUpgrade(
				server.powerUserWorkspaceID,
				server.catalogEntryID,
				server.id,
				request
			);
		}
		if (server.mcpCatalogID) {
			return AdminService.applyMCPCatalogServerUpgrade(server.mcpCatalogID, server.id, request);
		}
		return UserService.applyMcpServerCatalogUpgrade(server.id, request);
	}

	async function clearCatalogUpdateOAuth(server: MCPCatalogServer) {
		if (server.powerUserWorkspaceID && isMultiUserServer(server)) {
			await UserService.clearWorkspaceMcpServerOAuth(server.powerUserWorkspaceID, server.id);
		} else if (server.mcpCatalogID) {
			await AdminService.clearMCPCatalogServerOAuth(server.mcpCatalogID, server.id);
		} else {
			await UserService.clearMcpServerOAuth(server.id);
		}
	}

	async function configureUpdatedCatalogServer(lf: LaunchFormData) {
		if (!server || !upgradePlan) return;
		await applyCatalogUpdate(server, upgradePlan, catalogUpgradeConfiguration(lf), lf.url);
	}

	async function updateExistingComposite(lf: CompositeLaunchFormData) {
		if (!server) return;
		// Composite flow using CatalogConfigureForm data
		if ('componentConfigs' in lf) {
			const payload = convertCompositeLaunchFormDataToPayload(lf);
			await UserService.configureCompositeMcpServer(server.id, payload);
		}
		await updateServerAlias(lf.name?.trim() ?? '');
	}

	async function handleConfigureForm() {
		if (!server) return;
		if (!configureForm) return;

		editing = true;
		try {
			const { timeout1, timeout2, timeout3 } = initUpdatingOrLaunchProgress();
			if (mode === 'catalog-update') {
				const lf = configureForm as LaunchFormData;
				await configureUpdatedCatalogServer(lf);
			} else if (entry?.manifest.runtime === 'composite') {
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
			await onUpdateConfigure?.();
			// to allow user to see launch completed to 100%
			await new Promise((resolve) => setTimeout(resolve, 1000));
			configDialog?.close();

			setTimeout(() => {
				editing = false;
			}, 1000);
		} catch (_error) {
			console.error('Error during configuration:', _error);
			launchError = _error instanceof Error ? _error.message : 'An unknown error occurred';
		}
	}
</script>

<CatalogConfigureForm
	bind:this={configDialog}
	bind:form={configureForm}
	error={editingError}
	icon={editingManifest?.icon}
	name={getMCPDisplayName(server)}
	onSave={handleConfigureForm}
	submitText={mode === 'catalog-update' && upgradePlan?.oauthReauthorizationRequired
		? 'Clear OAuth and Update'
		: 'Update'}
	loading={editing}
	disableSave={!!secretBindingEngineError}
	isNew={false}
	showAlias={mode === 'edit'}
	configurationTitle={mode === 'catalog-update' ? 'Required Configuration' : undefined}
	secretBindingTargets={editableSecretBindingTargets}
	disableEnvSecretBindings={editingManifest?.runtime === 'remote'}
	{deprecated}
>
	{#snippet loadingContent()}
		<div in:fade class="h-full w-full flex items-center justify-center">
			{#if launchError}
				<div class="flex flex-col gap-1 mb-4 w-full h-full" in:fade>
					<div class="notification-error flex items-center gap-2">
						<CircleAlert class="size-6 text-error" />
						<h4 class="text-md font-medium">MCP Server Launch Failed</h4>
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
						<p class="text-sm self-start">An issue occurred while launching the MCP server.</p>
					{/if}

					<div class="flex w-full flex-col items-center gap-2 md:flex-row mt-2">
						{#if entry}
							<button
								class="btn btn-primary w-full md:w-1/2 md:flex-1"
								onclick={() => {
									launchError = undefined;
									launchProgress = 0;
									launchLogs = [];
									editing = false;
								}}
							>
								Update Configuration and Try Again
							</button>
						{/if}
					</div>
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

<CatalogEditAliasForm bind:this={editAliasDialog} {server} {onUpdateConfigure} />
