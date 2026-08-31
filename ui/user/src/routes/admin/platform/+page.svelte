<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import TabLayout from '$lib/components/TabLayout.svelte';
	import ApiKeyRevealDialog from '$lib/components/agent-auth-scope/ApiKeyRevealDialog.svelte';
	import CreateAgentAuthScopeForm from '$lib/components/agent-auth-scope/CreateAgentAuthScopeForm.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import type { APIKeyCreateResponse } from '$lib/services/api-keys/types';
	import { profile, version } from '$lib/stores';
	import { compileAppPreferences } from '$lib/stores/appPreferences.svelte';
	import { goto } from '$lib/url';
	import BrandingConfigurationSidebar from './BrandingConfigurationSidebar.svelte';
	import BrandingView from './BrandingView.svelte';
	import LicenseView from './LicenseView.svelte';
	import McpConfigView from './McpConfigView.svelte';
	import NotificationsView from './NotificationsView.svelte';
	import RegistryConnectionsView from './RegistryConnectionsView.svelte';
	import { Plus } from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	const duration = PAGE_TRANSITION_DURATION;

	let { data } = $props();
	let apiKeys = $state(untrack(() => data.apiKeys));
	let createdKeyValue = $state<string>();
	let registryView = $state<ReturnType<typeof RegistryConnectionsView>>();
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	let creatingRegistryConnection = $derived(
		page.url.searchParams.get('view') === 'registry-connections' && page.url.searchParams.has('new')
	);
	let brandingPreferences = $derived(data.appPreferences ?? compileAppPreferences());

	let views = $derived([
		{ label: 'License', value: 'license', content: license },
		{ label: 'Branding', value: 'branding', content: branding },
		{ label: 'Notifications', value: 'notifications', content: notifications },
		...(version.current.engine === 'kubernetes'
			? [{ label: 'MCP Config', value: 'mcp-config', content: mcpConfig }]
			: []),
		{ label: 'Registry Connections', value: 'registry-connections', content: registryConnections }
	]);

	$effect(() => {
		apiKeys = data.apiKeys;
	});

	function hideCreateForm() {
		const url = new URL(page.url);
		url.searchParams.delete('new');
		goto(url, { replaceState: true });
	}

	function handleCreate(newKey: APIKeyCreateResponse) {
		apiKeys = [newKey, ...apiKeys];
		createdKeyValue = newKey.key;
		hideCreateForm();
	}
</script>

<svelte:head>
	<title>Obot | Platform</title>
</svelte:head>

{#if creatingRegistryConnection}
	<Layout title="Create Agent Identity" showBackButton onBackButtonClick={hideCreateForm}>
		<div
			class="h-full w-full"
			in:fly={{ x: 100, delay: duration, duration }}
			out:fly={{ x: -100, duration }}
		>
			<CreateAgentAuthScopeForm onCreate={handleCreate} onCancel={hideCreateForm} />
		</div>
	</Layout>
{:else}
	<TabLayout
		title="Platform"
		defaultView="license"
		classes={{ container: 'pb-0', childrenContainer: 'max-w-none' }}
		rightNavActions={navActions}
		rightSidebar={viewSidebar}
		{views}
	/>
{/if}

{#snippet viewSidebar(view: string)}
	{#if view === 'branding'}
		<BrandingConfigurationSidebar initialAppPreferences={brandingPreferences} />
	{/if}
{/snippet}

{#snippet navActions(view: string)}
	{#if view === 'registry-connections' && !isAdminReadonly}
		<button
			class="btn btn-primary flex items-center gap-2 text-sm"
			onclick={() => registryView?.openCreateForm()}
		>
			<Plus class="size-4" />
			Create Agent Auth Scope
		</button>
	{/if}
{/snippet}

{#snippet license()}
	<LicenseView license={data.license} />
{/snippet}

{#snippet branding()}
	<BrandingView />
{/snippet}

{#snippet notifications()}
	<NotificationsView appNotification={data.appNotification} />
{/snippet}

{#snippet mcpConfig()}
	<McpConfigView k8sSettings={data.k8sSettings} />
{/snippet}

{#snippet registryConnections()}
	<RegistryConnectionsView bind:this={registryView} bind:apiKeys users={data.users} />
{/snippet}

<ApiKeyRevealDialog keyValue={createdKeyValue} onClose={() => (createdKeyValue = undefined)} />
