<script lang="ts">
	import { resolve } from '$app/paths';
	import CatalogConfigureForm, {
		type CompositeLaunchFormData,
		type LaunchFormData
	} from '$lib/components/mcp/CatalogConfigureForm.svelte';
	import McpDeprecatedNotice from '$lib/components/mcp/McpDeprecatedNotice.svelte';
	import BetaLogo from '$lib/components/navbar/BetaLogo.svelte';
	import { HttpError } from '$lib/errors';
	import { m } from '$lib/i18n';
	import { UserService, type OAuthConsent } from '$lib/services';
	import { getManifestConfiguration } from '$lib/services/user/mcp';
	import {
		convertEnvHeadersToRecord,
		hasEditableConfiguration,
		hasSecretBinding,
		isDeprecatedMCPServer
	} from '$lib/services/user/mcp';
	import { ExternalLink, SettingsIcon, ShieldAlertIcon } from '@lucide/svelte';
	import { onMount, tick, untrack } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	type Props = {
		data: {
			consent: OAuthConsent;
		};
	};

	let { data }: Props = $props();
	let currentConsent = $state(untrack(() => data.consent));
	let configureForm = $state<LaunchFormData | CompositeLaunchFormData>();
	let configDialog = $state<ReturnType<typeof CatalogConfigureForm>>();
	let configError = $state('');
	let loadingConfig = $state(false);
	let savingConfig = $state(false);

	const consent = $derived(currentConsent);
	const requiresMCPConfiguration = $derived(consent.mcpConfigRequired);
	const scopes = $derived(consent.scope?.split(' ').filter(Boolean) ?? []);
	const showMCPAuthNotice = $derived(consent.mcpAuthRequired || consent.userHasSecondLevelOAuthed);
	const deprecated = $derived(isDeprecatedMCPServer(consent.mcpServer));
	const hasConfigurableMCPConfiguration = $derived.by(() => {
		if (consent.vmcpInstanceID) {
			return consent.vmcpComponents?.some(hasEditableVMCPConfiguration);
		}
		if (consent.mcpServer) {
			return hasEditableConfiguration(consent.mcpServer);
		}
		if (consent.mcpServerInstance) {
			return (
				(consent.mcpServerInstance.config ?? []).filter(
					(field) => field.usage === 'header' && field.userAllowed
				) ?? []
			).some((header) => !hasSecretBinding(header));
		}
		return false;
	});
	const clientCredentialSourceLabel = $derived(
		clientCredentialSourceLabelFor(consent.clientCredentialSource)
	);

	type DetailRow =
		| { label: string; type: 'text'; value: string; valueClass?: string }
		| { label: string; type: 'link'; value: string }
		| { label: string; type: 'scopes'; values: string[] };

	const details = $derived.by((): DetailRow[] => {
		const rows: DetailRow[] = [
			{
				label: m.auth_consent_application(),
				type: 'text',
				value: consent.clientName,
				valueClass: 'wrap-break-word font-medium'
			}
		];

		if (consent.clientURI) {
			rows.push({
				label: m.auth_consent_application_url(),
				type: 'link',
				value: consent.clientURI
			});
		}

		rows.push({
			label: m.auth_consent_oauth_client(),
			type: 'text',
			value: clientCredentialSourceLabel,
			valueClass: 'wrap-break-word'
		});

		rows.push({
			label: m.auth_consent_redirect_url(),
			type: 'text',
			value: consent.redirectURI,
			valueClass: 'break-all'
		});

		if (scopes.length) {
			rows.push({ label: m.auth_consent_scopes(), type: 'scopes', values: scopes });
		}

		if (consent.mcpAuthRequired || consent.userHasSecondLevelOAuthed) {
			rows.push({
				label: m.auth_consent_mcp_server(),
				type: 'text',
				value: consent.mcpServerName ?? '',
				valueClass: 'wrap-break-word'
			});
			rows.push({
				label: m.auth_consent_third_party_oauth(),
				type: 'text',
				value: consent.userHasSecondLevelOAuthed
					? m.auth_consent_already_authorized()
					: m.auth_consent_authorization_required(),
				valueClass: 'wrap-break-word'
			});

			if (consent.mcpServerURL) {
				rows.push({
					label: m.auth_consent_mcp_url(),
					type: 'text',
					value: consent.mcpServerURL,
					valueClass: 'break-all'
				});
			}

			if (consent.thirdPartyAuthURL) {
				rows.push({
					label: m.auth_consent_oauth_url(),
					type: 'text',
					value: consent.thirdPartyAuthURL,
					valueClass: 'break-all'
				});
			}
		}

		if (consent.policyURI) {
			rows.push({
				label: m.auth_consent_privacy_policy(),
				type: 'link',
				value: consent.policyURI
			});
		}

		if (consent.tosURI) {
			rows.push({ label: m.auth_consent_terms(), type: 'link', value: consent.tosURI });
		}

		return rows;
	});

	onMount(() => {
		if (requiresMCPConfiguration || hasConfigurableMCPConfiguration) {
			void loadMCPConfiguration(currentConsent);
		}
	});

	async function loadMCPConfiguration(nextConsent: OAuthConsent) {
		loadingConfig = true;
		configError = '';
		try {
			let values: Record<string, string> = {};
			if (nextConsent.vmcpInstanceID && nextConsent.vmcpComponents) {
				const configuration = await UserService.revealVMCPInstance(nextConsent.vmcpInstanceID, {
					dontLogErrors: true
				});
				configureForm = {
					componentConfigs: Object.fromEntries(
						nextConsent.vmcpComponents.map((component) => {
							const componentID = component.id ?? component.mcpServerCatalogEntryID;
							const editableKeys = new Set(
								component.configuration
									?.filter((field) => field.policy === 'userAllowed')
									.map((field) => field.key)
							);
							const manifestConfiguration = getManifestConfiguration(
								component.catalogEntry.manifest
							);
							return [
								componentID,
								{
									name: component.name,
									envs: manifestConfiguration.env
										.filter(
											(field) =>
												editableKeys.has(field.key) &&
												!field.value &&
												!field.static &&
												!hasSecretBinding(field)
										)
										.map((field) => ({
											...field,
											value: configuration.components[componentID]?.[field.key] ?? '',
											isStatic: false
										})),
									headers: (component.catalogEntry.manifest.config ?? [])
										.filter(
											(field) =>
												field.usage === 'header' &&
												editableKeys.has(field.key) &&
												!field.value &&
												!field.static &&
												!hasSecretBinding(field)
										)
										.map(({ usage: _usage, ...field }) => ({
											...field,
											value: configuration.components[componentID]?.[field.key] ?? '',
											isStatic: false
										}))
								}
							];
						})
					)
				};
			} else if (nextConsent.mcpServerInstance?.id) {
				values = await revealExistingConfiguration(() =>
					UserService.revealMcpServerInstance(nextConsent.mcpServerInstance!.id, {
						dontLogErrors: true
					})
				);
				configureForm = {
					headers: (nextConsent.mcpServerInstance.config ?? [])
						.filter((field) => field.usage === 'header' && field.userAllowed)
						?.map((header) => ({
							...header,
							value: values[header.key] ?? '',
							isStatic: false
						}))
				};
			} else if (nextConsent.mcpServer?.id) {
				values = await revealExistingConfiguration(() =>
					UserService.revealSingleOrRemoteMcpServer(nextConsent.mcpServer!.id, {
						dontLogErrors: true
					})
				);
				configureForm = {
					envs: getManifestConfiguration(nextConsent.mcpServer.manifest).env?.map((env) => ({
						...env,
						value: values[env.key] ?? '',
						isStatic: Boolean(env.value || env.static)
					})),
					headers: getManifestConfiguration(nextConsent.mcpServer.manifest).headers?.map(
						(header) => ({
							...header,
							value: values[header.key] ?? '',
							isStatic: Boolean(header.value || header.static)
						})
					),
					url: nextConsent.mcpServer.manifest.remoteConfig?.url,
					hostname: nextConsent.mcpServer.manifest.remoteConfig?.hostname
				};
			}
		} catch (_err) {
			configureForm = undefined;
			configError = m.auth_consent_load_config_failed();
		} finally {
			loadingConfig = false;
		}
	}

	async function openMCPConfiguration() {
		if (!configureForm) {
			await loadMCPConfiguration(consent);
		}
		if (!configureForm) return;
		await tick();
		configDialog?.open();
	}

	async function revealExistingConfiguration(
		reveal: () => Promise<Record<string, string>>
	): Promise<Record<string, string>> {
		try {
			return await reveal();
		} catch (err) {
			if (err instanceof HttpError && err.statusCode === 404) {
				return {};
			}
			throw err;
		}
	}

	function hasEditableVMCPConfiguration(
		component: NonNullable<OAuthConsent['vmcpComponents']>[number]
	) {
		const editableKeys = new Set(
			component.configuration
				?.filter((field) => field.policy === 'userAllowed')
				.map((field) => field.key)
		);
		const manifest = component.catalogEntry.manifest;
		return [
			...getManifestConfiguration(manifest).env,
			...(manifest.config ?? []).filter((field) => field.usage === 'header')
		].some(
			(field) =>
				editableKeys.has(field.key) && !field.value && !field.static && !hasSecretBinding(field)
		);
	}

	async function saveMCPConfiguration() {
		if (!configureForm) return;

		configError = '';
		savingConfig = true;
		try {
			if (consent.vmcpInstanceID && 'componentConfigs' in configureForm) {
				await UserService.configureVMCPInstance(consent.vmcpInstanceID, {
					components: Object.fromEntries(
						Object.entries(configureForm.componentConfigs).map(([componentID, component]) => [
							componentID,
							convertEnvHeadersToRecord(component.envs, component.headers)
						])
					)
				});
			} else if (consent.mcpServerInstance?.id && !('componentConfigs' in configureForm)) {
				const payload = convertEnvHeadersToRecord(undefined, configureForm.headers);
				await UserService.configureMcpServerInstance(consent.mcpServerInstance.id, payload);
			} else if (consent.mcpServer?.id && !('componentConfigs' in configureForm)) {
				const payload = convertEnvHeadersToRecord(configureForm.envs, configureForm.headers);
				if (configureForm.hostname && configureForm.url) {
					payload.__url = configureForm.url.trim();
				}
				await UserService.configureSingleOrRemoteMcpServer(consent.mcpServer.id, payload);
			} else {
				throw new Error('Missing MCP server configuration target');
			}
			const nextConsent = await UserService.getOAuthConsent(consent.authRequestID);
			currentConsent = nextConsent;
			if (nextConsent.mcpConfigRequired) {
				await loadMCPConfiguration(nextConsent);
				configError = m.auth_consent_config_still_missing();
			} else {
				configureForm = undefined;
				configDialog?.close();
			}
		} catch (err) {
			configError = err instanceof Error ? err.message : m.auth_consent_save_config_failed();
		} finally {
			savingConfig = false;
		}
	}

	function clientCredentialSourceLabelFor(source: OAuthConsent['clientCredentialSource']) {
		switch (source) {
			case 'client_id_metadata_document':
				return m.auth_consent_source_cimd();
			case 'static_client_credentials':
				return m.auth_consent_source_static();
			case 'dynamic_client':
				return m.auth_consent_source_dynamic();
			default:
				return m.core_unknown();
		}
	}
