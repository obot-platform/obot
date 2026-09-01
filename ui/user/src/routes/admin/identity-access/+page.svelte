<script lang="ts">
	import TabLayout from '$lib/components/TabLayout.svelte';
	import { profile } from '$lib/stores';
	import AuthProvidersView from './AuthProvidersView.svelte';
	import GroupsView from './GroupsView.svelte';
	import RolesView from './RolesView.svelte';
	import UsersView from './UsersView.svelte';
	import { Plus } from '@lucide/svelte';

	let { data } = $props();
	let groupsView = $state<ReturnType<typeof GroupsView>>();
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
</script>

<svelte:head>
	<title>Obot | Identity & Access</title>
</svelte:head>

<TabLayout
	title="Identity & Access"
	defaultView="users"
	rightNavActions={navActions}
	classes={{ childrenContainer: 'max-w-none' }}
	views={[
		{ label: 'Users', value: 'users', content: users },
		{ label: 'Groups', value: 'groups', content: groups },
		{ label: 'Roles', value: 'roles', content: roles },
		{ label: 'Auth Providers', value: 'auth-providers', content: authProviders }
	]}
/>

{#snippet navActions(view: string)}
	{#if view === 'groups' && !isAdminReadonly}
		<button
			class="btn btn-primary w-full text-sm sm:w-auto"
			onclick={() => groupsView?.openAddAssignment()}
		>
			<Plus class="size-4" /> Add Assignment
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
