<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import TabLayout from '$lib/components/TabLayout.svelte';
	import GitCredentialsView from '$lib/components/admin/GitCredentialsView.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import type { ImagePullSecret, ImagePullSecretCapability } from '$lib/services';
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
	import { fade } from 'svelte/transition';

	const duration = PAGE_TRANSITION_DURATION;
	const REGISTRY_CONNECTIONS_PATH = '/admin/platform?view=registry-connections';

	let { data } = $props();
	let capability = $state<ImagePullSecretCapability>(
		untrack(() => data.capability ?? { available: false })
	);
	let imagePullSecrets = $state<ImagePullSecret[]>(untrack(() => data.imagePullSecrets ?? []));
	let registryView = $state<ReturnType<typeof RegistryConnectionsView>>();
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	let creatingRegistryConnection = $derived(
		page.url.searchParams.get('view') === 'registry-connections' &&
			(page.url.searchParams.get('create') === 'true' || Boolean(page.url.searchParams.get('id')))
	);
	let registryFormTitle = $derived(
		page.url.searchParams.get('create') === 'true'
			? 'Create Image Pull Secret'
			: 'Edit Image Pull Secret'
	);
	let brandingPreferences = $derived(data.appPreferences ?? compileAppPreferences());

	let views = $derived([
		{ label: 'License', value: 'license', content: license },
		{ label: 'Branding', value: 'branding', content: branding },
		{ label: 'Notifications', value: 'notifications', content: notifications },
		...(version.current.engine === 'kubernetes' && !version.current.hideK8sDetails
			? [{ label: 'MCP Config', value: 'mcp-config', content: mcpConfig }]
			: []),
		...(version.current.engine === 'kubernetes'
			? [
					{
						label: 'Registry Connections',
						value: 'registry-connections',
						content: registryConnections
					}
				]
			: []),
		{ label: 'Git Credentials', value: 'git-credentials', content: gitCredentials }
	]);

	$effect(() => {
		capability = data.capability ?? { available: false };
		imagePullSecrets = data.imagePullSecrets ?? [];
	});

	function hideRegistryForm() {
		goto(REGISTRY_CONNECTIONS_PATH, { replaceState: true, noScroll: true });
	}
</script>

<svelte:head>
	<title>Obot | Platform</title>
</svelte:head>

{#if creatingRegistryConnection}
	<Layout title={registryFormTitle} showBackButton onBackButtonClick={hideRegistryForm}>
		<div class="h-full w-full" in:fade={{ duration }}>
			<RegistryConnectionsView bind:this={registryView} bind:capability bind:imagePullSecrets />
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
	{#if view === 'registry-connections' && !isAdminReadonly && capability.available}
		<button
			class="btn btn-primary flex items-center gap-2 text-sm"
			onclick={() => registryView?.openCreateForm()}
		>
			<Plus class="size-4" />
			Create New Secret
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
	<RegistryConnectionsView bind:this={registryView} bind:capability bind:imagePullSecrets />
{/snippet}

{#snippet gitCredentials()}
	<GitCredentialsView gitCredentials={data.gitCredentials} />
{/snippet}
