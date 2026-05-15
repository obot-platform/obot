<script lang="ts">
	import { VirtualPageTable } from '$lib/components/ui';
	import { mcpServersAndEntries, timePreference } from '$lib/stores';
	import { formatLogTimestamp } from '$lib/time';
	import { throttle } from '$lib/utils';
	import { GripVertical } from 'lucide-svelte';
	import { tick } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	let { data = [], onSelectRow, emptyContent, getUserDisplayName } = $props();

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

{#snippet td(content: string)}
	<td class="text-sm whitespace-nowrap">
		<div class="box-content flex h-full px-6">
			<div class="flex-1 truncate py-4">
				{content}
			</div>
			{@render tdResizeHandler()}
		</div>
	</td>
{/snippet}

{#snippet mutationIndicators(requestMutated: boolean, responseMutated: boolean)}
	<td class="text-sm whitespace-nowrap">
		<div class="box-content flex h-full px-6">
			<div class="flex flex-1 items-center gap-1 py-4">
				{#if requestMutated}
					<span
						class="rounded-full bg-orange-500/15 px-2 py-0.5 text-[11px] font-medium text-orange-600 dark:text-orange-400"
						title="Request was mutated"
					>
						Req
					</span>
				{/if}
				{#if responseMutated}
					<span
						class="rounded-full bg-orange-500/15 px-2 py-0.5 text-[11px] font-medium text-orange-600 dark:text-orange-400"
						title="Response was mutated"
					>
						Res
					</span>
				{/if}
			</div>
			{@render tdResizeHandler()}
		</div>
	</td>
{/snippet}

<!-- Data Table -->
<div
	bind:this={tableContainer}
	class="dark:bg-base-300 bg-base-100 flex w-full min-w-full flex-1 divide-y divide-gray-200 overflow-x-auto overflow-y-visible rounded-lg border border-transparent shadow-sm"
>
	{#if data.length}
		<VirtualPageTable class={twMerge('w-full flex-1 table-fixed border-collapse border-spacing-0')}>
			{#snippet header()}
				<thead>
					<tr bind:this={headerRowElement}>
						<th
							class="bg-base-300 dark:bg-base-200 text-muted-content sticky top-0 box-content w-[4ch] px-6 py-3 text-left text-xs font-medium tracking-wider uppercase"
						>
							<div>#</div>
						</th>

						{@render th('Timestamp', { class: 'w-[34ch]', minWidth: '34ch' })}

						{@render th('User', { class: 'w-[30ch]', minWidth: '30ch' })}

						{@render th('Server', { class: 'w-[24ch]', minWidth: '24ch' })}

						{@render th('Type', { class: 'w-[30ch]', minWidth: '30ch' })}

						{@render th('Identifier', { class: 'w-[24ch]', minWidth: '24ch' })}

						{@render th('Response Code', { class: 'w-[22ch]', minWidth: '22ch' })}

						{@render th('Response Time (ms)', { class: 'w-[26ch]', minWidth: '26ch' })}

						{@render th('Mutated', { class: 'w-[16ch]', minWidth: '16ch' })}

						{@render th('Client', { class: 'w-[19ch]', minWidth: '19ch' })}

						{@render th('IP Address', { class: 'w-[24ch]', minWidth: '24ch' })}
					</tr>
				</thead>
			{/snippet}

			{#snippet children({ items }: { items: { index: number; data: (typeof data)[0] }[] })}
				{#each items as item (item.data.id)}
					{@const d = item.data}

					<tr
						class={twMerge(
							'group m-0 h-14 text-sm leading-0 text-[0] transition-colors duration-300',
							onSelectRow && 'hover:bg-base-200 dark:hover:bg-base-400 cursor-pointer'
						)}
						onclick={() => onSelectRow?.(d)}
					>
						<td class="px-6 py-3">
							{item.index + 1}
						</td>
						{@render td(formatLogTimestamp(d.createdAt, timePreference.timeFormat))}
						{@render td(getUserDisplayName(d.userID))}
						{@render td(
							d.mcpID
								? serverAliases.get(d.mcpID) || d.mcpServerDisplayName
								: d.mcpServerDisplayName
						)}
						{@render td(d.callType)}
						{@render td(d.callIdentifier)}
						{@render td(d.responseStatus)}
						{@render td(d.processingTimeMs)}
						{@render mutationIndicators(d.requestMutated, d.responseMutated)}
						{@render td(d.client?.name)}
						{@render td(d.clientIP)}
					</tr>
				{/each}
			{/snippet}
		</VirtualPageTable>
	{:else}
		{@render emptyContent?.()}
	{/if}
</div>
