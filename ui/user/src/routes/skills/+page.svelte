<script lang="ts">
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import ObotCliBanner from '$lib/components/ObotCliBanner.svelte';
	import Search from '$lib/components/Search.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import type { Skill } from '$lib/services/nanobot/types';
	import { formatTimeAgo } from '$lib/time';
	import { setUrlParamAndUpdateUrl } from '$lib/url.js';
	import { TriangleAlert, PencilRuler } from 'lucide-svelte';
	import { untrack } from 'svelte';

	let { data } = $props();
	let query = $derived(page.url.searchParams.get('query') ?? '');

	let skills = $state<Skill[]>(untrack(() => data?.skills ?? []));
	let skillsTableData = $derived(
		query
			? skills.filter(
					(d) =>
						d.displayName?.toLowerCase().includes(query.toLowerCase()) ||
						d.name?.toLowerCase().includes(query.toLowerCase()) ||
						d.description?.toLowerCase().includes(query.toLowerCase())
				)
			: skills
	);

	function updateSearchQuery(value: string) {
		setUrlParamAndUpdateUrl(page.url, 'query', value);
	}
</script>

<Layout classes={{ navbar: 'bg-base-200' }} title="Skills">
	<div class="flex min-h-full flex-col gap-2">
		<ObotCliBanner description="Easily discover and install skills." />
		<div class="flex min-h-full flex-col">
			<div class="bg-base-200 dark:bg-base-100 sticky top-16 left-0 z-20 w-full py-1">
				<div class="mb-2">
					<Search
						class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
						value={query}
						onChange={updateSearchQuery}
						placeholder="Search skills..."
					/>
				</div>
			</div>

			<div class="dark:bg-base-300 bg-base-100 rounded-t-md shadow-sm">
				{@render skillsView()}
			</div>
		</div>
	</div>
</Layout>

{#snippet skillsView()}
	<div class="flex flex-col gap-2">
		{#if skills.length > 0}
			<Table
				data={skillsTableData}
				fields={['displayName', 'description', 'created']}
				noDataMessage="No skills found."
				classes={{
					root: 'rounded-none rounded-b-md shadow-none'
				}}
				columnMaxWidths={{ created: 240 }}
				sortable={['displayName', 'created']}
				headers={[
					{
						title: 'Name',
						property: 'displayName'
					}
				]}
				setRowClasses={(d) => {
					if (d.validationError) {
						return 'opacity-50 cursor-default dark:hover:bg-transparent hover:bg-transparent';
					}
					return '';
				}}
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
					{:else}
						{d[property as keyof typeof d]}
					{/if}
				{/snippet}
				{#snippet actions(_d)}
					<div></div>
				{/snippet}
			</Table>
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
{/snippet}

<svelte:head>
	<title>Obot | Skills</title>
</svelte:head>
