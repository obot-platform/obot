<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import TabLayout, { type TabView } from '$lib/components/TabLayout.svelte';
	import DefaultModels from '$lib/components/admin/DefaultModels.svelte';
	import { getAdminModels, initModels } from '$lib/context/admin/models.svelte.js';
	import { profile } from '$lib/stores';
	import { goto } from '$lib/url';
	import AccessPoliciesView from './AccessPoliciesView.svelte';
	import ModelProvidersView from './ModelProvidersView.svelte';
	import ModelsView from './ModelsView.svelte';
	import { Plus } from '@lucide/svelte';

	let { data } = $props();
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	let hasAdminAccess = $derived(profile.current.hasAdminAccess?.() ?? data.hasAdminAccess);
	let creating = $derived(
		hasAdminAccess &&
			!isAdminReadonly &&
			page.url.searchParams.get('view') === 'access-policies' &&
			page.url.searchParams.has('new')
	);

	initModels([]);
	const adminModels = getAdminModels();
	let defaultModelsDialog = $state<ReturnType<typeof DefaultModels>>();

	let views = $derived.by(() => {
		const items: TabView[] = [{ label: 'Models', value: 'models', content: models }];
		if (hasAdminAccess) {
			items.push(
				{ label: 'Model Providers', value: 'model-providers', content: modelProviders },
				{ label: 'Access Policies', value: 'access-policies', content: accessPolicies }
			);
		}
		return items;
	});

	function hideCreate() {
		const url = new URL(page.url);
		url.searchParams.delete('new');
		goto(url, { replaceState: true });
	}

	function showCreate() {
		goto(`${page.url.pathname}?view=access-policies&new=true`);
	}

	function handleFirstConfigure(required: boolean) {
		defaultModelsDialog?.open(required);
	}
</script>

<svelte:head>
	<title>Obot | {creating ? 'Create Model Access Policy' : 'Models'}</title>
</svelte:head>

{#if creating}
	<Layout title="Create Model Access Policy" showBackButton onBackButtonClick={hideCreate}>
		<AccessPoliciesView modelAccessPolicies={data.modelAccessPolicies} creating />
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
		<button class="btn btn-primary flex items-center gap-1 text-sm" onclick={showCreate}>
			<Plus class="size-4" /> Add Access Policy
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
