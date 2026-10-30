<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import SupervisorInsights from '$lib/components/supervision/SupervisorInsights.svelte';
	import SurfaceBadge from '$lib/components/supervision/SurfaceBadge.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import {
		deletePolicy,
		effectBadgeClass,
		effectLabel,
		getSupervisorInsights,
		listPolicies,
		modeLabel,
		updatePolicy,
		type Insight,
		type Policy
	} from '$lib/services/provenance';
	import { profile } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { goto, resolve } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import { Plus, ShieldCheck, Sparkles, Trash2 } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	// Obot Policy list. Mode and enabled toggle inline; built-ins can be switched but not deleted.
	// Above it, the Supervisor suggests shadow policies that would have caught real activity.

	let policies = $state<Policy[]>([]);
	let loading = $state(true);
	let error = $state('');
	let toDelete = $state<Policy>();
	let busy = $state<string>();

	let readonly = $derived(profile.current.isAdminReadonly?.());
	let query = $derived(page.url.searchParams.get('query') || '');

	async function load() {
		loading = true;
		error = '';
		try {
			policies = await listPolicies({ dontLogErrors: true });
		} catch (e) {
			error = parseErrorContent(e).message;
		} finally {
			loading = false;
		}
	}

	let suggestions = $state<Insight[]>([]);

	async function loadSuggestions() {
		try {
			const insights = await getSupervisorInsights(false, { dontLogErrors: true });
			suggestions = insights.items.filter((i) => i.kind === 'shadow-policy');
		} catch {
			// Suggestions are optional; the list works without the Supervisor.
			suggestions = [];
		}
	}

	onMount(() => {
		load();
		loadSuggestions();
	});

	let rows = $derived(
		policies
			.map((p) => ({
				...p,
				id: p.id ?? p.name,
				effectLabel: effectLabel(p.effect),
				modeLabel: modeLabel(p.mode),
				surfacesLabel: p.surfaces?.length
					? p.surfaces.join(', ')
					: m.supervision_policies_all_surfaces()
			}))
			.sort((a, b) => b.priority - a.priority || a.name.localeCompare(b.name))
	);
	let filteredRows = $derived(
		rows.filter((r) =>
			[r.name, r.description, r.condition, r.effectLabel, r.modeLabel, r.surfacesLabel].some((v) =>
				v?.toLowerCase().includes(query.toLowerCase())
			)
		)
	);

	function updateQuery(value: string) {
		const url = new URL(page.url.toString());
		if (value) url.searchParams.set('query', value);
		else url.searchParams.delete('query');
		// Not a shallow replaceState: the list filters on page.url, which only a real navigation updates.
		goto(url, { replaceState: true, noScroll: true, keepFocus: true });
	}

	async function patch(p: Policy, changes: Partial<Policy>, toast: string) {
		if (!p.id) return;
		busy = p.id;
		error = '';
		try {
			const saved = await updatePolicy(p.id, { ...p, ...changes }, { dontLogErrors: true });
			policies = policies.map((x) => (x.id === saved.id ? saved : x));
			success.add(toast);
		} catch (e) {
			error = parseErrorContent(e).message;
		} finally {
			busy = undefined;
		}
	}

	async function remove() {
		if (!toDelete?.id) return;
		try {
			await deletePolicy(toDelete.id, { dontLogErrors: true });
			success.add(m.supervision_policies_deleted());
			policies = policies.filter((p) => p.id !== toDelete?.id);
		} catch (e) {
			error = parseErrorContent(e).message;
		}
		toDelete = undefined;
	}
</script>

<svelte:head>
	<title>Obot | {m.supervision_policies_title()}</title>
</svelte:head>

