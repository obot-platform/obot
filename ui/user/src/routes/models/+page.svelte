<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import TabLayout, { type TabView } from '$lib/components/TabLayout.svelte';
	import DefaultModels from '$lib/components/admin/DefaultModels.svelte';
	import MessagePoliciesView from '$lib/components/admin/MessagePoliciesView.svelte';
	import { getAdminModels, initModels } from '$lib/context/admin/models.svelte.js';
	import { m } from '$lib/i18n';
	import { profile, version } from '$lib/stores';
	import { goto } from '$lib/url';
	import AccessPoliciesView from './AccessPoliciesView.svelte';
	import ModelProvidersView from './ModelProvidersView.svelte';
	import ModelsView from './ModelsView.svelte';
	import { Plus } from '@lucide/svelte';

	let { data } = $props();
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	let hasAdminAccess = $derived(profile.current.hasAdminAccess?.() ?? data.hasAdminAccess);
	let messagePoliciesEnabled = $derived(version.current.messagePoliciesEnabled === true);
	let creatingView = $derived(page.url.searchParams.get('view'));
	let creating = $derived(
		hasAdminAccess &&
			!isAdminReadonly &&
			page.url.searchParams.has('new') &&
			(creatingView === 'access-policies' ||
				(messagePoliciesEnabled && creatingView === 'ai-judge-policies'))
	);

	initModels([]);
	const adminModels = getAdminModels();
	let defaultModelsDialog = $state<ReturnType<typeof DefaultModels>>();

	let views = $derived.by(() => {
		const items: TabView[] = [
			{
				label: m.models_title(),
				value: 'models',
				content: models,
				tooltip: m.models_tab_tooltip()
			}
		];
		if (hasAdminAccess) {
			items.push(
				{
					label: m.models_providers_tab(),
					value: 'model-providers',
					content: modelProviders,
					tooltip: m.models_providers_tab_tooltip()
				},
				{
					label: m.models_access_policies_tab(),
					value: 'access-policies',
					content: accessPolicies,
					tooltip: m.models_access_policies_tab_tooltip()
				}
			);
			if (messagePoliciesEnabled) {
				items.push({
					label: m.models_tab_ai_judge(),
					value: 'ai-judge-policies',
					content: messagePolicies,
					tooltip: m.models_tab_ai_judge_tooltip()
				});
			}
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

	let title = $derived.by(() => {
		if (!creating) return m.models_title();
		return creatingView === 'ai-judge-policies'
			? m.models_create_ai_judge_policy()
			: m.models_create_access_policy();
	});

	function handleFirstConfigure(required: boolean) {
		defaultModelsDialog?.open(required);
	}
</script>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>

{#if creating}
	<Layout {title} showBackButton onBackButtonClick={hideCreate}>
		{#if creatingView === 'ai-judge-policies'}
			<MessagePoliciesView
				messagePolicies={data.messagePolicies ?? []}
				policyDirection="user-message"
				creating
			/>
		{:else}
			<AccessPoliciesView modelAccessPolicies={data.modelAccessPolicies} creating />
		{/if}
	</Layout>
{:else}
	<TabLayout
		title={m.models_title()}
		defaultView="models"
		rightNavActions={navActions}
		{views}
		classes={{ childrenContainer: 'max-w-none' }}
	/>
{/if}

{#snippet navActions(view: string)}
	{#if view === 'model-providers'}
		<DefaultModels
			bind:this={defaultModelsDialog}
			availableModels={adminModels.items}
			readonly={isAdminReadonly}
		/>
	{:else if view === 'access-policies' && !isAdminReadonly}
		<button
			class="btn btn-primary flex items-center gap-1 text-sm"
			onclick={() => showCreate('access-policies')}
		>
			<Plus class="size-4" />
			{m.models_add_access_policy()}
		</button>
	{:else if view === 'ai-judge-policies' && messagePoliciesEnabled && !isAdminReadonly}
		<button
			class="btn btn-primary flex items-center gap-1 text-sm"
			onclick={() => showCreate('ai-judge-policies')}
		>
			<Plus class="size-4" />
			{m.models_add_ai_judge_policy()}
		</button>
	{/if}
{/snippet}

{#snippet models()}
	<ModelsView />
{/snippet}

{#snippet modelProviders()}
	<ModelProvidersView
		modelProviders={data.modelProviders}
		onFirstConfigure={handleFirstConfigure}
	/>
{/snippet}

{#snippet accessPolicies()}
	<AccessPoliciesView modelAccessPolicies={data.modelAccessPolicies} />
{/snippet}

{#snippet messagePolicies()}
	<MessagePoliciesView
		messagePolicies={data.messagePolicies ?? []}
		policyDirection="user-message"
	/>
{/snippet}
