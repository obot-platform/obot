<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import Search from '$lib/components/Search.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import { AdminService } from '$lib/services';
	import type { MCPFilter, ModelAccessPolicy } from '$lib/services/admin/types';
	import { guardrailTypeLabel } from '$lib/services/llm-gateway/guardrail-text';
	import {
		deleteGuardrail,
		guardrailCategoryLabel,
		listGuardrailPolicies,
		listGuardrails,
		listGuardrailProviders,
		type Guardrail,
		type GuardrailPolicy,
		type GuardrailProvider
	} from '$lib/services/llm-gateway/guardrails';
	import { profile } from '$lib/stores';
	import { resolve } from '$lib/url';
	import {
		clearUrlParams,
		getTableUrlParamsFilters,
		getTableUrlParamsSort,
		goto,
		replaceState,
		setFilterUrlParams,
		setSortUrlParams
	} from '$lib/url';
	import { openUrl } from '$lib/utils';
	import GuardrailTypePicker from './GuardrailTypePicker.svelte';
	import { CircleAlert, Info, ShieldCheck, Trash2 } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	// The guardrails list on the Guardrails tab of LLM Gateway. Guardrails are used by both
	// LLM guardrail policies and MCP guardrail filters.

	let guardrails = $state<Guardrail[]>([]);
	let providers = $state<GuardrailProvider[]>([]);
	let policies = $state<GuardrailPolicy[]>([]);
	let filters = $state<MCPFilter[]>([]);
	let profileNames = $state<Record<string, string>>({});
	let loading = $state(true);
	let toDelete = $state<Guardrail>();
	let deleteError = $state('');
	let typePicker = $state<ReturnType<typeof GuardrailTypePicker>>();

	let readonly = $derived(profile.current.isAdminReadonly?.());
	let query = $derived(page.url.searchParams.get('query') || '');
	let urlFilters = $derived(getTableUrlParamsFilters());
	let initSort = $derived(getTableUrlParamsSort());

	async function load() {
		loading = true;
		try {
			// What uses each guardrail is a convenience; the list shows without it.
			let profiles: ModelAccessPolicy[];
			[guardrails, providers, policies, filters, profiles] = await Promise.all([
				listGuardrails(),
				listGuardrailProviders(),
				listGuardrailPolicies().catch((): GuardrailPolicy[] => []),
				AdminService.listMCPFilters().catch((): MCPFilter[] => []),
				AdminService.listModelAccessPolicies().catch((): ModelAccessPolicy[] => [])
			]);
			profileNames = Object.fromEntries(profiles.map((p) => [p.id, p.displayName || p.id]));
		} finally {
			loading = false;
		}
	}

	onMount(load);

	export function openTypePicker() {
		typePicker?.open();
	}

	let providersByType = $derived(Object.fromEntries(providers.map((p) => [p.type, p])));
	// Rows keep the server's English type name, which the table filters and sorts on and saves
	// in the URL; it's translated only for display.
	let typeLabels = $derived(
		Object.fromEntries(providers.map((p) => [p.label, guardrailTypeLabel(p)]))
	);

	function namesUsing(id: string | undefined) {
		if (!id) return { policies: [], filters: [] };
		return {
			// An applied guardrail is named with the profile it's applied in.
			policies: policies
				.filter((p) => p.guardrailIDs?.includes(id))
				.map((p) =>
					profileNames[p.profileID] ? `${profileNames[p.profileID]} / ${p.name}` : p.name
				),
			filters: filters
				.filter((f) => f.guardrail?.guardrailIDs?.includes(id))
				.map((f) => f.name || f.id)
		};
	}

	let rows = $derived(
		guardrails.map((p) => {
			const usedBy = namesUsing(p.id);
			return {
				...p,
				id: p.id ?? p.name,
				typeLabel: providersByType[p.type]?.label ?? p.type,
				categoryLabel: providersByType[p.type]?.category ?? '',
				statusLabel: p.error ? p.error : m.core_status_ready(),
				usedBy,
				usedByLabel:
					[...usedBy.policies, ...usedBy.filters].join(', ') || m.guardrails_used_by_none()
			};
		})
	);
	let filteredRows = $derived(
		rows.filter((r) =>
			[
				r.name,
				r.typeLabel,
				typeLabels[r.typeLabel],
				guardrailCategoryLabel(r.categoryLabel),
				r.usedByLabel
			].some((v) => v?.toLowerCase().includes(query.toLowerCase()))
		)
	);

	function updateQuery(value: string) {
		const url = new URL(page.url.toString());
		if (value) {
			url.searchParams.set('query', value);
		} else {
			url.searchParams.delete('query');
		}
		replaceState(url, { query: value });
	}

	function clearFilters() {
		clearUrlParams(Array.from(page.url.searchParams.keys()).filter((key) => key !== 'view'));
	}
</script>

