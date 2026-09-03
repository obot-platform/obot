<script lang="ts">
	import { page } from '$app/state';
	import TabLayout from '$lib/components/TabLayout.svelte';
	import { profile } from '$lib/stores';
	import AgentsView from './AgentsView.svelte';
	import AuthProvidersView from './AuthProvidersView.svelte';
	import GroupsView from './GroupsView.svelte';
	import RolesView from './RolesView.svelte';
	import UsersView from './UsersView.svelte';
	import { Plus } from '@lucide/svelte';

	let { data } = $props();
	let groupsView = $state<ReturnType<typeof GroupsView>>();
	let agentsView = $state<ReturnType<typeof AgentsView>>();
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	let hasAdminAccess = $derived(profile.current.hasAdminAccess?.());
	let showCreateAgent = $derived(page.url.searchParams.has('new'));
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
			Create Agent Auth Scope
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
	<AuthProvidersView authProviders={data.authProviders} authEnabled={data.authEnabled} />
{/snippet}

{#snippet agents()}
	<AgentsView
		bind:this={agentsView}
		apiKeys={data.apiKeys}
		users={data.users}
		isAdmin={Boolean(hasAdminAccess)}
	/>
{/snippet}
