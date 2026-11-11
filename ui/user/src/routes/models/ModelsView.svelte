<script lang="ts">
	import { afterNavigate } from '$app/navigation';
	import { resolve } from '$app/paths';
	import CopyField from '$lib/components/CopyField.svelte';
	import ApiKeyRevealNotice from '$lib/components/agent-auth-scope/ApiKeyRevealNotice.svelte';
	import GatewayConnect from '$lib/components/llm-gateway/GatewayConnect.svelte';
	import GatewayModelsDialog, {
		gatewayModelNames
	} from '$lib/components/llm-gateway/GatewayModelsDialog.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { ApiKeysService, type APIKey } from '$lib/services';
	import { doGet } from '$lib/services/http';
	import { profile } from '$lib/stores';
	import accessibleModels from '$lib/stores/accessibleModels.svelte';
	import { goto } from '$lib/url';
	import { Cpu, Plus } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { fade, slide } from 'svelte/transition';

	interface Props {
		apiKeys?: APIKey[];
	}

	let { apiKeys = [] }: Props = $props();

	let obotURL = $state('');
	// The store only holds the LLM gateway's models.
	let gatewayModels = $derived(accessibleModels.current);
	let subscriptionPassthrough = $state(false);

	onMount(() => {
		obotURL = window.location.origin;
		// Without it, the setup instructions just leave out the Claude subscription option.
		(
			doGet('/llm-gateway/settings', { dontLogErrors: true }) as Promise<{
				subscriptionPassthrough: boolean;
			}>
		)
			.then((s) => (subscriptionPassthrough = s.subscriptionPassthrough))
			.catch(() => {});
	});

	afterNavigate(({ type }) => {
		if (type !== 'enter') {
			void accessibleModels.refresh();
		}
	});

	let ready = $derived(obotURL !== '');
	let modelCount = $derived(gatewayModelNames(gatewayModels).length);
	let modelsDialog = $state<ReturnType<typeof GatewayModelsDialog>>();
	// Only administrators can add the providers the gateway serves models from.
	let canAddProvider = $derived(profile.current.isAdmin?.() ?? false);
	let generatedToken = $state('');
	let creatingToken = $state(false);

	// A usable gateway key has LLM scope and has not expired. No expiration date means it does not expire.
	let hasValidIdentityApiKeys = $derived(
		apiKeys.some((key) => {
			if (!key.canAccessLLMProxy) return false;
			if (!key.expiresAt) return true;
			const expires = new Date(key.expiresAt).getTime();
			return !Number.isNaN(expires) && expires > Date.now();
		})
	);

	async function generateToken() {
		if (creatingToken) return;
		creatingToken = true;
		try {
			const response = await ApiKeysService.createApiKey({
				name: m.models_gateway_access_token_title(),
				mcpServerIds: [],
				canAccessLLMProxy: true
			});
			generatedToken = response.key;
		} catch {
			// The HTTP client records the error.
		} finally {
			creatingToken = false;
		}
	}

	const duration = PAGE_TRANSITION_DURATION;
</script>