</script>

<svelte:head>
	<title>{m.auth_consent_page_title()}</title>
</svelte:head>

<div class="bg-base-200 dark:bg-base-100 flex min-h-screen items-center justify-center p-4">
	<main class="paper w-full max-w-lg overflow-hidden p-0">
		<BetaLogo class="self-center mt-6" />
		<h1 class="text-xl font-semibold text-center px-4">
			{requiresMCPConfiguration
				? m.auth_consent_configure_named({
						name: consent.mcpServerName || m.auth_consent_mcp_server()
					})
				: m.auth_consent_authorize_named({ name: consent.clientName })}
		</h1>

		{#if requiresMCPConfiguration}
			<section class="flex flex-col gap-5 p-4 py-0">
				<McpDeprecatedNotice {deprecated} variant="notification" />

				<div class="notification-info flex items-center gap-3 p-3">
					<SettingsIcon class="size-5 shrink-0" />
					<p class="min-w-0 text-sm">
						<b class="font-semibold"
							>{consent.mcpServerName || m.auth_consent_this_mcp_server_capitalized()}</b
						>{m.auth_consent_needs_config_suffix()}
					</p>
				</div>

				{#if configError}
					<p class="text-error text-sm mt-4">{configError}</p>
				{/if}
			</section>
		{:else}
			<section class="flex flex-col gap-5 p-4 pt-0">
				<McpDeprecatedNotice {deprecated} variant="notification" />

				{#if showMCPAuthNotice}
					<div class="notification-info flex items-center gap-3 p-3">
						<ShieldAlertIcon class="size-5 shrink-0" />
						<p class="min-w-0 text-sm">
							{#if consent.mcpAuthRequired}
								<b class="font-semibold"
									>{consent.mcpServerName || m.auth_consent_this_mcp_server_capitalized()}</b
								>{m.auth_consent_requires_oauth_redirect_suffix()}
							{:else if consent.userHasSecondLevelOAuthed}
								<b class="font-semibold"
									>{consent.mcpServerName || m.auth_consent_this_mcp_server_capitalized()}</b
								>{m.auth_consent_requires_oauth_authorized_suffix()}
							{/if}
						</p>
					</div>
				{/if}

				{#if hasConfigurableMCPConfiguration}
					<div
						class="border-base-300 bg-base-100 dark:bg-base-200 flex items-center gap-3 rounded-md border p-3"
					>
						<SettingsIcon class="text-muted-content size-4 shrink-0" />
						<div
							class="flex min-w-0 flex-1 flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"
						>
							<p class="text-muted-content min-w-0 text-xs">
								{m.auth_consent_update_config_prefix()}<b class="font-semibold"
									>{consent.mcpServerName || m.auth_consent_this_mcp_server()}</b
								>{m.auth_consent_update_config_suffix()}
							</p>
							<button
								class="btn btn-text btn-sm flex shrink-0 items-center gap-2"
								type="button"
								onclick={openMCPConfiguration}
								disabled={loadingConfig || savingConfig}
							>
								<SettingsIcon class="size-3.5" />
								{loadingConfig ? m.auth_loading() : m.auth_configure()}
							</button>
						</div>
					</div>
				{/if}

				{#if configError}
					<p class="text-error text-sm">{configError}</p>
				{/if}

				<p class="text-sm">
					{m.auth_consent_wants_to_authenticate({ client: consent.clientName })}
				</p>

				<div>
					<details
						class="collapse collapse-arrow border border-base-300"
						name="more-details-content"
					>
						<summary class="collapse-title text-muted-content text-xs font-medium"
							>{m.auth_consent_see_details()}</summary
						>

						<div class="collapse-content space-y-3 overflow-y-auto default-scrollbar-thin max-h-64">
							{#each details as detail (detail.label)}
								<div class="grid grid-cols-[9rem_minmax(0,1fr)] gap-3 text-xs max-sm:grid-cols-1">
									<div class="text-muted-content font-medium">{detail.label}</div>

									{#if detail.type === 'text'}
										<div class="min-w-0 {detail.valueClass ?? ''}">{detail.value}</div>
									{:else if detail.type === 'link'}
										<a
											class="link flex min-w-0 items-center gap-1 break-all"
											href={detail.value}
											rel="external noreferrer noopener"
										>
											<span class="truncate break-all">{detail.value}</span>
											<ExternalLink class="size-3 shrink-0" />
										</a>
									{:else}
										<div class="flex min-w-0 flex-wrap gap-2">
											{#each detail.values as scope, i (i)}
												<span class="badge badge-secondary badge-xs">{scope}</span>
											{/each}
										</div>
									{/if}
								</div>
							{/each}
						</div>
					</details>
				</div>
			</section>
		{/if}

		<footer
			class={twMerge(
				'border-base-300 bg-base-100 dark:bg-base-200 flex justify-end gap-3 border-t p-3 max-sm:flex-col-reverse',
				requiresMCPConfiguration && 'border-t-0'
			)}
		>
			<form method="POST" action={resolve(consent.cancelURL as `/${string}`)}>
				<button class="btn btn-text w-full" type="submit" disabled={savingConfig}
					>{m.common_cancel()}</button
				>
			</form>
			{#if requiresMCPConfiguration}
				<button
					class="btn btn-primary flex w-full items-center gap-2"
					type="button"
					onclick={openMCPConfiguration}
					disabled={loadingConfig || savingConfig}
				>
					<SettingsIcon class="size-4" />
					{loadingConfig ? m.auth_loading() : m.auth_configure()}
				</button>
			{:else}
				<form method="POST" action={resolve(consent.continueURL as `/${string}`)}>
					<button
						class="btn btn-primary w-full"
						type="submit"
						disabled={loadingConfig || savingConfig}
					>
						{m.core_continue()}
					</button>
				</form>
			{/if}
		</footer>
	</main>
</div>

<CatalogConfigureForm
	bind:this={configDialog}
	bind:form={configureForm}
	name={consent.mcpServerName || m.auth_consent_mcp_server()}
	onSave={saveMCPConfiguration}
	onCancel={() => configDialog?.close()}
	loading={savingConfig}
	error={configError}
	{deprecated}
	showComponentToggle={false}
	cancelText={m.core_close()}
	submitText={m.core_save()}
	configurationTitle={m.auth_consent_mcp_server_configuration()}
	disableOutsideClick
/>
