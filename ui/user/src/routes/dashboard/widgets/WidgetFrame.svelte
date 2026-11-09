<script lang="ts">
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import TitleChromeProvider from '$lib/components/graph/TitleChromeProvider.svelte';
	import { m } from '$lib/i18n';
	import {
		DEVICE_STAT_IDS,
		MODEL_STAT_IDS,
		PLATFORM_STAT_IDS,
		removeWidget,
		setHideWhenEmpty,
		toggleDeviceStat,
		toggleModelStat,
		togglePlatformStat,
		type DashboardConfig,
		type WidgetPlacement
	} from '../dashboardLayout';
	import { deviceStatLabel, modelStatLabel, platformStatLabel, widgetLabel } from './labels';
	import { EllipsisVertical, GripVertical, XIcon } from '@lucide/svelte';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	type Props = {
		placement: WidgetPlacement;
		config: DashboardConfig;
		dragging?: boolean;
		onChange: (config: DashboardConfig) => void;
		onDragStart: (event: PointerEvent) => void;
		children: Snippet;
	};

	let { placement, config, dragging = false, onChange, onDragStart, children }: Props = $props();
	const label = $derived(widgetLabel(placement.id));
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

	function checked(event: Event): boolean | undefined {
		const target = event.currentTarget;
		if (!(target instanceof HTMLInputElement)) return undefined;
		return target.checked;
	}
</script>

<TitleChromeProvider {trailing}>
	<div
		class={twMerge(
			'group relative h-full',
			dragging && 'is-dragging rounded-md opacity-40 cursor-grabbing',
			menuOpen && 'is-menu-open'
		)}
	>
		<button
			type="button"
			class={twMerge(
				'border-base-200 dark:border-base-400 bg-base-100 text-muted-content dark:bg-base-200 absolute top-1.5 left-0 z-20 flex h-8 w-5 -translate-x-full cursor-grab items-center justify-center rounded-l-md border dark:border-r-0  opacity-0 transition-[transform,opacity] duration-200 ease-out pointer-events-none touch-none group-hover:pointer-events-auto group-hover:-translate-x-full group-hover:opacity-100',
				controlsVisible && 'pointer-events-auto opacity-100'
			)}
			aria-label="{m.dashboard_drag()} {label}"
			onpointerdown={onDragStart}
		>
			<GripVertical class="size-3.5" />
		</button>
		{@render children()}
	</div>
</TitleChromeProvider>

{#snippet menuIcon()}
	<EllipsisVertical class="size-4" />
{/snippet}

{#snippet trailing()}
	<div
		class={twMerge(
			'pointer-events-none absolute top-1/2 right-1.5 z-20 -translate-y-1/2 opacity-0 transition-opacity duration-200 group-hover:pointer-events-auto group-hover:opacity-100',
			controlsVisible && 'pointer-events-auto opacity-100'
		)}
	>
		<DotDotDot
			class="btn-xs shrink-0"
			icon={menuIcon}
			placement="bottom-end"
			ariaLabel="{m.core_row_actions()}: {label}"
			onClick={() => (menuOpen = true)}
		>
			{#snippet children({ open })}
				{#if open}
					<span class="hidden" use:holdMenuOpen></span>
					<div role="presentation" onclick={keepMenuOpen} onpointerdown={keepMenuOpen}>
						<label class="flex items-center gap-2 m-2 text-xs">
							<input
								type="checkbox"
								class="checkbox checkbox-xs checked:checkbox-primary"
								checked={placement.hideWhenEmpty}
								aria-label="{m.dashboard_hide_when_empty()}: {label}"
								onchange={(event) => {
									const value = checked(event);
									if (value === undefined) return;
									onChange(setHideWhenEmpty(config, placement.id, value));
								}}
							/>
							{m.dashboard_hide_when_empty()}
						</label>
						{#if placement.id === 'platform-stats'}
							{#each PLATFORM_STAT_IDS as statId (statId)}
								<label class="flex items-center gap-2 text-xs m-2">
									<input
										type="checkbox"
										class="checkbox checkbox-xs checked:checkbox-primary"
										checked={config.platformStats.includes(statId)}
										onchange={() => onChange(togglePlatformStat(config, statId))}
									/>
									{platformStatLabel(statId)}
								</label>
							{/each}
						{:else if placement.id === 'model-stats'}
							{#each MODEL_STAT_IDS as statId (statId)}
								<label class="flex items-center gap-2 text-xs m-2">
									<input
										type="checkbox"
										class="checkbox checkbox-xs checked:checkbox-primary"
										checked={config.modelStats.includes(statId)}
										onchange={() => onChange(toggleModelStat(config, statId))}
									/>
									{modelStatLabel(statId)}
								</label>
							{/each}
						{:else if placement.id === 'device-scan-stats'}
							{#each DEVICE_STAT_IDS as statId (statId)}
								<label class="flex items-center gap-2 text-xs m-2">
									<input
										type="checkbox"
										class="checkbox checkbox-xs checked:checkbox-primary"
										checked={config.deviceStats.includes(statId)}
										onchange={() => onChange(toggleDeviceStat(config, statId))}
									/>
									{deviceStatLabel(statId)}
								</label>
							{/each}
						{/if}
						<button
							type="button"
							class="w-full mt-3 menu-button text-xs border-t border-base-300 rounded-t-none hover:rounded-md"
							aria-label="{m.dashboard_remove()} {label}"
							onclick={() => onChange(removeWidget(config, placement.id))}
						>
							<XIcon class="size-4" />
							{m.dashboard_remove()}
						</button>
					</div>
				{/if}
			{/snippet}
		</DotDotDot>
	</div>
{/snippet}
