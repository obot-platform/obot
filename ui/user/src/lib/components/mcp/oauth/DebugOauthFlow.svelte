<script lang="ts">
	import { m } from '$lib/i18n';
	import type {
		MCPCatalogServer,
		OAuthDebuggerAuthorizationURL,
		OAuthDebuggerRegisterClientResponse
	} from '$lib/services';
	import AdminService from '$lib/services/admin';
	import { profile } from '$lib/stores';
	import OAuthMetadataDebug from '../OAuthMetadataDebug.svelte';
	import DebugOauthSection from './DebugOauthSection.svelte';
	import { onMount } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		mcpServer: MCPCatalogServer;
		classes?: {
			footer?: string;
		};
	}

	let { mcpServer, classes }: Props = $props();
	const DEBUG_FLOW_STEPS = {
		metadataDiscovery: 'metadataDiscovery',
		clientRegistration: 'clientRegistration',
		preparingAuthorization: 'preparingAuthorization',
		authorizationCode: 'authorizationCode',
		tokenRequest: 'tokenRequest',
		authenticationComplete: 'authenticationComplete'
	};

	let currentStep = $state(DEBUG_FLOW_STEPS.metadataDiscovery);
	let authorizationCodeInput = $state<string>('');
	let showRequired = $state(false);
	let isAdminReadonly = $derived(profile.current?.isAdminReadonly?.());

	function resetExpanded() {
		return {
			metadataDiscovery: false,
			clientRegistration: false,
			preparingAuthorization: false,
			authorizationCode: false,
			tokenRequest: false
		};
	}

	function resetLoading() {
		return {
			metadataDiscovery: true,
			clientRegistration: false,
			preparingAuthorization: false,
			authorizationCode: false,
			tokenRequest: false
		};
	}

	function resetErrors() {
		return {
			clientRegistration: null,
			preparingAuthorization: null,
			authorizationCode: null,
			tokenRequest: null
		};
	}

	function resetResults() {
		return {
			clientRegistration: null,
			preparingAuthorization: null,
			authorizationCode: null,
			tokenRequest: null
		};
	}
	let expanded = $state<Record<string, boolean>>(resetExpanded());

	let loading = $state<Record<string, boolean>>(resetLoading());

	let errors = $state<Record<string, string | null>>(resetErrors());

	let results = $state<Record<string, unknown | null>>(resetResults());

	function fetchClientRegistration() {
		expanded.metadataDiscovery = false;
		loading.clientRegistration = true;
		AdminService.registerMcpServerOAuthDebuggerClient(mcpServer, { dontLogErrors: true })
			.then((result) => {
				results.clientRegistration = result;
			})
			.catch((error) => {
				errors.clientRegistration = error instanceof Error ? error.message : String(error);
			})
			.finally(() => {
				loading.clientRegistration = false;
				expanded.clientRegistration = true;
			});
	}

	function fetchAuthorizationURL(clientRegistration: OAuthDebuggerRegisterClientResponse) {
		expanded.clientRegistration = false;
		loading.preparingAuthorization = true;
		AdminService.getMCPServerOAuthDebuggerAuthorizationURL(
			mcpServer,
			{
				state: clientRegistration.state
			},
			{ dontLogErrors: true }
		)
			.then((result) => {
				results.preparingAuthorization = result;
			})
			.catch((error) => {
				errors.preparingAuthorization = error instanceof Error ? error.message : String(error);
			})
			.finally(() => {
				loading.preparingAuthorization = false;
				expanded.preparingAuthorization = true;
				expanded.authorizationCode = true;
			});
	}

	function fetchTokenRequest(authorizationCode: string) {
		if (!results.clientRegistration) {
			errors.tokenRequest = m.mcp_debug_oauth_client_registration_required();
			expanded.tokenRequest = true;
			return;
		}

		expanded.authorizationCode = false;
		loading.tokenRequest = true;
		AdminService.exchangeMCPServerOAuthDebuggerToken(
			mcpServer,
			{
				code: authorizationCode,
				state: (results.clientRegistration as OAuthDebuggerRegisterClientResponse).state as string
			},
			{ dontLogErrors: true }
		)
			.then((result) => {
				results.tokenRequest = result;
				currentStep = DEBUG_FLOW_STEPS.authenticationComplete;
			})
			.catch((error) => {
				errors.tokenRequest = error instanceof Error ? error.message : String(error);
			})
			.finally(() => {
				results.authorizationCode = m.mcp_debug_oauth_code_exchanged();
				loading.tokenRequest = false;
				expanded.tokenRequest = true;
			});
	}

	function handleNextStep() {
		if (stepLoading) {
			return;
		}

		switch (currentStep) {
			case DEBUG_FLOW_STEPS.metadataDiscovery:
				fetchClientRegistration();
				currentStep = DEBUG_FLOW_STEPS.clientRegistration;
				break;
			case DEBUG_FLOW_STEPS.clientRegistration:
				fetchAuthorizationURL(results.clientRegistration as OAuthDebuggerRegisterClientResponse);
				currentStep = DEBUG_FLOW_STEPS.preparingAuthorization;
				break;
			case DEBUG_FLOW_STEPS.preparingAuthorization:
				if (!authorizationCodeInput) {
					showRequired = true;
					return;
				}
				fetchTokenRequest(authorizationCodeInput);
				currentStep = DEBUG_FLOW_STEPS.tokenRequest;
				break;
			default:
				break;
		}
	}

	function handleRestart() {
		expanded = resetExpanded();
		errors = resetErrors();
		results = resetResults();
		loading = resetLoading();
		currentStep = DEBUG_FLOW_STEPS.metadataDiscovery;
		authorizationCodeInput = '';
		showRequired = false;
		validateMetadata();
	}

	function validateMetadata() {
		if (mcpServer.oauthMetadata) {
			results.metadataDiscovery = mcpServer.oauthMetadata;
		} else {
			errors.metadataDiscovery = m.mcp_oauth_metadata_none_returned();
		}
		expanded.metadataDiscovery = true;
		loading.metadataDiscovery = false;
	}

	onMount(() => {
		validateMetadata();
	});

	const stepLoading = $derived(Object.values(loading).some(Boolean));
	const clientRegistrationTitle = $derived(
		mcpServer.oauthMetadata?.clientIdMetadataDocumentSupported
			? m.mcp_debug_oauth_client_id_metadata_document()
			: m.mcp_oauth_client_registration()
	);
