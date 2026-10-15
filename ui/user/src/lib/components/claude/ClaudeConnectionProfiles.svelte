<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { m } from '$lib/i18n';
	import { type OrgGroup, type OrgUser } from '$lib/services';
	import { doGet, doPut } from '$lib/services/http';
	import { success } from '$lib/stores/success';
	import { convertSubjectsToTableData, resolveSubjects } from '$lib/subjectResolver';
	import type { ClaudeConfig, ClaudeEnvironment, EnvironmentProfile } from './types';
	import { Plus, Trash2, UsersRound } from '@lucide/svelte';
	import { untrack } from 'svelte';

	let {
		environment,
		onUpdated,
		onCreate,
		onEdit
	}: {
		environment: ClaudeEnvironment;
		onUpdated: (profiles: EnvironmentProfile[]) => void;
		onCreate: () => void;
		onEdit: (profile: EnvironmentProfile) => void;
	} = $props();

	let profiles = $state<EnvironmentProfile[]>([]);
	let saving = $state(false);
	let error = $state('');
	let directoryUsers = $state<OrgUser[]>([]);
	let resolvedGroups = $state<OrgGroup[]>([]);
	let confirmDelete = $state<string>();
	let loadedId = $state<string | undefined>(undefined);

	$effect(() => {
		const id = environment.id;
		if (untrack(() => loadedId) === id) return;
		loadedId = id;
		profiles = structuredClone($state.snapshot(untrack(() => environment.profiles) ?? []));
		error = '';
	});

	$effect(() => {
		const subjects = profiles.flatMap((profile) => profile.subjects);
		if (!subjects.some((subject) => subject.type !== 'selector')) return;

		const controller = new AbortController();
		resolveSubjects(
			subjects,
			untrack(() => ({
				users: directoryUsers.length > 0 ? directoryUsers : undefined,
				groups: resolvedGroups
			})),
			{ signal: controller.signal }
		)
			.then((resolved) => {
				if (controller.signal.aborted) return;
				directoryUsers = resolved.users;
				rememberResolvedGroups(resolved.groups);
			})
			.catch((err) => {
				if (controller.signal.aborted) return;
				console.error('Failed to resolve identities:', err);
			});
		return () => controller.abort();
	});

	function subjectLabel(profile: EnvironmentProfile) {
		const rows = convertSubjectsToTableData(profile.subjects, directoryUsers, resolvedGroups);
		return rows.map((row) => row.displayName).join(', ');
	}

	function rememberResolvedGroups(groups: OrgGroup[]) {
		if (groups.length === 0) return;
		// eslint-disable-next-line svelte/prefer-svelte-reactivity
		const next = new Map(resolvedGroups.map((group) => [group.id, group]));
		let changed = false;
		for (const group of groups) {
			if (next.get(group.id)?.name === group.name) continue;
			next.set(group.id, group);
			changed = true;
		}
		if (changed) resolvedGroups = [...next.values()];
	}

	async function persist(next: EnvironmentProfile[]) {
		if (!environment.id) {
			profiles = next;
			onUpdated(next);
			return true;
		}
		saving = true;
		error = '';
		try {
			const latest = (await doGet('/claude-environments')) as ClaudeConfig;
			const current = latest.environments?.find((item) => item.id === environment.id);
			if (!current) throw new Error(m.hosted_agents_env_not_found());
			await doPut(`/claude-environments/${current.id}`, {
				...structuredClone(current),
				organizationID: undefined,
				environmentID: undefined,
				profiles: next
			});
			profiles = next;
			onUpdated(next);
			return true;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
			return false;
		} finally {
			saving = false;
		}
	}

	async function deleteProfile(name: string) {
		if (saving) return;
		const next = profiles.filter((profile) => profile.name !== name);
		if (!(await persist(next))) return;
		success.add(m.hosted_agents_profile_deleted({ name }));
		confirmDelete = undefined;
	}
</script>

<div class="pt-1 @container">
	{#if error}<p role="alert" class="text-error mb-3 text-sm">{error}</p>{/if}
	<p class="text-muted-content mt-2 mb-4 text-sm font-light">
		{m.hosted_agents_profiles_help()}
	</p>
	{#if profiles.length > 0}
		<div class="grid grid-cols-1 @2xl:grid-cols-2 @4xl:grid-cols-3 gap-4">
			{#each profiles as profile (profile.name)}
				{@const subjects = subjectLabel(profile)}
				<article
					class="group border-base-300 bg-base-100 dark:border-base-400 dark:bg-base-300 hover:border-primary/80 dark:hover:border-primary/80 relative rounded-xl border p-4 shadow-sm transition min-h-32"
				>
					<button
						type="button"
						class="absolute inset-0 rounded-xl"
						aria-label={m.hosted_agents_profile_edit_named({ name: profile.name })}
						onclick={() => onEdit(profile)}
					></button>
					<div class="pointer-events-none relative">
						<div class="flex items-start justify-between gap-3">
							<div class="min-w-0">
								<h3 class="truncate font-semibold">{profile.name}</h3>
								<p class="text-muted-content mt-1 flex items-center gap-1.5 text-xs">
									<UsersRound class="size-3 shrink-0" />
									<span class="truncate">
										{subjects || m.hosted_agents_profile_no_subjects()}
									</span>
								</p>
							</div>
							<IconButton
								variant="danger"
								class="pointer-events-auto"
								disabled={saving}
								tooltip={{ text: m.hosted_agents_profile_remove(), placement: 'bottom' }}
								onclick={() => (confirmDelete = profile.name)}
							>
								<Trash2 class="size-4" />
							</IconButton>
						</div>
					</div>
				</article>
			{/each}
		</div>
	{:else}
		<div class="flex flex-col items-center justify-center text-center">
			<div
				class="bg-primary/10 text-primary mb-4 flex size-9 items-center justify-center rounded-md"
			>
				<UsersRound class="size-4" />
			</div>
			<h2 class="font-semibold">{m.hosted_agents_profile_empty()}</h2>
			<p class="text-muted-content mt-1 max-w-sm text-xs">{m.hosted_agents_profiles_help()}</p>
			<button class="btn btn-primary btn-sm mt-5" type="button" onclick={onCreate}>
				<Plus class="size-4" />
				{m.hosted_agents_profile_create()}
			</button>
		</div>
	{/if}
</div>

<Confirm
	show={confirmDelete !== undefined}
	onsuccess={() => {
		if (confirmDelete) void deleteProfile(confirmDelete);
	}}
	oncancel={() => (confirmDelete = undefined)}
	msg={m.hosted_agents_profile_delete_question({ name: confirmDelete ?? '' })}
	loading={saving}
	title={m.hosted_agents_profile_remove()}
/>
