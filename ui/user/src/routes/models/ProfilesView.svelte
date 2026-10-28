<script lang="ts">
	import { resolve } from '$app/paths';
	import Confirm from '$lib/components/Confirm.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { m } from '$lib/i18n';
	import { AdminService } from '$lib/services';
	import type { ModelAccessPolicy } from '$lib/services/admin/types';
	import { listBudgets, type Budget } from '$lib/services/llm-gateway/gateway';
	import {
		listGuardrailPolicies,
		type GuardrailPolicy
	} from '$lib/services/llm-gateway/guardrails';
	import { accessibleModels, profile } from '$lib/stores';
	import {
		convertSubjectsToTableData,
		resolveSubjects,
		type ResolvedSubjects
	} from '$lib/subjectResolver';
	import { goto } from '$lib/url';
	import { Plus, Trash2, UsersRound } from '@lucide/svelte';
	import { onMount } from 'svelte';

	let profiles = $state<ModelAccessPolicy[]>([]);
	let budgets = $state<Budget[]>([]);
	let policies = $state<GuardrailPolicy[]>([]);
	let loading = $state(true);
	// The users and groups the profiles name, to show who each profile is for. Until they load,
	// the cards count them.
	let usersAndGroups = $state<ResolvedSubjects>();
	let toDelete = $state<ModelAccessPolicy>();

	let readonly = $derived(profile.current.isAdminReadonly?.());

	async function load() {
		loading = true;
		try {
			[profiles, budgets, policies] = await Promise.all([
				AdminService.listModelAccessPolicies(),
				listBudgets().catch(() => []),
				listGuardrailPolicies().catch(() => [])
			]);
		} finally {
			loading = false;
		}
		resolveSubjects(profiles.flatMap((p) => p.subjects ?? []))
			.then((resolved) => (usersAndGroups = resolved))
			.catch((err) => console.error('Failed to load users and groups:', err));
	}

	// How many users and groups a card names before it says how many more there are; the card's
	// tooltip names them all.
	const SUBJECTS_SHOWN = 3;

	onMount(load);

	function count<T extends { profileID: string }>(items: T[], id: string) {
		return items.filter((i) => i.profileID === id).length;
	}

	function subjectNames(p: ModelAccessPolicy) {
		if (!usersAndGroups) return [];
		return convertSubjectsToTableData(
			p.subjects ?? [],
			usersAndGroups.users,
			usersAndGroups.groups
		).map((row) => row.displayName);
	}

	function subjectsLabel(p: ModelAccessPolicy, names: string[]) {
		const subjects = p.subjects ?? [];
		if (subjects.some((s) => s.type === 'selector' && s.id === '*'))
			return m.models_profiles_everyone();
		if (names.length > 0) {
			const shown = names.slice(0, SUBJECTS_SHOWN).join(', ');
			return names.length > SUBJECTS_SHOWN
				? m.models_profiles_subjects_more({
						names: shown,
						count: names.length - SUBJECTS_SHOWN
					})
				: shown;
		}
		const users = subjects.filter((s) => s.type === 'user').length;
		const groups = subjects.length - users;
		return (
			[
				users &&
					(users === 1
						? m.models_profiles_users_one({ count: users })
						: m.models_profiles_users_other({ count: users })),
				groups &&
					(groups === 1
						? m.models_profiles_groups_one({ count: groups })
						: m.models_profiles_groups_other({ count: groups }))
			]
				.filter(Boolean)
				.join(', ') || m.models_profiles_no_one()
		);
	}

	let cards = $derived(
		profiles.map((p) => {
			const models = p.models?.length ?? 0;
			const budgetCount = count(budgets, p.id);
			const guardrailCount = count(
				policies.filter((r) => r.target === 'llm'),
				p.id
			);
			const names = subjectNames(p);
			return {
				...p,
				subjectsLabel: subjectsLabel(p, names),
				subjectsTitle: names.join('\n'),
				facts: [
					p.models?.some((model) => model.id === '*')
						? m.models_budgets_all_models()
						: models === 1
							? m.models_budgets_models_count_one({ count: models })
							: m.models_budgets_models_count_other({ count: models }),
					budgetCount === 1
						? m.models_profiles_card_budgets_one({ count: budgetCount })
						: m.models_profiles_card_budgets_other({ count: budgetCount }),
					guardrailCount === 1
						? m.models_profiles_card_guardrails_one({ count: guardrailCount })
						: m.models_profiles_card_guardrails_other({ count: guardrailCount })
				]
			};
		})
	);
