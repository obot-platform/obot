<script lang="ts">
	import Skeleton from '$lib/components/Skeleton.svelte';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		approveToolLabelSuggestions,
		getToolLabelSuggestions,
		suggestToolLabels,
		type ToolLabelResource,
		type ToolLabelSuggestions,
		type ToolLabelSuggestion
	} from '$lib/services/provenance';
	import { profile } from '$lib/stores';
	import { isWebURL } from '$lib/url';
	import LabelBadges from './LabelBadges.svelte';
	import { Check, RefreshCw, Sparkles } from '@lucide/svelte';

	let { resourceType, resourceID }: ToolLabelResource = $props();
	let result = $state<ToolLabelSuggestions>();
	let loading = $state(true);
	let busy = $state<'suggest' | 'approve'>();
	let selected = $state<string[]>([]);
	let error = $state('');
	let notice = $state('');
	let generation = 0;
	let canApprove = $derived(!!profile.current.isAdmin?.() && !profile.current.isAdminReadonly?.());
	let selectedItems = $derived(
		(result?.items ?? []).filter((item) => selected.includes(item.tool) && eligible(item))
	);

	function changed(item: ToolLabelSuggestion) {
		return (
			!!item.proposal &&
			[...(item.labels ?? [])].sort().join(',') !== [...item.proposal.labels].sort().join(',')
		);
	}
	function eligible(item: ToolLabelSuggestion) {
		return !!item.proposal && !item.stale;
	}
	function toggle(tool: string, checked: boolean) {
		selected = checked ? [...selected, tool] : selected.filter((value) => value !== tool);
	}
	$effect(() => {
		const resource = { resourceType, resourceID };
		const gen = ++generation;
		const controller = new AbortController();
		loading = true;
		result = undefined;
		busy = undefined;
		selected = [];
		error = '';
		notice = '';
		getToolLabelSuggestions(resource, { signal: controller.signal, dontLogErrors: true })
			.then((value) => {
				if (gen === generation) result = value;
			})
			.catch((e) => {
				if (gen === generation && !controller.signal.aborted) error = parseErrorContent(e).message;
			})
			.finally(() => {
				if (gen === generation) loading = false;
			});
		return () => {
			controller.abort();
		};
	});
	async function refresh() {
		const gen = generation;
		loading = true;
		error = '';
		try {
			const value = await getToolLabelSuggestions(
				{ resourceType, resourceID },
				{ dontLogErrors: true }
			);
			if (gen === generation) {
				result = value;
				selected = [];
			}
		} catch (e) {
			if (gen === generation) error = parseErrorContent(e).message;
		} finally {
			if (gen === generation) loading = false;
		}
	}
	async function suggest() {
		const gen = generation;
		busy = 'suggest';
		error = '';
		notice = '';
		try {
			const value = await suggestToolLabels({ resourceType, resourceID }, { dontLogErrors: true });
			if (gen === generation) {
				result = value;
				selected = [];
			}
		} catch (e) {
			if (gen === generation) error = parseErrorContent(e).message;
		} finally {
			if (gen === generation) busy = undefined;
		}
	}
	async function approve() {
		if (!canApprove || !selectedItems.length) return;
		const gen = generation;
		const resource = { resourceType, resourceID };
		const tools = selectedItems.map((item) => ({
			tool: item.tool,
			schemaHash: item.proposal!.schemaHash
		}));
		busy = 'approve';
		error = '';
		notice = '';
		try {
			const value = await approveToolLabelSuggestions(resource, tools, { dontLogErrors: true });
			if (gen === generation) {
				result = value;
				selected = [];
				notice = m.supervision_tool_labels_saved();
			}
		} catch (e) {
			if (gen !== generation) return;
			const parsed = parseErrorContent(e);
			error = parsed.status === 409 ? m.supervision_tool_labels_stale_error() : parsed.message;
			selected = [];
			// Reload the authoritative snapshot after a conflict while retaining the explanation.
			if (parsed.status === 409) {
				try {
					const value = await getToolLabelSuggestions(resource, { dontLogErrors: true });
					if (gen === generation) result = value;
				} catch {
					/* Keep the original conflict visible. */
				}
			}
		} finally {
			if (gen === generation) busy = undefined;
		}
	}
</script>