</script>

<div class="flex flex-col gap-2 p-4 md:pt-0">
	<p class="text-muted-content text-sm font-light pb-2">
		{m.mcp_debug_oauth_intro()}
	</p>

	<DebugOauthSection
		classes={{ content: 'p-0 pt-0' }}
		bind:open={expanded.metadataDiscovery}
		loading={loading.metadataDiscovery}
		title={m.mcp_debug_oauth_metadata_discovery()}
		errors={errors.metadataDiscovery}
		hasResults={Boolean(results.metadataDiscovery)}
	>
		<OAuthMetadataDebug compact metadata={mcpServer.oauthMetadata} />
	</DebugOauthSection>

	<DebugOauthSection
		bind:open={expanded.clientRegistration}
		loading={loading.clientRegistration}
		title={clientRegistrationTitle}
		errors={errors.clientRegistration}
		hasResults={Boolean(results.clientRegistration)}
	>
		<pre class="p-2 rounded-md overflow-x-auto text-xs my-0">{JSON.stringify(
				results.clientRegistration,
				null,
				2
			)}</pre>
	</DebugOauthSection>

	<DebugOauthSection
		bind:open={expanded.preparingAuthorization}
		loading={loading.preparingAuthorization}
		title={m.mcp_debug_oauth_preparing_authorization()}
		errors={errors.preparingAuthorization}
		hasResults={Boolean(results.preparingAuthorization)}
	>
		{#if results.preparingAuthorization}
			{@const authorizationURL = (results.preparingAuthorization as OAuthDebuggerAuthorizationURL)
				.oauthURL}
			<div class="flex flex-col gap-2">
				<pre class="p-2 rounded-md overflow-x-auto text-xs my-0">{JSON.stringify(
						results.preparingAuthorization,
						null,
						2
					)}</pre>

				<p class="text-xs text-muted-content">
					{m.mcp_debug_oauth_click_button()}
				</p>
				<p class="text-xs text-muted-content">
					{m.mcp_debug_oauth_paste_code()}
				</p>
				<a
					href={authorizationURL}
					target="_blank"
					rel="external noopener noreferrer"
					class="btn btn-primary text-sm text-center"
					onclick={() => {
						expanded.authorizationCode = true;
					}}
				>
					{m.mcp_debug_oauth_get_code()}
				</a>
			</div>
		{/if}
	</DebugOauthSection>

	<DebugOauthSection
		bind:open={expanded.authorizationCode}
		loading={loading.authorizationCode}
		title={m.mcp_debug_oauth_request_code()}
		errors={errors.authorizationCode}
		hasResults={Boolean(results.authorizationCode)}
		showContent={currentStep === DEBUG_FLOW_STEPS.preparingAuthorization}
	>
		{#if results.authorizationCode}
			<pre class="p-2 rounded-md overflow-x-auto text-xs my-0">{JSON.stringify(
					results.authorizationCode,
					null,
					2
				)}</pre>
		{:else}
			<label for="authorization-code" class="text-sm text-muted-content w-full">
				{m.mcp_debug_oauth_enter_code()}
				<input
					bind:value={authorizationCodeInput}
					type="text"
					id="authorization-code"
					class={twMerge(
						'text-input-filled bg-base-200 dark:bg-base-100 mt-0.5',
						showRequired && 'error'
					)}
					oninput={() => {
						showRequired = false;
					}}
				/>
				{#if showRequired}
					<p class="text-xs text-error my-1">{m.mcp_debug_oauth_code_required()}</p>
				{/if}
			</label>
		{/if}
	</DebugOauthSection>

	<DebugOauthSection
		bind:open={expanded.tokenRequest}
		loading={loading.tokenRequest}
		title={m.mcp_debug_oauth_token_request()}
		errors={errors.tokenRequest}
		hasResults={Boolean(results.tokenRequest)}
	>
		<pre class="p-2 rounded-md overflow-x-auto text-xs my-0">{JSON.stringify(
				results.tokenRequest,
				null,
				2
			)}</pre>
	</DebugOauthSection>

	<DebugOauthSection
		bind:open={expanded.tokenRequest}
		loading={loading.tokenRequest}
		title={m.mcp_debug_oauth_complete()}
		errors={errors.tokenRequest}
		hasResults={Boolean(results.tokenRequest)}
	>
		<p class="text-sm">
			{m.mcp_debug_oauth_complete_description()}
		</p>
	</DebugOauthSection>
</div>
<div
	class={twMerge(
		'w-full bg-base-100 dark:bg-base-200 p-4 border-t border-base-300 dark:border-base-200',
		classes?.footer
	)}
>
	<div class="flex justify-between gap-4">
		<button
			class="btn btn-primary text-sm"
			disabled={stepLoading ||
				currentStep === DEBUG_FLOW_STEPS.authenticationComplete ||
				Object.values(errors).some(Boolean) ||
				isAdminReadonly}
			onclick={handleNextStep}
		>
			{m.mcp_debug_oauth_continue()}
		</button>

		<button class="btn btn-secondary text-sm" onclick={handleRestart} disabled={isAdminReadonly}>
			{m.mcp_debug_oauth_restart()}
		</button>
	</div>
</div>
