<script lang="ts" generics="T extends object">
	import Skeleton from '$lib/components/Skeleton.svelte';
	import GraphTitle from '$lib/components/graph/GraphTitle.svelte';
	import HorizontalBarList from '$lib/components/graph/HorizontalBarList.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { m } from '$lib/i18n';
	import { ChevronsLeft, ChevronsRight } from '@lucide/svelte';

	type Props<T extends object> = {
		loading: boolean;
		title: string;
		data: T[];
		labelKey: Extract<keyof T, string>;
		valueKey: Extract<keyof T, string>;
		formatLabel?: (label: T[keyof T]) => string;
		formatTooltipText?: (row: T) => string;
		pageSize?: number;
	};

	let {
		loading,
		title,
		data,
		labelKey,
		valueKey,
		formatLabel,
		formatTooltipText,
		pageSize = 10
	}: Props<T> = $props();

	let page = $state(0);
	const total = $derived(data.length);
	const maxPage = $derived(Math.max(0, Math.ceil(total / pageSize) - 1));
	const currentPage = $derived(Math.min(page, maxPage));
	const paginated = $derived(data.slice(currentPage * pageSize, (currentPage + 1) * pageSize));
</script>

{#if loading}
	<Skeleton type="card" class="h-full min-h-72 w-full" />
{:else}
	<div class="paper h-full min-h-72 w-full gap-0 p-0">
		<GraphTitle {title} element="h4" />
		<div class="p-4">
			{#if paginated.length > 0}
				<HorizontalBarList
					data={paginated}
					{labelKey}
					{valueKey}
					{formatLabel}
					formatValue={(value) => Math.round(value).toString()}
				>
					{#snippet tooltipContent(item)}
						<div class="flex flex-col gap-0 text-xs">
							<div class="text-muted-content text-xs">{item.label}</div>
						</div>
						<div class="text-base-content font-semibold">
							{formatTooltipText ? formatTooltipText(item.row) : `${item.value}`}
						</div>
					{/snippet}
				</HorizontalBarList>
			{:else}
				<div class="text-muted-content flex h-72 items-center justify-center text-sm font-light">
					{m.audit_usage_usage_no_data_panel()}
				</div>
			{/if}

			{#if maxPage > 0}
				<div class="mt-4 flex items-center justify-center gap-4 border-t border-base-400 p-4 pb-0">
					<IconButton
						onclick={() => (page = Math.max(0, currentPage - 1))}
						disabled={currentPage === 0}
						tooltip={{ text: m.audit_usage_audit_logs_previous_page() }}
					>
						<ChevronsLeft class="size-5" />
					</IconButton>
					<span class="text-sm">
						{m.audit_usage_usage_page_info({
							page: currentPage + 1,
							pages: maxPage + 1,
							shown: Math.min(pageSize, total - currentPage * pageSize),
							total
						})}
					</span>
					<IconButton
						onclick={() => (page = Math.min(maxPage, currentPage + 1))}
						disabled={currentPage >= maxPage}
						tooltip={{ text: m.audit_usage_audit_logs_next_page() }}
					>
						<ChevronsRight class="size-5" />
					</IconButton>
				</div>
			{/if}
		</div>
	</div>
{/if}
