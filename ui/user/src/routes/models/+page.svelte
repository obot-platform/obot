<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import TabLayout, { type TabView } from '$lib/components/TabLayout.svelte';
	import DefaultModels from '$lib/components/admin/DefaultModels.svelte';
	import MessagePoliciesView from '$lib/components/admin/MessagePoliciesView.svelte';
	import { getAdminModels, initModels } from '$lib/context/admin/models.svelte.js';
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
				label: 'Models',
				value: 'models',
				content: models,
				tooltip: 'Access and set up your AI client with models you have access to.'
			}
		];
		if (hasAdminAccess) {
			items.push(
				{
					label: 'Model Providers',
					value: 'model-providers',
					content: modelProviders,
					tooltip:
						'Set up and manage LLM model providers to enforce what models your organization can use or supply to the vMCP Inspector.'
				},
				{
					label: 'Access Policies',
					value: 'access-policies',
					content: accessPolicies,
					tooltip: 'Manage which models a user or group can access.'
				}
			);
			if (messagePoliciesEnabled) {
				items.push({
					label: 'AI Judge Policies',
					value: 'ai-judge-policies',
					content: messagePolicies,
					tooltip:
						'Enforce user messages with the LLM or view policy violations against existing policies.'
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
		if (!creating) return 'Models';
		return creatingView === 'ai-judge-policies'
			? 'Create AI Judge Policy'
			: 'Create Model Access Policy';
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
		title="Models"
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
			<Plus class="size-4" /> Add Access Policy
		</button>
	{:else if view === 'ai-judge-policies' && messagePoliciesEnabled && !isAdminReadonly}
		<button
			class="btn btn-primary flex items-center gap-1 text-sm"
			onclick={() => showCreate('ai-judge-policies')}
		>
			<Plus class="size-4" /> Add AI Judge Policy
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
