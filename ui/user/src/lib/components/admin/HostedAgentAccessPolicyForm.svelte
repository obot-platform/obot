<script lang="ts">
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		UserService,
		type AccessControlRuleSubject,
		type OrgUser,
		type OrgGroup,
		type HostedAgent,
		type HostedAgentAccessPolicy,
		type HostedAgentAccessPolicyResource
	} from '$lib/services';
	import { errors } from '$lib/stores';
	import { goto } from '$lib/url';
	import { getUserDisplayName } from '$lib/utils';
	import Confirm from '../Confirm.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import Table from '../table/Table.svelte';
	import SearchHostedAgents from './SearchHostedAgents.svelte';
	import SearchUsers from './SearchUsers.svelte';
	import { Plus, Trash2 } from '@lucide/svelte';
	import { onMount, untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	interface Props {
		hostedAgentAccessPolicy?: HostedAgentAccessPolicy;
		onCreate?: (hostedAgentAccessPolicy: HostedAgentAccessPolicy) => void;
		onUpdate?: (hostedAgentAccessPolicy: HostedAgentAccessPolicy) => void;
		readonly?: boolean;
	}

	let {
		hostedAgentAccessPolicy: initialHostedAgentAccessPolicy,
		onCreate,
		onUpdate,
		readonly
	}: Props = $props();

	const duration = PAGE_TRANSITION_DURATION;
	let policy = $state(
		untrack(
			() =>
				initialHostedAgentAccessPolicy ?? {
					id: undefined,
					displayName: '',
					subjects: [],
					resources: []
				}
		)
	);

	let saving = $state<boolean | undefined>();
	let usersAndGroups = $state<{ users: OrgUser[]; groups: OrgGroup[] }>();
	let loadingUsersAndGroups = $state(false);
	let loadingHostedAgents = $state(true);
	let hostedAgents = $state<HostedAgent[]>([]);

	let addUserGroupDialog = $state<ReturnType<typeof SearchUsers>>();
	let addHostedAgentDialog = $state<ReturnType<typeof SearchHostedAgents>>();

	let deletingPolicy = $state(false);

	let initialPolicyJson = $derived(
		initialHostedAgentAccessPolicy
			? JSON.stringify({
					subjects: initialHostedAgentAccessPolicy.subjects,
					resources: initialHostedAgentAccessPolicy.resources
				})
			: ''
	);

	let hasChanges = $derived(
		!initialPolicyJson ||
			JSON.stringify({
				subjects: policy.subjects,
				resources: policy.resources
			}) !== initialPolicyJson
	);

	onMount(async () => {
		try {
			hostedAgents = await AdminService.listHostedAgents();
		} catch (error) {
			errors.append(`Failed to load agents: ${error}`);
		} finally {
			loadingHostedAgents = false;
		}
	});

	let hostedAgentsMap = $derived(new Map(hostedAgents.map((a) => [a.id, a])));

	$effect(() => {
		// Prevent loading users and groups if the policy has no subjects
		if (!policy.subjects || policy.subjects?.length === 0) {
			return;
		}

		loadingUsersAndGroups = true;

		// Prevent refetching when adding new users or groups
		const promises: [Promise<OrgUser[] | undefined>, Promise<OrgGroup[] | undefined>] = [
			Promise.resolve(undefined),
			Promise.resolve(undefined)
		];

		if (!usersAndGroups?.users) {
			promises[0] = UserService.listUsers();
		}
		if (!usersAndGroups?.groups) {
			promises[1] = UserService.listGroups();
		}

		Promise.all(promises)
			.then(([users, groups]) => {
				if (!usersAndGroups) {
					usersAndGroups = { users: [], groups: [] };
				}

				if (users) {
					usersAndGroups!.users = users;
				}

				if (groups) {
					usersAndGroups!.groups = groups;
				}

				loadingUsersAndGroups = false;
			})
			.catch((error) => {
				console.error('Failed to load users and groups:', error);
				loadingUsersAndGroups = false;
			});
	});

	function convertResourcesToTableData(resources: HostedAgentAccessPolicyResource[]) {
		return (
			resources
				.map((resource) => {
					if (resource.type === 'hostedAgent') {
						const match = hostedAgentsMap.get(resource.id);
						return {
							id: resource.id,
							name: match?.name || '-',
							description: match?.description || '-',
							type: 'Agent'
						};
					} else if (resource.type === 'selector') {
						return {
							id: resource.id,
							name: resource.id === '*' ? 'All Agents' : resource.id,
							description: '',
							type: 'Selector'
						};
					}
					return undefined;
				})
				.filter((resource) => resource !== undefined) ?? []
		);
	}

	function convertSubjectsToTableData(
		subjects: AccessControlRuleSubject[],
		users: OrgUser[],
		groups: OrgGroup[]
	) {
		const userMap = new Map(users?.map((user) => [user.id, user]));
		const groupMap = new Map(groups?.map((group) => [group.id, group]));

		return (
			subjects
				.map((subject) => {
					if (subject.type === 'user') {
						return {
							id: subject.id,
							displayName: getUserDisplayName(userMap, subject.id),
							type: 'User'
						};
					}

					if (subject.type === 'group') {
						const group = groupMap.get(subject.id);
						if (!group) {
							return undefined;
						}

						return {
							id: subject.id,
							displayName: group.name,
							type: 'Group'
						};
					}

					return {
						id: subject.id,
						displayName: subject.id === '*' ? 'All Obot Users' : subject.id,
						type: 'Selector'
					};
				})
				.filter((subject) => subject !== undefined) ?? []
		);
	}

	function validate(p: typeof policy) {
		if (!p) return false;

		return (
			p.displayName.length > 0 && (p.subjects?.length ?? 0) > 0 && (p.resources?.length ?? 0) > 0
		);
	}

	const subjectTableData = $derived(
		convertSubjectsToTableData(
			policy.subjects ?? [],
			usersAndGroups?.users ?? [],
			usersAndGroups?.groups ?? []
		)
	);
	const resourceTableData = $derived(convertResourcesToTableData(policy.resources ?? []));
</script>

<div
	class="flex h-full w-full flex-col gap-4"
	out:fly={{ x: 100, duration }}
	in:fly={{ x: 100, delay: duration }}
>
	<div class="flex grow flex-col gap-4" out:fly={{ x: -100, duration }} in:fly={{ x: -100 }}>
		{#if policy.id}
			<div class="flex w-full items-center justify-between gap-4">
				<div class="flex items-center gap-2">
					<h1 class="flex items-center gap-4 text-2xl font-semibold">
						{policy.displayName}
					</h1>
				</div>
				{#if !readonly}
					<IconButton
						variant="danger2"
						tooltip={{ text: 'Delete Policy' }}
						onclick={() => {
							deletingPolicy = true;
						}}
					>
						<Trash2 class="size-4" />
					</IconButton>
				{/if}
			</div>
		{/if}

		{#if !policy.id}
			<div
				class="dark:bg-base-400 dark:border-base-400 bg-base-100 rounded-lg border border-transparent p-4"
			>
				<div class="flex flex-col gap-6">
					<div class="flex flex-col gap-2">
						<label
							for="hosted-agent-access-policy-name"
							class="flex-1 text-sm font-light capitalize"
						>
							Name
						</label>
						<input
							id="hosted-agent-access-policy-name"
							bind:value={policy.displayName}
							class="text-input-filled mt-0.5"
							disabled={readonly}
						/>
					</div>
				</div>
			</div>
		{/if}

		<div class="flex flex-col gap-2">
			<div class="mb-2 flex items-center justify-between">
				<h2 class="text-lg font-semibold">Users & Groups</h2>
				{#if !readonly}
					<div class="relative flex items-center gap-4">
						<button
							class="btn btn-primary flex items-center gap-1 text-sm"
							disabled={loadingUsersAndGroups}
							onclick={() => {
								addUserGroupDialog?.open();
							}}
						>
							<Plus class="size-4" /> Add User/Group
						</button>
					</div>
				{/if}
			</div>
			{#if loadingUsersAndGroups}
				<div class="my-2 flex items-center justify-center">
					<Loading class="size-6" />
				</div>
			{:else}
				<Table
					data={subjectTableData}
					fields={['displayName', 'type']}
					headers={[{ property: 'displayName', title: 'Name' }]}
					noDataMessage="No users or groups added."
				>
					{#snippet actions(d)}
						{#if !readonly}
							<IconButton
								variant="danger"
								onclick={() => {
									policy.subjects = policy.subjects?.filter((subject) => subject.id !== d.id);
								}}
								tooltip={{ text: 'Delete User/Group' }}
							>
								<Trash2 class="size-4" />
							</IconButton>
						{/if}
					{/snippet}
				</Table>
			{/if}
		</div>

		<div class="flex flex-col gap-2">
			<div class="mb-2 flex items-center justify-between">
				<h2 class="text-lg font-semibold">Agents</h2>
				{#if !readonly}
					<button
						class="btn btn-primary flex items-center gap-1 text-sm"
						onclick={() => {
							addHostedAgentDialog?.open();
						}}
					>
						<Plus class="size-4" /> Add Agent
					</button>
				{/if}
			</div>
			{#if loadingHostedAgents}
				<div class="my-2 flex items-center justify-center">
					<Loading class="size-6" />
				</div>
			{:else}
				<Table
					data={resourceTableData}
					fields={['name', 'description']}
					headers={[
						{ property: 'name', title: 'Agent' },
						{ property: 'description', title: 'Description' }
					]}
					noDataMessage="No agents added."
				>
					{#snippet onRenderColumn(field, d)}
						{#if field === 'name'}
							<span class="font-light">{d.name}</span>
						{:else}
							{d[field as keyof typeof d]}
						{/if}
					{/snippet}
					{#snippet actions(d)}
						{#if !readonly}
							<IconButton
								variant="danger"
								onclick={() => {
									policy.resources = policy.resources?.filter((r) => r.id !== d.id) ?? [];
								}}
								tooltip={{ text: 'Remove Agent' }}
							>
								<Trash2 class="size-4" />
							</IconButton>
						{/if}
					{/snippet}
				</Table>
			{/if}
		</div>
	</div>
	{#if !readonly}
		<div
			class="bg-base-200 text-muted-content dark:bg-base-100 sticky bottom-0 left-0 z-50 flex w-full justify-end gap-2 py-4"
			out:fly={{ x: -100, duration }}
			in:fly={{ x: -100 }}
		>
			<div class="flex w-full justify-end gap-2">
				{#if !policy.id}
					<button
						class="btn btn-secondary text-sm"
						onclick={() => {
							goto('/admin/hosted-agent-access-policies');
						}}
					>
						Cancel
					</button>
					<button
						class="btn btn-primary text-sm"
						disabled={!validate(policy) || saving}
						onclick={async () => {
							saving = true;
							try {
								const response = await AdminService.createHostedAgentAccessPolicy(policy);
								policy = response;
								onCreate?.(response);
							} finally {
								saving = false;
							}
						}}
					>
						{#if saving}
							<Loading class="size-4" />
						{:else}
							Save
						{/if}
					</button>
				{:else}
					<button
						class="btn btn-primary text-sm"
						disabled={!validate(policy) || !hasChanges || saving}
						onclick={async () => {
							if (!policy.id) return;
							saving = true;
							try {
								const response = await AdminService.updateHostedAgentAccessPolicy(
									policy.id,
									policy
								);
								policy = response;
								onUpdate?.(response);
							} finally {
								saving = false;
							}
						}}
					>
						{#if saving}
							<Loading class="size-4" />
						{:else}
							Update
						{/if}
					</button>
				{/if}
			</div>
		</div>
	{/if}
</div>

<SearchHostedAgents
	bind:this={addHostedAgentDialog}
	{hostedAgents}
	onAdd={(resources: HostedAgentAccessPolicyResource[]) => {
		policy.resources = [
			...(policy.resources ?? []),
			...resources.filter((resource) => !policy.resources?.some((r) => r.id === resource.id))
		];
	}}
	exclude={policy.resources?.map((resource) => resource.id) ?? []}
/>

<SearchUsers
	bind:this={addUserGroupDialog}
	filterIds={policy.subjects?.map((subject) => subject.id) ?? []}
	onAdd={async (users: OrgUser[], groups: OrgGroup[]) => {
		const existingSubjectIds = new Set(policy.subjects?.map((subject) => subject.id) ?? []);
		const newSubjects = [
			...users
				.filter((user: OrgUser) => !existingSubjectIds.has(user.id))
				.map((user: OrgUser) => ({
					type: 'user' as const,
					id: user.id
				})),
			...groups
				.filter((group: OrgGroup) => !existingSubjectIds.has(group.id))
				.map((group: OrgGroup) => ({
					type: group.id === '*' ? ('selector' as const) : ('group' as const),
					id: group.id
				}))
		];
		policy.subjects = [...(policy.subjects ?? []), ...newSubjects];
	}}
/>

<Confirm
	msg={`Delete ${policy.displayName || 'this policy'}?`}
	show={deletingPolicy}
	onsuccess={async () => {
		if (!policy.id) return;
		saving = true;
		await AdminService.deleteHostedAgentAccessPolicy(policy.id);
		goto('/admin/hosted-agent-access-policies');
	}}
	oncancel={() => (deletingPolicy = false)}
/>
