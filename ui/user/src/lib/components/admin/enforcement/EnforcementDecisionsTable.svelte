<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { VirtualPageTable } from '$lib/components/ui';
	import { agentLabel, allowlistServerLabel, kindLabel } from '$lib/enforcement';
	import type { EnforcementDecisionEvent } from '$lib/services';
	import { formatAuditLogTableTimestamp } from '$lib/time';
	import { throttle } from '$lib/utils';
	import { GripVertical, ShieldQuestionMark } from '@lucide/svelte';
	import { tick } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		onSelectRow?: (decision: EnforcementDecisionEvent) => void;
		// getDeviceHostname returns the enrolled hostname for a device, or undefined when
		// it could not be resolved — the Device cell then shows only the ID.
		getDeviceHostname: (deviceID?: string) => string | undefined;
	}

	let { onSelectRow, getDeviceHostname }: Props = $props();

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

	// serverDisplay prefers the server name the agent used, falling back to
	// whichever identity the device managed to resolve so a row is never blank.
	function serverDisplay(decision: EnforcementDecisionEvent): string {
		if (decision.serverName) return decision.serverName;
		const server = decision.server;
		if (!server) return '';
		if (server.hostname || server.url || server.package || server.connector) {
			return allowlistServerLabel(server);
		}
		return server.command ?? '';
	}

	// identifierParts splits a decision into the combined Identifier cell: the MCP server on the
	// primary line and the tool as a muted subline. When no server could be identified the tool
	// moves up to the primary line so the cell is never blank.
	function identifierParts(decision: EnforcementDecisionEvent) {
		const server = serverDisplay(decision);
		if (!server) return { primary: decision.tool || 'Unknown', secondary: undefined };
		return { primary: server, secondary: decision.tool || undefined };
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

{#snippet resultCell(decision: EnforcementDecisionEvent)}
	<td class="text-sm whitespace-nowrap">
		<div class="box-content flex h-full px-6">
			<div class="flex min-w-0 flex-1 items-center gap-2 py-4">
				{#if decision.decision === 'allow'}
					<span class="badge badge-success badge-sm shrink-0">Allowed</span>
				{:else}
					<span class="badge badge-error badge-sm shrink-0">Blocked</span>
				{/if}
				{#if decision.unresolved}
					<span
						class="text-warning inline-flex shrink-0"
						aria-label="Target could not be identified"
						use:tooltip={{ text: 'The device could not identify what this call targets' }}
					>
						<ShieldQuestionMark class="size-4" />
					</span>
				{/if}
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

{#snippet identifierCell(decision: EnforcementDecisionEvent)}
	{@const identifier = identifierParts(decision)}
	<td class="text-sm whitespace-nowrap">
		<div class="box-content flex h-full px-6">
			<div class="flex min-w-0 flex-1 flex-col justify-center py-2 leading-tight">
				<div class="flex min-w-0 items-center gap-2">
					<span class="truncate">{identifier.primary}</span>
					{#if decision.obotHosted}
						<span
							class="badge badge-ghost badge-sm shrink-0"
							use:tooltip={{ text: 'Hosted by this Obot instance' }}
						>
							Obot
						</span>
					{/if}
				</div>
				{#if identifier.secondary}
					<div class="text-muted-content mt-1 truncate text-xs">{identifier.secondary}</div>
				{/if}
			</div>
			{@render tdResizeHandler()}
		</div>
	</td>
{/snippet}

<!-- Data Table -->
<div>
	<div
		class="dark:bg-base-300 bg-base-100 flex w-full min-w-full flex-1 divide-y divide-gray-200 rounded-lg border border-transparent shadow-sm"
	>
		<VirtualPageTable class={twMerge('w-full flex-1 table-fixed border-collapse border-spacing-0')}>
			{#snippet header()}
				<thead>
					<tr bind:this={headerRowElement}>
						{@render th('Timestamp', { class: 'w-[28ch]', minWidth: '28ch' })}
						{@render th('Result', { class: 'w-[22ch]', minWidth: '22ch' })}
						{@render th('Agent', { class: 'w-[18ch]', minWidth: '18ch' })}
						{@render th('Tool Type', { class: 'w-[14ch]', minWidth: '14ch' })}
						{@render th('Identifier', { class: 'w-[36ch]', minWidth: '26ch' })}
						{@render th('Device', { class: 'w-[28ch]', minWidth: '20ch' })}
						{@render th('Reason', { class: 'w-[40ch]', minWidth: '24ch' })}
						{@render th('IP Address', { class: 'w-[22ch]', minWidth: '22ch' })}
					</tr>
				</thead>
			{/snippet}

			{#snippet children({ items }: { items: { index: number; data: EnforcementDecisionEvent }[] })}
				{#each items as item (item.data.id)}
					{@const d = item.data}
					<tr
						class={twMerge(
							'group m-0 h-14 text-sm leading-0 text-[0] transition-colors duration-300',
							onSelectRow && 'hover:bg-base-200 dark:hover:bg-base-400 cursor-pointer',
							d.decision === 'deny' && 'border-error border-l-2'
						)}
						onclick={() => onSelectRow?.(d)}
					>
						{@render td(formatAuditLogTableTimestamp(d.createdAt))}
						{@render resultCell(d)}
						{@render td(agentLabel(d.agent))}
						{@render td(kindLabel(d.kind))}
						{@render identifierCell(d)}
						{@render twoLine(d.deviceID, getDeviceHostname(d.deviceID))}
						{@render td(d.unresolvedReason || d.reason)}
						{@render td(d.clientIP)}
					</tr>
				{/each}
			{/snippet}
		</VirtualPageTable>
	</div>
</div>
