<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import { m } from '$lib/i18n';
	import { createLayoutDrag, locationKey } from '$lib/runes/dashboard/layoutDrag.svelte';
	import {
		DEVICE_STAT_IDS,
		MODEL_STAT_IDS,
		PLATFORM_STAT_IDS,
		appendBox,
		appendHeader,
		appendWidget,
		defaultHideWhenEmpty,
		findHeader,
		findPlacement,
		placedWidgetIds,
		removeBox,
		removeHeader,
		removeWidget,
		rowsAt,
		setHeaderText,
		setHideWhenEmpty,
		toggleDeviceStat,
		toggleModelStat,
		togglePlatformStat,
		type DashboardConfig,
		type DashboardRow,
		type WidgetId
	} from '$lib/services/dashboard/layout';
	import HeaderVisibilityOptions from './HeaderVisibilityOptions.svelte';
	import {
		deviceStatLabel,
		modelStatLabel,
		platformStatLabel,
		widgetLabel
	} from './widgets/labels';
	import { GripVertical, X } from '@lucide/svelte';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	type Props = {
		config: DashboardConfig;
		defaults: DashboardConfig;
		available: WidgetId[];
		emptyWidgets: WidgetId[];
		onChange: (config: DashboardConfig) => void;
		onReset: () => void;
	};

	let { config, defaults, available, emptyWidgets, onChange, onReset }: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let layoutScroller = $state<HTMLElement>();

	function copyConfig(value: DashboardConfig): DashboardConfig {
		return $state.snapshot(value);
	}

	let draft = $state<DashboardConfig | null>(null);
	let resetPending = false;
	const editing = $derived(draft ?? config);
	const layoutDrag = createLayoutDrag({
		editing: () => editing,
		stage,
		scroller: () => layoutScroller
	});

	const placed = $derived(placedWidgetIds(editing.rows));
	const palette = $derived(available.filter((id) => !placed.has(id)));

	function stage(next: DashboardConfig) {
		resetPending = false;
		draft = next;
	}

	export function open() {
		draft = copyConfig(config);
		resetPending = false;
		dialog?.open();
	}

	function save() {
		if (resetPending) onReset();
		else if (draft && JSON.stringify(editing) !== JSON.stringify(config)) onChange(editing);
		dialog?.close();
	}

	function cancel() {
		dialog?.close();
	}

	function discard() {
		draft = null;
		resetPending = false;
		layoutDrag.end();
	}

	function resetDraft() {
		draft = copyConfig(defaults);
		resetPending = true;
	}

	function headerText(id: string): string {
		return findHeader(editing.rows, id)?.text ?? '';
	}

	function previewHideWhenEmpty(id: WidgetId): boolean {
		return findPlacement(editing.rows, id)?.hideWhenEmpty ?? defaultHideWhenEmpty(id);
	}

	function dragPreviewModel():
		| { kind: 'widget'; id: WidgetId; label: string }
		| { kind: 'header' | 'box'; label: string }
		| null {
		const subject = layoutDrag.dragging;
		if (!subject) return null;
		if (subject.kind === 'widget')
			return { kind: 'widget', id: subject.id, label: widgetLabel(subject.id) };
		if (subject.kind === 'new-header') return { kind: 'header', label: '' };
		if (subject.kind === 'header') return { kind: 'header', label: headerText(subject.id) };
		return { kind: 'box', label: m.dashboard_box() };
	}

	function gapPreviewSpacing(parent: number[], index: number): string {
		const rows = rowsAt(editing.rows, parent) ?? [];
		const insideBox = parent.length > 0;
		const betweenRows = index > 0 && index < rows.length;
		const boxStart = insideBox && index === 0;
		if (betweenRows || boxStart) return 'my-3 mx-3';
		if (index === rows.length) return 'mt-3 mx-3';
		if (index === 0) return 'mb-3 mx-3';
		return '';
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	onClose={discard}
	title={m.dashboard_configure()}
	class="w-dvw h-dvh max-w-[calc(100dvw-2rem)] max-h-[calc(100dvh-2rem)]"
	classes={{
		header: 'mb-2'
	}}
>
	<div class="flex gap-4 h-[calc(100%-6rem)]">
		<section class="flex min-w-0 grow flex-col">
			<div
				class="flex items-end justify-between gap-2 border border-b-none border-base-300 dark:border-base-100 rounded-t-xl p-3"
			>
				<h4 class="text-xs font-medium uppercase">{m.dashboard_layout()}</h4>
			</div>
			<div
				data-layout-scroll
				bind:this={layoutScroller}
				class="flex min-h-80 flex-col grow gap-1 overflow-y-auto default-scrollbar-thin rounded-b-xl rounded-t-none bg-base-200 dark:bg-base-100/50 p-3"
			>
				{#if editing.rows.length === 0}
					<p class="text-muted-content py-8 text-center text-sm">{m.dashboard_no_widgets()}</p>
				{/if}
				{@render layoutList(editing.rows, [])}
			</div>
		</section>
		<section class="flex w-xl flex-col">
			<div
				class="flex items-end justify-between gap-2 border border-b-none border-base-300 dark:border-base-100 rounded-t-xl p-3"
			>
				<h4 class="text-xs font-medium uppercase">{m.dashboard_available_widgets()}</h4>
			</div>
			<ul
				data-palette-scroll
				class="flex flex-col grow gap-2 rounded-xl rounded-t-none bg-base-200 dark:bg-base-100/50 p-3 overflow-y-auto default-scrollbar-thin"
			>
				<li
					class="group flex cursor-grab items-center justify-between gap-2 rounded-lg bg-base-100 px-3 py-2 dark:bg-base-300"
					data-drag-preview
					data-palette-header
					onpointerdown={(event) => layoutDrag.begin(event, { kind: 'new-header' })}
				>
					<div class="flex items-center gap-2">
						<GripVertical
							class="text-muted-content group-hover:text-base-content size-4 shrink-0 transition-colors"
						/>
						<span class="text-sm">{m.dashboard_header()}</span>
					</div>
					<button
						type="button"
						class="btn btn-ghost btn-xs"
						aria-label="{m.dashboard_add()} {m.dashboard_header()}"
						onclick={() => stage(appendHeader(editing))}
					>
						{m.dashboard_add()}
					</button>
				</li>
				<li
					class="group flex cursor-grab items-center justify-between gap-2 rounded-lg bg-base-100 px-3 py-2 dark:bg-base-300"
					data-drag-preview
					data-palette-box
					onpointerdown={(event) => layoutDrag.begin(event, { kind: 'new-box' })}
				>
					<div class="flex items-center gap-2">
						<GripVertical
							class="text-muted-content group-hover:text-base-content size-4 shrink-0 transition-colors"
						/>
						<span class="text-sm">{m.dashboard_box()}</span>
					</div>
					<button
						type="button"
						class="btn btn-ghost btn-xs"
						aria-label="{m.dashboard_add()} {m.dashboard_box()}"
						onclick={() => stage(appendBox(editing))}
					>
						{m.dashboard_add()}
					</button>
				</li>
				{#each palette as id (id)}
					{@const label = widgetLabel(id)}
					<li
						class="flex items-center justify-between gap-2 rounded-lg px-3 py-2 bg-base-100 dark:bg-base-300 cursor-grab group"
						data-drag-preview
						onpointerdown={(event) => layoutDrag.begin(event, { kind: 'widget', id })}
						out:fade={{ duration: 100 }}
					>
						<div class="flex items-center gap-2">
							<GripVertical
								class="text-muted-content transition-colors group-hover:text-base-content size-4 shrink-0"
							/>
							<span class="text-sm">{label}</span>
						</div>
						<button
							type="button"
							class="btn btn-ghost btn-xs"
							aria-label="{m.dashboard_add()} {label}"
							onclick={() => stage(appendWidget(editing, id))}
						>
							{m.dashboard_add()}
						</button>
					</li>
				{/each}
			</ul>
		</section>
	</div>
	<div class="flex justify-end gap-2 pt-4">
		<button
			type="button"
			class="btn btn-ghost text-muted-content hover:text-base-content"
			onclick={resetDraft}
		>
			{m.dashboard_reset_layout()}
		</button>
		<button type="button" class="btn btn-secondary" onclick={cancel}>
			{m.common_cancel()}
		</button>
		<button type="button" class="btn btn-primary" onclick={save}>
			{m.core_save()}
		</button>
	</div>
</ResponsiveDialog>

{#snippet settledWidget(id: WidgetId, label: string, spacing = '')}
	<div
		class={twMerge(
			'bg-base-100 dark:bg-base-300 pointer-events-none min-w-0 flex-1 rounded-lg p-3 opacity-65',
			spacing
		)}
		data-drag-settled
		aria-hidden="true"
	>
		<div class="flex items-start justify-between gap-2">
			<div class="mt-0.5 flex min-w-0 flex-1 items-center gap-2">
				<GripVertical class="text-muted-content size-4 shrink-0" />
				<span class="truncate text-sm font-medium">{label}</span>
			</div>
			<X class="size-4 shrink-0" />
		</div>
		{#if id === 'platform-stats'}
			<div class="mt-3 flex flex-col gap-1">
				{#each PLATFORM_STAT_IDS as statId (statId)}
					<label class="flex items-center gap-2 text-xs">
						<input
							type="checkbox"
							class="checkbox checkbox-xs"
							tabindex="-1"
							checked={editing.platformStats.includes(statId)}
						/>
						{platformStatLabel(statId)}
					</label>
				{/each}
			</div>
		{:else if id === 'model-stats'}
			<div class="mt-3 flex flex-col gap-1">
				{#each MODEL_STAT_IDS as statId (statId)}
					<label class="flex items-center gap-2 text-xs">
						<input
							type="checkbox"
							class="checkbox checkbox-xs"
							tabindex="-1"
							checked={editing.modelStats.includes(statId)}
						/>
						{modelStatLabel(statId)}
					</label>
				{/each}
			</div>
		{:else if id === 'device-scan-stats'}
			<div class="mt-3 flex flex-col gap-1">
				{#each DEVICE_STAT_IDS as statId (statId)}
					<label class="flex items-center gap-2 text-xs">
						<input
							type="checkbox"
							class="checkbox checkbox-xs"
							tabindex="-1"
							checked={editing.deviceStats.includes(statId)}
						/>
						{deviceStatLabel(statId)}
					</label>
				{/each}
			</div>
		{/if}
		<div class="mt-3 flex items-center gap-2 text-xs">
			<input
				type="checkbox"
				class="checkbox checkbox-xs"
				tabindex="-1"
				checked={previewHideWhenEmpty(id)}
			/>
			{m.dashboard_hide_when_empty()}
		</div>
	</div>
{/snippet}

{#snippet settledGap(spacing: string)}
	{@const model = dragPreviewModel()}
	{#if model?.kind === 'widget'}
		<div class="pointer-events-none flex items-stretch gap-1">
			{@render settledWidget(model.id, model.label, spacing)}
		</div>
	{:else if model?.kind === 'header'}
		<div
			class={twMerge(
				'bg-base-100 dark:bg-base-300 pointer-events-none mx-3 rounded-lg p-3 opacity-50',
				spacing
			)}
			data-drag-settled
			aria-hidden="true"
		>
			<div class="flex items-center gap-2">
				<GripVertical class="text-muted-content size-4 shrink-0" />
				<div class="input-text-filled w-full text-sm">
					{model.label || m.dashboard_header_placeholder()}
				</div>
			</div>
		</div>
	{:else if model}
		<div
			class={twMerge(
				'border-base-300 dark:border-base-400 bg-base-100/35 dark:bg-base-300/35 pointer-events-none mx-3 rounded-xl border px-3 py-2 opacity-50',
				spacing
			)}
			data-drag-settled
			aria-hidden="true"
		>
			<div class="flex items-center gap-2">
				<GripVertical class="text-muted-content size-4 shrink-0" />
				<span class="text-sm font-medium">{model.label}</span>
			</div>
		</div>
	{/if}
{/snippet}

{#snippet rowDropSlot(slotParent: number[], index: number)}
	{@const key = locationKey([...slotParent, index])}
	{#if layoutDrag.gapGhost(key)}
		{@render settledGap(gapPreviewSpacing(slotParent, index))}
	{/if}
	<div
		role="presentation"
		data-drop-slot={key}
		data-layout-gap={key}
		data-drop-active={layoutDrag.dropKey === `gap-${key}` ? 'true' : undefined}
		class={twMerge(
			'mx-3 shrink-0 rounded-full transition-colors',
			layoutDrag.gapGhost(key) ? 'h-0' : 'h-3',
			layoutDrag.dropKey === `gap-${key}` && !layoutDrag.gapGhost(key) && 'bg-primary/50'
		)}
	></div>
{/snippet}

{#snippet layoutList(rows: DashboardRow[], parent: number[])}
	{#each rows as row, rowIndex (row.kind === 'widgets' ? row.items
				.map((placement) => placement.id)
				.join() : row.id)}
		{#if row.kind === 'box'}
			{#if rowIndex === 0 || rows[rowIndex - 1].kind !== 'box'}
				{@render rowDropSlot(parent, rowIndex)}
			{/if}
			<div
				class="rounded-xl border-base-300 dark:border-base-400 bg-base-100/35 dark:bg-base-300/35 border px-3 py-2 mx-3"
				role="group"
				data-layout-box
				data-box-location={locationKey([...parent, rowIndex])}
				data-child-count={row.rows.length}
			>
				<div class="mb-1 flex items-center gap-2">
					<button
						type="button"
						class="text-muted-content hover:text-base-content flex shrink-0 cursor-grab items-center gap-2"
						data-drag-preview
						aria-label="{m.dashboard_drag()} {m.dashboard_box()}"
						onpointerdown={(event) => layoutDrag.begin(event, { kind: 'box', id: row.id })}
					>
						<GripVertical class="size-4" />
						<span class="text-sm text-base-content font-medium">{m.dashboard_box()}</span>
					</button>
					<button
						type="button"
						class="btn btn-ghost btn-square btn-xs ml-auto shrink-0"
						aria-label="{m.dashboard_remove()} {m.dashboard_box()}"
						onclick={() => stage(removeBox(editing, row.id))}
					>
						<X class="size-4" />
					</button>
				</div>
				{@render layoutList(row.rows, [...parent, rowIndex])}
			</div>
			{@render rowDropSlot(parent, rowIndex + 1)}
		{:else if row.kind === 'header'}
			{#if rowIndex === 0 || rows[rowIndex - 1].kind !== 'box'}
				{@render rowDropSlot(parent, rowIndex)}
			{/if}
			<div
				class="rounded-lg bg-base-100 p-3 dark:bg-base-300 mx-3"
				role="group"
				data-layout-header
				data-layout-location={locationKey([...parent, rowIndex])}
			>
				<div class="flex items-center gap-2">
					<button
						type="button"
						class="text-muted-content hover:text-base-content flex shrink-0 cursor-grab items-center"
						data-drag-preview
						aria-label="{m.dashboard_drag()} {row.text || m.dashboard_header()}"
						onpointerdown={(event) => layoutDrag.begin(event, { kind: 'header', id: row.id })}
					>
						<GripVertical class="size-4" />
					</button>
					<input
						class="input-text-filled w-full"
						placeholder={m.dashboard_header_placeholder()}
						aria-label={m.dashboard_header()}
						value={row.text}
						oninput={(event) => {
							const target = event.currentTarget;
							if (!(target instanceof HTMLInputElement)) return;
							stage(setHeaderText(editing, row.id, target.value));
						}}
					/>
					<button
						type="button"
						class="btn btn-ghost btn-square btn-xs shrink-0"
						aria-label="{m.dashboard_remove()} {row.text || m.dashboard_header()}"
						onclick={() => stage(removeHeader(editing, row.id))}
					>
						<X class="size-4" />
					</button>
				</div>
				<HeaderVisibilityOptions
					class="mt-2"
					{row}
					config={editing}
					onChange={stage}
					scope="configure"
				/>
			</div>
		{:else}
			{@const endKey = `${locationKey([...parent, rowIndex])}-${row.items.length}`}
			{#if rowIndex === 0 || rows[rowIndex - 1].kind !== 'box'}
				{@render rowDropSlot(parent, rowIndex)}
			{/if}
			<div
				class="flex items-stretch gap-1"
				role="group"
				data-layout-row={locationKey([...parent, rowIndex])}
			>
				{#each row.items as placement, index (placement.id)}
					{@const label = widgetLabel(placement.id)}
					{@const slotKey = `${locationKey([...parent, rowIndex])}-${index}`}
					<div
						role="presentation"
						data-drop-slot={slotKey}
						data-drop-active={layoutDrag.dropKey === `slot-${slotKey}` ? 'true' : undefined}
						class={twMerge(
							'w-3 shrink-0 rounded transition-colors',
							layoutDrag.dropKey === `slot-${slotKey}` &&
								!layoutDrag.slotGhost(slotKey) &&
								'bg-primary/50'
						)}
					></div>
					{#if layoutDrag.slotGhost(slotKey)}
						{@const preview = dragPreviewModel()}
						{#if preview?.kind === 'widget'}
							{@render settledWidget(preview.id, preview.label, 'mr-3')}
						{/if}
					{/if}
					<div class="min-w-0 flex-1 rounded-lg bg-base-100 dark:bg-base-300 p-3" data-layout-card>
						<div class="flex items-start justify-between gap-2">
							<button
								type="button"
								class="mt-0.5 flex min-w-0 flex-1 cursor-grab items-center gap-2 text-left group"
								data-drag-preview
								aria-label="{m.dashboard_drag()} {label}"
								onpointerdown={(event) =>
									layoutDrag.begin(event, { kind: 'widget', id: placement.id })}
							>
								<GripVertical
									class="text-muted-content group-hover:text-base-content transition-colors size-4 shrink-0"
								/>
								<span class="truncate text-sm font-medium">{label}</span>
							</button>
							<button
								type="button"
								class="btn btn-ghost btn-square btn-xs shrink-0"
								aria-label="{m.dashboard_remove()} {label}"
								onclick={() => stage(removeWidget(editing, placement.id))}
							>
								<X class="size-4" />
							</button>
						</div>
						{#if placement.hideWhenEmpty && emptyWidgets.includes(placement.id)}
							<p class="text-muted-content mt-2 text-xs">
								{m.dashboard_hidden_while_empty()}
							</p>
						{/if}
						{#if placement.id === 'platform-stats'}
							<div class="mt-3 flex flex-col gap-1">
								{#each PLATFORM_STAT_IDS as statId (statId)}
									<label class="flex items-center gap-2 text-xs">
										<input
											type="checkbox"
											class="checkbox checkbox-xs checked:checkbox-primary"
											checked={editing.platformStats.includes(statId)}
											onchange={() => stage(togglePlatformStat(editing, statId))}
										/>
										{platformStatLabel(statId)}
									</label>
								{/each}
							</div>
						{:else if placement.id === 'model-stats'}
							<div class="mt-3 flex flex-col gap-1">
								{#each MODEL_STAT_IDS as statId (statId)}
									<label class="flex items-center gap-2 text-xs">
										<input
											type="checkbox"
											class="checkbox checkbox-xs checked:checkbox-primary"
											checked={editing.modelStats.includes(statId)}
											onchange={() => stage(toggleModelStat(editing, statId))}
										/>
										{modelStatLabel(statId)}
									</label>
								{/each}
							</div>
						{:else if placement.id === 'device-scan-stats'}
							<div class="mt-3 flex flex-col gap-1">
								{#each DEVICE_STAT_IDS as statId (statId)}
									<label class="flex items-center gap-2 text-xs">
										<input
											type="checkbox"
											class="checkbox checkbox-xs checked:checkbox-primary"
											checked={editing.deviceStats.includes(statId)}
											onchange={() => stage(toggleDeviceStat(editing, statId))}
										/>
										{deviceStatLabel(statId)}
									</label>
								{/each}
							</div>
						{/if}
						<label
							class={twMerge(
								'flex items-center gap-2 text-xs',
								placement.id === 'platform-stats' ||
									placement.id === 'device-scan-stats' ||
									placement.id === 'model-stats'
									? 'mt-1'
									: 'mt-3'
							)}
						>
							<input
								type="checkbox"
								class="checkbox checkbox-xs checked:checkbox-primary"
								checked={placement.hideWhenEmpty}
								aria-label="{m.dashboard_hide_when_empty()}: {label}"
								onchange={(event) => {
									const target = event.currentTarget;
									if (!(target instanceof HTMLInputElement)) return;
									stage(setHideWhenEmpty(editing, placement.id, target.checked));
								}}
							/>
							{m.dashboard_hide_when_empty()}
						</label>
					</div>
				{/each}
				<div
					role="presentation"
					data-drop-slot={endKey}
					data-drop-active={layoutDrag.dropKey === `slot-${endKey}` ? 'true' : undefined}
					class={twMerge(
						'w-2 shrink-0 rounded transition-colors',
						layoutDrag.dropKey === `slot-${endKey}` &&
							!layoutDrag.slotGhost(endKey) &&
							'bg-primary/50'
					)}
				></div>
				{#if layoutDrag.slotGhost(endKey)}
					{@const preview = dragPreviewModel()}
					{#if preview?.kind === 'widget'}
						{@render settledWidget(preview.id, preview.label, 'mr-3')}
					{/if}
				{/if}
			</div>
		{/if}
	{/each}
	{#if rows.at(-1)?.kind !== 'box'}
		{@render rowDropSlot(parent, rows.length)}
	{/if}
{/snippet}

<style>
	:global(html.dashboard-dragging),
	:global(html.dashboard-dragging *) {
		cursor: grabbing !important;
	}
</style>
