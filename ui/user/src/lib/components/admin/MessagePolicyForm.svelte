<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		PolicyDirectionLabels,
		type MessagePolicy,
		type MessagePolicyManifest,
		type OrgUser,
		type OrgGroup,
		type PolicyDirection
	} from '$lib/services';
	import { goto } from '$lib/url';
	import {
		convertSubjectsToTableData,
		resolveSubjects,
		resolveSubjectFromGroup,
		resolveSubjectPickerById
	} from '../../subjectResolver';
	import Confirm from '../Confirm.svelte';
	import Select from '../Select.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import Table from '../table/Table.svelte';
	import SearchUsers from './SearchUsers.svelte';
	import SubjectName from './SubjectName.svelte';
	import { CircleQuestionMark, Plus, Trash2 } from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	interface Props {
		messagePolicy?: MessagePolicy;
		onCreate?: (messagePolicy: MessagePolicy) => void;
		onUpdate?: (messagePolicy: MessagePolicy) => void;
		onCancel?: () => void;
		fixedDirection?: PolicyDirection;
		listHref: string;
		readonly?: boolean;
	}

	let {
		messagePolicy: initialMessagePolicy,
		onCreate,
		onUpdate,
		onCancel,
		fixedDirection,
		listHref,
		readonly
	}: Props = $props();

	const duration = PAGE_TRANSITION_DURATION;
	let messagePolicy = $state(
		untrack(
			() =>
				initialMessagePolicy ??
				({
					displayName: '',
					definition: '',
					direction: (fixedDirection ?? 'both') as PolicyDirection,
					subjects: []
				} as MessagePolicyManifest)
		)
	);

	let saving = $state<boolean | undefined>();
	let usersAndGroups = $state<{ users: OrgUser[]; groups: OrgGroup[] }>();
	let loadingUsersAndGroups = $state(false);

	let addUserGroupDialog = $state<ReturnType<typeof SearchUsers>>();

	let deletingPolicy = $state(false);

	let initialPolicyJson = $derived(
		initialMessagePolicy
			? JSON.stringify({
					displayName: initialMessagePolicy.displayName,
					definition: initialMessagePolicy.definition,
					direction: initialMessagePolicy.direction,
					subjects: initialMessagePolicy.subjects
				})
			: ''
	);

	let hasChanges = $derived(
		!initialPolicyJson ||
			JSON.stringify({
				displayName: messagePolicy.displayName,
				definition: messagePolicy.definition,
				direction: messagePolicy.direction,
				subjects: messagePolicy.subjects
			}) !== initialPolicyJson
	);

	$effect(() => {
		if (!messagePolicy.subjects || messagePolicy.subjects?.length === 0) {
			loadingUsersAndGroups = false;
			return;
		}

		loadingUsersAndGroups = true;

		// Groups are resolved by ID, not listed: the directory can hold tens of thousands of them and
		// only the ones attached here are needed.
		const controller = new AbortController();

		resolveSubjects(
			messagePolicy.subjects,
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

	const directionOptions = (['user-message', 'tool-calls', 'both'] as const).map((id) => ({
		id,
		label: PolicyDirectionLabels[id]
	}));

	function validate(policy: typeof messagePolicy) {
		if (!policy) return false;

		return (
			policy.displayName.length > 0 &&
			policy.definition.length > 0 &&
			(['user-message', 'tool-calls', 'both'] as PolicyDirection[]).includes(policy.direction) &&
			(policy.subjects?.length ?? 0) > 0
		);
	}
</script>

<div
	class="flex h-full w-full flex-col gap-4"
	out:fly={{ x: 100, duration }}
	in:fly={{ x: 100, delay: duration }}
>
	<div class="flex grow flex-col gap-4" out:fly={{ x: -100, duration }} in:fly={{ x: -100 }}>
		{#if messagePolicy.id}
			<div class="flex w-full items-center justify-between gap-4">
				<div class="flex items-center gap-2">
					<h1 class="flex items-center gap-4 text-2xl font-semibold">
						{messagePolicy.displayName}
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

		<div class="paper p-4">
			<div class="flex flex-col gap-6">
				{#if !messagePolicy.id}
					<div class="flex flex-col gap-2">
						<label for="message-policy-name" class="flex-1 text-sm font-light capitalize">
							{m.core_name()}
						</label>
						<input
							id="message-policy-name"
							bind:value={messagePolicy.displayName}
							class="text-input-filled mt-0.5"
							disabled={readonly}
						/>
					</div>
				{/if}

				<div class="flex flex-col gap-2">
					<label
						for="message-policy-definition"
						class="flex items-center gap-1 text-sm font-light capitalize"
					>
						{m.ai_judge_definition()}
						<div
							use:tooltip={{
								text: m.ai_judge_definition_tooltip(),
								classes: ['w-72', 'break-normal', 'whitespace-pre-wrap', 'z-[60]']
							}}
						>
							<CircleQuestionMark class="text-muted-content size-3.5" />
						</div>
					</label>
					<textarea
						id="message-policy-definition"
						bind:value={messagePolicy.definition}
						class="text-input-filled mt-0.5 min-h-24 resize-y"
						placeholder={m.ai_judge_definition_placeholder()}
						disabled={readonly}
						rows="3"></textarea>
				</div>

				<div class="flex flex-col gap-1">
					<label for="message-policy-direction" class="flex-1 text-sm font-light capitalize">
						{m.ai_judge_applies_to()}
					</label>
					<Select
						id="message-policy-direction"
						options={directionOptions}
						selected={messagePolicy.direction}
						onSelect={(option) => {
							messagePolicy.direction = option.id as PolicyDirection;
						}}
						disabled
						class="bg-base-200 dark:bg-base-200 dark:border-base-400 flex-1 border border-transparent shadow-inner"
					/>
				</div>
			</div>
		</div>

		<div class="flex flex-col gap-2">
			<div class="mb-2 flex items-center justify-between">
				<h2 class="text-lg font-semibold">{m.core_users_and_groups()}</h2>
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
				{@const tableData = convertSubjectsToTableData(
					messagePolicy.subjects ?? [],
					usersAndGroups?.users ?? [],
					usersAndGroups?.groups ?? []
				)}
				<Table
					data={tableData}
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
									messagePolicy.subjects = messagePolicy.subjects?.filter(
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
	</div>
	{#if !readonly}
		<div
			class="bg-base-200 text-muted-content dark:bg-base-100 sticky bottom-0 left-0 z-50 flex w-full justify-end gap-2 py-4"
			out:fly={{ x: -100, duration }}
			in:fly={{ x: -100 }}
		>
			<div class="flex w-full justify-end gap-2">
				{#if !messagePolicy.id}
					<button
						class="btn btn-secondary text-sm"
						onclick={() => {
							if (onCancel) {
								onCancel();
								return;
							}
							goto(listHref);
						}}
					>
						{m.common_cancel()}
					</button>
					<button
						class="btn btn-primary text-sm"
						disabled={!validate(messagePolicy) || saving}
						onclick={async () => {
							saving = true;
							try {
								const response = await AdminService.createMessagePolicy(messagePolicy);
								messagePolicy = response;
								onCreate?.(response);
							} finally {
								saving = false;
							}
						}}
					>
						{#if saving}
							<Loading class="size-4" />
						{:else}
							{m.core_save()}
						{/if}
					</button>
				{:else}
					<button
						class="btn btn-primary text-sm"
						disabled={!validate(messagePolicy) || !hasChanges || saving}
						onclick={async () => {
							if (!messagePolicy.id) return;
							saving = true;
							try {
								const response = await AdminService.updateMessagePolicy(
									messagePolicy.id,
									messagePolicy
								);
								messagePolicy = response;
								onUpdate?.(response);
							} finally {
								saving = false;
							}
						}}
					>
						{#if saving}
							<Loading class="size-4" />
						{:else}
							{m.core_update()}
						{/if}
					</button>
				{/if}
			</div>
		</div>
	{/if}
</div>

<SearchUsers
	bind:this={addUserGroupDialog}
	filterIds={messagePolicy.subjects?.map(resolveSubjectPickerById) ?? []}
	onAdd={async (users: OrgUser[], groups: OrgGroup[]) => {
		const existingSubjectIds = new Set(messagePolicy.subjects?.map(resolveSubjectPickerById) ?? []);
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
		messagePolicy.subjects = [...(messagePolicy.subjects ?? []), ...newSubjects];
	}}
/>

<Confirm
	msg={messagePolicy.displayName
		? m.core_delete_named_component({ name: messagePolicy.displayName })
		: m.core_delete_this_policy()}
	show={deletingPolicy}
	onsuccess={async () => {
		if (!messagePolicy.id) return;
		saving = true;
		await AdminService.deleteMessagePolicy(messagePolicy.id);
		goto(listHref);
	}}
	oncancel={() => (deletingPolicy = false)}
/>