<section class="flex flex-col gap-5" aria-label={m.supervision_tool_labels_title()}>
	<div class="paper gap-3">
		<div class="flex flex-wrap items-start justify-between gap-3">
			<div class="flex min-w-0 flex-col gap-1">
				<h2 class="text-xl font-semibold">{result?.displayName ?? resourceID}</h2>
				<p class="text-muted-content text-sm">{m.supervision_tool_labels_description()}</p>
			</div>
			<div class="flex flex-wrap gap-2">
				<button
					type="button"
					class="btn btn-secondary btn-sm"
					disabled={loading || !!busy}
					onclick={refresh}><RefreshCw class="size-4" />{m.supervision_refresh()}</button
				>
				{#if canApprove}<button
						type="button"
						class="btn btn-primary btn-sm"
						disabled={loading || !!busy || !result?.items?.length}
						onclick={suggest}
						>{#if busy === 'suggest'}<Loading class="size-4" />{:else}<Sparkles
								class="size-4"
							/>{/if}{m.supervision_tool_labels_suggest()}</button
					>{/if}
			</div>
		</div>
		{#if result?.sourceURL}
			<p class="text-muted-content text-sm">
				{m.supervision_tool_labels_git()}
				{#if isWebURL(result.sourceURL)}<a
						href={result.sourceURL}
						target="_blank"
						rel="noopener noreferrer"
						class="text-link break-all">{result.sourceURL}</a
					>{:else}<span class="break-all">{result.sourceURL}</span>{/if}
			</p>
		{/if}
		<p class="text-muted-content text-xs">{m.supervision_tool_labels_sidecar()}</p>
		{#if !canApprove}<p class="text-muted-content text-sm">
				{m.supervision_tool_labels_readonly()}
			</p>{/if}
	</div>
	{#if error}<p class="notification-error rounded-md p-3 text-sm" role="alert">{error}</p>{/if}
	{#if notice}<p class="notification-success rounded-md p-3 text-sm" role="status">{notice}</p>{/if}
	{#if loading && !result}<Skeleton type="list" count={3} />
	{:else if result?.items?.length === 0}<p class="paper text-muted-content text-sm">
			{m.supervision_tool_labels_empty()}
		</p>
	{:else if result}
		<div class="flex flex-col gap-3">
			{#each result.items as item (item.tool)}
				<article class="paper gap-3" aria-label={item.tool}>
					<div class="flex items-start gap-3">
						{#if canApprove}<input
								type="checkbox"
								class="checkbox checkbox-sm mt-1"
								aria-label={m.supervision_tool_labels_select({ tool: item.tool })}
								checked={selected.includes(item.tool)}
								disabled={!!busy || loading || !eligible(item)}
								onchange={(event) => toggle(item.tool, event.currentTarget.checked)}
							/>{/if}
						<div class="min-w-0 flex-1">
							<h3 class="font-mono text-sm font-semibold break-all">{item.tool}</h3>
							{#if item.description}<p class="text-muted-content mt-1 line-clamp-2 text-sm">
									{item.description}
								</p>{/if}
						</div>
					</div>
					<dl class="grid gap-3 text-sm sm:grid-cols-2">
						<div class="flex flex-col gap-1">
							<dt class="text-muted-content text-xs">
								{m.supervision_tool_labels_current()} · {item.source}
							</dt>
							<dd>
								{#if item.labels?.length}<LabelBadges
										labels={item.labels}
									/>{:else}{m.supervision_none()}{/if}
							</dd>
						</div>
						<div class="flex flex-col gap-1">
							<dt class="text-muted-content text-xs">{m.supervision_tool_labels_proposed()}</dt>
							<dd>
								{#if item.proposal}<LabelBadges
										labels={item.proposal.labels}
									/>{#if !item.proposal.labels.length}{m.supervision_none()}{/if}{:else}<span
										class="text-muted-content">{m.supervision_tool_labels_not_suggested()}</span
									>{/if}
							</dd>
						</div>
					</dl>
					{#if item.proposal}
						{#if item.proposal.labels.some((label) => !item.labels.includes(label))}<p
								class="text-success text-xs"
							>
								{m.supervision_tool_labels_add({
									labels: item.proposal.labels
										.filter((label) => !item.labels.includes(label))
										.join(', ')
								})}
							</p>{/if}
						{#if item.labels.some((label) => !item.proposal!.labels.includes(label))}<p
								class="text-warning text-xs"
							>
								{m.supervision_tool_labels_remove({
									labels: item.labels
										.filter((label) => !item.proposal!.labels.includes(label))
										.join(', ')
								})}
							</p>{/if}
						<p class="text-muted-content text-sm">{item.proposal.rationale}</p>
						{#if item.stale}<p class="notification-warning rounded-md p-2 text-xs">
								{m.supervision_tool_labels_stale()}
							</p>{:else if !changed(item)}<p class="text-muted-content text-xs">
								{m.supervision_tool_labels_unchanged()}
							</p>{/if}
					{/if}
				</article>
			{/each}
		</div>
		{#if canApprove}<div class="flex justify-end">
				<button
					type="button"
					class="btn btn-primary"
					disabled={!!busy || loading || !selectedItems.length}
					onclick={approve}
					>{#if busy === 'approve'}<Loading class="size-4" />{:else}<Check
							class="size-4"
						/>{/if}{m.supervision_tool_labels_approve({ count: selectedItems.length })}</button
				>
			</div>{/if}
	{/if}
</section>
