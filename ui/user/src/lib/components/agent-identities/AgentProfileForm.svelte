<script lang="ts">
	import SearchUsers from '$lib/components/admin/SearchUsers.svelte';
	import SubjectName from '$lib/components/admin/SubjectName.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { httpErrorReason } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import type { AccessControlRuleSubject, OrgGroup, OrgUser } from '$lib/services';
	import { profileError, type ProfileErrorField } from '$lib/services/agent-identities/helpers';
	import type { AgentProfile } from '$lib/services/agent-identities/types';
	import {
		convertSubjectsToTableData,
		resolveSubjectPickerById,
		resolveSubjects,
		subjectsFromPicker
	} from '$lib/subjectResolver';
	import { Plus, Trash2 } from '@lucide/svelte';
	import { untrack } from 'svelte';

	interface Props {
		// The entry being edited, or none for a new one.
		entry?: AgentProfile;
		// Saves the entry. It throws when the server refuses it.
		onSave: (entry: AgentProfile) => Promise<void>;
		onCancel: () => void;
	}

	let { entry, onSave, onCancel }: Props = $props();

	let name = $state(untrack(() => entry?.name ?? ''));
	let subjects = $state<AccessControlRuleSubject[]>(untrack(() => [...(entry?.subjects ?? [])]));
	let saving = $state(false);
	let showValidation = $state(false);
	let serverError = $state<{ field?: ProfileErrorField; text: string }>();
	let usersAndGroups = $state<{ users: OrgUser[]; groups: OrgGroup[] }>();
	let addSubjects = $state<ReturnType<typeof SearchUsers>>();

	// Users and groups are resolved by ID, as the policy forms do, and only the ones attached here are fetched.
	$effect(() => {
		if (subjects.length === 0) return;
		const controller = new AbortController();
		resolveSubjects(
			subjects,
			untrack(() => usersAndGroups),
			{ signal: controller.signal }
		)
			.then((resolved) => {
				if (!controller.signal.aborted) usersAndGroups = resolved;
			})
			.catch((error) => {
				if (!controller.signal.aborted) console.error('Failed to load users and groups:', error);
			});
		return () => controller.abort();
	});

	const subjectRows = $derived(
		convertSubjectsToTableData(subjects, usersAndGroups?.users ?? [], usersAndGroups?.groups ?? [])
	);

	const nameMissing = $derived(showValidation && !name.trim());
	const subjectsMissing = $derived(showValidation && subjects.length === 0);

	async function save(e?: SubmitEvent) {
		e?.preventDefault();
		showValidation = true;
		serverError = undefined;
		if (!name.trim() || subjects.length === 0) return;

		saving = true;
		try {
			await onSave({
				...(entry?.id ? { id: entry.id } : {}),
				name: name.trim(),
				subjects
			});
		} catch (error) {
			const reason = httpErrorReason(error);
			serverError = reason
				? profileError(reason)
				: { text: m.identity_access_agent_profiles_failed() };
		} finally {
			saving = false;
		}
	}
</script>

{#snippet fieldError(field: ProfileErrorField)}
	{#if serverError?.field === field}
		<p class="text-error text-xs" role="alert">{serverError.text}</p>
	{/if}
{/snippet}

<form class="paper flex flex-col gap-6 p-4 text-sm" onsubmit={save}>
	<h2 class="text-lg font-semibold">
		{entry
			? m.identity_access_agent_profiles_edit_title()
			: m.identity_access_agent_profiles_add_title()}
	</h2>

	<label class="flex flex-col gap-1" for="agent-profile-name">
		<span class="input-label">
			{m.core_name()}
			{#if nameMissing}
				<span class="text-error text-xs">{m.identity_access_api_keys_name_is_required()}</span>
			{/if}
		</span>
		<input id="agent-profile-name" class="text-input-filled" bind:value={name} disabled={saving} />
		{@render fieldError('name')}
	</label>

	<div class="flex flex-col gap-2">
		<div class="flex items-center justify-between gap-2">
			<div class="flex flex-col gap-1">
				<span class="input-label">
					{m.identity_access_agent_profiles_subjects_label()}
					{#if subjectsMissing}
						<span class="text-error text-xs"
							>{m.identity_access_agent_profiles_subjects_required()}</span
						>
					{/if}
				</span>
				<p class="input-description">{m.identity_access_agent_profiles_subjects_description()}</p>
			</div>
			<button
				type="button"
				class="btn btn-primary flex items-center gap-1 text-sm"
				disabled={saving}
				onclick={() => addSubjects?.open()}
			>
				<Plus class="size-4" />
				{m.core_add_user_group()}
			</button>
		</div>
		<Table
			data={subjectRows}
			fields={['displayName', 'type']}
			headers={[
				{ property: 'displayName', title: m.core_name() },
				{ property: 'type', title: m.core_type() }
			]}
			noDataMessage={m.identity_access_agent_profiles_subjects_none()}
		>
			{#snippet onRenderColumn(property, d)}
				{#if property === 'displayName'}
					<SubjectName name={d.displayName} disabled={d.disabled} />
				{:else}
					{d[property as keyof typeof d]}
				{/if}
			{/snippet}
			{#snippet actions(d)}
				<IconButton
					variant="danger"
					disabled={saving}
					onclick={() =>
						(subjects = subjects.filter((subject) => resolveSubjectPickerById(subject) !== d.id))}
					tooltip={{ text: m.core_delete_user_group() }}
				>
					<Trash2 class="size-4" />
				</IconButton>
			{/snippet}
		</Table>
		{@render fieldError('subjects')}
	</div>

	{#if serverError && !serverError.field}
		<p class="text-error text-xs" role="alert">{serverError.text}</p>
	{/if}

	<div class="flex justify-end gap-2">
		<button class="btn btn-secondary text-sm" type="button" disabled={saving} onclick={onCancel}>
			{m.common_cancel()}
		</button>
		<button class="btn btn-primary text-sm" type="submit" disabled={saving}>
			{#if saving}
				<Loading class="size-4" />
			{:else}
				{m.core_save()}
			{/if}
		</button>
	</div>
</form>

<!-- Agents are never offered: an agent can't be acted for. All Obot Users must stand alone. -->
<SearchUsers
	bind:this={addSubjects}
	wildcardMustBeAlone
	filterIds={subjects.map(resolveSubjectPickerById)}
	onAdd={(users: OrgUser[], groups: OrgGroup[]) => {
		subjects = [
			...subjects,
			...subjectsFromPicker(subjects.map(resolveSubjectPickerById), users, groups)
		];
	}}
/>
