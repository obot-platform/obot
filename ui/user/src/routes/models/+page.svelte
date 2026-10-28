<script lang="ts">
	import { page } from '$app/state';
	import TabLayout, { type TabView } from '$lib/components/TabLayout.svelte';
	import DefaultModels from '$lib/components/admin/DefaultModels.svelte';
	import GuardrailsList from '$lib/components/guardrails/GuardrailsList.svelte';
	import { getAdminModels, initModels } from '$lib/context/admin/models.svelte.js';
	import { m } from '$lib/i18n';
	import { AdminService } from '$lib/services';
	import { profile } from '$lib/stores';
	import { goto } from '$lib/url';
	import GatewaySettingsView from './GatewaySettingsView.svelte';
	import ModelsView from './ModelsView.svelte';
	import ProfilesView from './ProfilesView.svelte';
	import ProvidersView from './ProvidersView.svelte';
	import RoutingView from './RoutingView.svelte';
	import { Plus } from '@lucide/svelte';
	import { untrack } from 'svelte';

	let { data } = $props();
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	let hasAdminAccess = $derived(profile.current.hasAdminAccess?.() ?? data.hasAdminAccess);

	initModels([]);
	const adminModels = getAdminModels();
	let providersView = $state<ReturnType<typeof ProvidersView>>();
	let routingView = $state<ReturnType<typeof RoutingView>>();
	let guardrailsList = $state<ReturnType<typeof GuardrailsList>>();
	let gatewayReady = $state(false);

	const titles: Record<string, string> = {
		connect: m.models_page_title(),
		providers: m.models_providers_tab(),
		profiles: m.models_profiles_page_title(),
		guardrails: m.guardrails_page_title(),
		routing: m.models_routing_page_title(),
		settings: m.models_settings_page_title()
	};

	let views = $derived.by(() => {
		const items: TabView[] = [
			{
				label: m.models_connect_tab(),
				value: 'connect',
				content: connect,
				tooltip: m.models_connect_tab_tooltip()
			}
		];
		if (hasAdminAccess) {
			items.push(
				{
					label: m.models_tab_providers(),
					value: 'providers',
					content: providers,
					tooltip: m.models_providers_tab_tooltip()
				},
				{
					label: m.models_profiles_tab(),
					value: 'profiles',
					content: profiles,
					tooltip: m.models_profiles_tab_tooltip()
				},
				{
					label: m.guardrails_tab(),
					value: 'guardrails',
					content: guardrails,
					tooltip: m.models_guardrails_tab_tooltip()
				},
				{
					label: m.models_tab_routing(),
					value: 'routing',
					content: routing,
					tooltip: m.models_routing_tab_tooltip()
				},
				{
					label: m.models_tab_settings(),
					value: 'settings',
					content: settings,
					tooltip: m.models_settings_tab_tooltip()
				}
			);
		}
		return items;
	});

	let selectedView = $derived.by(() => {
		const requested = page.url.searchParams.get('view') ?? 'connect';
		return views.some((v) => v.value === requested) ? requested : 'connect';
	});

	let title = $derived(titles[selectedView] ?? m.models_page_title());

	// The default models dialog on the Providers tab chooses from every model the gateway has.
	$effect(() => {
		if (selectedView === 'providers' && hasAdminAccess) {
			untrack(loadAdminModels);
		}
	});

	async function loadAdminModels() {
		try {
			adminModels.items = await AdminService.listModels({ all: true });
		} catch {
			// The dialog stays unavailable; the Providers tab reports gateway errors itself.
		}
	}
</script>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>

<TabLayout
	title={m.models_page_title()}
	defaultView="connect"
	rightNavActions={navActions}
	{views}
	classes={{ childrenContainer: 'max-w-none' }}
/>

{#snippet navActions(view: string)}
	{#if view === 'providers'}
		<div class="flex items-center gap-2">
			<DefaultModels availableModels={adminModels.items} readonly={isAdminReadonly} />
			{#if !isAdminReadonly && gatewayReady}
				<button
					class="btn btn-primary flex items-center gap-1 text-sm"
					onclick={() => providersView?.openAddCustomProvider()}
				>
					<Plus class="size-4" />
					{m.models_providers_add_custom()}
				</button>
			{/if}
		</div>
	{:else if !isAdminReadonly && gatewayReady && view === 'routing'}
		<button
			class="btn btn-primary flex items-center gap-1 text-sm"
			onclick={() => routingView?.open()}
		>
			<Plus class="size-4" />
			{m.models_routing_add_rule()}
		</button>
	{:else if view === 'profiles' && !isAdminReadonly}
		<button
			class="btn btn-primary flex items-center gap-1 text-sm"
			onclick={() => goto('/models/profiles/new')}
		>
			<Plus class="size-4" />
			{m.models_profiles_add()}
		</button>
	{:else if view === 'guardrails' && !isAdminReadonly}
		<button
			class="btn btn-primary flex items-center gap-1 text-sm"
			onclick={() => guardrailsList?.openTypePicker()}
		>
			<Plus class="size-4" />
			{m.guardrails_add()}
		</button>
	{/if}
{/snippet}

{#snippet connect()}
	<ModelsView />
{/snippet}

{#snippet providers()}
	<ProvidersView bind:this={providersView} onReadyChange={(ready) => (gatewayReady = ready)} />
{/snippet}

{#snippet routing()}
	<RoutingView bind:this={routingView} onReadyChange={(ready) => (gatewayReady = ready)} />
{/snippet}

{#snippet profiles()}
	<ProfilesView />
{/snippet}

{#snippet guardrails()}
	<GuardrailsList bind:this={guardrailsList} />
{/snippet}

{#snippet settings()}
	<GatewaySettingsView />
{/snippet}
