<script lang="ts">
	import { VirtualPageTable } from '$lib/components/ui';
	import type { AuditLogEvent } from '$lib/services';
	import { mcpServersAndEntries } from '$lib/stores';
	import { formatAuditLogTableTimestamp } from '$lib/time';
	import { throttle } from '$lib/utils';
	import { GripVertical } from '@lucide/svelte';
	import { tick, type Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		data?: AuditLogEvent[];
		onSelectRow?: (auditLog: AuditLogEvent) => void;
		emptyContent?: Snippet;
		getUserDisplayName: (userId: string, hasConflict?: () => boolean) => string;
	}

	let { data = [], onSelectRow, emptyContent, getUserDisplayName }: Props = $props();

	let startX = 0;
	let startWidth = 0;
	let currentCell: HTMLElement | null | undefined = undefined;
	let cellHandle: HTMLElement | null | undefined = undefined;

	let headerRowElement: HTMLElement | null | undefined = $state();

	let tableContainer: HTMLElement | null | undefined = $state();

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

	const serverAliases = $derived(
		new Map(
			[
				...mcpServersAndEntries.current.servers.filter((server) => server.alias),
				...mcpServersAndEntries.current.userConfiguredServers.filter((server) => server.alias)
			].map((server) => [server.id, server.alias])
		)
	);

	function actorLabel(actor: (typeof data)[number]['actor']) {
		if (actor.actorType === 'user' && actor.id) return getUserDisplayName(actor.id);
		return actor.id || (actor.actorType === 'unknown' ? 'Unknown' : actor.actorType);
	}

	function resolveServerName(ref?: { id?: string; name?: string }) {
		if (!ref) return undefined;
		const alias = ref.id ? serverAliases.get(ref.id) : undefined;
		return alias || ref.name || ref.id || undefined;
	}

	// identifierParts splits a target into the combined Identifier cell: the MCP server on the primary
	// line and the tool as a muted "› tool" subline. For MCP tool/resource/prompt events the server is
	// the parent and the tool is target.name; for server-level events there is no tool; for local tool
	// calls with no MCP server the tool is shown on the primary line.
	function identifierParts(target: (typeof data)[number]['target']) {
		if (target.parent) {
			const server = resolveServerName(target.parent);
			const tool = target.name || target.id;
			return {
				primary: server || tool || 'Unknown',
				secondary: server && tool ? `› ${tool}` : undefined
			};
		}
		if (target.targetType === 'mcp_server') {
			return { primary: resolveServerName(target) || 'Unknown', secondary: undefined };
		}
		return { primary: target.name || target.id || 'Unknown', secondary: undefined };
	}

	function eventTypeLabel(eventType: (typeof data)[number]['eventType']) {
		return eventType === 'mcp_call' ? 'Obot Gateway' : 'Local Agent Hook';
	}

	function formatDuration(ms?: number) {
		if (ms === undefined || ms === null) return '—';
		if (ms < 1000) return `${ms} ms`;
		return `${(ms / 1000).toFixed(ms < 10000 ? 2 : 1)} s`;
	}
</script>

{#snippet thResizeHandler()}
	<button
		class="resize-handle sticky right-0 ml-auto flex min-h-full cursor-col-resize items-center outline-none"
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
			'dark:bg-base-200 bg-base-300 text-muted-content sticky top-0 box-content w-[24ch] truncate text-left text-xs font-medium tracking-wider uppercase',
			klass
		)}
		data-min-width={minWidth}
	>
		<div class="box-content flex h-full px-6">
			<div class=" self-center py-3 whitespace-break-spaces">{content}</div>
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

{#snippet outcomeCell(outcome: (typeof data)[number]['outcome'])}
	<td class="text-sm whitespace-nowrap">
		<div class="box-content flex h-full px-6">
			<div class="flex min-w-0 flex-1 flex-col justify-center py-2 leading-tight">
				<span
					class={twMerge(
						'w-fit rounded-full px-2 py-0.5 text-xs font-medium capitalize',
						outcome.status === 'success' && 'bg-success/15 text-success',
						['failure', 'denied', 'timeout'].includes(outcome.status) && 'bg-error/15 text-error',
						outcome.status === 'unknown' && 'bg-base-400 text-muted-content'
					)}>{outcome.status}</span
				>
				{#if outcome.httpStatus || outcome.reason}
					<div class="text-muted-content mt-1 truncate text-xs">
						{outcome.httpStatus || outcome.reason}
					</div>
				{/if}
			</div>
			{@render tdResizeHandler()}
		</div>
	</td>
{/snippet}

<!-- Data Table -->
<div
	bind:this={tableContainer}
	id="mcp-audit-logs-table"
	class="dark:bg-base-300 bg-base-100 flex w-full min-w-full flex-1 divide-y divide-gray-200 overflow-x-auto overflow-y-visible rounded-lg border border-transparent shadow-sm"
>
	{#if data.length}
		<VirtualPageTable class={twMerge('w-full flex-1 table-fixed border-collapse border-spacing-0')}>
			{#snippet header()}
				<thead>
					<tr bind:this={headerRowElement}>
						{@render th('Time', { class: 'w-[28ch]', minWidth: '24ch' })}
						{@render th('Source', { class: 'w-[20ch]', minWidth: '18ch' })}
						{@render th('Actor', { class: 'w-[26ch]', minWidth: '22ch' })}
						{@render th('Operation', { class: 'w-[20ch]', minWidth: '18ch' })}
						{@render th('Identifier', { class: 'w-[32ch]', minWidth: '26ch' })}
						{@render th('Status', { class: 'w-[18ch]', minWidth: '16ch' })}
						{@render th('Client', { class: 'w-[22ch]', minWidth: '18ch' })}
						{@render th('Duration', { class: 'w-[16ch]', minWidth: '14ch' })}
					</tr>
				</thead>
			{/snippet}

			{#snippet children({ items }: { items: { index: number; data: (typeof data)[0] }[] })}
				{#each items as item (item.data.id)}
					{@const d = item.data}
					{@const identifier = identifierParts(d.target)}

					<tr
						id={`mcp-audit-log-${item.index}`}
						class={twMerge(
							'virtual-list-row group m-0 h-14 text-sm leading-0 text-[0] transition-colors duration-300',
							onSelectRow && 'hover:bg-base-200 dark:hover:bg-base-400 cursor-pointer'
						)}
						onclick={() => onSelectRow?.(d)}
					>
						{@render td(formatAuditLogTableTimestamp(d.timestamp.occurredAt))}
						{@render td(eventTypeLabel(d.eventType))}
						{@render twoLine(actorLabel(d.actor), d.actor.actorType)}
						{@render td(d.action.operation)}
						{@render twoLine(identifier.primary, identifier.secondary)}
						{@render outcomeCell(d.outcome)}
						{@render td(d.client || '—')}
						{@render td(formatDuration(d.outcome.durationMs))}
					</tr>
				{/each}
			{/snippet}
		</VirtualPageTable>
	{:else}
		{@render emptyContent?.()}
	{/if}
</div>
