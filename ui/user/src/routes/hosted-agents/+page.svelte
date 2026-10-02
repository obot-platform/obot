<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import TabLayout, { type TabView } from '$lib/components/TabLayout.svelte';
	import { m } from '$lib/i18n';
	import { profile } from '$lib/stores';
	import { goto } from '$lib/url';
	import AccessPolicyView from './AccessPolicyView.svelte';
	import AgentsView from './AgentsView.svelte';
	import ConfigSourcesView from './ConfigSourcesView.svelte';
	import HarnessesView from './HarnessesView.svelte';
	import PoolsView from './PoolsView.svelte';
	import TemplatesView from './TemplatesView.svelte';
	import { Plus } from '@lucide/svelte';

	let { data } = $props();
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	let hasAdminAccess = $derived(profile.current.hasAdminAccess?.() ?? data.hasAdminAccess);
	let selectedView = $derived(page.url.searchParams.get('view') ?? 'agents');
	let creating = $derived(
		hasAdminAccess &&
			!isAdminReadonly &&
			['access-policies', 'templates'].includes(selectedView) &&
			page.url.searchParams.has('new')
	);
	let createTitle = $derived(
		selectedView === 'access-policies'
			? m.chat_ha_create_access_policy()
			: m.chat_ha_create_template()
	);

	let harnessesView = $state<ReturnType<typeof HarnessesView>>();
	let configSourcesView = $state<ReturnType<typeof ConfigSourcesView>>();

	let views = $derived.by(() => {
		const items: TabView[] = [{ label: m.chat_ha_tab_agents(), value: 'agents', content: agents }];
		if (hasAdminAccess) {
			items.push(
				{ label: m.chat_ha_tab_templates(), value: 'templates', content: templates },
				{ label: m.chat_ha_tab_harnesses(), value: 'harnesses', content: harnesses },
				{ label: m.chat_ha_tab_pools(), value: 'pools', content: pools },
				{ label: m.chat_ha_tab_config_sources(), value: 'config-sources', content: configSources },
				{
					label: m.chat_ha_tab_access_policies(),
					value: 'access-policies',
					content: accessPolicy
				}
			);
		}
		return items;
	});

	function hideCreate() {
		const url = new URL(page.url);
		url.searchParams.delete('new');
		goto(url, { replaceState: true });
	}

	function showCreate(view: string) {
		goto(`${page.url.pathname}?view=${view}&new=true`);
	}
</script>

<svelte:head>
	<title>{m.chat_page_title_named({ name: creating ? createTitle : m.nav_hosted_agents() })}</title>
</svelte:head>

{#if creating}
	<Layout title={createTitle} showBackButton onBackButtonClick={hideCreate}>
		{#if selectedView === 'access-policies'}
			<AccessPolicyView hostedAgentAccessPolicies={data.hostedAgentAccessPolicies} creating />
		{:else}
			<TemplatesView templates={data.templates} harnesses={data.harnesses} creating />
		{/if}
	</Layout>
{:else}
	<TabLayout
		title={m.nav_hosted_agents()}
		defaultView="agents"
		rightNavActions={navActions}
		{views}
		classes={{ childrenContainer: 'max-w-none' }}
	/>
{/if}

{#snippet navActions(view: string)}
	{#if !isAdminReadonly && view === 'templates'}
		<button
			class="btn btn-primary flex items-center gap-1 text-sm"
			onclick={() => showCreate(view)}
		>
			<Plus class="size-4" />
			{m.chat_ha_add_template()}
		</button>
	{:else if !isAdminReadonly && view === 'harnesses'}
		<button
			class="btn btn-primary flex items-center gap-1 text-sm"
			onclick={() => harnessesView?.openCreate()}
		>
			<Plus class="size-4" />
			{m.chat_ha_add_harness()}
		</button>
	{:else if !isAdminReadonly && view === 'config-sources'}
		<button
			class="btn btn-primary flex items-center gap-1 text-sm"
			onclick={() => configSourcesView?.openCreate()}
		>
			<Plus class="size-4" />
			{m.chat_ha_add_config_source()}
		</button>
	{:else if !isAdminReadonly && view === 'access-policies'}
		<button
			class="btn btn-primary flex items-center gap-1 text-sm"
			onclick={() => showCreate(view)}
		>
			<Plus class="size-4" />
			{m.chat_ha_add_access_policy()}
		</button>
	{/if}
{/snippet}

{#snippet agents()}
	<AgentsView hostedAgents={data.hostedAgents} instances={data.instances} pools={data.pools} />
{/snippet}

{#snippet templates()}
	<TemplatesView templates={data.templates} harnesses={data.harnesses} />
{/snippet}

{#snippet harnesses()}
	<HarnessesView bind:this={harnessesView} harnesses={data.harnesses} />
{/snippet}

{#snippet pools()}
	<PoolsView
		pools={data.adminPools}
		assignments={data.adminAssignments}
		poolDefaults={data.poolDefaults}
	/>
{/snippet}

{#snippet configSources()}
	<ConfigSourcesView bind:this={configSourcesView} agentCatalogs={data.agentCatalogs} />
{/snippet}

{#snippet accessPolicy()}
	<AccessPolicyView hostedAgentAccessPolicies={data.hostedAgentAccessPolicies} />
{/snippet}
