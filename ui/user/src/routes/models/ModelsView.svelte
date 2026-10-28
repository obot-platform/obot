<script lang="ts">
	import { afterNavigate } from '$app/navigation';
	import CopyField from '$lib/components/CopyField.svelte';
	import GatewayConnect from '$lib/components/llm-gateway/GatewayConnect.svelte';
	import GatewayModelsDialog, {
		gatewayModelNames
	} from '$lib/components/llm-gateway/GatewayModelsDialog.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { m } from '$lib/i18n';
	import { doGet } from '$lib/services/http';
	import { profile } from '$lib/stores';
	import accessibleModels from '$lib/stores/accessibleModels.svelte';
	import { goto } from '$lib/url';
	import { Cpu, Plus } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';

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

	const duration = PAGE_TRANSITION_DURATION;
</script>

<div class="flex h-full w-full flex-col gap-6" in:fade={{ duration }}>
	{#if ready}
		{#if gatewayModels.length > 0}
			<div class="paper flex flex-col gap-3 p-4">
				<div class="flex items-center gap-3">
					<img src="/admin/assets/llm_gateway_icon.svg" alt="" class="size-8 rounded-md" />
					<div class="flex flex-col">
						<h3 class="text-base font-semibold">{m.models_page_title()}</h3>
						<button
							type="button"
							class="text-link self-start text-xs"
							onclick={() => modelsDialog?.open()}
						>
							{modelCount === 1
								? m.models_gateway_models_available_one({ count: modelCount })
								: m.models_gateway_models_available_other({ count: modelCount })}
						</button>
					</div>
				</div>
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
				<p class="text-muted-content text-xs">
					{m.models_connect_formats_note()}
					{m.models_connect_naming_prefix()}
					<code>provider/model</code>{m.models_connect_naming_middle()}
					<code>claude-sonnet-4-5</code>
					{m.models_connect_naming_suffix()}
				</p>
			</div>
			<GatewayConnect {obotURL} models={gatewayModels} {subscriptionPassthrough} />
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
