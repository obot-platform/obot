<script lang="ts">
	import { resolve } from '$app/paths';
	import CatalogConfigureForm, {
		type CompositeLaunchFormData,
		type LaunchFormData
	} from '$lib/components/mcp/CatalogConfigureForm.svelte';
	import McpDeprecatedNotice from '$lib/components/mcp/McpDeprecatedNotice.svelte';
	import BetaLogo from '$lib/components/navbar/BetaLogo.svelte';
	import { HttpError } from '$lib/errors';
	import { UserService, type OAuthConsent } from '$lib/services';
	import {
		convertCompositeInfoToLaunchFormData,
		convertCompositeLaunchFormDataToPayload,
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
	let hasConfiguredComposite = $state(false);

	const consent = $derived(currentConsent);
	const isCompositeMCPServer = $derived(consent.mcpServer?.manifest.runtime === 'composite');
	const requiresMCPConfiguration = $derived(
		consent.mcpConfigRequired || (isCompositeMCPServer && !hasConfiguredComposite)
	);
	const scopes = $derived(consent.scope?.split(' ').filter(Boolean) ?? []);
	const showMCPAuthNotice = $derived(consent.mcpAuthRequired || consent.userHasSecondLevelOAuthed);
	const deprecated = $derived(isDeprecatedMCPServer(consent.mcpServer));
	const hasConfigurableMCPConfiguration = $derived.by(() => {
		if (consent.mcpServer) {
			return hasEditableConfiguration(consent.mcpServer);
		}
		if (consent.mcpServerInstance) {
			return (consent.mcpServerInstance.multiUserConfig?.userDefinedHeaders ?? []).some(
				(header) => !hasSecretBinding(header)
			);
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
				label: 'Application',
				type: 'text',
				value: consent.clientName,
				valueClass: 'wrap-break-word font-medium'
			}
		];

		if (consent.clientURI) {
			rows.push({ label: 'Application URL', type: 'link', value: consent.clientURI });
		}

		rows.push({
			label: 'OAuth client',
			type: 'text',
			value: clientCredentialSourceLabel,
			valueClass: 'wrap-break-word'
		});

		rows.push({
			label: 'Redirect URL',
			type: 'text',
			value: consent.redirectURI,
			valueClass: 'break-all'
		});

		if (scopes.length) {
			rows.push({ label: 'Scopes', type: 'scopes', values: scopes });
		}

		if (consent.mcpAuthRequired || consent.userHasSecondLevelOAuthed) {
			rows.push({
				label: 'MCP server',
				type: 'text',
				value: consent.mcpServerName ?? '',
				valueClass: 'wrap-break-word'
			});
			rows.push({
				label: 'Third-party OAuth',
				type: 'text',
				value: consent.userHasSecondLevelOAuthed ? 'Already authorized' : 'Authorization required',
				valueClass: 'wrap-break-word'
			});

			if (consent.mcpServerURL) {
				rows.push({
					label: 'MCP URL',
					type: 'text',
					value: consent.mcpServerURL,
					valueClass: 'break-all'
				});
			}

			if (consent.thirdPartyAuthURL) {
				rows.push({
					label: 'OAuth URL',
					type: 'text',
					value: consent.thirdPartyAuthURL,
					valueClass: 'break-all'
				});
			}
		}

		if (consent.policyURI) {
			rows.push({ label: 'Privacy policy', type: 'link', value: consent.policyURI });
		}

		if (consent.tosURI) {
			rows.push({ label: 'Terms', type: 'link', value: consent.tosURI });
		}

		return rows;
	});

	onMount(() => {
		if (requiresMCPConfiguration) {
			void loadMCPConfiguration(currentConsent);
		}
	});

	async function loadMCPConfiguration(nextConsent: OAuthConsent) {
		loadingConfig = true;
		configError = '';
		try {
			let values: Record<string, string> = {};
			if (nextConsent.mcpServerInstance?.id) {
				values = await revealExistingConfiguration(() =>
					UserService.revealMcpServerInstance(nextConsent.mcpServerInstance!.id, {
						dontLogErrors: true
					})
				);
				configureForm = {
					headers: nextConsent.mcpServerInstance.multiUserConfig?.userDefinedHeaders?.map(
						(header) => ({
							...header,
							value: values[header.key] ?? '',
							isStatic: false
						})
					)
				};
			} else if (nextConsent.mcpServer?.id) {
				if (nextConsent.mcpServer.manifest.runtime === 'composite') {
					configureForm = await convertCompositeInfoToLaunchFormData(nextConsent.mcpServer);
					return;
				}

				values = await revealExistingConfiguration(() =>
					UserService.revealSingleOrRemoteMcpServer(nextConsent.mcpServer!.id, {
						dontLogErrors: true
					})
				);
				configureForm = {
					envs: nextConsent.mcpServer.manifest.env?.map((env) => ({
						...env,
						value: values[env.key] ?? ''
					})),
					headers: nextConsent.mcpServer.manifest.remoteConfig?.headers?.map((header) => ({
						...header,
						value: values[header.key] ?? '',
						isStatic: Boolean(header.value)
					})),
					url: nextConsent.mcpServer.manifest.remoteConfig?.url,
					hostname: nextConsent.mcpServer.manifest.remoteConfig?.hostname
				};
			}
		} catch (_err) {
			configureForm = undefined;
			configError = 'Failed to load current MCP server configuration.';
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

	async function saveMCPConfiguration() {
		if (!configureForm) return;

		configError = '';
		savingConfig = true;
		try {
			if (consent.mcpServerInstance?.id) {
				if (isCompositeForm(configureForm)) {
					throw new Error('Unexpected composite configuration for MCP server instance');
				}
				const payload = convertEnvHeadersToRecord(undefined, configureForm.headers);
				await UserService.configureMcpServerInstance(consent.mcpServerInstance.id, payload);
			} else if (consent.mcpServer?.id) {
				if (isCompositeForm(configureForm)) {
					const payload = convertCompositeLaunchFormDataToPayload(configureForm);
					await UserService.configureCompositeMcpServer(consent.mcpServer.id, payload);
					hasConfiguredComposite = true;
				} else {
					const payload = convertEnvHeadersToRecord(configureForm.envs, configureForm.headers);
					if (configureForm.hostname && configureForm.url) {
						payload.__url = configureForm.url.trim();
					}
					await UserService.configureSingleOrRemoteMcpServer(consent.mcpServer.id, payload);
				}
			} else {
				throw new Error('Missing MCP server configuration target');
			}
			const nextConsent = await UserService.getOAuthConsent(consent.authRequestID);
			currentConsent = nextConsent;
			if (nextConsent.mcpConfigRequired) {
				await loadMCPConfiguration(nextConsent);
				configError =
					'Configuration was saved, but additional required configuration is still missing.';
			} else {
				configureForm = undefined;
				configDialog?.close();
			}
		} catch (err) {
			configError = err instanceof Error ? err.message : 'Failed to save MCP server configuration.';
		} finally {
			savingConfig = false;
		}
	}

	function isCompositeForm(
		form: LaunchFormData | CompositeLaunchFormData
	): form is CompositeLaunchFormData {
		return 'componentConfigs' in form;
	}

	function clientCredentialSourceLabelFor(source: OAuthConsent['clientCredentialSource']) {
		switch (source) {
			case 'client_id_metadata_document':
				return 'Client ID Metadata Document';
			case 'static_client_credentials':
				return 'Static client credentials';
			case 'dynamic_client':
				return 'Dynamic client';
			default:
				return 'Unknown';
		}
	}
</script>

<svelte:head>
	<title>Authorize OAuth Access</title>
</svelte:head>

<div class="bg-base-200 dark:bg-base-100 flex min-h-screen items-center justify-center p-4">
	<main class="paper w-full max-w-lg overflow-hidden p-0">
		<BetaLogo class="self-center mt-6" />
		<h1 class="text-xl font-semibold text-center px-4">
			{requiresMCPConfiguration
				? `Configure ${consent.mcpServerName || 'MCP server'}`
				: `Authorize ${consent.clientName}`}
		</h1>

		{#if requiresMCPConfiguration}
			<section class="flex flex-col gap-5 p-4 py-0">
				<McpDeprecatedNotice {deprecated} variant="notification" />

				<div class="notification-info flex items-center gap-3 p-3">
					<SettingsIcon class="size-5 shrink-0" />
					<p class="min-w-0 text-sm">
						{#if isCompositeMCPServer && !hasConfiguredComposite}
							Configure <b class="font-semibold">{consent.mcpServerName || 'this MCP server'}</b> to choose
							which composite servers to use before continuing.
						{:else}
							<b class="font-semibold">{consent.mcpServerName || 'This MCP server'}</b> needs required
							configuration before Obot can finish authorizing this connection.
						{/if}
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
								<b class="font-semibold">{consent.mcpServerName || 'This MCP server'}</b> requires its
								own third-party OAuth authorization. You will be redirected to the third-party OAuth provider
								to complete the authorization.
							{:else if consent.userHasSecondLevelOAuthed}
								<b class="font-semibold">{consent.mcpServerName || 'This MCP server'}</b> requires its
								own third-party OAuth authorization, and you have already authorized it.
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
								You can update the configuration for
								<b class="font-semibold">{consent.mcpServerName || 'this MCP server'}</b>
							</p>
							<button
								class="btn btn-text btn-sm flex shrink-0 items-center gap-2"
								type="button"
								onclick={openMCPConfiguration}
								disabled={loadingConfig || savingConfig}
							>
								<SettingsIcon class="size-3.5" />
								{loadingConfig ? 'Loading...' : 'Configure'}
							</button>
						</div>
					</div>
				{/if}

				{#if configError}
					<p class="text-error text-sm">{configError}</p>
				{/if}

				<p class="text-sm">
					{consent.clientName} wants to authenticate with Obot for an MCP server connection. If you approve,
					Obot will redirect you back to the OAuth client that started this request.
				</p>

				<div>
					<details
						class="collapse collapse-arrow border border-base-300"
						name="more-details-content"
					>
						<summary class="collapse-title text-muted-content text-xs font-medium"
							>See details</summary
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
				<button class="btn btn-text w-full" type="submit" disabled={savingConfig}>Cancel</button>
			</form>
			{#if requiresMCPConfiguration}
				<button
					class="btn btn-primary flex w-full items-center gap-2"
					type="button"
					onclick={openMCPConfiguration}
					disabled={loadingConfig || savingConfig}
				>
					<SettingsIcon class="size-4" />
					{loadingConfig ? 'Loading...' : 'Configure'}
				</button>
			{:else}
				<form method="POST" action={resolve(consent.continueURL as `/${string}`)}>
					<button class="btn btn-primary w-full" type="submit">Continue</button>
				</form>
			{/if}
		</footer>
	</main>
</div>

<CatalogConfigureForm
	bind:this={configDialog}
	bind:form={configureForm}
	name={consent.mcpServerName || 'MCP server'}
	onSave={saveMCPConfiguration}
	onCancel={() => configDialog?.close()}
	loading={savingConfig}
	error={configError}
	{deprecated}
	cancelText="Close"
	submitText="Save"
	configurationTitle="MCP Server Configuration"
	disableOutsideClick
/>
