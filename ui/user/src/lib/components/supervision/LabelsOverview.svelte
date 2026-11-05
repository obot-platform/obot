<script lang="ts">
	import Skeleton from '$lib/components/Skeleton.svelte';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		getLabelInventory,
		toolLabelReviewURL,
		type LabelInventory,
		type LabelResourceSummary
	} from '$lib/services/provenance';
	import { formatTimeAgo } from '$lib/time';
	import { resolve } from '$lib/url';
	import LabelBadges from './LabelBadges.svelte';
	import { RefreshCw } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	type View = 'resources' | 'library' | 'observed';
	const views: View[] = ['resources', 'library', 'observed'];
	let view = $state<View>('resources');
	let data = $state<LabelInventory>();
	let loading = $state(true);
	let error = $state('');
	let query = $state('');
	let needsReview = $state(false);
	let controller: AbortController | undefined;
	let reviewTotal = $derived(
		data?.resources.reduce(
			(count, resource) => count + resource.pendingToolCount + resource.staleToolCount,
			0
		) ?? 0
	);
	let reviewedTotal = $derived(
		data?.resources.reduce((count, resource) => count + resource.reviewedToolCount, 0) ?? 0
	);
	let toolTotal = $derived(
		data?.resources.reduce((count, resource) => count + resource.toolCount, 0) ?? 0
	);
	let resources = $derived(
		(data?.resources ?? []).filter(
			(resource) =>
				(!needsReview || resource.pendingToolCount + resource.staleToolCount > 0) &&
				matches([
					resource.displayName,
					resource.resourceID,
					resource.sourceURL ?? '',
					...resource.labels
				])
		)
	);
	let observed = $derived(
		(data?.observedTools ?? []).filter((tool) =>
			matches([
				tool.server,
				tool.tool,
				sourceName(tool.source),
				tool.rationale ?? '',
				...tool.labels
			])
		)
	);
	let capabilities = $derived(
		(data?.capabilities ?? []).filter((label) => matches([label, meaning(label)]))
	);
	let contentLabels = $derived(
		(data?.contentLabels ?? []).filter((label) => matches([label, meaning(label)]))
	);

	function matches(values: string[]) {
		return values.join(' ').toLowerCase().includes(query.trim().toLowerCase());
	}
	function viewName(value: View) {
		return value === 'resources'
			? m.supervision_labels_resources()
			: value === 'library'
				? m.supervision_labels_library()
				: m.supervision_labels_observed();
	}
	function changeView(value: View) {
		view = value;
		query = '';
	}
	function navigateTabs(event: KeyboardEvent) {
		if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
		event.preventDefault();
		const next =
			event.key === 'Home'
				? 0
				: event.key === 'End'
					? views.length - 1
					: (views.indexOf(view) + (event.key === 'ArrowRight' ? 1 : -1) + views.length) %
						views.length;
		changeView(views[next]);
		document.getElementById(`labels-tab-${views[next]}`)?.focus();
	}
	function sourceName(source: string) {
		if (source === 'admin') return m.supervision_labels_source_admin();
		if (source === 'heuristic') return m.supervision_labels_source_heuristic();
		if (source === 'supervisor') return m.supervision_labels_source_legacy_ai();
		return m.supervision_labels_source_other({ source });
	}
	function resourceEditor(resource: LabelResourceSummary) {
		return resource.resourceType === 'vmcp'
			? resolve(`/vmcps/${encodeURIComponent(resource.resourceID)}`)
			: resolve(`/mcp-servers/c/${encodeURIComponent(resource.resourceID)}`) + '?view=tools';
	}
	function updated(value?: string) {
		if (!value || value.startsWith('0001-') || !Number.isFinite(Date.parse(value)))
			return { relativeTime: m.supervision_labels_not_recorded(), fullDate: '' };
		return formatTimeAgo(value);
	}
	function meaning(label: string): string {
		const descriptions: Record<string, () => string> = {
			'reads-private': m.supervision_labels_meaning_reads_private,
			'reads-untrusted': m.supervision_labels_meaning_reads_untrusted,
			'writes-external': m.supervision_labels_meaning_writes_external,
			'writes-internal': m.supervision_labels_meaning_writes_internal,
			exec: m.supervision_labels_meaning_exec,
			destructive: m.supervision_labels_meaning_destructive,
			'credential-bearing': m.supervision_labels_meaning_credential_bearing,
			pii: m.supervision_labels_meaning_pii,
			secret: m.supervision_labels_meaning_secret,
			'customer-data': m.supervision_labels_meaning_customer_data,
			'source-code': m.supervision_labels_meaning_source_code,
			financial: m.supervision_labels_meaning_financial,
			health: m.supervision_labels_meaning_health,
			confidential: m.supervision_labels_meaning_confidential,
			untrusted: m.supervision_labels_meaning_untrusted,
			honeytoken: m.supervision_labels_meaning_honeytoken
		};
		return descriptions[label]?.() ?? m.supervision_labels_meaning_other();
	}
	async function load() {
		controller?.abort();
		const request = new AbortController();
		controller = request;
		loading = true;
		error = '';
		try {
			const value = await getLabelInventory({ signal: request.signal, dontLogErrors: true });
			if (!request.signal.aborted) data = value;
		} catch (e) {
			if (!request.signal.aborted) error = parseErrorContent(e).message;
		} finally {
			if (controller === request) loading = false;
		}
	}
	onMount(() => {
		void load();
		return () => controller?.abort();
	});
