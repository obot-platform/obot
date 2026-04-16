<script lang="ts">
	import { autoUpdate, computePosition, flip, offset, shift } from '@floating-ui/dom';
	import { getHours, getMinutes, setHours, setMinutes } from 'date-fns';
	import { twMerge } from 'tailwind-merge';

	export type AnchorPlacement = 'top' | 'right' | 'bottom' | 'left';

	type Props = {
		open: boolean;
		onOpenChange: (open: boolean) => void;
		date: Date;
		onApply: (date: Date) => void;
		format: '12h' | '24h';
		initialMode: 'hour' | 'minute';
		/** When set, the popover is placed relative to the anchor (flip/shift) instead of centered */
		anchor?: HTMLElement | null;
		/** Preferred side of the anchor when `anchor` is set. Defaults to `bottom`. */
		anchorPlacement?: AnchorPlacement;
	};

	let {
		open,
		onOpenChange,
		date,
		onApply,
		format,
		initialMode,
		anchor = null,
		anchorPlacement = 'bottom'
	}: Props = $props();

	const flipFallbacks: Record<AnchorPlacement, AnchorPlacement[]> = {
		top: ['bottom', 'left', 'right'],
		bottom: ['top', 'left', 'right'],
		left: ['right', 'top', 'bottom'],
		right: ['left', 'top', 'bottom']
	};

	const is24h = $derived(format === '24h');

	let tempDate = $state(new Date());
	let mode = $state<'hour' | 'minute'>('hour');
	/** For minute: fine adjustment 0–4 added to the 5-minute bucket */
	let minuteExtra = $state(0);

	let panelEl = $state<HTMLElement | null>(null);
	let anchoredPositionReady = $state(false);

	let lastSyncedTime: number | null = null;

	$effect(() => {
		if (!open) {
			lastSyncedTime = null;
			return;
		}
		const t = date.getTime();
		if (lastSyncedTime === t) return;
		lastSyncedTime = t;
		tempDate = new Date(t);
		minuteExtra = getMinutes(tempDate) % 5;
	});

	$effect(() => {
		if (!open) return;
		mode = initialMode;
	});

	$effect(() => {
		if (!open) {
			anchoredPositionReady = false;
			return;
		}
		if (!anchor || !panelEl) {
			anchoredPositionReady = true;
			return;
		}

		anchoredPositionReady = false;

		async function updatePosition() {
			if (!anchor || !panelEl) return;
			const { x, y } = await computePosition(anchor, panelEl, {
				placement: anchorPlacement,
				strategy: 'fixed',
				middleware: [
					offset(8),
					flip({ fallbackPlacements: flipFallbacks[anchorPlacement] }),
					shift({ padding: 8 })
				]
			});
			Object.assign(panelEl.style, { left: `${x}px`, top: `${y}px` });
			anchoredPositionReady = true;
		}

		void updatePosition();
		return autoUpdate(anchor, panelEl, updatePosition);
	});

	$effect(() => {
		if (!open || !panelEl || anchor) return;
		panelEl.style.removeProperty('left');
		panelEl.style.removeProperty('top');
	});

	const h24 = $derived(getHours(tempDate));
	const m60 = $derived(getMinutes(tempDate));
	const hour12 = $derived(h24 % 12 === 0 ? 12 : h24 % 12);
	const isAm = $derived(h24 < 12);

	const INNER_HOURS = [0, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23] as const;

	function polarStyle(index1Based: number, n: number, radiusPct: number): string {
		const angle = (index1Based / n) * 2 * Math.PI - Math.PI / 2;
		const x = 50 + radiusPct * Math.cos(angle);
		const y = 50 + radiusPct * Math.sin(angle);
		return `left:${x}%;top:${y}%;transform:translate(-50%,-50%)`;
	}

	function polarStyleIndex(i: number, n: number, radiusPct: number): string {
		const angle = (i / n) * 2 * Math.PI - Math.PI / 2;
		const x = 50 + radiusPct * Math.cos(angle);
		const y = 50 + radiusPct * Math.sin(angle);
		return `left:${x}%;top:${y}%;transform:translate(-50%,-50%)`;
	}

	function setHour24(h: number) {
		tempDate = setHours(tempDate, h);
		mode = 'minute';
	}

	function setHour12(h12: number) {
		if (isAm) {
			tempDate = setHours(tempDate, h12 === 12 ? 0 : h12);
		} else {
			tempDate = setHours(tempDate, h12 === 12 ? 12 : h12 + 12);
		}
		mode = 'minute';
	}

	function toggleAmPm(nextAm: boolean) {
		const ch = getHours(tempDate);
		if (nextAm && ch >= 12) {
			tempDate = setHours(tempDate, ch - 12);
		} else if (!nextAm && ch < 12) {
			tempDate = setHours(tempDate, ch + 12);
		}
	}

	function setMinuteFromClock(fiveSlot: number) {
		const base = fiveSlot * 5;
		const next = Math.min(59, base + minuteExtra);
		tempDate = setMinutes(tempDate, next);
	}

	function setMinuteExtra(extra: number) {
		minuteExtra = extra;
		const base = Math.floor(getMinutes(tempDate) / 5) * 5;
		tempDate = setMinutes(tempDate, Math.min(59, base + extra));
	}

	function handAngleForHour24(selected: number): number {
		if (selected >= 1 && selected <= 12) {
			return (selected / 12) * 2 * Math.PI - Math.PI / 2;
		}
		const idx = INNER_HOURS.findIndex((v) => v === selected);
		if (idx >= 0) {
			return (idx / 12) * 2 * Math.PI - Math.PI / 2;
		}
		return -Math.PI / 2;
	}

	function handAngleForHour12(h12: number): number {
		return (h12 / 12) * 2 * Math.PI - Math.PI / 2;
	}

	function handAngleForMinute(min: number): number {
		return (min / 60) * 2 * Math.PI - Math.PI / 2;
	}

	function cancel() {
		onOpenChange(false);
	}

	function ok() {
		onApply(new Date(tempDate.getTime()));
		onOpenChange(false);
	}

	$effect(() => {
		if (!open) return;
		const onKey = (e: KeyboardEvent) => {
			if (e.key === 'Escape') onOpenChange(false);
		};
		document.addEventListener('keydown', onKey);
		return () => document.removeEventListener('keydown', onKey);
	});
