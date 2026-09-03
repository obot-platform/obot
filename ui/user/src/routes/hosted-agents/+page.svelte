<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import TabLayout, { type TabView } from '$lib/components/TabLayout.svelte';
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
			? 'Create Hosted Agent Access Policy'
			: 'Create Agent Template'
	);

	let harnessesView = $state<ReturnType<typeof HarnessesView>>();
	let configSourcesView = $state<ReturnType<typeof ConfigSourcesView>>();

	let views = $derived.by(() => {
		const items: TabView[] = [{ label: 'Agents', value: 'agents', content: agents }];
		if (hasAdminAccess) {
			items.push(
				{ label: 'Templates', value: 'templates', content: templates },
				{ label: 'Harnesses', value: 'harnesses', content: harnesses },
				{ label: 'Pools', value: 'pools', content: pools },
				{ label: 'Config Sources', value: 'config-sources', content: configSources },
				{ label: 'Access Policies', value: 'access-policies', content: accessPolicy }
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
	<title>Obot | {creating ? createTitle : 'Hosted Agents'}</title>
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
		title="Hosted Agents"
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
			<Plus class="size-4" /> Add Template
		</button>
	{:else if !isAdminReadonly && view === 'harnesses'}
		<button
			class="btn btn-primary flex items-center gap-1 text-sm"
			onclick={() => harnessesView?.openCreate()}
		>
			<Plus class="size-4" /> Add Harness
		</button>
	{:else if !isAdminReadonly && view === 'config-sources'}
		<button
			class="btn btn-primary flex items-center gap-1 text-sm"
			onclick={() => configSourcesView?.openCreate()}
		>
			<Plus class="size-4" /> Add Config Source
		</button>
	{:else if !isAdminReadonly && view === 'access-policies'}
		<button
			class="btn btn-primary flex items-center gap-1 text-sm"
			onclick={() => showCreate(view)}
		>
			<Plus class="size-4" /> Add Access Policy
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
