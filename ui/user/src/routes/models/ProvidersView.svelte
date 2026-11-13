<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import ProviderCard from '$lib/components/admin/ProviderCard.svelte';
	import AddProviderDialog from '$lib/components/llm-gateway/AddProviderDialog.svelte';
	import ConfigureProviderDialog from '$lib/components/llm-gateway/ConfigureProviderDialog.svelte';
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
		providerIconClass,
		providerLabel
	} from '$lib/services/llm-gateway/providers';
	import { profile } from '$lib/stores';
	import { adminConfigStore } from '$lib/stores/adminConfig.svelte';
	import { success } from '$lib/stores/success';
	import { replaceState } from '$lib/url';
	import { TriangleAlert } from '@lucide/svelte';
	import { onMount } from 'svelte';

	const FALLBACK_ICON = '/admin/assets/llm_gateway_icon.svg';

	interface Props {
		// Reports whether the gateway is running, so the page can offer actions that need it.
		onReadyChange?: (ready: boolean) => void;
		onModelsChange?: (change?: { providers: number }) => void;
	}

	let { onReadyChange, onModelsChange }: Props = $props();

	let status = $state<GatewayStatus>();
	let providers = $state<Provider[]>([]);
	let keys = $state<Record<string, ProviderKey[]>>({});
	let loading = $state(true);
	let addDialog = $state<ReturnType<typeof AddProviderDialog>>();
	let configureDialog = $state<ReturnType<typeof ConfigureProviderDialog>>();
	let openingProvider = $state<string>();
	let openRequest = 0;
	let removing = $state<string>();
	// With Claude subscriptions allowed, Anthropic works without keys of its own.
	let subscriptionPassthrough = $state(false);

	let readonly = $derived(profile.current.isAdminReadonly?.());
	let keyNamesByProvider = $derived(
		Object.fromEntries(Object.entries(keys).map(([p, ks]) => [p, ks.map((k) => k.name)]))
	);
	let allKeyNames = $derived(Object.values(keyNamesByProvider).flat());

	async function load(options?: { silent?: boolean }) {
		if (!options?.silent) loading = true;
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
			if (!options?.silent) loading = false;
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

	async function openProvider(name: string) {
		const request = ++openRequest;
		openingProvider = name;
		let opened = false;
		try {
			opened = (await configureDialog?.open(name)) ?? false;
		} catch {
			// The request reports its own error.
		}
		if (request !== openRequest) return;
		openingProvider = undefined;
		if (!opened) void load({ silent: true });
	}

	async function openLinkedProvider() {
		const name = page.url.searchParams.get('provider');
		if (!name) return;
		const url = new URL(page.url.toString());
		url.searchParams.delete('provider');
		replaceState(url);
		if (providers.some((p) => p.name === name)) await openProvider(name);
	}

	onMount(async () => {
		await load();
		await openLinkedProvider();
	});
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
						iconClass={providerIconClass(p.custom_provider_config?.base_provider_type ?? p.name)}
						{readonly}
						configureLoading={openingProvider === p.name}
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
						iconClass={providerIconClass(card.id)}
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
		onModelsChange?.({ providers: providers.length });
	}}
	oncancel={() => (removing = undefined)}
/>

<AddProviderDialog
	bind:this={addDialog}
	configured={providers.map((p) => p.name)}
	existingKeyNames={allKeyNames}
	onCreated={async (name) => {
		adminConfigStore.setModelProviderConfigured(true);
		success.add(m.models_providers_added({ name: providerLabel(name) }));
		await openProvider(name);
	}}
/>

<ConfigureProviderDialog
	bind:this={configureDialog}
	{keyNamesByProvider}
	onClosed={() => {
		load({ silent: true });
		onModelsChange?.();
	}}
/>