</script>

{#if open}
	<div class="fixed inset-0 z-80" onclick={() => onOpenChange(false)} role="presentation"></div>
	<div
		bind:this={panelEl}
		class={twMerge(
			'popover flex flex-col overflow-hidden max-h-[min(90vh,520px)] fixed z-90',
			anchor
				? twMerge(
						'w-[min(340px,calc(100vw-16px))]',
						!anchoredPositionReady && 'pointer-events-none opacity-0'
					)
				: 'left-1/2 top-1/2 w-[min(340px,calc(100%-2rem))] max-w-[340px] -translate-x-1/2 -translate-y-1/2'
		)}
		role="dialog"
		aria-modal="true"
		aria-label="Select time"
		tabindex="0"
	>
		<!-- Header -->
		<div
			class={twMerge(
				'flex shrink-0 items-center justify-center gap-1 px-4 py-5 text-4xl font-light tabular-nums',
				'text-white bg-primary'
			)}
		>
			{#if is24h}
				<button
					type="button"
					class={twMerge(
						'rounded px-1 transition-opacity',
						mode === 'hour' ? 'opacity-100' : 'opacity-60 hover:opacity-90'
					)}
					onclick={() => (mode = 'hour')}>{String(h24).padStart(2, '0')}</button
				>
				<span class="opacity-90">:</span>
				<button
					type="button"
					class={twMerge(
						'rounded px-1 transition-opacity',
						mode === 'minute' ? 'opacity-100' : 'opacity-60 hover:opacity-90'
					)}
					onclick={() => (mode = 'minute')}>{String(m60).padStart(2, '0')}</button
				>
			{:else}
				<button
					type="button"
					class={twMerge(
						'rounded px-1 transition-opacity',
						mode === 'hour' ? 'opacity-100' : 'opacity-60 hover:opacity-90'
					)}
					onclick={() => (mode = 'hour')}>{String(hour12).padStart(2, '0')}</button
				>
				<span class="opacity-90">:</span>
				<button
					type="button"
					class={twMerge(
						'rounded px-1 transition-opacity',
						mode === 'minute' ? 'opacity-100' : 'opacity-60 hover:opacity-90'
					)}
					onclick={() => (mode = 'minute')}>{String(m60).padStart(2, '0')}</button
				>
				<div class="ml-3 flex flex-col gap-0.5 text-sm font-medium">
					<button
						type="button"
						class={twMerge('rounded px-2 py-0.5', isAm ? 'opacity-100' : 'opacity-50')}
						onclick={() => toggleAmPm(true)}>AM</button
					>
					<button
						type="button"
						class={twMerge('rounded px-2 py-0.5', !isAm ? 'opacity-100' : 'opacity-50')}
						onclick={() => toggleAmPm(false)}>PM</button
					>
				</div>
			{/if}
		</div>

		<!-- Clock face -->
		<div class="relative flex flex-1 flex-col items-center justify-center px-3 pt-4 pb-6">
			<div class="relative aspect-square w-full max-w-[280px] pb-4">
				<!-- Dial background -->
				<div class="bg-surface1 absolute inset-[0%] rounded-full"></div>

				{#if mode === 'hour' && is24h}
					{@const selAngle = handAngleForHour24(h24)}
					<svg
						class="pointer-events-none absolute inset-[8%] h-[84%] w-[84%]"
						viewBox="0 0 100 100"
					>
						<line
							x1="50"
							y1="50"
							x2={50 + 38 * Math.cos(selAngle)}
							y2={50 + 38 * Math.sin(selAngle)}
							stroke="currentColor"
							stroke-width="1.5"
							class="text-primary"
						/>
						<circle cx="50" cy="50" r="3" class="fill-primary" />
					</svg>
					{#each Array.from({ length: 12 }, (_, i) => i + 1) as h (h)}
						<button
							type="button"
							style={polarStyle(h, 12, 40)}
							class={twMerge(
								'bg-surface1 absolute z-10 flex h-9 w-9 items-center justify-center rounded-full text-sm',
								h24 === h ? 'bg-primary text-white scale-110' : 'hover:bg-surface3 '
							)}
							onclick={() => setHour24(h)}>{h}</button
						>
					{/each}
					{#each INNER_HOURS as hv, i (hv)}
						<button
							type="button"
							style={polarStyleIndex(i, 12, 24)}
							class={twMerge(
								'bg-surface1 absolute z-10 flex h-8 w-8 items-center justify-center rounded-full text-xs',
								h24 === hv ? 'bg-primary text-white scale-110' : 'hover:bg-surface3 '
							)}
							onclick={() => setHour24(hv)}>{String(hv).padStart(2, '0')}</button
						>
					{/each}
				{:else if mode === 'hour' && !is24h}
					{@const selAngle = handAngleForHour12(hour12)}
					<svg
						class="pointer-events-none absolute inset-[8%] h-[84%] w-[84%]"
						viewBox="0 0 100 100"
					>
						<line
							x1="50"
							y1="50"
							x2={50 + 40 * Math.cos(selAngle)}
							y2={50 + 40 * Math.sin(selAngle)}
							stroke="currentColor"
							stroke-width="1.5"
							class="text-primary"
						/>
						<circle cx="50" cy="50" r="3" class="fill-primary" />
					</svg>
					{#each Array.from({ length: 12 }, (_, i) => i + 1) as h (h)}
						<button
							type="button"
							style={polarStyle(h, 12, 40)}
							class={twMerge(
								'bg-surface1 absolute z-10 flex h-10 w-10 items-center justify-center rounded-full text-base',
								hour12 === h ? 'bg-primary text-white scale-110' : 'hover:bg-surface3 '
							)}
							onclick={() => setHour12(h)}>{h}</button
						>
					{/each}
				{:else}
					{@const selMin = m60}
					{@const selAngle = handAngleForMinute(selMin)}
					<svg
						class="pointer-events-none absolute inset-[8%] h-[84%] w-[84%]"
						viewBox="0 0 100 100"
					>
						<line
							x1="50"
							y1="50"
							x2={50 + 40 * Math.cos(selAngle)}
							y2={50 + 40 * Math.sin(selAngle)}
							stroke="currentColor"
							stroke-width="1.5"
							class="text-primary"
						/>
						<circle cx="50" cy="50" r="3" class="fill-primary" />
					</svg>
					{#each Array.from({ length: 12 }, (_, i) => i) as slot (slot)}
						{@const label = slot * 5}
						<button
							type="button"
							style={polarStyle(slot === 0 ? 12 : slot, 12, 38)}
							class={twMerge(
								'bg-surface1 absolute z-10 flex h-9 w-9 items-center justify-center rounded-full text-xs',
								Math.floor(selMin / 5) === slot
									? 'bg-primary text-primary-content scale-110'
									: 'hover:bg-surface3 '
							)}
							onclick={() => {
								minuteExtra = 0;
								setMinuteFromClock(slot);
							}}>{String(label).padStart(2, '0')}</button
						>
					{/each}
				{/if}
			</div>

			{#if mode === 'minute'}
				<div class="w-full flex justify-center gap-1 pt-4 absolute bottom-2 left-0 right-0">
					{#each [0, 1, 2, 3, 4] as e (e)}
						<button
							type="button"
							class={twMerge(
								'rounded px-2 py-1 text-xs',
								minuteExtra === e ? 'bg-primary text-white' : 'bg-surface3/50 text-on-background'
							)}
							onclick={() => setMinuteExtra(e)}>+{e}</button
						>
					{/each}
				</div>
			{/if}
		</div>

		<!-- Footer -->
		<div class="border-surface3 flex justify-end gap-2 border-t px-3 py-2">
			<button type="button" class="button text-xs uppercase" onclick={cancel}>Cancel</button>
			<button type="button" class="button-primary text-xs uppercase" onclick={ok}>OK</button>
		</div>
	</div>
{/if}
