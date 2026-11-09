<script lang="ts">
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import { m } from '$lib/i18n';
	import HeaderVisibilityOptions from './HeaderVisibilityOptions.svelte';
	import type { DashboardConfig, HeaderRow } from './dashboardLayout';
	import { EllipsisVertical, GripVertical } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	type Props = {
		row: HeaderRow;
		parent: string;
		rowIndex: number;
		dragging?: boolean;
		config: DashboardConfig;
		onChange: (config: DashboardConfig) => void;
		onDragStart: (event: PointerEvent) => void;
	};

	let { row, parent, rowIndex, dragging = false, config, onChange, onDragStart }: Props = $props();
	const name = $derived(row.text || m.dashboard_header());
	let menuOpen = $state(false);
	const controlsVisible = $derived(menuOpen || dragging);

	function holdMenuOpen(node: HTMLElement) {
		void node;
		menuOpen = true;
		return {
			destroy() {
				menuOpen = false;
			}
		};
	}

	function keepMenuOpen(event: Event) {
		event.stopPropagation();
	}
</script>

<div
	class={twMerge(
		'border-primary bg-base-100 dark:bg-base-200 group relative col-span-12 flex items-center rounded-xs border-l-3 py-2 pr-4 pl-3.5 transition-[padding] duration-200 group-hover:pr-12',
		(dragging || menuOpen) && 'pr-12',
		dragging && 'is-dragging ring-primary opacity-40'
	)}
	data-dashboard-header
	data-header-id={row.id}
	data-parent={parent}
	data-row={rowIndex}
>
	<button
		type="button"
		class={twMerge(
			'border-base-200 dark:border-base-400 bg-base-100 text-muted-content dark:bg-base-200 absolute top-1/2 left-0 z-20 flex h-8 w-5 -translate-x-full -translate-y-1/2 cursor-grab items-center justify-center rounded-l-md border opacity-0 transition-[transform,opacity] duration-200 ease-out pointer-events-none touch-none group-hover:pointer-events-auto group-hover:-translate-x-full group-hover:opacity-100',
			controlsVisible && 'pointer-events-auto opacity-100'
		)}
		aria-label="{m.dashboard_drag()} {name}"
		onpointerdown={onDragStart}
	>
		<GripVertical class="size-3.5" />
	</button>
	<h2 class="min-w-0 flex-1 truncate text-lg font-semibold">{row.text}</h2>
	<div
		class={twMerge(
			'pointer-events-none absolute top-1/2 right-1.5 z-20 -translate-y-1/2 opacity-0 transition-opacity duration-200 group-hover:pointer-events-auto group-hover:opacity-100',
			controlsVisible && 'pointer-events-auto opacity-100'
		)}
	>
		<DotDotDot
			class="btn-xs shrink-0"
			placement="bottom-end"
			ariaLabel="{m.core_row_actions()}: {name}"
			onClick={() => (menuOpen = true)}
		>
			{#snippet icon()}
				<EllipsisVertical class="size-4" />
			{/snippet}
			{#snippet children({ open })}
				{#if open}
					<span class="hidden" use:holdMenuOpen></span>
					<div role="presentation" onclick={keepMenuOpen} onpointerdown={keepMenuOpen}>
						<HeaderVisibilityOptions {row} {config} {onChange} scope="header-menu" />
					</div>
				{/if}
			{/snippet}
		</DotDotDot>
	</div>
</div>
