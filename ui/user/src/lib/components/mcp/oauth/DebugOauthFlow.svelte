<script lang="ts">
	import type {
		MCPCatalogServer,
		OAuthDebuggerAuthorizationURL,
		OAuthDebuggerRegisterClientResponse
	} from '$lib/services';
	import AdminService from '$lib/services/admin';
	import OAuthMetadataDebug from '../OAuthMetadataDebug.svelte';
	import DebugOauthSection from './DebugOauthSection.svelte';
	import { onMount } from 'svelte';

	interface Props {
		mcpServer: MCPCatalogServer;
	}

	let { mcpServer }: Props = $props();
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

	let expanded = $state<Record<string, boolean>>({
		metadataDiscovery: false,
		clientRegistration: false,
		preparingAuthorization: false,
		authorizationCode: false,
		tokenRequest: false
	});

	let loading = $state<Record<string, boolean>>({
		metadataDiscovery: true,
		clientRegistration: false,
		preparingAuthorization: false,
		authorizationCode: false,
		tokenRequest: false
	});

	let errors = $state<Record<string, string | null>>({
		clientRegistration: null,
		preparingAuthorization: null,
		authorizationCode: null,
		tokenRequest: null
	});

	let results = $state<Record<string, unknown | null>>({
		clientRegistration: null,
		preparingAuthorization: null,
		authorizationCode: null,
		tokenRequest: null
	});

	function fetchClientRegistration() {
		expanded.metadataDiscovery = false;
		loading.clientRegistration = true;
		AdminService.registerMcpServerOAuthDebuggerClient(mcpServer.id)
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
		AdminService.getMCPServerOAuthDebuggerAuthorizationURL(mcpServer.id, {
			state: clientRegistration.state
		})
			.then((result) => {
				results.preparingAuthorization = result;
			})
			.catch((error) => {
				errors.preparingAuthorization = error instanceof Error ? error.message : String(error);
			})
			.finally(() => {
				loading.preparingAuthorization = false;
				expanded.preparingAuthorization = true;
			});
	}

	function fetchTokenRequest(authorizationCode: string) {
		if (!results.clientRegistration) {
			errors.tokenRequest = 'Client registration information is required to request a token.';
			expanded.tokenRequest = true;
			return;
		}

		expanded.authorizationCode = false;
		loading.tokenRequest = true;
		AdminService.exchangeMCPServerOAuthDebuggerToken(mcpServer.id, {
			code: authorizationCode,
			state: (results.clientRegistration as OAuthDebuggerRegisterClientResponse).state as string
		})
			.then((result) => {
				results.tokenRequest = result;
				currentStep = DEBUG_FLOW_STEPS.authenticationComplete;
			})
			.catch((error) => {
				errors.tokenRequest = error instanceof Error ? error.message : String(error);
			})
			.finally(() => {
				results.authorizationCode = 'Authorization code has been exchanged.';
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
				fetchTokenRequest(authorizationCodeInput);
				currentStep = DEBUG_FLOW_STEPS.tokenRequest;
				break;
			default:
				break;
		}
	}

	onMount(() => {
		if (mcpServer.oauthMetadata) {
			results.metadataDiscovery = mcpServer.oauthMetadata;
		} else {
			errors.metadataDiscovery = 'No OAuth metadata was returned by this MCP server.';
		}
		expanded.metadataDiscovery = true;
		loading.metadataDiscovery = false;
	});

	const stepLoading = $derived(Object.values(loading).some(Boolean));
</script>

<div class="flex flex-col gap-2 p-4 md:pt-0">
	<p class="text-on-surface1 text-sm font-light pb-2">
		This is a guided step-by-step process of the OAuth flow. Follow the instructions to complete
		authentication.
	</p>

	<DebugOauthSection
		classes={{ content: 'p-0 pt-0' }}
		bind:open={expanded.metadataDiscovery}
		loading={loading.metadataDiscovery}
		title="Metadata Discovery"
		errors={errors.metadataDiscovery}
		hasResults={Boolean(results.metadataDiscovery)}
	>
		<OAuthMetadataDebug compact metadata={mcpServer.oauthMetadata} />
	</DebugOauthSection>

	<DebugOauthSection
		bind:open={expanded.clientRegistration}
		loading={loading.clientRegistration}
		title="Client Registration"
		errors={errors.clientRegistration}
		hasResults={Boolean(results.clientRegistration)}
	>
		<pre
			class="bg-surface2 p-2 rounded-md overflow-x-auto text-xs my-0 text-on-background">{JSON.stringify(
				results.clientRegistration,
				null,
				2
			)}</pre>
	</DebugOauthSection>

	<DebugOauthSection
		bind:open={expanded.preparingAuthorization}
		loading={loading.preparingAuthorization}
		title="Preparing Authorization"
		errors={errors.preparingAuthorization}
		hasResults={Boolean(results.preparingAuthorization)}
	>
		{#if results.preparingAuthorization}
			{@const authorizationURL = (results.preparingAuthorization as OAuthDebuggerAuthorizationURL)
				.oauthURL}
			<div class="flex flex-col gap-2">
				<pre
					class="bg-surface2 p-2 rounded-md overflow-x-auto text-xs my-0 text-on-background">{JSON.stringify(
						results.preparingAuthorization,
						null,
						2
					)}</pre>

				<p class="text-xs text-on-surface2">
					Click the button below or copy the URL above to your browser to request authorization and
					acquire an authorization code.
				</p>
				<p class="text-xs text-on-surface2">
					Copy & paste the authorization code into the next step below to continue.
				</p>
				<a
					href={authorizationURL}
					target="_blank"
					rel="external"
					class="button-primary text-sm text-center"
					onclick={() => {
						expanded.authorizationCode = true;
						expanded.preparingAuthorization = false;
					}}
				>
					Get Authorization Code
				</a>
			</div>
		{/if}
	</DebugOauthSection>

	<DebugOauthSection
		bind:open={expanded.authorizationCode}
		loading={loading.authorizationCode}
		title="Request & Acquire Authorization Code"
		errors={errors.authorizationCode}
		hasResults={Boolean(results.authorizationCode)}
		showContent={currentStep === DEBUG_FLOW_STEPS.preparingAuthorization}
	>
		{#if results.authorizationCode}
			<pre
				class="bg-surface2 p-2 rounded-md overflow-x-auto text-xs my-0 text-on-background">{JSON.stringify(
					results.authorizationCode,
					null,
					2
				)}</pre>
		{:else}
			<label for="authorization-code" class="text-sm text-on-surface w-full">
				Enter the authorization code here:
				<input
					bind:value={authorizationCodeInput}
					type="text"
					id="authorization-code"
					class="input-text-filled bg-surface1 dark:bg-background mt-0.5"
				/>
			</label>
		{/if}
	</DebugOauthSection>

	<DebugOauthSection
		bind:open={expanded.tokenRequest}
		loading={loading.tokenRequest}
		title="Token Request"
		errors={errors.tokenRequest}
		hasResults={Boolean(results.tokenRequest)}
	>
		<pre
			class="bg-surface2 p-2 rounded-md overflow-x-auto text-xs my-0 text-on-background">{JSON.stringify(
				results.tokenRequest,
				null,
				2
			)}</pre>
	</DebugOauthSection>

	<DebugOauthSection
		bind:open={expanded.tokenRequest}
		loading={loading.tokenRequest}
		title="Authentication Complete"
		errors={errors.tokenRequest}
		hasResults={Boolean(results.tokenRequest)}
	>
		<p class="text-sm">
			Authentication has successfully been completed! You can now close this window and return to
			the MCP server.
		</p>
	</DebugOauthSection>
</div>
<div
	class="sticky bottom-0 left-0 w-full bg-background dark:bg-surface1 p-4 border-t border-surface2 dark:border-surface1"
>
	<button
		class="button-primary text-sm"
		disabled={stepLoading || currentStep === DEBUG_FLOW_STEPS.authenticationComplete}
		onclick={handleNextStep}
	>
		Continue Next Step
	</button>
</div>
