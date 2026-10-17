<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import ProviderCard from '$lib/components/admin/ProviderCard.svelte';
	import AddProviderDialog from '$lib/components/llm-gateway/AddProviderDialog.svelte';
	import GatewayStatusBanner from '$lib/components/llm-gateway/GatewayStatusBanner.svelte';
	import { m } from '$lib/i18n';
	import type { BaseProvider } from '$lib/services/admin/types';
	import {
		deleteProvider,
		getSettings,
		getStatus,
		listProviderKeys,
		listProviders,
		type GatewayStatus,
		type Provider,
		type ProviderKey
	} from '$lib/services/llm-gateway/gateway';
	import {
		PROVIDER_CATALOG,
		providerIcon,
		providerLabel
	} from '$lib/services/llm-gateway/providers';
	import { profile } from '$lib/stores';
	import { adminConfigStore } from '$lib/stores/adminConfig.svelte';
	import { success } from '$lib/stores/success';
	import { goto } from '$lib/url';
	import { TriangleAlert } from '@lucide/svelte';
	import { onMount } from 'svelte';

	const FALLBACK_ICON = '/admin/assets/llm_gateway_icon.svg';

	interface Props {
		// Reports whether the gateway is running, so the page can offer actions that need it.
		onReadyChange?: (ready: boolean) => void;
	}

	let { onReadyChange }: Props = $props();

	let status = $state<GatewayStatus>();
	let providers = $state<Provider[]>([]);
	let keys = $state<Record<string, ProviderKey[]>>({});
	let loading = $state(true);
	let addDialog = $state<ReturnType<typeof AddProviderDialog>>();
	let removing = $state<string>();
	// With Claude subscriptions allowed, Anthropic works without keys of its own.
	let subscriptionPassthrough = $state(false);

	let readonly = $derived(profile.current.isAdminReadonly?.());
	let allKeyNames = $derived(Object.values(keys).flatMap((ks) => ks.map((k) => k.name)));

	async function load() {
		loading = true;
		try {
			status = await getStatus();
			onReadyChange?.(!!status.ready);
			if (!status.ready) return;
			providers = (await listProviders()).sort((a, b) => a.name.localeCompare(b.name));
			adminConfigStore.setModelProviderConfigured(providers.length > 0);
			const entries = await Promise.all(
				providers.map(async (p) => [p.name, await listProviderKeys(p.name).catch(() => [])])
			);
			keys = Object.fromEntries(entries);
			if (providers.some((p) => p.name === 'anthropic')) {
				subscriptionPassthrough = (await getSettings().catch(() => undefined))
					?.subscriptionPassthrough
					? true
					: false;
			}
		} finally {
			loading = false;
		}
	}

	export function openAddCustomProvider() {
		addDialog?.open('custom');
	}

	// Every provider the gateway can be configured with, as a card: the configured ones first,
	// custom providers included, then the rest of the catalog in its own order.
	let cards = $derived.by(() => {
		const configured = providers
			.map((p) => ({ card: toCard(p), provider: p as Provider | undefined }))
			.sort((a, b) => a.card.name.localeCompare(b.card.name));
		const names = new Set(providers.map((p) => p.name));
		const available = PROVIDER_CATALOG.filter((entry) => !names.has(entry.id)).map((entry) => ({
			card: {
				id: entry.id,
				name: entry.label,
				icon: entry.icon ?? FALLBACK_ICON,
				configured: false,
				created: '',
				image: '',
				port: 0
			} as BaseProvider,
			provider: undefined as Provider | undefined
		}));
		return [...configured, ...available];
	});

	function toCard(p: Provider): BaseProvider {
		const base = p.custom_provider_config?.base_provider_type;
		return {
			id: p.name,
			name: base ? p.name : providerLabel(p.name),
			icon: providerIcon(base ?? p.name) ?? FALLBACK_ICON,
			configured: true,
			created: '',
			image: '',
			port: 0
		};
	}

	function openProvider(name: string) {
		goto(`/models/providers/${encodeURIComponent(name)}`);
	}

	onMount(load);
</script>

<div class="flex flex-col gap-8">
	<GatewayStatusBanner {status} showNoProviders={false} />

	{#if loading}
		<div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
			{#each Array.from({ length: 8 }) as _, i (i)}
				<Skeleton type="card" class="h-56 w-full" />
			{/each}
		</div>
	{:else if status?.ready}
		{#if providers.length === 0}
			<div class="notification-alert flex flex-col gap-2">
				<div class="flex items-center gap-2">
					<TriangleAlert class="size-6 shrink-0 self-start text-warning" />
					<p class="my-0.5 flex flex-col text-sm font-semibold">
						{m.models_providers_none_configured()}
					</p>
				</div>
				<span class="text-sm font-light">
					{m.models_providers_none_configured_description()}
				</span>
			</div>
		{/if}
		<div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
			{#each cards as { card, provider: p } (card.id)}
				{#if p}
					{@const ks = keys[p.name] ?? []}
					{@const enabled = ks.filter((k) => k.enabled !== false).length}
					<ProviderCard
						provider={card}
						{readonly}
						onConfigure={() => openProvider(p.name)}
						onDeconfigure={() => (removing = p.name)}
					>
						{#snippet configuredActions()}
							<span class="text-muted-content mr-2 flex items-center gap-2 text-xs">
								{#if p.provider_status && p.provider_status !== 'active'}
									<span class="text-warning">{p.provider_status}</span>
								{/if}
								{#if p.custom_provider_config?.is_key_less}
									<span>{m.models_providers_no_keys()}</span>
								{:else if ks.length === 0 && p.name === 'anthropic' && subscriptionPassthrough}
									<span>{m.models_providers_subscriptions_only()}</span>
								{:else if ks.length === 0}
									<span class="text-warning">{m.models_providers_no_keys_add_one()}</span>
								{:else}
									<span>
										{enabled !== ks.length
											? ks.length === 1
												? m.models_providers_key_count_enabled_one({ count: ks.length, enabled })
												: m.models_providers_key_count_enabled_other({ count: ks.length, enabled })
											: ks.length === 1
												? m.models_providers_key_count_one({ count: ks.length })
												: m.models_providers_key_count_other({ count: ks.length })}
									</span>
								{/if}
							</span>
						{/snippet}
					</ProviderCard>
				{:else}
					<ProviderCard
						provider={card}
						{readonly}
						disableConfigure={readonly}
						onConfigure={() => addDialog?.open(card.id)}
					/>
				{/if}
			{/each}
		</div>
	{/if}
</div>

<Confirm
	show={!!removing}
	msg={m.models_providers_remove_confirm({ name: removing ? providerLabel(removing) : '' })}
	onsuccess={async () => {
		if (removing) {
			const name = providerLabel(removing);
			await deleteProvider(removing);
			success.add(m.models_providers_removed({ name }));
		}
		removing = undefined;
		await load();
	}}
	oncancel={() => (removing = undefined)}
/>

<AddProviderDialog
	bind:this={addDialog}
	configured={providers.map((p) => p.name)}
	existingKeyNames={allKeyNames}
	onCreated={(name) => {
		adminConfigStore.setModelProviderConfigured(true);
		success.add(m.models_providers_added({ name: providerLabel(name) }));
		openProvider(name);
	}}
/>
