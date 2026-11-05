<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import { m } from '$lib/i18n';
	import {
		DEVICE_STAT_IDS,
		MAX_WIDGETS_PER_ROW,
		PLATFORM_STAT_IDS,
		appendBox,
		appendHeader,
		appendWidget,
		insertBox,
		insertHeader,
		moveBox,
		moveHeader,
		placeWidget,
		removeBox,
		removeHeader,
		removeWidget,
		setHeaderOnlyWithDeviceScans,
		setHeaderText,
		setHideWhenEmpty,
		toggleDeviceStat,
		togglePlatformStat,
		type DashboardConfig,
		type DashboardRow,
		type WidgetId
	} from './dashboardLayout';
	import { deviceStatLabel, platformStatLabel, widgetLabel } from './widgets/labels';
	import { GripVertical, X } from '@lucide/svelte';
	import { onDestroy } from 'svelte';
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

	type DragSubject =
		| { kind: 'widget'; id: WidgetId }
		| { kind: 'header'; id: string }
		| { kind: 'new-header' }
		| { kind: 'box'; id: string }
		| { kind: 'new-box' };

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let dragging = $state<DragSubject | null>(null);
	let dropKey = $state<string | null>(null);
	let dragPreview: HTMLElement | null = null;

	function copyConfig(value: DashboardConfig): DashboardConfig {
		return $state.snapshot(value);
	}

	let draft = $state<DashboardConfig | null>(null);
	let resetPending = false;
	const editing = $derived(draft ?? config);

	const placed = $derived(new Set(placedIds(editing.rows)));
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
		endDrag();
	}

	function resetDraft() {
		draft = copyConfig(defaults);
		resetPending = true;
	}

	type PendingDrop =
		| { kind: 'slot'; parent: number[]; rowIndex: number; index: number }
		| { kind: 'gap'; parent: number[]; index: number };

	let pendingDrop: PendingDrop | null = null;

	function clearDragPreview() {
		dragPreview?.remove();
		dragPreview = null;
	}

	function setGrabbingCursor(active: boolean) {
		if (typeof document === 'undefined') return;
		document.documentElement.classList.toggle('dashboard-dragging', active);
	}

	function clearPendingDrop() {
		pendingDrop = null;
		dropKey = null;
	}

	function rememberDrop(next: PendingDrop) {
		pendingDrop = next;
		dropKey =
			next.kind === 'gap'
				? `gap-${locationKey([...next.parent, next.index])}`
				: `slot-${locationKey([...next.parent, next.rowIndex])}-${next.index}`;
	}

	function moveDragPreview(x: number, y: number) {
		if (!dragPreview) return;
		dragPreview.style.left = `${x - 14}px`;
		dragPreview.style.top = `${y - (dragPreview.offsetHeight / 2 || 12)}px`;
	}

	function mountDragPreview(source: HTMLElement, x: number, y: number) {
		const origin = source.closest('[data-drag-preview]');
		if (!(origin instanceof HTMLElement)) return;
		const preview = origin.cloneNode(true);
		if (!(preview instanceof HTMLElement)) return;

		clearDragPreview();
		preview.setAttribute('aria-hidden', 'true');
		preview.setAttribute('data-drag-clone', '');
		preview.style.position = 'fixed';
		preview.style.zIndex = '10000';
		preview.style.width = 'max-content';
		preview.style.maxWidth = 'none';
		preview.style.margin = '0';
		preview.style.opacity = '1';
		preview.style.visibility = 'visible';
		preview.style.pointerEvents = 'none';
		preview.classList.remove('min-w-0', 'flex-1');
		preview.classList.add(
			'border-base-300',
			'bg-base-200',
			'rounded-lg',
			'border',
			'px-2',
			'py-1',
			'shadow-lg'
		);
		preview.querySelector('span')?.classList.remove('truncate');
		const layer = source.closest('dialog') ?? document.body;
		layer.appendChild(preview);
		dragPreview = preview;
		moveDragPreview(x, y);
	}

	function detachDragListeners() {
		if (typeof window === 'undefined') return;
		window.removeEventListener('pointermove', onPointerMove);
		window.removeEventListener('pointerup', onPointerUp);
		window.removeEventListener('pointercancel', onPointerCancel);
		window.removeEventListener('keydown', onDragKeyDown);
	}

	function beginDrag(event: PointerEvent, subject: DragSubject) {
		if (event.button !== 0) return;
		const current = event.currentTarget;
		if (!(current instanceof HTMLElement)) return;
		const target = event.target;
		if (target instanceof Element) {
			const interactive = target.closest('button, input, textarea, a, label');
			if (interactive && interactive !== current) return;
		}
		event.preventDefault();
		dragging = subject;
		clearPendingDrop();
		setGrabbingCursor(true);
		mountDragPreview(current, event.clientX, event.clientY);
		window.addEventListener('pointermove', onPointerMove);
		window.addEventListener('pointerup', onPointerUp);
		window.addEventListener('pointercancel', onPointerCancel);
		window.addEventListener('keydown', onDragKeyDown);
	}

	function endDrag() {
		dragging = null;
		clearPendingDrop();
		clearDragPreview();
		setGrabbingCursor(false);
		detachDragListeners();
	}

	function placedIds(rows: DashboardRow[]): WidgetId[] {
		return rows.flatMap((row) => {
			if (row.kind === 'widgets') return row.items.map((placement) => placement.id);
			if (row.kind === 'box') return placedIds(row.rows);
			return [];
		});
	}

	function rowAt(path: number[]): DashboardRow | undefined {
		let rows = editing.rows;
		for (let depth = 0; depth < path.length; depth += 1) {
			const row = rows[path[depth]];
			if (!row) return undefined;
			if (depth === path.length - 1) return row;
			if (row.kind !== 'box') return undefined;
			rows = row.rows;
		}
		return undefined;
	}

	function locationKey(path: number[]) {
		return path.join('-');
	}

	function rowAccepts(parent: number[], rowIndex: number) {
		const subject = dragging;
		const row = rowAt([...parent, rowIndex]);
		if (!row || row.kind !== 'widgets' || !subject || subject.kind !== 'widget') return false;
		return (
			row.items.some((placement) => placement.id === subject.id) ||
			row.items.length < MAX_WIDGETS_PER_ROW
		);
	}

	function insertionIndex(row: HTMLElement, clientX: number): number {
		const cards = row.querySelectorAll('[data-layout-card]');
		for (let index = 0; index < cards.length; index += 1) {
			const card = cards[index];
			if (!(card instanceof HTMLElement)) continue;
			const rect = card.getBoundingClientRect();
			if (clientX < rect.left + rect.width / 2) return index;
		}
		return cards.length;
	}

	function draggingSingleton() {
		return (
			dragging?.kind === 'header' ||
			dragging?.kind === 'new-header' ||
			dragging?.kind === 'box' ||
			dragging?.kind === 'new-box'
		);
	}

	function singletonAt(parent: number[], index: number): DashboardConfig | undefined {
		if (dragging?.kind === 'new-header') return insertHeader(editing, index, '', parent);
		if (dragging?.kind === 'header') return moveHeader(editing, dragging.id, index, parent);
		if (dragging?.kind === 'new-box') return insertBox(editing, index, parent);
		if (dragging?.kind === 'box') return moveBox(editing, dragging.id, index, parent);
		return undefined;
	}

	function locationFrom(key: string | null): { parent: number[]; index: number } | null {
		if (!key) return null;
		const parts = key.split('-').map((part) => Number(part));
		if (parts.length === 0 || parts.some((part) => !Number.isInteger(part))) return null;
		const index = parts[parts.length - 1];
		return { parent: parts.slice(0, -1), index };
	}

	function highlightAt(x: number, y: number) {
		if (typeof document === 'undefined') return;
		const node = document.elementFromPoint(x, y);
		const target = node instanceof Element ? node : null;
		const gap = target?.closest('[data-layout-gap]');
		if (gap instanceof HTMLElement) {
			const location = locationFrom(gap.getAttribute('data-layout-gap'));
			if (location) rememberDrop({ kind: 'gap', parent: location.parent, index: location.index });
			return;
		}
		const row = target?.closest('[data-layout-row]');
		if (row instanceof HTMLElement) {
			const location = locationFrom(row.getAttribute('data-layout-row'));
			if (!location) return;
			if (draggingSingleton()) {
				const rect = row.getBoundingClientRect();
				const index = y < rect.top + rect.height / 2 ? location.index : location.index + 1;
				rememberDrop({ kind: 'gap', parent: location.parent, index });
				return;
			}
			if (!rowAccepts(location.parent, location.index)) {
				clearPendingDrop();
				return;
			}
			rememberDrop({
				kind: 'slot',
				parent: location.parent,
				rowIndex: location.index,
				index: insertionIndex(row, x)
			});
			return;
		}
		const header = target?.closest('[data-layout-header]');
		if (header instanceof HTMLElement && draggingSingleton()) {
			const location = locationFrom(header.getAttribute('data-layout-location'));
			if (!location) return;
			const rect = header.getBoundingClientRect();
			const index = y < rect.top + rect.height / 2 ? location.index : location.index + 1;
			rememberDrop({ kind: 'gap', parent: location.parent, index });
			return;
		}
		clearPendingDrop();
	}

	function applyPendingDrop() {
		const drop = pendingDrop;
		if (!drop || !dragging) return;
		if (drop.kind === 'gap') {
			if (dragging.kind === 'widget') {
				stage(
					placeWidget(editing, dragging.id, {
						kind: 'new-row',
						path: drop.parent,
						index: drop.index
					})
				);
				return;
			}
			const next = singletonAt(drop.parent, drop.index);
			if (next) stage(next);
			return;
		}
		if (dragging.kind !== 'widget' || !rowAccepts(drop.parent, drop.rowIndex)) return;
		stage(
			placeWidget(editing, dragging.id, {
				kind: 'slot',
				path: [...drop.parent, drop.rowIndex],
				index: drop.index
			})
		);
	}

	function onPointerMove(event: PointerEvent) {
		if (!dragging) return;
		moveDragPreview(event.clientX, event.clientY);
		highlightAt(event.clientX, event.clientY);
	}

	function onPointerUp(event: PointerEvent) {
		if (!dragging) return;
		highlightAt(event.clientX, event.clientY);
		applyPendingDrop();
		endDrag();
	}

	function onPointerCancel() {
		endDrag();
	}

	function onDragKeyDown(event: KeyboardEvent) {
		if (event.key !== 'Escape') return;
		event.preventDefault();
		endDrag();
	}

	onDestroy(endDrag);
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
		<section class="flex min-w-0 grow flex-col gap-2">
			<div class="flex items-end justify-between gap-2">
				<h4 class="text-xs font-medium uppercase">{m.dashboard_layout()}</h4>
			</div>
			<div
				class="flex min-h-80 flex-col grow gap-1 overflow-y-auto default-scrollbar-thin rounded-xl bg-base-200 dark:bg-base-100/50 p-3"
			>
				{#if editing.rows.length === 0}
					<p class="text-muted-content py-8 text-center text-sm">{m.dashboard_no_widgets()}</p>
				{/if}
				{@render layoutList(editing.rows, [])}
			</div>
		</section>
		<section class="flex w-xl flex-col gap-2">
			<h4 class="text-xs font-medium uppercase">{m.dashboard_available_widgets()}</h4>
			<ul
				class="flex flex-col grow gap-2 rounded-xl bg-base-200 dark:bg-base-100/50 p-3 overflow-y-auto default-scrollbar-thin"
			>
				<li
					class="group flex cursor-grab items-center justify-between gap-2 rounded-lg bg-base-100 px-3 py-2 dark:bg-base-300"
					data-drag-preview
					data-palette-header
					onpointerdown={(event) => beginDrag(event, { kind: 'new-header' })}
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
					onpointerdown={(event) => beginDrag(event, { kind: 'new-box' })}
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
						onpointerdown={(event) => beginDrag(event, { kind: 'widget', id })}
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

{#snippet layoutList(rows: DashboardRow[], parent: number[])}
	<div
		role="presentation"
		data-layout-gap={locationKey([...parent, 0])}
		class={twMerge(
			'h-3 rounded transition-colors',
			dropKey === `gap-${locationKey([...parent, 0])}` && 'bg-primary/30'
		)}
	></div>
	{#each rows as row, rowIndex (row.kind === 'widgets' ? row.items
				.map((placement) => placement.id)
				.join() : row.id)}
		{#if row.kind === 'box'}
			<div
				class="rounded-xl border-base-300 dark:border-base-400 bg-base-100/35 dark:bg-base-300/35 border px-3 py-2 mx-3"
				role="group"
				data-layout-box
			>
				<div class="mb-1 flex items-center gap-2">
					<button
						type="button"
						class="text-muted-content hover:text-base-content flex shrink-0 cursor-grab items-center gap-2"
						data-drag-preview
						aria-label="{m.dashboard_drag()} {m.dashboard_box()}"
						onpointerdown={(event) => beginDrag(event, { kind: 'box', id: row.id })}
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
		{:else if row.kind === 'header'}
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
						onpointerdown={(event) => beginDrag(event, { kind: 'header', id: row.id })}
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
				<label class="mt-3 flex items-center gap-2 text-xs">
					<input
						type="checkbox"
						class="checkbox checkbox-xs checked:checkbox-primary"
						checked={row.onlyWithDeviceScans}
						aria-label="{m.dashboard_header_only_with_scans()}: {row.text || m.dashboard_header()}"
						onchange={(event) => {
							const target = event.currentTarget;
							if (!(target instanceof HTMLInputElement)) return;
							stage(setHeaderOnlyWithDeviceScans(editing, row.id, target.checked));
						}}
					/>
					{m.dashboard_header_only_with_scans()}
				</label>
			</div>
		{:else}
			<div
				class="flex items-stretch gap-1"
				role="group"
				data-layout-row={locationKey([...parent, rowIndex])}
			>
				{#each row.items as placement, index (placement.id)}
					{@const label = widgetLabel(placement.id)}
					<div
						role="presentation"
						data-drop-slot="{locationKey([...parent, rowIndex])}-{index}"
						data-drop-active={dropKey === `slot-${locationKey([...parent, rowIndex])}-${index}`
							? 'true'
							: undefined}
						class={twMerge(
							'w-2 shrink-0 rounded transition-colors',
							dropKey === `slot-${locationKey([...parent, rowIndex])}-${index}` && 'bg-primary'
						)}
					></div>
					<div class="min-w-0 flex-1 rounded-lg bg-base-100 dark:bg-base-300 p-3" data-layout-card>
						<div class="flex items-start justify-between gap-2">
							<button
								type="button"
								class="mt-0.5 flex min-w-0 flex-1 cursor-grab items-center gap-2 text-left group"
								data-drag-preview
								aria-label="{m.dashboard_drag()} {label}"
								onpointerdown={(event) => beginDrag(event, { kind: 'widget', id: placement.id })}
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
								placement.id === 'platform-stats' || placement.id === 'device-scan-stats'
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
					data-drop-slot="{locationKey([...parent, rowIndex])}-{row.items.length}"
					data-drop-active={dropKey ===
					`slot-${locationKey([...parent, rowIndex])}-${row.items.length}`
						? 'true'
						: undefined}
					class={twMerge(
						'w-2 shrink-0 rounded transition-colors',
						dropKey === `slot-${locationKey([...parent, rowIndex])}-${row.items.length}` &&
							'bg-primary'
					)}
				></div>
			</div>
		{/if}
		<div
			role="presentation"
			data-layout-gap={locationKey([...parent, rowIndex + 1])}
			class={twMerge(
				'h-3 rounded transition-colors',
				dropKey === `gap-${locationKey([...parent, rowIndex + 1])}` && 'bg-primary/30'
			)}
		></div>
	{/each}
{/snippet}

<style>
	:global(html.dashboard-dragging),
	:global(html.dashboard-dragging *) {
		cursor: grabbing !important;
	}
</style>
