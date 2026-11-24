<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { httpErrorReason } from '$lib/errors';
	import { m } from '$lib/i18n';
	import type { OrgGroup, OrgUser } from '$lib/services';
	import type { AgentIdentity, AgentProfile } from '$lib/services/agent-identities/types';
	import { convertSubjectsToTableData, resolveSubjects } from '$lib/subjectResolver';
	import AgentProfileForm from './AgentProfileForm.svelte';
	import AgentReturnHosts from './AgentReturnHosts.svelte';
	import { Plus } from '@lucide/svelte';
	import { untrack } from 'svelte';

	interface Props {
		agent: AgentIdentity;
		readonly?: boolean;
		// Saves a change to the agent. An update replaces the whole manifest, so the page sends back what isn't being
		// changed. It throws when the server refuses the change.
		onSave: (change: { profiles?: AgentProfile[]; allowedReturnHosts?: string[] }) => Promise<void>;
	}

	let { agent, readonly, onSave }: Props = $props();

	// The entry in the form: a new one, or one being edited. Neither when the list is shown.
	let editing = $state<{ entry?: AgentProfile }>();
	let deleting = $state<AgentProfile>();
	let busy = $state(false);
	// Why the last delete failed, shown under the list.
	let deleteError = $state<string>();
	let usersAndGroups = $state<{ users: OrgUser[]; groups: OrgGroup[] }>();

	const entries = $derived(agent.profiles ?? []);

	// The subjects of every entry, to name them in the list.
	$effect(() => {
		const subjects = entries.flatMap((entry) => entry.subjects);
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

	const rows = $derived(
		entries.map((entry) => {
			const names = convertSubjectsToTableData(
				entry.subjects,
				usersAndGroups?.users ?? [],
				usersAndGroups?.groups ?? []
			).map((row) => row.displayName);
			return {
				id: entry.id ?? entry.name,
				name: entry.name,
				subjects:
					names.length > 2
						? `${names.slice(0, 2).join(', ')} +${names.length - 2}`
						: names.join(', ')
			};
		})
	);

	function openForm(entry?: AgentProfile) {
		deleteError = undefined;
		editing = { entry };
	}

	async function saveEntry(entry: AgentProfile) {
		const current = editing?.entry;
		const next = current?.id
			? entries.map((candidate) => (candidate.id === current.id ? entry : candidate))
			: [...entries, entry];
		await onSave({ profiles: next });
		editing = undefined;
	}

	async function deleteEntry() {
		const target = deleting;
		if (!target) return;
		busy = true;
		deleteError = undefined;
		try {
			await onSave({ profiles: entries.filter((candidate) => candidate.id !== target.id) });
		} catch (error) {
			deleteError = httpErrorReason(error) || m.identity_access_agent_profiles_failed();
		} finally {
			busy = false;
			deleting = undefined;
		}
	}
</script>

{#if editing}
	<AgentProfileForm
		entry={editing.entry}
		onSave={saveEntry}
		onCancel={() => (editing = undefined)}
	/>
{:else}
	<div class="flex flex-col gap-4">
		<section class="paper flex flex-col gap-3 p-4" aria-labelledby="agent-profiles-title">
			<div class="flex flex-wrap items-start justify-between gap-2">
				<div class="flex max-w-3xl flex-col gap-1">
					<h2 id="agent-profiles-title" class="text-lg font-semibold">
						{m.identity_access_agent_profiles_tab()}
					</h2>
					<p class="text-muted-content text-sm font-light">
						{m.identity_access_agent_profiles_description()}
					</p>
				</div>
				{#if !readonly && agent.status !== 'deleted'}
					<button class="btn btn-primary text-sm" onclick={() => openForm()}>
						<Plus class="size-4" />
						{m.identity_access_agent_profiles_add()}
					</button>
				{/if}
			</div>

			{#if rows.length === 0}
				<p class="text-muted-content py-4 text-center text-sm">
					{m.identity_access_agent_profiles_none()}
				</p>
			{:else}
				<Table
					data={rows}
					fields={['name', 'subjects']}
					headers={[
						{ title: m.core_name(), property: 'name' },
						{ title: m.identity_access_agent_profiles_col_subjects(), property: 'subjects' }
					]}
				>
					{#snippet actions(d)}
						{#if !readonly}
							<div class="flex gap-1">
								<button
									class="btn btn-secondary btn-sm"
									disabled={busy}
									onclick={() =>
										openForm(entries.find((entry) => (entry.id ?? entry.name) === d.id))}
								>
									{m.identity_access_agent_profiles_edit()}
								</button>
								<button
									class="btn btn-error btn-soft btn-sm"
									disabled={busy}
									onclick={() =>
										(deleting = entries.find((entry) => (entry.id ?? entry.name) === d.id))}
								>
									{m.core_delete()}
								</button>
							</div>
						{/if}
					{/snippet}
				</Table>
			{/if}
			{#if deleteError}
				<p class="text-error text-xs" role="alert">{deleteError}</p>
			{/if}
		</section>

		<AgentReturnHosts
			hosts={agent.allowedReturnHosts ?? []}
			{readonly}
			onSave={(hosts) => onSave({ allowedReturnHosts: hosts })}
		/>
	</div>
{/if}

<Confirm
	msg={m.identity_access_agent_profiles_confirm_delete({ name: deleting?.name ?? '' })}
	note={m.identity_access_agent_profiles_confirm_delete_note()}
	show={Boolean(deleting)}
	loading={busy}
	onsuccess={deleteEntry}
	oncancel={() => (deleting = undefined)}
/>
