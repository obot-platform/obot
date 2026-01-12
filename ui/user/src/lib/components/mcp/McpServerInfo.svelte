<script lang="ts">
	import { browser } from '$app/environment';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { toHTMLFromMarkdownWithNewTabLinks } from '$lib/markdown';
	import { type MCPCatalogServer, type ProjectMCP } from '$lib/services';
	import type { MCPCatalogEntry } from '$lib/services/admin/types';
	import { responsive } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		entry: MCPCatalogEntry | MCPCatalogServer | ProjectMCP;
		parent?: Props['entry'];
		descriptionPlaceholder?: string;
		preContent?: Snippet;
	}

	type EntryDetail = {
		label: string;
		value: string | string[];
		link?: string;
		class?: string;
		showTooltip?: boolean;
		editable?: boolean;
		catalogId?: string;
	};

	function convertEntryDetails(entry: MCPCatalogEntry | MCPCatalogServer | ProjectMCP) {
		let items: Record<string, EntryDetail> = {};
		if (!('isCatalogEntry' in entry) && ('manifest' in entry || 'mcpID' in entry)) {
			items = {
				requiredConfig: {
					label: 'Required Configuration',
					value:
						'manifest' in entry ? (entry.manifest?.env?.map((e) => e.key).join(', ') ?? []) : []
				},
				users: {
					label: 'Users',
					value: ''
				},
				published: {
					label: 'Published',
					value: formatTimeAgo(entry.created).relativeTime
				},
				moreInfo: {
					label: 'More Information',
					value: ''
				},
				monthlyToolCalls: {
					label: 'Monthly Tool Calls',
					value: ''
				},
				lastUpdated: {
					label: 'Last Updated',
					value: 'updated' in entry ? formatTimeAgo(entry.updated).relativeTime : ''
				}
			};
		} else if ('isCatalogEntry' in entry) {
			items = {
				requiredConfig: {
					label: 'Required Configuration',
					value:
						entry.manifest?.env
							?.filter((e) => e.required)
							.map((e) => e.name)
							.join(', ') ?? []
				},
				users: {
					label: 'Users',
					value: ''
				},
				published: {
					label: 'Published',
					value: formatTimeAgo(entry.created).relativeTime
				},
				moreInfo: {
					label: 'More Information',
					value: entry.manifest?.repoURL ?? '',
					link: entry.manifest?.repoURL ?? '',
					class: 'line-clamp-1',
					showTooltip: true
				},
				monthlyToolCalls: {
					label: 'Monthly Tool Calls',
					value: ''
				},
				lastUpdated: {
					label: 'Last Updated',
					value: ''
				}
			};
		}

		const details = responsive.isMobile
			? [
					items.requiredConfig,
					items.moreInfo,
					items.users,
					items.monthlyToolCalls,
					items.published,
					items.lastUpdated
				]
			: [
					items.requiredConfig,
					items.users,
					items.published,
					items.moreInfo,
					items.monthlyToolCalls,
					items.lastUpdated
				];
		return details.filter((d) => d);
	}

	let {
		entry,
		parent,
		descriptionPlaceholder = 'No description available',
		preContent
	}: Props = $props();
	let details = $derived(convertEntryDetails(entry));
	let description = $derived.by(() => {
		const descriptions = [
			() => ('description' in entry ? entry.description : undefined),
			() => ('manifest' in entry ? entry.manifest.description : undefined),
			() => (parent && 'manifest' in parent ? parent?.manifest?.description : undefined)
		];

		for (const fn of descriptions) {
			const desc = fn();
			if (desc) {
				return desc;
			}
		}

		return '';
	});
</script>

{#if preContent}
	{@render preContent()}
{/if}

<div class="flex w-full flex-col gap-4 md:flex-row">
	<div
		class="dark:bg-surface1 dark:border-surface3 bg-background flex h-fit w-full flex-col gap-4 rounded-lg border border-transparent p-4 shadow-sm"
	>
		{#if description && browser}
			<div class="milkdown-content">
				{@html toHTMLFromMarkdownWithNewTabLinks(description)}
			</div>
		{:else}
			<p class="text-md text-on-surface1 text-center font-light italic">
				{descriptionPlaceholder}
			</p>
		{/if}

		{@render detailsSection()}
	</div>
</div>

{#snippet detailsSection()}
	<div class="flex flex-col gap-2">
		<h4 class="text-md font-semibold">Details</h4>
		<div class="flex flex-col gap-4">
			{#each details.filter( (d) => (Array.isArray(d.value) ? d.value.length > 0 : d.value) ) as detail, i (i)}
				<div
					class="dark:bg-surface2 dark:border-surface3 border-surface2 rounded-md border bg-gray-50 p-3"
				>
					<p class="mb-1 text-xs font-medium">{detail.label}</p>
					{#if detail.link}
						<!-- eslint-disable svelte/no-navigation-without-resolve -- external URL? -->
						<a
							href={detail.link}
							class="text-link"
							target="_blank"
							rel="noopener noreferrer external"
						>
							{#if detail.showTooltip && typeof detail.value === 'string'}
								<span use:tooltip={detail.value}>
									{@render detailSection(detail)}
								</span>
							{:else}
								{@render detailSection(detail)}
							{/if}
						</a>
						<!-- eslint-enable svelte/no-navigation-without-resolve -->
					{:else if detail.showTooltip && typeof detail.value === 'string'}
						<span use:tooltip={detail.value}>
							{@render detailSection(detail)}
						</span>
					{:else}
						{@render detailSection(detail)}
					{/if}
				</div>
			{/each}
		</div>
	</div>
{/snippet}

{#snippet detailSection(detail: EntryDetail)}
	{#if typeof detail.value === 'string'}
		<p class={twMerge('text-xs font-light', detail.class)}>{detail.value}</p>
	{:else if Array.isArray(detail.value)}
		<ul class="flex flex-col gap-1">
			{#each detail.value as value, i (i)}
				<li class="text-xs font-light">{value}</li>
			{/each}
		</ul>
	{:else}
		<p class="text-xs font-light">-</p>
	{/if}
{/snippet}
