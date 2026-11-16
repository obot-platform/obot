<script lang="ts">
	import { page } from '$app/state';
	import TabLayout from '$lib/components/TabLayout.svelte';
	import {
		AUTH_PROVIDERS_VIEW_PATH,
		CommonAuthProviderIds,
		isSCIMView,
		SCIM_VIEW_PATH
	} from '$lib/constants';
	import { m } from '$lib/i18n';
	import { profile } from '$lib/stores';
	import { adminConfigStore } from '$lib/stores/adminConfig.svelte';
	import { resolve } from '$lib/url';
	import AgentIdentitiesView from './AgentIdentitiesView.svelte';
	import ApiKeysView from './ApiKeysView.svelte';
	import AuthProvidersView from './AuthProvidersView.svelte';
	import GroupsView from './GroupsView.svelte';
	import RolesView from './RolesView.svelte';
	import ScimView from './ScimView.svelte';
	import UsersView from './UsersView.svelte';
	import { Plus } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	let { data } = $props();
	let groupsView = $state<ReturnType<typeof GroupsView>>();
	let apiKeysView = $state<ReturnType<typeof ApiKeysView>>();
	let agentIdentitiesView = $state<ReturnType<typeof AgentIdentitiesView>>();
	let usersView = $state<ReturnType<typeof UsersView>>();
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	let hasAdminAccess = $derived(profile.current.hasAdminAccess?.());
	let showCreateApiKey = $derived(page.url.searchParams.has('new'));
	// The Auth Providers tab shows the auth providers, or SCIM.
	let showSCIM = $derived(isSCIMView(page.url.searchParams));
	let localAuthProviderAvailable = $derived(
		$adminConfigStore.authProviders.some(
			(provider) => provider.id === CommonAuthProviderIds.LOCAL && provider.configured
		)
	);
</script>

<svelte:head>
	<title
		>Obot | {showCreateApiKey
			? m.identity_access_create_api_key()
			: m.identity_access_title()}</title
	>
</svelte:head>

<TabLayout
	title={showCreateApiKey ? m.identity_access_create_api_key() : m.identity_access_title()}
	defaultView={hasAdminAccess ? 'users' : 'api-keys'}
	showBackButton={showCreateApiKey}
	onBackButtonClick={() => apiKeysView?.hideCreateForm()}
	rightNavActions={navActions}
	classes={{ childrenContainer: 'max-w-none' }}
	views={hasAdminAccess
		? [
				{ label: m.identity_access_users_tab(), value: 'users', content: users },
				{ label: m.identity_access_api_keys_tab(), value: 'api-keys', content: apiKeys },
				{
					label: m.identity_access_agents_tab(),
					value: 'agent-identities',
					content: agentIdentities
				},
				{ label: m.identity_access_groups_tab(), value: 'groups', content: groups },
				{ label: m.identity_access_roles_tab(), value: 'roles', content: roles },
				{
					label: m.identity_access_auth_providers_tab(),
					value: 'auth-providers',
					content: authProviders
				}
			]
		: [{ label: m.identity_access_api_keys_tab(), value: 'api-keys', content: apiKeys }]}
/>

{#snippet navActions(view: string)}
	{#if view === 'groups' && !isAdminReadonly}
		<button
			class="btn btn-primary w-full text-sm sm:w-auto"
			onclick={() => groupsView?.openAddAssignment()}
		>
			<Plus class="size-4" />
			{m.identity_access_add_assignment()}
		</button>
	{:else if view === 'api-keys' && !showCreateApiKey}
		<button
			class="btn btn-primary flex items-center gap-2 text-sm"
			onclick={() => apiKeysView?.showCreateForm()}
		>
			<Plus class="size-4" />
			{m.identity_access_create_api_key()}
		</button>
	{:else if view === 'agent-identities' && !isAdminReadonly}
		<button
			class="btn btn-primary flex items-center gap-2 text-sm"
			onclick={() => agentIdentitiesView?.openCreate()}
		>
			<Plus class="size-4" />
			{m.identity_access_create_agent_identity()}
		</button>
	{:else if view === 'users' && !isAdminReadonly && localAuthProviderAvailable}
		<button
			class="btn btn-primary w-full text-sm sm:w-auto"
			onclick={() => usersView?.openCreateUser()}
		>
			<Plus class="size-4" />
			{m.identity_access_auth_providers_local_auth_create_new_user()}
		</button>
	{/if}
{/snippet}

{#snippet users()}
	<UsersView
		bind:this={usersView}
		users={data.users}
		{localAuthProviderAvailable}
		providerDiscoveryComplete={!$adminConfigStore.loading && $adminConfigStore.lastFetched !== null}
	/>
{/snippet}

{#snippet groups()}
	<GroupsView
		bind:this={groupsView}
		groups={data.groups}
		groupRoleAssignments={data.groupRoleAssignments}
	/>
{/snippet}

{#snippet roles()}
	<RolesView defaultUsersRole={data.defaultUsersRole} />
{/snippet}

{#snippet authProviders()}
	<div class="flex flex-col gap-4">
		<nav class="flex" aria-label={m.identity_access_auth_providers_nav()}>
			<div class="tabs tabs-box bg-base-100 shadow-sm dark:bg-base-300">
				{@render subview(
					m.identity_access_auth_providers_tab_providers(),
					AUTH_PROVIDERS_VIEW_PATH,
					!showSCIM
				)}
				{@render subview(m.identity_access_scim_tab(), SCIM_VIEW_PATH, showSCIM)}
			</div>
		</nav>
		{#if showSCIM}
			<ScimView
				review={data.scimReview}
				enablePreview={data.scimEnablePreview}
				pageSize={data.scimPageSize}
			/>
		{:else}
			<AuthProvidersView authProviders={data.authProviders} authEnabled={data.authEnabled} />
		{/if}
	</div>
{/snippet}

{#snippet subview(
	label: string,
	path: typeof AUTH_PROVIDERS_VIEW_PATH | typeof SCIM_VIEW_PATH,
	active: boolean
)}
	<a
		href={resolve(path)}
		class={twMerge('tab text-xs min-w-24', active && 'tab-active bg-base-300 dark:bg-base-100')}
		aria-current={active ? 'page' : undefined}
	>
		{label}
	</a>
{/snippet}

{#snippet agentIdentities()}
	<AgentIdentitiesView bind:this={agentIdentitiesView} agents={data.agents} users={data.users} />
{/snippet}

{#snippet apiKeys()}
	<ApiKeysView
		bind:this={apiKeysView}
		apiKeys={data.apiKeys}
		users={data.users}
		isAdmin={Boolean(hasAdminAccess)}
	/>
{/snippet}
