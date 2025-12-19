<script lang="ts">
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { AdminService } from '$lib/services';
	import {
		type ModelAccessPolicy,
		type ModelAccessPolicyManifest,
		type ModelResource,
		type AccessControlRuleSubject,
		type OrgUser,
		type OrgGroup
	} from '$lib/services/admin/types';
	import type { Model } from '$lib/services/chat/types';
	import { LoaderCircle, Plus, Trash2 } from 'lucide-svelte';
	import { onMount, untrack } from 'svelte';
	import { fly } from 'svelte/transition';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Table from '../table/Table.svelte';
	import SearchUsers from './SearchUsers.svelte';
	import Confirm from '../Confirm.svelte';
	import { goto } from '$lib/url';
	import SearchModels from './SearchModels.svelte';
	import { getUserDisplayName } from '$lib/utils';

	interface Props {
		modelAccessPolicy?: ModelAccessPolicy;
		onCreate?: (modelAccessPolicy: ModelAccessPolicy) => void;
		onUpdate?: (modelAccessPolicy: ModelAccessPolicy) => void;
		readonly?: boolean;
	}

	let {
		modelAccessPolicy: initialModelAccessPolicy,
		onCreate,
		onUpdate,
		readonly
	}: Props = $props();

	const duration = PAGE_TRANSITION_DURATION;
	let modelAccessPolicy = $state(
		untrack(
			() =>
				initialModelAccessPolicy ??
				({
					displayName: '',
					subjects: [],
					models: []
				} as ModelAccessPolicyManifest)
		)
	);

	let saving = $state<boolean | undefined>();
	let usersAndGroups = $state<{ users: OrgUser[]; groups: OrgGroup[] }>();
	let loadingUsersAndGroups = $state(false);
	let models = $state<Model[]>([]);

	let addUserGroupDialog = $state<ReturnType<typeof SearchUsers>>();
	let addModelDialog = $state<ReturnType<typeof SearchModels>>();

	let deletingPolicy = $state(false);

	onMount(async () => {
		models = await AdminService.listModels();
	});

	let modelsMap = $derived(new Map(models.map((m) => [m.id, m])));

	let modelsTableData = $derived.by(() => {
		if (modelsMap) {
			return convertModelsToTableData(modelAccessPolicy.models ?? []);
		}
		return [];
	});

	$effect(() => {
		// Prevent loading users and groups if rule has no subjects
		if (!modelAccessPolicy.subjects || modelAccessPolicy.subjects?.length === 0) {
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
			// Include restricted groups in the results so that groups added to polcies before the group
			// restriction was configured are still visible in the UI.
			promises[1] = AdminService.listGroups({ includeRestricted: true });
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
						type: 'Group'
					};
				})
				.filter((subject) => subject !== undefined) ?? []
		);
	}

	function convertModelsToTableData(models: ModelResource[]) {
		return models.map((model) => {
			if (model.id === '*') {
				return {
					id: model.id,
					name: 'All Models',
					provider: '-'
				};
			}

			const m = modelsMap.get(model.id);
			return {
				id: model.id,
				name: m?.displayName || m?.name || model.id,
				provider: m?.modelProviderName || '-'
			};
		});
	}

	function validate(policy: typeof modelAccessPolicy) {
		if (!policy) return false;

		return policy.displayName.length > 0;
	}
</script>

<div
	class="flex h-full w-full flex-col gap-4"
	out:fly={{ x: 100, duration }}
	in:fly={{ x: 100, delay: duration }}