<div class="flex h-full w-full flex-col gap-6" in:fade={{ duration }}>
	{#if ready}
		{#if gatewayModels.length > 0}
			<details
				class="collapse collapse-arrow w-full border border-transparent bg-base-100 shadow-sm dark:border-base-400 dark:bg-base-200"
				open
			>
				<summary class="collapse-title">
					<div class="flex items-center gap-3">
						<img src="/admin/assets/llm_gateway_icon.svg" alt="" class="size-8 rounded-md" />
						<div class="flex flex-col">
							<h3 class="text-base font-semibold">{m.models_page_title()}</h3>
							<button
								type="button"
								class="text-link self-start text-xs"
								onclick={(event) => {
									event.stopPropagation();
									modelsDialog?.open();
								}}
							>
								{modelCount === 1
									? m.models_gateway_models_available_one({ count: modelCount })
									: m.models_gateway_models_available_other({ count: modelCount })}
							</button>
						</div>
					</div>
				</summary>
				<div class="collapse-content flex flex-col gap-3">
					<!-- OpenAI clients append /chat/completions or /responses to the base URL, and
				Anthropic clients /v1/messages, so each needs its own. -->
					{#each [{ label: m.models_gateway_base_url_openai(), url: `${obotURL}/api/llm-proxy/v1`, id: 'openai' }, { label: m.models_gateway_base_url_anthropic(), url: `${obotURL}/api/llm-proxy`, id: 'anthropic' }] as base (base.url)}
						<div class="flex flex-col gap-0.5">
							<CopyField
								id={base.id}
								value={base.url}
								label={base.label}
								class="dark:bg-base-100"
								classes={{ label: 'text-xs' }}
							/>
						</div>
					{/each}

					<p class="text-xs font-light">
						{m.models_connect_formats_note()}
						{m.models_connect_naming_prefix()}
						<code>provider/model</code>{m.models_connect_naming_middle()}
						<code>claude-sonnet-4-5</code>
						{m.models_connect_naming_suffix()}
					</p>

					<div class="divider my-0"></div>

					<div class="px-3 flex flex-col gap-3">
						<div class="flex flex-col gap-3 @container">
							<div class="flex items-start justify-between gap-4 flex-col @lg:flex-row">
								<div>
									<h4 class="text-sm font-semibold">{m.models_gateway_access_token_title()}</h4>
									<p class="text-xs font-light">
										{m.models_gateway_access_token_description()}
									</p>
								</div>

								<div class="hidden @lg:block">
									{@render generateTokenButton()}
								</div>
							</div>

							{#if hasValidIdentityApiKeys}
								<div class="block @lg:hidden">
									{@render validIdentityApiKeysNotice()}
								</div>
							{/if}

							<div class="block @lg:hidden">
								{@render generateTokenButton()}
							</div>

							{#if generatedToken}
								<div in:slide={{ axis: 'y', duration }}>
									<ApiKeyRevealNotice
										keyValue={generatedToken}
										id="llm-gateway-access-token"
										class="gap-3"
									/>
								</div>
							{:else if hasValidIdentityApiKeys}
								<div class="hidden @lg:block">
									{@render validIdentityApiKeysNotice()}
								</div>
							{/if}
						</div>
					</div>
				</div>
			</details>
			<GatewayConnect
				{obotURL}
				models={gatewayModels}
				{subscriptionPassthrough}
				accessToken={generatedToken}
				{hasValidIdentityApiKeys}
				{creatingToken}
				onGenerateToken={generateToken}
			/>
			<GatewayModelsDialog bind:this={modelsDialog} models={gatewayModels} />
		{:else}
			<div class="mt-12 flex w-md max-w-full flex-col items-center gap-4 self-center text-center">
				<Cpu class="text-muted-content size-24 opacity-25" />
				<div>
					<h4 class="text-muted-content text-lg font-semibold">
						{canAddProvider
							? m.models_gateway_no_models_admin_title()
							: m.models_gateway_no_models_title()}
					</h4>
					<p class="text-muted-content text-sm font-light">
						{canAddProvider
							? m.models_gateway_no_models_admin_description()
							: m.models_gateway_no_models_description()}
					</p>
				</div>
				{#if canAddProvider}
					<button class="btn btn-primary" onclick={() => goto('/models?view=providers')}>
						<Plus class="size-4" />
						{m.models_providers_add_provider()}
					</button>
				{/if}
			</div>
		{/if}
	{/if}
</div>

{#snippet generateTokenButton()}
	<button
		type="button"
		class="text-nowrap btn btn-primary shrink-0 text-xs @lg:w-fit w-full"
		disabled={creatingToken}
		onclick={generateToken}
	>
		{#if creatingToken}
			<Loading class="size-4 text-primary-content" />
		{/if}
		{#if hasValidIdentityApiKeys}
			{m.models_gateway_generate_new_key()}
		{:else}
			{m.models_gateway_generate_token()}
		{/if}
	</button>
{/snippet}

{#snippet validIdentityApiKeysNotice()}
	<div class="notification-info @container">
		<p class="mb-1 text-sm font-medium">{m.models_gateway_existing_keys_title()}</p>
		<p class="text-xs font-light">
			{m.models_gateway_existing_keys_description()}
		</p>
		<p class="mt-1 text-xs font-light @lg:mt-0">
			{m.models_gateway_existing_keys_manage_prefix()}<a
				href={resolve('/identity-access?view=agents')}
				class="text-link">{m.models_gateway_existing_keys_manage_link()}</a
			>{m.models_gateway_existing_keys_manage_suffix()}
		</p>
	</div>
{/snippet}
