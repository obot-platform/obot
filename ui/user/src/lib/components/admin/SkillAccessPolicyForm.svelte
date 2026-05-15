<script lang="ts">
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService } from '$lib/services';
	import {
		type AccessControlRuleSubject,
		type OrgUser,
		type OrgGroup,
		type SkillAccessPolicy,
		type SkillAccessPolicyResource,
		type SkillRepository
	} from '$lib/services/admin/types';
	import type { Skill } from '$lib/services/nanobot/types';
	import { errors } from '$lib/stores';
	import { goto } from '$lib/url';
	import { getUserDisplayName } from '$lib/utils';
	import Confirm from '../Confirm.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import Table from '../table/Table.svelte';
	import SearchSkills from './SearchSkills.svelte';
	import SearchUsers from './SearchUsers.svelte';
	import { Plus, Trash2 } from 'lucide-svelte';
	import { onMount, untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	interface Props {
		skillAccessPolicy?: SkillAccessPolicy;
		onCreate?: (skillAccessPolicy: SkillAccessPolicy) => void;
		onUpdate?: (skillAccessPolicy: SkillAccessPolicy) => void;
		readonly?: boolean;
	}

	let {
		skillAccessPolicy: initialSkillAccessPolicy,
		onCreate,
		onUpdate,
		readonly
	}: Props = $props();

	const duration = PAGE_TRANSITION_DURATION;
	let skillAccessPolicy = $state(
		untrack(
			() =>
				initialSkillAccessPolicy ?? {
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
	let loadingSkills = $state(true);
	let skills = $state<Skill[]>([]);
	let skillRepositories = $state<SkillRepository[]>([]);

	let addUserGroupDialog = $state<ReturnType<typeof SearchUsers>>();
	let addSkillDialog = $state<ReturnType<typeof SearchSkills>>();

	let deletingPolicy = $state(false);

	let initialPolicyJson = $derived(
		initialSkillAccessPolicy
			? JSON.stringify({
					subjects: initialSkillAccessPolicy.subjects,
					resources: initialSkillAccessPolicy.resources
				})
			: ''
	);

	let hasChanges = $derived(
		!initialPolicyJson ||
			JSON.stringify({
				subjects: skillAccessPolicy.subjects,
				resources: skillAccessPolicy.resources
			}) !== initialPolicyJson
	);

	onMount(async () => {
		try {
			skills = await AdminService.listAllSkills();
			skillRepositories = await AdminService.listSkillRepositories();
		} catch (error) {
			errors.append(`Failed to load skills or skill repositories: ${error}`);
		} finally {
			loadingSkills = false;
		}
	});

	let skillsMap = $derived(new Map(skills.map((s) => [s.id, s])));
	let skillRepositoriesMap = $derived(new Map(skillRepositories.map((s) => [s.id, s])));

	$effect(() => {
		// Prevent loading users and groups if rule has no subjects
		if (!skillAccessPolicy.subjects || skillAccessPolicy.subjects?.length === 0) {
			return;
		}

		loadingUsersAndGroups = true;

		// Prevent refetching when adding new users or groups
		const promises: [Promise<OrgUser[] | undefined>, Promise<OrgGroup[] | undefined>] = [
			Promise.resolve(undefined),
			Promise.resolve(undefined)
		];

		if (!usersAndGroups?.users) {
			promises[0] = AdminService.listUsers();
		}
		if (!usersAndGroups?.groups) {
			promises[1] = AdminService.listGroups();
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

	function convertResourcesToTableData(resources: SkillAccessPolicyResource[]) {
		return (
			resources
				.map((resource) => {
					if (resource.type === 'skill') {
						const match = skillsMap.get(resource.id);
						return {
							id: resource.id,
							name: match?.name || '-',
							description: match?.description || '-',
							type: 'Skill'
						};
					} else if (resource.type === 'skillRepository') {
						const match = skillRepositoriesMap.get(resource.id);
						return {
							id: resource.id,
							name: match?.displayName || '-',
							description: '',
							type: 'Skill Repository'
						};
					} else if (resource.type === 'selector') {
						return {
							id: resource.id,
							name: resource.id === '*' ? 'All Skills' : resource.id,
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

	function validate(policy: typeof skillAccessPolicy) {
		if (!policy) return false;

		return (
			policy.displayName.length > 0 &&
			(policy.subjects?.length ?? 0) > 0 &&
			(policy.resources?.length ?? 0) > 0
		);
	}

	const subjectTableData = $derived(
		convertSubjectsToTableData(
			skillAccessPolicy.subjects ?? [],
			usersAndGroups?.users ?? [],
			usersAndGroups?.groups ?? []
		)
	);
	const resourceTableData = $derived(
		convertResourcesToTableData(skillAccessPolicy.resources ?? [])
	);
</script>

<div
	class="flex h-full w-full flex-col gap-4"
	out:fly={{ x: 100, duration }}
	in:fly={{ x: 100, delay: duration }}
>
	<div class="flex grow flex-col gap-4" out:fly={{ x: -100, duration }} in:fly={{ x: -100 }}>
		{#if skillAccessPolicy.id}
			<div class="flex w-full items-center justify-between gap-4">
				<div class="flex items-center gap-2">
					<h1 class="flex items-center gap-4 text-2xl font-semibold">
						{skillAccessPolicy.displayName}
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

		{#if !skillAccessPolicy.id}
			<div
				class="dark:bg-base-400 dark:border-base-400 bg-base-100 rounded-lg border border-transparent p-4"
			>
				<div class="flex flex-col gap-6">
					<div class="flex flex-col gap-2">
						<label for="model-access-policy-name" class="flex-1 text-sm font-light capitalize">
							Name
						</label>
						<input
							id="model-access-policy-name"
							bind:value={skillAccessPolicy.displayName}
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
						{#if loadingUsersAndGroups}
							<button class="btn btn-primary flex items-center gap-1 text-sm" disabled>
								<Plus class="size-4" /> Add User/Group
							</button>
						{:else}
							<button
								class="btn btn-primary flex items-center gap-1 text-sm"
								onclick={() => {
									addUserGroupDialog?.open();
								}}
							>
								<Plus class="size-4" /> Add User/Group
							</button>
						{/if}
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
									skillAccessPolicy.subjects = skillAccessPolicy.subjects?.filter(
										(subject) => subject.id !== d.id
									);
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
				<h2 class="text-lg font-semibold">Skills</h2>
				{#if !readonly}
					<button
						class="btn btn-primary flex items-center gap-1 text-sm"
						onclick={() => {
							addSkillDialog?.open();
						}}
					>
						<Plus class="size-4" /> Add Skill
					</button>
				{/if}
			</div>
			{#if loadingSkills}
				<div class="my-2 flex items-center justify-center">
					<Loading class="size-6" />
				</div>
			{:else}
				<Table
					data={resourceTableData}
					fields={['name', 'description']}
					headers={[
						{ property: 'name', title: 'Skill' },
						{ property: 'description', title: 'Description' }
					]}
					noDataMessage="No skills added."
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
									skillAccessPolicy.resources =
										skillAccessPolicy.resources?.filter((r) => r.id !== d.id) ?? [];
								}}
								tooltip={{ text: 'Remove Skill' }}
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
				{#if !skillAccessPolicy.id}
					<button
						class="btn btn-secondary text-sm"
						onclick={() => {
							goto('/admin/skill-access-policies');
						}}
					>
						Cancel
					</button>
					<button
						class="btn btn-primary text-sm"
						disabled={!validate(skillAccessPolicy) || saving}
						onclick={async () => {
							saving = true;
							try {
								const response = await AdminService.createSkillAccessPolicy(skillAccessPolicy);
								skillAccessPolicy = response;
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
						disabled={!validate(skillAccessPolicy) || !hasChanges || saving}
						onclick={async () => {
							if (!skillAccessPolicy.id) return;
							saving = true;
							try {
								const response = await AdminService.updateSkillAccessPolicy(
									skillAccessPolicy.id,
									skillAccessPolicy
								);
								skillAccessPolicy = response;
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

<SearchSkills
	bind:this={addSkillDialog}
	{skills}
	{skillRepositories}
	onAdd={async (resources: SkillAccessPolicyResource[]) => {
		skillAccessPolicy.resources = [
			...(skillAccessPolicy.resources ?? []),
			...resources.filter(
				(resource) => !skillAccessPolicy.resources?.some((r) => r.id === resource.id)
			)
		];
	}}
	exclude={skillAccessPolicy.resources?.map((resource) => resource.id) ?? []}
/>

<SearchUsers
	bind:this={addUserGroupDialog}
	filterIds={skillAccessPolicy.subjects?.map((subject) => subject.id) ?? []}
	onAdd={async (users: OrgUser[], groups: OrgGroup[]) => {
		const existingSubjectIds = new Set(
			skillAccessPolicy.subjects?.map((subject) => subject.id) ?? []
		);
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
		skillAccessPolicy.subjects = [...(skillAccessPolicy.subjects ?? []), ...newSubjects];
	}}
/>

<Confirm
	msg={`Delete ${skillAccessPolicy.displayName || 'this policy'}?`}
	show={deletingPolicy}
	onsuccess={async () => {
		if (!skillAccessPolicy.id) return;
		saving = true;
		await AdminService.deleteSkillAccessPolicy(skillAccessPolicy.id);
		goto('/admin/skill-access-policies');
	}}
	oncancel={() => (deletingPolicy = false)}
/>
