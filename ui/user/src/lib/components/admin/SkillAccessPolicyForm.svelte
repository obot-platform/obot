<script lang="ts">
	import { FORM_PARENT_ROUTES, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { FormValidation } from '$lib/form.svelte';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		type OrgUser,
		type OrgGroup,
		type SkillAccessPolicy,
		type SkillAccessPolicyResource,
		type SkillRepository,
		type Skill
	} from '$lib/services';
	import { notifications } from '$lib/stores';
	import { goto } from '$lib/url';
	import {
		convertSubjectsToTableData,
		resolveSubjects,
		resolveSubjectFromGroup,
		resolveSubjectPickerById
	} from '../../subjectResolver';
	import Confirm from '../Confirm.svelte';
	import FieldError from '../FieldError.svelte';
	import RequiredMark from '../RequiredMark.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import Table from '../table/Table.svelte';
	import SearchSkills from './SearchSkills.svelte';
	import SearchUsers from './SearchUsers.svelte';
	import SubjectName from './SubjectName.svelte';
	import { Plus, Trash2 } from '@lucide/svelte';
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

	onMount(async () => {
		try {
			skills = await AdminService.listAllSkills();
			skillRepositories = await AdminService.listSkillRepositories();
		} catch (error) {
			notifications.add(`Failed to load skills or skill repositories: ${error}`);
		} finally {
			loadingSkills = false;
		}
	});

	let skillsMap = $derived(new Map(skills.map((s) => [s.id, s])));
	let skillRepositoriesMap = $derived(new Map(skillRepositories.map((s) => [s.id, s])));

	$effect(() => {
		// Prevent loading users and groups if rule has no subjects
		if (!skillAccessPolicy.subjects || skillAccessPolicy.subjects?.length === 0) {
			loadingUsersAndGroups = false;
			return;
		}

		loadingUsersAndGroups = true;

		// Groups are resolved by ID, not listed: the directory can hold tens of thousands of them and
		// only the ones attached here are needed.
		const controller = new AbortController();

		resolveSubjects(
			skillAccessPolicy.subjects,
			untrack(() => usersAndGroups),
			{ signal: controller.signal }
		)
			.then((resolved) => {
				if (controller.signal.aborted) return;
				usersAndGroups = resolved;
				loadingUsersAndGroups = false;
			})
			.catch((error) => {
				if (controller.signal.aborted) return;
				console.error('Failed to load users and groups:', error);
				loadingUsersAndGroups = false;
			});

		return () => controller.abort();
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
							name: resource.id === '*' ? m.skills_access_policies_all_skills() : resource.id,
							description: '',
							type: 'Selector'
						};
					}
					return undefined;
				})
				.filter((resource) => resource !== undefined) ?? []
		);
	}

	const validation = new FormValidation<'name' | 'subjects' | 'resources'>(() => ({
		...(!skillAccessPolicy.displayName.trim() && { name: m.core_name_required() }),
		...(!skillAccessPolicy.subjects?.length && { subjects: m.core_users_groups_required() }),
		...(!skillAccessPolicy.resources?.length && {
			resources: m.skills_access_policy_skills_required()
		})
	}));

	function cancel() {
		goto(FORM_PARENT_ROUTES.skillAccessPolicies);
	}

	async function save() {
		if (!(await validation.check())) return;
		saving = true;
		try {
			if (skillAccessPolicy.id) {
				const response = await AdminService.updateSkillAccessPolicy(
					skillAccessPolicy.id,
					skillAccessPolicy
				);
				skillAccessPolicy = response;
				onUpdate?.(response);
			} else {
				const response = await AdminService.createSkillAccessPolicy(skillAccessPolicy);
				skillAccessPolicy = response;
				onCreate?.(response);
			}
		} finally {
			saving = false;
		}
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
	{@attach validation.attach}
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
						tooltip={{ text: m.core_delete_policy() }}
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
						<label
							for="skill-access-policy-name"
							class="flex-1 text-sm font-light capitalize"
							class:error={validation.has('name')}
						>
							{m.core_name()}
							<RequiredMark invalid={validation.has('name')} />
						</label>
						<input
							{...validation.attrs('name', 'skill-access-policy-name')}
							aria-required="true"
							bind:value={skillAccessPolicy.displayName}
							class="text-input-filled mt-0.5"
							class:error={validation.has('name')}
							disabled={readonly}
						/>
						<FieldError id="skill-access-policy-name" message={validation.message('name')} />
					</div>
				</div>
			</div>
		{/if}

		<div class="flex flex-col gap-2" data-invalid={validation.has('subjects') ? '' : undefined}>
			<div class="mb-2 flex items-center justify-between">
				<div class="flex flex-col">
					<h2 class="text-lg font-semibold" class:text-error={validation.has('subjects')}>
						{m.core_users_and_groups()}
						<RequiredMark invalid={validation.has('subjects')} textMessage={m.core_required_sr()} />
					</h2>
					<FieldError id="skill-access-policy-subjects" message={validation.message('subjects')} />
				</div>
				{#if !readonly}
					<div class="relative flex items-center gap-4">
						{#if loadingUsersAndGroups}
							<button class="btn btn-primary flex items-center gap-1 text-sm" disabled>
								<Plus class="size-4" />
								{m.core_add_user_group()}
							</button>
						{:else}
							<button
								class="btn btn-primary flex items-center gap-1 text-sm"
								onclick={() => {
									addUserGroupDialog?.open();
								}}
							>
								<Plus class="size-4" />
								{m.core_add_user_group()}
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
					headers={[
						{ property: 'displayName', title: m.core_name() },
						{ property: 'type', title: m.core_type() }
					]}
					noDataMessage={m.core_no_users_or_groups_added()}
				>
					{#snippet onRenderColumn(property, d)}
						{#if property === 'displayName'}
							<SubjectName name={d.displayName} disabled={d.disabled} />
						{:else}
							{d[property as keyof typeof d]}
						{/if}
					{/snippet}
					{#snippet actions(d)}
						{#if !readonly}
							<IconButton
								variant="danger"
								onclick={() => {
									skillAccessPolicy.subjects = skillAccessPolicy.subjects?.filter(
										(subject) => resolveSubjectPickerById(subject) !== d.id
									);
								}}
								tooltip={{ text: m.core_delete_user_group() }}
							>
								<Trash2 class="size-4" />
							</IconButton>
						{/if}
					{/snippet}
				</Table>
			{/if}
		</div>

		<div class="flex flex-col gap-2" data-invalid={validation.has('resources') ? '' : undefined}>
			<div class="mb-2 flex items-center justify-between">
				<div class="flex flex-col">
					<h2 class="text-lg font-semibold" class:text-error={validation.has('resources')}>
						{m.skills_title()}
						<RequiredMark
							invalid={validation.has('resources')}
							textMessage={m.core_required_sr()}
						/>
					</h2>
					<FieldError
						id="skill-access-policy-resources"
						message={validation.message('resources')}
					/>
				</div>
				{#if !readonly}
					<button
						class="btn btn-primary flex items-center gap-1 text-sm"
						onclick={() => {
							addSkillDialog?.open();
						}}
					>
						<Plus class="size-4" />
						{m.skills_access_policies_add_skill()}
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
						{ property: 'name', title: m.skills_skill_fallback_title() },
						{ property: 'description', title: m.core_description() }
					]}
					noDataMessage={m.skills_access_policies_no_skills_added()}
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
								tooltip={{ text: m.skills_access_policies_remove_skill() }}
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
				<button class="btn btn-secondary text-sm" disabled={saving} onclick={cancel}>
					{m.common_cancel()}
				</button>
				<button class="btn btn-primary text-sm" disabled={saving} onclick={save}>
					{#if saving}
						<Loading class="size-4" />
					{:else if skillAccessPolicy.id}
						{m.core_update()}
					{:else}
						{m.core_save()}
					{/if}
				</button>
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
	filterIds={skillAccessPolicy.subjects?.map(resolveSubjectPickerById) ?? []}
	onAdd={async (users: OrgUser[], groups: OrgGroup[]) => {
		const existingSubjectIds = new Set(
			skillAccessPolicy.subjects?.map(resolveSubjectPickerById) ?? []
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
				.map((group: OrgGroup) => resolveSubjectFromGroup(group))
		];
		skillAccessPolicy.subjects = [...(skillAccessPolicy.subjects ?? []), ...newSubjects];
	}}
/>

<Confirm
	msg={skillAccessPolicy.displayName
		? m.core_delete_named_component({ name: skillAccessPolicy.displayName })
		: m.core_delete_this_policy()}
	show={deletingPolicy}
	onsuccess={async () => {
		if (!skillAccessPolicy.id) return;
		saving = true;
		await AdminService.deleteSkillAccessPolicy(skillAccessPolicy.id);
		goto(FORM_PARENT_ROUTES.skillAccessPolicies);
	}}
	oncancel={() => (deletingPolicy = false)}
/>
