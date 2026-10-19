<script lang="ts">
	import { VirtualPageTable } from '$lib/components/ui';
	import { m } from '$lib/i18n';
	import { type EgressAuditLog } from '$lib/services';
	import { formatAuditLogTableTimestamp } from '$lib/time';
	import { throttle } from '$lib/utils';
	import { GripVertical } from '@lucide/svelte';
	import { tick } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		onSelectRow?: (auditLog: EgressAuditLog) => void;
		getUserDisplayName: (userID: string) => string;
		getEnvironmentName: (environmentID: string) => string;
		outcomeLabel: (value: string) => string;
	}

	let { onSelectRow, getUserDisplayName, getEnvironmentName, outcomeLabel }: Props = $props();

	let startX = 0;
	let startWidth = 0;
	let currentCell: HTMLElement | null | undefined = undefined;
	let cellHandle: HTMLElement | null | undefined = undefined;

	let headerRowElement: HTMLElement | null | undefined = $state();

	const resizeColumn = throttle((ev: PointerEvent) => {
		const diff = ev.pageX - startX;
		const minWidth = currentCell?.getAttribute('data-min-width') ?? '0ch';

		currentCell!.style.width = `max(${minWidth}, ${startWidth + diff}px)`;
	}, 1000 / 60);

	const stopResize = async () => {
		document.removeEventListener('pointermove', resizeColumn);
		document.removeEventListener('pointerup', stopResize);

		await tick();

		cellHandle?.scrollIntoView({ block: 'nearest', inline: 'center', behavior: 'smooth' });
	};
</script>

