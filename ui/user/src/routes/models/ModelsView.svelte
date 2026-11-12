<script lang="ts">
	import { afterNavigate } from '$app/navigation';
	import CopyField from '$lib/components/CopyField.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
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
	import { goto, resolve } from '$lib/url';
	import { ChevronDown, Cpu, Plus } from '@lucide/svelte';
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
	let generatedApiKey = $state('');
	let creatingApiKey = $state(false);

	// A usable gateway key has LLM scope and has not expired. No expiration date means it does not expire.
	let hasValidIdentityApiKeys = $derived(
		apiKeys.some((key) => {
			if (!key.canAccessLLMProxy) return false;
			if (!key.expiresAt) return true;
			const expires = new Date(key.expiresAt).getTime();
			return !Number.isNaN(expires) && expires > Date.now();
		})
	);

	const GATEWAY_KEY_NAME = 'LLM Gateway API Key';

	async function generateApiKey(): Promise<boolean> {
		if (creatingApiKey) return false;
		creatingApiKey = true;
		try {
			const response = await ApiKeysService.createApiKey({
				name: GATEWAY_KEY_NAME,
				mcpServerIds: [],
				canAccessLLMProxy: true
			});
			generatedApiKey = response.key;
			return true;
		} catch {
			// The HTTP client records the error.
			return false;
		} finally {
			creatingApiKey = false;
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

					<div class="flex flex-col gap-3">
						<div class="flex flex-col gap-3 @container">
							<div class="flex items-start justify-between gap-4 flex-col @lg:flex-row">
								<div>
									<h4 class="text-sm font-semibold">{m.models_gateway_api_key_title()}</h4>
									<p class="text-xs font-light">
										{m.models_gateway_api_key_description()}
									</p>
								</div>

								{@render generateApiKeyButton()}
							</div>

							{#if generatedApiKey}
								<div in:slide={{ axis: 'y', duration }}>
									<ApiKeyRevealNotice
										keyValue={generatedApiKey}
										id="llm-gateway-api-key"
										class="gap-3"
									/>
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
				{generatedApiKey}
				{hasValidIdentityApiKeys}
				{creatingApiKey}
				onGenerateApiKey={generateApiKey}
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

{#snippet generateApiKeyButton()}
	{#if hasValidIdentityApiKeys || generatedApiKey}
		<div class="flex">
			<a
				class="btn btn-secondary btn-soft btn-sm text-base-content border-base-400 shrink-0 text-no-wrap @lg:w-fit w-full rounded-r-none!"
				href={resolve('/identity-access?view=agents')}
			>
				{m.models_gateway_manage_api_keys()}
			</a>
			<DotDotDot
				class="btn-block btn-secondary btn-sm text-base-content btn-soft pl-2! pr-2.5! border-base-400 @lg:w-fit w-full rounded-l-none! border-l-0!"
				placement="bottom-end"
				classes={{
					popover: 'w-44.5'
				}}
			>
				{#snippet icon()}
					<ChevronDown class="size-4" />
				{/snippet}
				{#snippet children({ toggle })}
					<button
						class="menu-button text-xs min-h-fit"
						disabled={creatingApiKey}
						onclick={async () => {
							await generateApiKey();
							toggle(false);
						}}
					>
						{#if creatingApiKey}
							<Loading class="size-4" />
						{:else}
							{m.models_gateway_generate_new_key()}
						{/if}
					</button>
				{/snippet}
			</DotDotDot>
		</div>
	{:else}
		<button
			type="button"
			class="text-nowrap btn btn-primary shrink-0 btn-sm @lg:w-fit w-full"
			disabled={creatingApiKey}
			onclick={generateApiKey}
		>
			{#if creatingApiKey}
				<Loading class="size-4 text-primary-content" />
			{:else}
				{m.models_gateway_generate_api_key()}
			{/if}
		</button>
	{/if}
{/snippet}