</script>

<div class="@container">
	{#if loading}
		<Skeleton type="items" count={3} class="h-32" />
	{:else if profiles.length === 0}
		<div class="mt-12 flex flex-col items-center justify-center text-center">
			<div
				class="bg-primary/10 text-primary mb-4 flex size-9 items-center justify-center rounded-md"
			>
				<UsersRound class="size-4" />
			</div>
			<h2 class="font-semibold">{m.models_profiles_empty_title()}</h2>
			<p class="text-muted-content mt-1 max-w-sm text-xs">
				{m.models_profiles_empty_description()}
			</p>
			{#if !readonly}
				<button class="btn btn-primary btn-sm mt-5" onclick={() => goto('/models/profiles/new')}>
					<Plus class="size-4" />
					{m.models_profiles_add()}
				</button>
			{/if}
		</div>
	{:else}
		<p class="text-muted-content mb-4 text-sm font-light">{m.models_profiles_intro()}</p>
		<div class="grid grid-cols-1 gap-4 @2xl:grid-cols-2 @4xl:grid-cols-3">
			{#each cards as card (card.id)}
				<article
					class="group border-base-300 bg-base-100 dark:border-base-400 dark:bg-base-300 hover:border-primary/80 dark:hover:border-primary/80 relative min-h-32 rounded-xl border p-4 shadow-sm transition"
				>
					<a
						href={resolve('/models/profiles/[id]', { id: card.id })}
						class="absolute inset-0 rounded-xl"
						aria-label={m.models_profiles_edit_item({ name: card.displayName || card.id })}
						title={card.subjectsTitle || undefined}
					></a>
					<div class="pointer-events-none relative">
						<div class="flex items-start justify-between gap-3">
							<div class="min-w-0">
								<h3 class="truncate font-semibold">{card.displayName || card.id}</h3>
								<p class="text-muted-content mt-1 flex items-center gap-1.5 text-xs">
									<UsersRound class="size-3 shrink-0" />
									<span class="truncate">{card.subjectsLabel}</span>
								</p>
							</div>
							{#if !readonly}
								<IconButton
									variant="danger"
									class="pointer-events-auto"
									tooltip={{ text: m.models_delete_profile(), placement: 'bottom' }}
									onclick={() => (toDelete = card)}
								>
									<Trash2 class="size-4" />
								</IconButton>
							{/if}
						</div>
						<ul class="mt-4 flex flex-wrap gap-1">
							{#each card.facts as fact (fact)}
								<li
									class="border-base-300 dark:border-base-400 group-hover:border-primary/40 text-muted-content rounded-md border px-2 py-0.5 text-xs transition-colors"
								>
									{fact}
								</li>
							{/each}
						</ul>
					</div>
				</article>
			{/each}
		</div>
	{/if}
</div>

<Confirm
	show={!!toDelete}
	msg={toDelete?.displayName
		? m.models_profiles_delete_confirm({ name: toDelete.displayName })
		: m.models_profiles_delete_confirm_unnamed()}
	onsuccess={async () => {
		if (toDelete) await AdminService.deleteModelAccessPolicy(toDelete.id);
		toDelete = undefined;
		await accessibleModels.refresh();
		await load();
	}}
	oncancel={() => (toDelete = undefined)}
/>