{#snippet thResizeHandler()}
	<button
		class="resize-handle sticky right-0 ml-auto flex min-h-full cursor-col-resize items-center outline-none"
		tabindex="-1"
		aria-hidden="true"
		{@attach (node) => {
			const pointerDownHandler = (ev: PointerEvent) => {
				currentCell = (ev.target as HTMLElement).closest('th');
				if (!currentCell) return;

				cellHandle = ev.currentTarget as typeof cellHandle;

				startX = ev.pageX;
				startWidth = currentCell.clientWidth;

				document.addEventListener('pointermove', resizeColumn);
				document.addEventListener('pointerup', stopResize);
			};

			node.addEventListener('pointerdown', pointerDownHandler);

			return () => {
				node.removeEventListener('pointerdown', pointerDownHandler);
			};
		}}
	>
		<GripVertical class="w-3" />
	</button>
{/snippet}

{#snippet tdResizeHandler()}
	<button
		class="resize-handle ml-auto flex min-h-full cursor-col-resize items-center opacity-0 outline-none group-hover:opacity-100"
		tabindex="-1"
		aria-hidden="true"
		onclick={(ev) => ev.stopPropagation()}
		{@attach (node) => {
			const pointerDownHandler = (ev: PointerEvent) => {
				const td = (ev.target as HTMLElement).closest('td');
				if (!td) return;

				cellHandle = ev.currentTarget as typeof cellHandle;

				const row = td.closest('tr');
				if (!row) return;

				const index = Array.from(row.children).indexOf(td);

				currentCell = headerRowElement?.children.item(index) as typeof currentCell;
				if (!currentCell) return;

				startX = ev.pageX;
				startWidth = currentCell.clientWidth;

				document.addEventListener('pointermove', resizeColumn);
				document.addEventListener('pointerup', stopResize);
			};

			node.addEventListener('pointerdown', pointerDownHandler);

			return () => {
				node.removeEventListener('pointerdown', pointerDownHandler);
			};
		}}
	>
		<GripVertical class="w-3" />
	</button>
{/snippet}

{#snippet th(content: string, { class: klass = '', minWidth = '0ch' } = {})}
	<th
		class={twMerge(
			'dark:bg-base-200 bg-base-300 text-muted-content box-content w-[24ch] truncate text-left text-xs font-medium tracking-wider uppercase',
			klass
		)}
		data-min-width={minWidth}
	>
		<div class="box-content flex h-full px-6">
			<div class=" self-center py-3 text-nowrap">{content}</div>
			{@render thResizeHandler()}
		</div>
	</th>
{/snippet}

{#snippet td(content: string | number | boolean | null | undefined)}
	<td class="text-sm whitespace-nowrap">
		<div class="box-content flex h-full px-6">
			<div class="flex-1 truncate py-4">
				{content}
			</div>
			{@render tdResizeHandler()}
		</div>
	</td>
{/snippet}

{#snippet twoLine(primary: string | number | undefined, secondary?: string | number)}
	<td class="text-sm whitespace-nowrap">
		<div class="box-content flex h-full px-6">
			<div class="flex min-w-0 flex-1 flex-col justify-center py-2 leading-tight">
				<div class="truncate">{primary ?? '—'}</div>
				{#if secondary !== undefined && secondary !== ''}
					<div class="text-muted-content mt-1 truncate text-xs">{secondary}</div>
				{/if}
			</div>
			{@render tdResizeHandler()}
		</div>
	</td>
{/snippet}

{#snippet timestampCell(timestamp: string)}
	<td class="text-sm whitespace-nowrap">
		<div class="box-content flex h-full px-6">
			<div class="flex min-w-0 flex-1 items-center gap-2 py-4">
				<span class="truncate">{timestamp}</span>
			</div>
			{@render tdResizeHandler()}
		</div>
	</td>
{/snippet}

{#snippet responseCell(entry: EgressAuditLog)}
	<td class="text-sm whitespace-nowrap">
		<div class="box-content flex h-full px-6">
			<div class="flex min-w-0 flex-1 gap-1 flex-col justify-center py-2 leading-tight">
				<span
					class={twMerge(
						'w-fit rounded-full px-2 py-0.5 text-xs font-medium capitalize',
						entry.responseStatus >= 200 &&
							entry.responseStatus < 300 &&
							'bg-success/15 text-success',
						entry.responseStatus >= 300 &&
							entry.responseStatus < 400 &&
							'bg-warning/15 text-warning',
						entry.responseStatus >= 400 && 'bg-error/15 text-error',
						!entry.responseStatus && 'bg-base-300 dark:bg-base-100'
					)}
				>
					{outcomeLabel(entry.outcome)}
				</span>
				{#if entry.responseStatus}
					<span class="text-muted-content text-xs">
						{entry.responseStatus}
					</span>
				{/if}
			</div>
			{@render tdResizeHandler()}
		</div>
	</td>
{/snippet}

{#snippet requestCell(method: string, host: string, path: string)}
	<td class="text-sm whitespace-nowrap">
		<div class="box-content flex h-full px-6">
			<div class="flex-1 truncate py-4">
				<span class="font-mono badge badge-soft badge-xs">{method}</span>
				{host}{path}
			</div>
			{@render tdResizeHandler()}
		</div>
	</td>
{/snippet}

<div>
	<div
		class="dark:bg-base-300 bg-base-100 flex w-full min-w-full flex-1 divide-y divide-gray-200 rounded-lg border border-transparent shadow-sm"
	>
		<VirtualPageTable class={twMerge('w-full flex-1 table-fixed border-collapse border-spacing-0')}>
			{#snippet header()}
				<thead>
					<tr bind:this={headerRowElement}>
						{@render th(m.core_col_timestamp(), { class: 'w-[28ch]', minWidth: '28ch' })}
						{@render th(m.audit_usage_egress_user(), { class: 'w-[24ch]', minWidth: '18ch' })}
						{@render th(m.audit_usage_egress_account(), { class: 'w-[22ch]', minWidth: '18ch' })}
						{@render th(m.audit_usage_egress_environment_session(), {
							class: 'w-[28ch]',
							minWidth: '24ch'
						})}
						{@render th(m.audit_usage_egress_request(), { class: 'w-[36ch]', minWidth: '24ch' })}
						{@render th(m.audit_usage_egress_response(), { class: 'w-[22ch]', minWidth: '18ch' })}
						{@render th(m.audit_usage_egress_duration(), { class: 'w-[16ch]', minWidth: '14ch' })}
					</tr>
				</thead>
			{/snippet}

			{#snippet children({ items }: { items: { index: number; data: EgressAuditLog }[] })}
				{#each items as item (item.data.id)}
					{@const entry = item.data}
					<tr
						class={twMerge(
							'virtual-list-row group m-0 h-14 text-sm leading-0 text-[0] transition-colors duration-300',
							onSelectRow &&
								'hover:bg-base-200 dark:hover:bg-base-400 focus-visible:bg-base-200 dark:focus-visible:bg-base-400 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-primary cursor-pointer'
						)}
						tabindex={onSelectRow ? 0 : undefined}
						onclick={() => onSelectRow?.(entry)}
						onkeydown={(event) => {
							if (event.target !== event.currentTarget || !onSelectRow) return;
							if (event.key === 'Enter' || event.key === ' ') {
								event.preventDefault();
								onSelectRow(entry);
							}
						}}
					>
						{@render timestampCell(formatAuditLogTableTimestamp(entry.createdAt))}
						{@render td(getUserDisplayName(entry.userID))}
						{@render td(entry.claudeAccount)}
						{@render twoLine(getEnvironmentName(entry.environmentID), entry.claudeSession)}
						{@render requestCell(entry.method, entry.host, entry.path)}
						{@render responseCell(entry)}
						{@render td(`${entry.durationMs} ms`)}
					</tr>
				{/each}
			{/snippet}
		</VirtualPageTable>
	</div>
</div>