<Layout title={m.supervision_policies_title()} rightNavActions={navActions}>
	<div class="flex flex-col gap-4 pb-8">
		<p class="text-muted-content text-sm">{m.supervision_policies_description()}</p>

		{#if error}
			<div class="notification-error rounded-md p-3 text-sm" role="alert">{error}</div>
		{/if}

		{#if suggestions.length}
			<SupervisorInsights
				items={suggestions}
				title={m.supervision_suggests_title()}
				description={m.supervision_suggests_description()}
				onActed={() => Promise.all([load(), loadSuggestions()])}
			/>
		{/if}

		{#if loading}
			<Skeleton type="table" count={5} />
		{:else if policies.length === 0}
			<div class="mt-12 flex w-md max-w-full flex-col items-center gap-4 self-center text-center">
				<ShieldCheck class="text-muted-content size-24 opacity-25" />
				<p class="text-muted-content text-sm">{m.supervision_policies_empty()}</p>
			</div>
		{:else}
			<div class="bg-base-200 dark:bg-base-100 sticky top-16 left-0 z-20 w-full py-1">
				<Search
					value={query}
					class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
					onChange={updateQuery}
					placeholder={m.supervision_policies_search_placeholder()}
				/>
			</div>

			<Table
				data={filteredRows}
				fields={['name', 'effectLabel', 'modeLabel', 'surfacesLabel', 'priority', 'enabled']}
				headers={[
					{ property: 'name', title: m.core_name() },
					{ property: 'effectLabel', title: m.supervision_policies_col_effect() },
					{ property: 'modeLabel', title: m.supervision_policies_col_mode() },
					{ property: 'surfacesLabel', title: m.supervision_policies_col_surfaces() },
					{ property: 'priority', title: m.supervision_policies_col_priority() },
					{ property: 'enabled', title: m.supervision_policies_col_enabled() }
				]}
				sortable={['name', 'effectLabel', 'modeLabel', 'priority']}
				noAutoHideFields={['name', 'modeLabel', 'enabled']}
				setRowClasses={(row) => (row.enabled ? '' : 'opacity-60')}
				onClickRow={(row, isCtrl) => openUrl(`/policies/${row.id}`, isCtrl)}
			>
				{#snippet onRenderColumn(property, row)}
					{#if property === 'name'}
						<span class="flex flex-col">
							<span class="flex items-center gap-2">
								<a href={resolve(`/policies/${row.id}`)} class="font-medium hover:underline">
									{row.name}
								</a>
								{#if row.builtIn}
									<span class="badge badge-xs badge-soft badge-primary"
										>{m.supervision_policies_built_in()}</span
									>
								{/if}
							</span>
							{#if row.description}
								<span class="text-muted-content line-clamp-1 text-xs">{row.description}</span>
							{/if}
						</span>
					{:else if property === 'effectLabel'}
						<span class={twMerge('badge badge-sm badge-soft', effectBadgeClass(row.effect))}>
							{row.effectLabel}
						</span>
					{:else if property === 'modeLabel'}
						<!-- svelte-ignore a11y_click_events_have_key_events -->
						<!-- svelte-ignore a11y_no_static_element_interactions -->
						<span class="flex items-center gap-2" onclick={(e) => e.stopPropagation()}>
							<Toggle
								label={m.supervision_policies_enforce_toggle({ name: row.name })}
								checked={row.mode === 'enforce'}
								disabled={readonly || busy === row.id}
								onChange={(v) =>
									patch(
										row,
										{ mode: v ? 'enforce' : 'shadow' },
										m.supervision_policies_mode_updated({
											name: row.name,
											mode: modeLabel(v ? 'enforce' : 'shadow')
										})
									)}
							/>
							<span
								class={twMerge(
									'text-xs',
									row.mode === 'enforce' ? 'text-error' : 'text-muted-content'
								)}
							>
								{row.modeLabel}
							</span>
						</span>
					{:else if property === 'surfacesLabel'}
						{#if row.surfaces?.length}
							<span class="flex flex-wrap gap-1">
								{#each row.surfaces as s (s)}
									<SurfaceBadge surface={s} short />
								{/each}
							</span>
						{:else}
							<span class="text-muted-content text-xs">{m.supervision_policies_all_surfaces()}</span
							>
						{/if}
					{:else if property === 'enabled'}
						<!-- svelte-ignore a11y_click_events_have_key_events -->
						<!-- svelte-ignore a11y_no_static_element_interactions -->
						<span onclick={(e) => e.stopPropagation()}>
							<Toggle
								label={m.supervision_policies_enabled_toggle({ name: row.name })}
								checked={row.enabled}
								disabled={readonly || busy === row.id}
								onChange={(v) =>
									patch(
										row,
										{ enabled: v },
										v
											? m.supervision_policies_enabled_toast({ name: row.name })
											: m.supervision_policies_disabled_toast({ name: row.name })
									)}
							/>
						</span>
					{:else}
						{row[property as keyof typeof row]}
					{/if}
				{/snippet}
				{#snippet actions(row)}
					{#if !readonly && !row.builtIn}
						<IconButton
							variant="danger"
							tooltip={{ text: m.supervision_policies_delete_tooltip() }}
							onclick={(e) => {
								e.stopPropagation();
								toDelete = row;
							}}
						>
							<Trash2 class="size-4" />
						</IconButton>
					{/if}
				{/snippet}
			</Table>
		{/if}
	</div>
</Layout>

{#snippet navActions()}
	{#if !readonly}
		<div class="flex items-center gap-2">
			<button
				type="button"
				class="btn btn-secondary rounded-4xl text-sm"
				onclick={() => goto('/policies/new?draft=1')}
			>
				<Sparkles class="size-4" />
				{m.supervision_policies_draft_ai()}
			</button>
			<button
				type="button"
				class="btn btn-primary rounded-4xl text-sm"
				onclick={() => goto('/policies/new')}
			>
				<Plus class="size-4" />
				{m.supervision_policies_new()}
			</button>
		</div>
	{/if}
{/snippet}

<Confirm
	show={!!toDelete}
	msg={m.core_delete_named_component({ name: toDelete?.name ?? '' })}
	onsuccess={remove}
	oncancel={() => (toDelete = undefined)}
/>