>
	<div class="flex grow flex-col gap-4" out:fly={{ x: -100, duration }} in:fly={{ x: -100 }}>
		{#if modelAccessPolicy.id}
			<div class="flex w-full items-center justify-between gap-4">
				<div class="flex items-center gap-2">
					<h1 class="flex items-center gap-4 text-2xl font-semibold">
						{modelAccessPolicy.displayName}
					</h1>
				</div>
				{#if !readonly}
					<button
						class="button-destructive flex items-center gap-1 text-xs font-normal"
						use:tooltip={'Delete Policy'}
						onclick={() => {
							deletingPolicy = true;
						}}
					>
						<Trash2 class="size-4" />
					</button>
				{/if}
			</div>
		{/if}

		{#if !modelAccessPolicy.id}
			<div
				class="dark:bg-surface2 dark:border-surface3 bg-background rounded-lg border border-transparent p-4"
			>
				<div class="flex flex-col gap-6">
					<div class="flex flex-col gap-2">
						<label for="model-access-policy-name" class="flex-1 text-sm font-light capitalize">
							Name
						</label>
						<input
							id="model-access-policy-name"
							bind:value={modelAccessPolicy.displayName}
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
							<button class="button-primary flex items-center gap-1 text-sm" disabled>
								<Plus class="size-4" /> Add User/Group
							</button>
						{:else}
							<button
								class="button-primary flex items-center gap-1 text-sm"
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
					<LoaderCircle class="size-6 animate-spin" />
				</div>
			{:else}
				{@const tableData = convertSubjectsToTableData(
					modelAccessPolicy.subjects ?? [],
					usersAndGroups?.users ?? [],
					usersAndGroups?.groups ?? []
				)}
				<Table
					data={tableData}
					fields={['displayName', 'type']}
					headers={[{ property: 'displayName', title: 'Name' }]}
					noDataMessage="No users or groups added."
				>
					{#snippet actions(d)}
						{#if !readonly}
							<button
								class="icon-button hover:text-red-500"
								onclick={() => {
									modelAccessPolicy.subjects = modelAccessPolicy.subjects?.filter(
										(subject) => subject.id !== d.id
									);
								}}
								use:tooltip={'Delete User/Group'}
							>
								<Trash2 class="size-4" />
							</button>
						{/if}
					{/snippet}
				</Table>
			{/if}
		</div>

		<div class="flex flex-col gap-2">
			<div class="mb-2 flex items-center justify-between">
				<h2 class="text-lg font-semibold">Models</h2>
				{#if !readonly}
					<div class="relative flex items-center gap-4">
						<button
							class="button-primary flex items-center gap-1 text-sm"
							onclick={() => {
								addModelDialog?.open();
							}}
						>
							<Plus class="size-4" /> Add Model
						</button>
					</div>
				{/if}
			</div>
			<Table
				data={modelsTableData}
				fields={['name', 'provider']}
				headers={[
					{ property: 'name', title: 'Model' },
					{ property: 'provider', title: 'Provider' }
				]}
				noDataMessage="No models added."
			>
				{#snippet actions(d)}
					{#if !readonly}
						<button
							class="icon-button hover:text-red-500"
							onclick={() => {
								modelAccessPolicy.models =
									modelAccessPolicy.models?.filter((m) => m.id !== d.id) ?? [];
							}}
							use:tooltip={'Remove Model'}
						>
							<Trash2 class="size-4" />
						</button>
					{/if}
				{/snippet}
			</Table>
		</div>
	</div>
	{#if !readonly}
		<div
			class="bg-surface1 text-on-surface1 dark:bg-background sticky bottom-0 left-0 flex w-full justify-end gap-2 py-4"
			out:fly={{ x: -100, duration }}
			in:fly={{ x: -100 }}
		>
			<div class="flex w-full justify-end gap-2">
				{#if !modelAccessPolicy.id}
					<button
						class="button text-sm"
						onclick={() => {
							goto('/admin/model-access');
						}}
					>
						Cancel
					</button>
					<button
						class="button-primary text-sm"
						disabled={!validate(modelAccessPolicy) || saving}
						onclick={async () => {
							saving = true;
							const response = await AdminService.createModelAccessPolicy(modelAccessPolicy);
							modelAccessPolicy = response;
							onCreate?.(response);
							saving = false;
						}}
					>
						{#if saving}
							<LoaderCircle class="size-4 animate-spin" />
						{:else}
							Save
						{/if}
					</button>
				{:else}
					<button
						class="button-primary text-sm"
						disabled={!validate(modelAccessPolicy) || saving}
						onclick={async () => {
							if (!modelAccessPolicy.id) return;
							saving = true;
							const response = await AdminService.updateModelAccessPolicy(
								modelAccessPolicy.id,
								modelAccessPolicy
							);
							modelAccessPolicy = response;
							onUpdate?.(response);
							saving = false;
						}}
					>
						{#if saving}
							<LoaderCircle class="size-4 animate-spin" />
						{:else}
							Update
						{/if}
					</button>
				{/if}
			</div>
		</div>
	{/if}
</div>

<SearchUsers
	bind:this={addUserGroupDialog}
	filterIds={modelAccessPolicy.subjects?.map((subject) => subject.id) ?? []}
	onAdd={async (users: OrgUser[], groups: OrgGroup[]) => {
		const existingSubjectIds = new Set(
			modelAccessPolicy.subjects?.map((subject) => subject.id) ?? []
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
		modelAccessPolicy.subjects = [...(modelAccessPolicy.subjects ?? []), ...newSubjects];
	}}
/>

<SearchModels
	bind:this={addModelDialog}
	exclude={modelAccessPolicy.models?.map((m) => m.id) ?? []}
	onAdd={async (modelIds: string[]) => {
		const existingModelIds = new Set(modelAccessPolicy.models?.map((m) => m.id) ?? []);
		const newModels = modelIds.filter((id) => !existingModelIds.has(id)).map((id) => ({ id: id }));

		modelAccessPolicy.models = [...(modelAccessPolicy.models ?? []), ...newModels];
	}}
/>

<Confirm
	msg="Are you sure you want to delete this policy?"
	show={deletingPolicy}
	onsuccess={async () => {
		if (!modelAccessPolicy.id) return;
		saving = true;
		await AdminService.deleteModelAccessPolicy(modelAccessPolicy.id);
		goto('/admin/model-access-policies');
	}}
	oncancel={() => (deletingPolicy = false)}
/>
