<script lang="ts">
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Search from '$lib/components/Search.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import type { SkillRepository } from '$lib/services/admin/types';
	import type { Skill } from '$lib/services/nanobot/types';
	import { formatTimeAgo } from '$lib/time';
	import { setUrlParamAndUpdateUrl } from '$lib/url';
	import { openUrl } from '$lib/utils.js';
	import { GitBranch, PencilRuler, TriangleAlert } from '@lucide/svelte';
	import { fade } from 'svelte/transition';

	interface Props {
		skills: Skill[];
		skillRepositories: SkillRepository[];
		showLicenseError: boolean;
		urlFilters: Record<string, string[]>;
		onFilter: (property: string, values: string[]) => void;
		onClearAllFilters: () => void;
	}

	let {
		skills,
		skillRepositories,
		showLicenseError,
		urlFilters,
		onFilter,
		onClearAllFilters
	}: Props = $props();

	let query = $derived(page.url.searchParams.get('query') ?? '');
	let skillRepositoriesMap = $derived(new Map(skillRepositories.map((d) => [d.id, d])));
	let skillsTableData = $derived(
		(query
			? skills.filter(
					(d) =>
						d.name?.toLowerCase().includes(query.toLowerCase()) ||
						d.description?.toLowerCase().includes(query.toLowerCase())
				)
			: skills
		).map((d) => ({
			...d,
			repository: d.repoID ? (skillRepositoriesMap.get(d.repoID)?.displayName ?? '') : ''
		}))
	);

	function updateSearchQuery(value: string) {
		setUrlParamAndUpdateUrl(page.url, 'query', value);
	}
</script>

<div class="flex min-h-full flex-col" in:fade={{ duration: PAGE_TRANSITION_DURATION }}>
	<div class="bg-base-200 dark:bg-base-100 sticky top-0 z-20 w-full">
		<div class="mb-2">
			<Search
				class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
				value={query}
				onChange={updateSearchQuery}
				placeholder="Search skills..."
			/>
		</div>
	</div>

	{#if skills.length > 0}
		<Table
			data={skillsTableData}
			fields={['displayName', 'description', 'created', 'repository']}
			noDataMessage="No skills found."
			classes={{
				root: 'rounded-md shadow-sm'
			}}
			columnMaxWidths={{ created: 240 }}
			sortable={['displayName', 'created', 'repository']}
			filterable={['repository']}
			headers={[
				{
					title: 'Name',
					property: 'displayName'
				}
			]}
			onClickRow={(d, isCtrlClick) => {
				if (d.valid) {
					openUrl(`/skills/${d.id}`, isCtrlClick);
				}
			}}
			setRowClasses={(d) => {
				if (d.validationError) {
					return 'opacity-50 cursor-default dark:hover:bg-transparent hover:bg-transparent';
				}
				return '';
			}}
			filters={urlFilters}
			{onFilter}
			{onClearAllFilters}
		>
			{#snippet onRenderColumn(property, d)}
				{#if property === 'displayName'}
					<span class="flex items-center gap-2">
						{d.displayName}
						{#if d.validationError}
							<div use:tooltip={{ text: d.validationError }}>
								<TriangleAlert class="size-3 text-warning" />
							</div>
						{/if}
					</span>
				{:else if property === 'created'}
					{formatTimeAgo(d.created).relativeTime}
				{:else if property === 'repository'}
					<span class="block min-w-0 truncate">{d.repository}</span>
				{:else if property === 'description'}
					<span class="line-clamp-2 text-sm">{d.description ?? '—'}</span>
				{:else}
					{d[property as keyof typeof d]}
				{/if}
			{/snippet}
			{#snippet actions(d)}
				<a
					class="btn btn-square btn-ghost hover:text-blue-500 btn-sm tooltip tooltip-left"
					href={`${d.repoURL}/tree/${d.repoRef || d.commitSHA || 'main'}/${d.relativePath}`}
					rel="external noopener noreferrer"
					target="_blank"
					onclick={(e) => e.stopPropagation()}
					data-tip="View Source on Git"
				>
					<GitBranch class="size-4" />
				</a>
			{/snippet}
		</Table>
	{:else if showLicenseError}
		<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
			<TriangleAlert class="size-12 text-warning" />
			<h4 class="text-muted-content text-lg font-semibold">License Error</h4>
			<p class="text-muted-content text-sm font-light">
				An issue occurred with fetching skills due to licensing. Please resolve outstanding
				licensing issues or contact support at
				<a href="mailto:info@obot.ai" class="text-link">info@obot.ai</a>.
			</p>
		</div>
	{:else}
		<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
			<PencilRuler class="text-base-content/80 size-24" />
			<h4 class="text-muted-content text-lg font-semibold">No current skills.</h4>
			<p class="text-muted-content text-sm font-light">
				Once a Git Source URL has been added, the skills <br />
				discovered will be viewable from here.
			</p>
		</div>
	{/if}
</div>