</script>

<section class="flex flex-col gap-5" aria-label={m.supervision_labels_title()}>
	<div class="flex flex-wrap items-start justify-between gap-3">
		<p class="text-muted-content max-w-3xl text-sm">{m.supervision_labels_intro()}</p>
		<button type="button" class="btn btn-secondary btn-sm" disabled={loading} onclick={load}
			>{#if loading}<Loading class="size-4" />{:else}<RefreshCw
					class="size-4"
				/>{/if}{m.supervision_refresh()}</button
		>
	</div>
	{#if error}<div
			class="notification-error flex flex-wrap items-center justify-between gap-3 rounded-md p-3 text-sm"
			role="alert"
		>
			<span>{error}</span><button
				type="button"
				class="btn btn-secondary btn-sm"
				disabled={loading}
				onclick={load}>{m.supervision_labels_retry()}</button
			>
		</div>{/if}
	{#if loading && !data}<Skeleton type="list" count={3} />
	{:else if data}
		<dl class="flex flex-wrap gap-x-10 gap-y-3 border-b border-base-300 pb-4 text-sm">
			<div>
				<dt class="text-muted-content text-xs">{m.supervision_labels_resources()}</dt>
				<dd class="mt-1 text-xl font-semibold tabular-nums">{data.resources.length}</dd>
			</div>
			<div>
				<dt class="text-muted-content text-xs">{m.supervision_labels_reviewed_summary()}</dt>
				<dd class="mt-1 text-xl font-semibold tabular-nums">{reviewedTotal} / {toolTotal}</dd>
			</div>
			<div>
				<dt class="text-muted-content text-xs">{m.supervision_labels_suggestions_summary()}</dt>
				<dd class="mt-1 text-xl font-semibold tabular-nums">{reviewTotal}</dd>
			</div>
		</dl>
		<div class="flex flex-wrap items-end justify-between gap-3">
			<div class="tabs tabs-box w-fit" role="tablist" aria-label={m.supervision_labels_views()}>
				{#each views as value (value)}<button
						type="button"
						id={`labels-tab-${value}`}
						role="tab"
						class={twMerge('tab', view === value && 'tab-active')}
						aria-selected={view === value}
						aria-controls={`labels-panel-${value}`}
						tabindex={view === value ? 0 : -1}
						onclick={() => changeView(value)}
						onkeydown={navigateTabs}>{viewName(value)}</button
					>{/each}
			</div>
			<div class="flex flex-wrap items-center gap-4">
				{#if view === 'resources'}<label class="flex items-center gap-2 text-sm"
						><input
							type="checkbox"
							class="checkbox checkbox-sm"
							bind:checked={needsReview}
						/>{m.supervision_labels_filter_review()}</label
					>{/if}
				<input
					type="search"
					class="text-input-filled w-64 max-w-full"
					aria-label={m.supervision_labels_search()}
					placeholder={m.supervision_labels_search()}
					bind:value={query}
				/>
			</div>
		</div>
		<div
			id={`labels-panel-${view}`}
			role="tabpanel"
			aria-labelledby={`labels-tab-${view}`}
			class="flex flex-col gap-4"
		>
			{#if view === 'resources'}
				<p class="text-muted-content text-xs">{m.supervision_labels_resources_help()}</p>
				{#if !data.resources.length}<p class="paper text-muted-content text-sm">
						{m.supervision_labels_empty_resources()}
					</p>
				{:else if !resources.length}<p class="paper text-muted-content text-sm">
						{m.supervision_labels_no_matches()}
					</p>
				{:else}<div class="overflow-x-auto rounded-md border border-base-300">
						<table class="table table-sm" aria-label={m.supervision_labels_resources()}>
							<thead>
								<tr>
									<th>{m.supervision_labels_resource()}</th>
									<th>{m.supervision_labels_effective()}</th>
									<th>{m.supervision_labels_reviewed()}</th>
									<th>{m.supervision_labels_review_state()}</th>
									<th>{m.supervision_labels_updated()}</th>
									<th><span class="sr-only">{m.supervision_labels_manage()}</span></th>
								</tr>
							</thead>
							<tbody>
								{#each resources as resource (`${resource.resourceType}:${resource.resourceID}`)}
									<tr>
										<td>
											<p class="font-medium">{resource.displayName || resource.resourceID}</p>
											<p class="text-muted-content mt-1 text-xs">
												{resource.resourceType === 'vmcp'
													? m.supervision_labels_vmcp()
													: m.supervision_labels_mcp()} · {resource.sourceURL
													? m.supervision_labels_git()
													: m.supervision_labels_ui()}
											</p>
											{#if resource.inventoryError}
												<p class="text-error mt-2 max-w-md text-xs">{resource.inventoryError}</p>
											{/if}
										</td>
										<td>
											{#if resource.inventoryError}
												<span class="text-muted-content text-xs"
													>{m.supervision_labels_unavailable()}</span
												>
											{:else if resource.labels.length}
												<LabelBadges labels={resource.labels} max={3} />
											{:else}
												<span class="text-muted-content">{m.supervision_none()}</span>
											{/if}
										</td>
										<td class="tabular-nums">
											{#if resource.inventoryError}
												<span class="text-muted-content text-xs"
													>{m.supervision_labels_unavailable()}</span
												>
											{:else}
												{resource.reviewedToolCount} / {resource.toolCount}
											{/if}
										</td>
										<td>
											<div class="flex flex-col gap-1 text-xs">
												{#if resource.inventoryError}
													<span class="text-warning"
														>{m.supervision_labels_inventory_attention()}</span
													>
												{:else}
													{#if resource.pendingToolCount}<span
															>{m.supervision_labels_pending({
																count: resource.pendingToolCount
															})}</span
														>{/if}
													{#if resource.staleToolCount}<span class="text-warning"
															>{m.supervision_labels_stale({
																count: resource.staleToolCount
															})}</span
														>{/if}
													{#if !resource.pendingToolCount && !resource.staleToolCount}<span
															class="text-muted-content"
															>{m.supervision_labels_no_suggestions()}</span
														>{/if}
												{/if}
											</div>
										</td>
										<td
											class="text-muted-content text-xs"
											title={updated(resource.updatedAt).fullDate}
											>{updated(resource.updatedAt).relativeTime}</td
										>
										<td>
											{#if resource.inventoryError}
												<a
													class="text-link text-sm whitespace-nowrap"
													href={resourceEditor(resource)}>{m.supervision_labels_open_resource()}</a
												>
											{:else}
												<a
													class="text-link text-sm whitespace-nowrap"
													href={toolLabelReviewURL(resource)}
													aria-label={m.supervision_labels_manage_resource({
														name: resource.displayName || resource.resourceID
													})}>{m.supervision_labels_manage()}</a
												>
											{/if}
										</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>{/if}
			{:else if view === 'library'}
				<section class="paper gap-2" aria-label={m.supervision_labels_ai_tagger()}>
					<div class="flex flex-wrap items-center gap-2">
						<h2 class="font-semibold">{m.supervision_labels_ai_tagger()}</h2>
						<span class="badge badge-sm badge-soft"
							>{data.contentTagging.enabled
								? m.supervision_labels_on()
								: m.supervision_labels_off()}</span
						>
					</div>
					<p class="text-muted-content text-sm">{m.supervision_labels_ai_help()}</p>
					{#if data.contentTagging.enabled}<p class="text-muted-content text-xs">
							{data.contentTagging.mode === 'sync'
								? m.supervision_labels_ai_sync()
								: m.supervision_labels_ai_async()}
						</p>{/if}
				</section>
				{#if !capabilities.length && !contentLabels.length}<p
						class="paper text-muted-content text-sm"
					>
						{m.supervision_labels_no_matches()}
					</p>{/if}
				{#each [{ title: m.supervision_labels_capabilities(), help: m.supervision_labels_capabilities_help(), labels: capabilities }, { title: m.supervision_labels_content(), help: m.supervision_labels_content_help(), labels: contentLabels }] as group (group.title)}
					{#if group.labels.length}<section class="paper gap-3" aria-label={group.title}>
							<h2 class="text-base font-semibold">{group.title}</h2>
							<p class="text-muted-content text-sm">{group.help}</p>
							<dl class="flex flex-col divide-y divide-base-300">
								{#each group.labels as label (label)}<div
										class="grid gap-1 py-3 text-sm sm:grid-cols-[13rem_1fr]"
									>
										<dt class="font-mono text-xs font-medium">{label}</dt>
										<dd class="text-muted-content">{meaning(label)}</dd>
									</div>{/each}
							</dl>
						</section>{/if}
				{/each}
			{:else}
				<p class="text-muted-content text-sm">{m.supervision_labels_observed_help()}</p>
				{#if !data.observedTools.length}<p class="paper text-muted-content text-sm">
						{m.supervision_labels_empty_observed()}
					</p>{:else if !observed.length}<p class="paper text-muted-content text-sm">
						{m.supervision_labels_no_matches()}
					</p>{:else}
					<div class="overflow-x-auto rounded-md border border-base-300">
						<table class="table table-sm" aria-label={m.supervision_labels_observed()}>
							<thead
								><tr
									><th>{m.supervision_labels_tool()}</th><th>{m.supervision_labels_recorded()}</th
									><th>{m.supervision_labels_source()}</th><th>{m.supervision_labels_updated()}</th
									></tr
								></thead
							><tbody
								>{#each observed as tool (`${tool.server}:${tool.tool}`)}<tr
										><td
											><p class="font-mono text-xs font-medium">{tool.tool}</p>
											<p class="text-muted-content mt-1 text-xs">{tool.server}</p></td
										><td
											><LabelBadges
												labels={tool.labels}
											/>{#if !tool.labels.length}{m.supervision_none()}{/if}{#if tool.rationale}<p
													class="text-muted-content mt-2 max-w-xl text-xs"
												>
													{tool.rationale}
												</p>{/if}{#if tool.proposal}<details class="mt-2 text-xs">
													<summary class="cursor-pointer">{m.supervision_labels_advisory()}</summary
													>
													<div class="mt-2 flex flex-col gap-2">
														<LabelBadges labels={tool.proposal.labels} />
														<p class="text-muted-content max-w-xl">{tool.proposal.rationale}</p>
													</div>
												</details>{/if}</td
										><td><span class="text-xs">{sourceName(tool.source)}</span></td><td
											class="text-muted-content text-xs"
											title={updated(tool.updatedAt).fullDate}
											>{updated(tool.updatedAt).relativeTime}</td
										></tr
									>{/each}</tbody
							>
						</table>
					</div>
				{/if}
			{/if}
		</div>
	{/if}
</section>
