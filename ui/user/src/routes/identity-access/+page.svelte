<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import TabLayout from '$lib/components/TabLayout.svelte';
	import { AUTH_PROVIDERS_VIEW_PATH, isSCIMView, SCIM_VIEW_PATH } from '$lib/constants';
	import { profile } from '$lib/stores';
	import AgentsView from './AgentsView.svelte';
	import AuthProvidersView from './AuthProvidersView.svelte';
	import GroupsView from './GroupsView.svelte';
	import RolesView from './RolesView.svelte';
	import ScimView from './ScimView.svelte';
	import UsersView from './UsersView.svelte';
	import { Plus } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	let { data } = $props();
	let groupsView = $state<ReturnType<typeof GroupsView>>();
	let agentsView = $state<ReturnType<typeof AgentsView>>();
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	let hasAdminAccess = $derived(profile.current.hasAdminAccess?.());
	let showCreateAgent = $derived(page.url.searchParams.has('new'));
	// The Auth Providers tab shows the auth providers, or SCIM.
	let showSCIM = $derived(isSCIMView(page.url.searchParams));
</script>

<svelte:head>
	<title>Obot | {showCreateAgent ? 'Create Agent Identity' : 'Identity & Access'}</title>
</svelte:head>

<TabLayout
	title={showCreateAgent ? 'Create Agent Identity' : 'Identity & Access'}
	defaultView={hasAdminAccess ? 'users' : 'agents'}
	showBackButton={showCreateAgent}
	onBackButtonClick={() => agentsView?.hideCreateForm()}
	rightNavActions={navActions}
	classes={{ childrenContainer: 'max-w-none' }}
	views={hasAdminAccess
		? [
				{ label: 'Users', value: 'users', content: users },
				{ label: 'Agents', value: 'agents', content: agents },
				{ label: 'Groups', value: 'groups', content: groups },
				{ label: 'Roles', value: 'roles', content: roles },
				{ label: 'Auth Providers', value: 'auth-providers', content: authProviders }
			]
		: [{ label: 'Agents', value: 'agents', content: agents }]}
/>

{#snippet navActions(view: string)}
	{#if view === 'groups' && !isAdminReadonly}
		<button
			class="btn btn-primary w-full text-sm sm:w-auto"
			onclick={() => groupsView?.openAddAssignment()}
		>
			<Plus class="size-4" /> Add Assignment
		</button>
	{:else if view === 'agents' && !showCreateAgent && !isAdminReadonly}
		<button
			class="btn btn-primary flex items-center gap-2 text-sm"
			onclick={() => agentsView?.showCreateForm()}
		>
			<Plus class="size-4" />
			Create Agent Identity
		</button>
	{/if}
{/snippet}

{#snippet users()}
	<UsersView users={data.users} />
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
		<nav class="flex" aria-label="Auth Providers">
			<div class="tabs tabs-box bg-base-100 shadow-sm dark:bg-base-300">
				{@render subview('Providers', AUTH_PROVIDERS_VIEW_PATH, !showSCIM)}
				{@render subview('SCIM', SCIM_VIEW_PATH, showSCIM)}
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

{#snippet agents()}
	<AgentsView
		bind:this={agentsView}
		apiKeys={data.apiKeys}
		users={data.users}
		isAdmin={Boolean(hasAdminAccess)}
	/>
{/snippet}