{#snippet pointer()}
	{m.guardrails_pointer_prefix()}
	<a href={resolve('/models?view=profiles')} class="text-link"
		>{m.guardrails_pointer_profiles_link()}</a
	>
	{m.guardrails_pointer_middle()}
	<a href={resolve('/mcp-servers?view=filters')} class="text-link"
		>{m.guardrails_pointer_filters_link()}</a
	>{m.guardrails_pointer_suffix()}
{/snippet}

{#if loading}
	<Skeleton
		type="table"
		count={5}
		classes={{ header: 'h-14 rounded-none', body: 'rounded-none' }}
	/>
{:else if guardrails.length === 0}
	<div class="mt-12 flex w-md max-w-full flex-col items-center gap-4 self-center text-center">
		<ShieldCheck class="text-muted-content size-24 opacity-25" />
		<h4 class="text-muted-content text-lg font-semibold">{m.guardrails_empty_title()}</h4>
		<p class="text-muted-content text-sm font-light">
			{m.guardrails_empty_description()}
			{@render pointer()}
		</p>
	</div>
{:else}
	<div class="flex flex-col gap-2">
		<p class="text-muted-content flex items-start gap-2 text-sm">
			<Info class="mt-0.5 size-4 shrink-0" />
			<span>{@render pointer()}</span>
		</p>

		{#if deleteError}
			<div class="notification-error flex items-center gap-2 p-3 text-sm">
				<CircleAlert class="size-4 shrink-0" />
				{deleteError}
			</div>
		{/if}

		<div class="bg-base-200 dark:bg-base-100 sticky top-16 left-0 z-20 w-full py-1">
			<Search
				value={query}
				class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
				onChange={updateQuery}
				placeholder={m.guardrails_search_placeholder()}
			/>
		</div>

		<Table
			data={filteredRows}
			fields={['name', 'typeLabel', 'categoryLabel', 'usedByLabel', 'statusLabel']}
			headers={[
				{ property: 'name', title: m.core_name() },
				{ property: 'typeLabel', title: m.core_type() },
				{ property: 'categoryLabel', title: m.guardrails_col_category() },
				{ property: 'usedByLabel', title: m.guardrails_col_used_by() },
				{ property: 'statusLabel', title: m.core_status() }
			]}
			sortable={['name', 'typeLabel', 'categoryLabel']}
			filterable={['typeLabel', 'categoryLabel']}
			filters={urlFilters}
			onFilter={(property, values) => setFilterUrlParams({ [property]: values })}
			onClearAllFilters={clearFilters}
			onSort={setSortUrlParams}
			{initSort}
			displayValue={(property, value) =>
				property === 'categoryLabel'
					? guardrailCategoryLabel(String(value ?? ''))
					: property === 'typeLabel'
						? (typeLabels[String(value ?? '')] ?? String(value ?? ''))
						: String(value ?? '')}
			onClickRow={(row, isCtrl) => openUrl(`/models/guardrails/${row.id}`, isCtrl)}
		>
			{#snippet onRenderColumn(property, row)}
				{#if property === 'usedByLabel'}
					{#if row.usedBy.policies.length || row.usedBy.filters.length}
						<div class="flex flex-col text-xs">
							{#if row.usedBy.policies.length}
								<span>
									{m.guardrails_used_by_policies({ names: row.usedBy.policies.join(', ') })}
								</span>
							{/if}
							{#if row.usedBy.filters.length}
								<span>
									{m.guardrails_used_by_filters({ names: row.usedBy.filters.join(', ') })}
								</span>
							{/if}
						</div>
					{:else}
						<span class="text-muted-content text-xs">{m.guardrails_used_by_none()}</span>
					{/if}
				{:else if property === 'statusLabel'}
					<span class={twMerge('text-xs', row.error ? 'text-error' : 'text-success')}>
						{row.statusLabel}
					</span>
				{:else if property === 'categoryLabel'}
					{guardrailCategoryLabel(row.categoryLabel)}
				{:else if property === 'typeLabel'}
					{typeLabels[row.typeLabel] ?? row.typeLabel}
				{:else}
					{row[property as keyof typeof row]}
				{/if}
			{/snippet}
			{#snippet actions(row)}
				{#if !readonly}
					<IconButton
						variant="danger"
						tooltip={{ text: m.guardrails_delete_tooltip() }}
						onclick={(e) => {
							e.stopPropagation();
							deleteError = '';
							toDelete = row;
						}}
					>
						<Trash2 class="size-4" />
					</IconButton>
				{/if}
			{/snippet}
		</Table>
	</div>
{/if}

<GuardrailTypePicker
	bind:this={typePicker}
	{providers}
	{guardrails}
	onSelect={(p) => goto(`/models/guardrails/new?type=${encodeURIComponent(p.type)}`)}
/>

<Confirm
	show={!!toDelete}
	msg={m.core_delete_named_component({ name: toDelete?.name ?? '' })}
	onsuccess={async () => {
		try {
			if (toDelete?.id) await deleteGuardrail(toDelete.id, { dontLogErrors: true });
		} catch (e) {
			deleteError = parseErrorContent(e).message;
		}
		toDelete = undefined;
		await load();
	}}
	oncancel={() => (toDelete = undefined)}
/>
