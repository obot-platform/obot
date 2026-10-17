<script lang="ts" generics="T extends object">
	import { lightenHex } from '$lib/colors';
	import { m } from '$lib/i18n';
	import { darkMode, userDeviceSettings } from '$lib/stores';
	import { formatLogTimestamp } from '$lib/time';
	import IconButton from '../primitives/IconButton.svelte';
	import { categoricalColor } from './palette';
	import { chartTooltip } from './tooltip';
	import { Ellipsis } from '@lucide/svelte';
	import {
		scaleBand,
		scaleLinear,
		scaleOrdinal,
		scaleTime,
		stack,
		union,
		extent,
		select,
		axisBottom,
		timeDays,
		timeHours,
		timeWeeks,
		timeMinutes,
		timeMonths,
		axisLeft,
		type NumberValue
	} from 'd3';
	import { timeFormat } from 'd3-time-format';
	import {
		startOfMonth,
		endOfMonth,
		isWithinInterval,
		startOfHour,
		endOfHour,
		startOfDay,
		startOfYear,
		intervalToDuration,
		startOfSecond,
		startOfMinute,
		startOfWeek,
		endOfWeek,
		getDate,
		max,
		min,
		set,
		endOfMinute,
		getHours,
		type Duration,
		getDay
	} from 'date-fns';
	import type { Snippet } from 'svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	/** Tooltip payload: segment key (category), stack value, bucket date, and aggregated counts/totals. */
	export type TooltipItem = {
		key: string;
		value: string;
		date: string;
		count: number;
		primaryTotal?: number;
		secondaryTotal?: number;
		hoveredPart?: 'primary' | 'secondary';
		primaryColor?: string;
		secondaryColor?: string;
		details?: Record<string, number>;
	};

	interface Props<T> {
		start: Date;
		end: Date;
		data: T[];
		categoryKey: keyof T;
		dateKey: keyof T;
		primaryValueKey?: keyof T;
		secondaryValueKey?: keyof T;
		tooltipValueKeys?: (keyof T)[];
		tooltipContent?: Snippet<[TooltipItem]>;
		legend?: {
			primaryLabel?: string;
			secondaryLabel?: string;
			showSecondaryLabel?: boolean;
			hideCategoryLabel?: boolean;
		};
		padding?: number;
		class?: string;
		classes?: {
			legend?: string;
		};
	}

	type FrameName = 'minute' | 'hour' | 'day' | 'month';
	type Frame = [name: FrameName, step: number, duration: number];

	let {
		start,
		end,
		data,
		categoryKey,
		dateKey,
		primaryValueKey,
		secondaryValueKey,
		tooltipValueKeys,
		legend,
		tooltipContent,
		class: klass = '',
		classes
	}: Props<T> = $props();

	let highlightedRectElement = $state<SVGGraphicsElement>();

	const yAxisLabelMinWidth = 24;
	const yAxisLabelPadding = 12;
	let yAxisLabelWidth = $state(yAxisLabelMinWidth);
	let yAxisLabelMeasureEl = $state<HTMLSpanElement | null>(null);
	let paddingLeft = $state(0);
	let paddingRight = $state(8);
	let paddingTop = $state(8);
	let paddingBottom = $state(16);

	let clientWidth = $state(0);
	let innerWidth = $derived(clientWidth - paddingLeft - paddingRight);

	let clientHeight = $state(0);
	let innerHeight = $derived(clientHeight - paddingTop - paddingBottom);

	const categorySet = $derived(union(data.map((d) => String(d[categoryKey]))));
	const categoryKeys = $derived(Array.from(categorySet));

	const durationInterval = $derived(intervalToDuration({ start, end }));

	const timeFrame: Frame = $derived.by(() => {
		const durationInMonths =
			durationToMonths(durationInterval) + (durationInterval?.days ?? 0) / 30;
		const durationInDays = durationToDays(durationInterval) + (durationInterval?.hours ?? 0) / 24;
		const durationInHours =
			durationToHours(durationInterval) + (durationInterval?.minutes ?? 0) / 60;
		const durationInMinutes =
			durationToMinutes(durationInterval) + (durationInterval?.seconds ?? 0) / 60;
		const n = data.length;

		// When data is large, prefer at least hour granularity so band count stays bounded
		if (n > 1000 && durationInHours >= 1) {
			if (durationInMonths > 4) return ['month', 1, durationInMonths];
			if (durationInDays > 20) return ['day', 1, durationInDays];
			if (durationInDays > 8) return ['hour', 12, durationInDays];
			if (durationInDays > 4) return ['hour', 6, durationInDays];
			if (durationInDays > 2) return ['hour', 3, durationInDays];
			if (durationInDays > 1) return ['hour', 2, durationInDays];
			return ['hour', 1, durationInHours];
		}

		if (durationInMonths > 4) {
			return ['month', 1, durationInMonths];
		}

		if (durationInDays > 20) {
			return ['day', 1, durationInDays];
		}

		if (durationInDays > 8) {
			return ['hour', 12, durationInDays];
		}

		if (durationInDays > 4) {
			return ['hour', 6, durationInDays];
		}

		if (durationInDays > 2) {
			return ['hour', 3, durationInDays];
		}

		if (durationInDays > 1) {
			return ['hour', 2, durationInDays];
		}

		if (durationInHours > 16) {
			return ['hour', 1, durationInHours];
		}

		if (durationInHours > 1) {
			const allowedSteps = [5, 10, 15, 20, 30];
			const minutes = Math.max(5, Math.floor(durationInMinutes / 24));
			const rounded = allowedSteps.find((step) => minutes <= step) ?? 60;

			return ['minute', rounded, durationInMinutes];
		}

		return ['minute', 1, durationInMinutes];
	});

	const boundaries = $derived.by(() => {
		const [frame, step] = timeFrame;

		if (frame === 'minute') {
			if (step === 1) {
				return [startOfMinute, endOfMinute];
			}

			// When step is > 1, add extra step to the end boundary to ensure the last items are rendered
			return [
				(d: Date) => set(d, { minutes: Math.floor(d.getMinutes() / step) * step, seconds: 0 }),
				(d: Date) => set(d, { minutes: Math.ceil(d.getMinutes() / step) * step + step, seconds: 0 })
			];
		}

		if (frame === 'hour') {
			if (step === 1) {
				return [startOfHour, endOfHour];
			}

			// make the start boundary to start of day to ensure days are rendered correctly in ticks
			// When step is > 1, add extra step to the end boundary to ensure the last items are rendered
			return [
				startOfDay,
				(d: Date) =>
					set(d, { hours: Math.ceil(d.getHours() / step) * step + step, minutes: 0, seconds: 0 })
			];
		}

		if (frame === 'day') {
			return [
				(d: Date) => max([startOfMonth(d), startOfWeek(d)]),
				(d: Date) => min([endOfMonth(d), endOfWeek(d)])
			];
		}

		return [startOfMonth, endOfMonth];
	});

	const timeFrameDomain: [Date, Date] = $derived.by(() => {
		const [setStartBoundary, setEndBoundary] = boundaries;

		return [setStartBoundary(start), setEndBoundary(end)];
	});

	const ticksRatio = $derived.by(() => {
		// Use container width so multiple graphs in a row each get appropriate tick density
		const width = clientWidth;

		if (width >= 1440) {
			return 1;
		}

		if (width >= 1280) {
			return 2;
		}

		if (width >= 1024) {
			return 3;
		}

		if (width >= 768) {
			return 4;
		}

		if (width >= 425) {
			return 5;
		}

		return 6;
	});

	const xAccessor = $derived.by(() => {
		const [frame, step] = timeFrame;

		const round = (d: Date) => {
			if (frame === 'minute') {
				if (step === 1) {
					return startOfMinute(d);
				}
				return set(d, {
					minutes: Math.floor(d.getMinutes() / step) * step,
					seconds: 0,
					milliseconds: 0
				});
			}

			if (frame === 'hour') {
				if (step === 1) {
					return startOfHour(d);
				}

				return set(d, {
					hours: Math.floor(d.getHours() / step) * step,
					minutes: 0,
					seconds: 0,
					milliseconds: 0
				});
			}

			if (frame === 'day') {
				if (step === 1) {
					return startOfDay(d);
				}

				return set(d, {
					date: Math.floor(d.getDate() / step) * step,
					hours: 0,
					minutes: 0,
					seconds: 0,
					milliseconds: 0
				});
			}

			if (frame === 'month') {
				if (step === 1) {
					return startOfMonth(d);
				}

				return set(d, {
					month: Math.floor(d.getMonth() / step) * step,
					date: 0,
					hours: 0,
					minutes: 0,
					seconds: 0,
					milliseconds: 0
				});
			}

			return startOfYear(d);
		};

		return (d: T) => round(new Date(d[dateKey] as string | Date)).toISOString();
	});

	const bands = $derived.by(() => {
		type Generator =
			| typeof timeMinutes
			| typeof timeHours
			| typeof timeDays
			| typeof timeWeeks
			| typeof timeMonths;

		const [start, end] = timeFrameDomain as [Date, Date];
		const [frame, frameStep] = timeFrame;
		const accessor = xAccessor;

		let generator: Generator = timeMinutes;
		let step = frameStep;

		if (frame === 'hour') {
			generator = timeHours;
		}

		if (frame === 'day') {
			generator = timeDays;
		}

		if (frame === 'month') {
			generator = timeMonths;
		}

		const generated = union(generator(start, end, step).map((d) => d.toISOString()));
		const fromData = union(data.map((d) => accessor(d)));
		const combined = new Set<string>([...generated, ...fromData]);
		return Array.from(combined).sort((a, b) => a.localeCompare(b));
	});

	const xRange = $derived([0, innerWidth]);

	const timeScale = $derived(scaleTime(timeFrameDomain, xRange));

	const xScale = $derived(scaleBand(xRange).domain(bands).paddingInner(0.1).paddingOuter(0.1));

	// Bars never fill a wide band: with only a few buckets they stay slim and centered.
	const MAX_BAR_WIDTH = 24;
	const BAR_RADIUS = 4;
	const STACK_GAP = 2;
	const EMPTY_STUB_HEIGHT = 2;
	const barWidth = $derived(Math.max(1, Math.min(xScale.bandwidth(), MAX_BAR_WIDTH)));
	const barOffset = $derived((xScale.bandwidth() - barWidth) / 2);

	const xAxisTicks = $derived.by(() => {
		const [frame, frameStep, duration] = timeFrame;

		let generator = timeMinutes;
		let step = frameStep * ticksRatio;

		if (frame === 'minute') {
			if (duration < 30) {
				step = 1 * ticksRatio;
			} else if (duration < 60) {
				step = 2 * ticksRatio;
			} else {
				step = frameStep * ticksRatio;
			}
		}

		if (frame === 'hour') {
			generator = timeHours;
		}

		if (frame === 'day') {
			generator = timeDays;
			step = Math.max(1, Math.ceil(duration / 31) * Math.round(ticksRatio / 2));
		}

		if (frame === 'month') {
			generator = timeMonths;
		}

		const [start, end] = timeFrameDomain;

		return generator(start, end, step);
	});

	const colorScale = $derived(
		scaleOrdinal(
			categoryKeys,
			categoryKeys.map((_, i) => categoricalColor(i, darkMode.isDark))
		)
	);

	const aggregated = $derived.by(() => {
		const snapshot = $state.snapshot(data);
		const accessor = xAccessor;
		const groupMap = new SvelteMap<string, Map<string, number>>();
		const primKey = primaryValueKey;
		const secKey = secondaryValueKey;
		const detailKeys = tooltipValueKeys ?? [];
		let primarySecondary: Record<
			string,
			Record<string, { primary: number; secondary: number }>
		> | null = null;
		let details: Record<string, Record<string, Record<string, number>>> | null = null;
		if (primKey != null) {
			primarySecondary = {};
		}
		if (detailKeys.length > 0) {
			details = {};
		}
		for (const row of snapshot) {
			const r = row as T;
			const bucket = accessor(r);
			const cat = String(r[categoryKey]);
			// group: bucket -> category -> count
			let byCat = groupMap.get(bucket);
			if (!byCat) {
				byCat = new SvelteMap<string, number>();
				groupMap.set(bucket, byCat);
			}
			byCat.set(cat, (byCat.get(cat) ?? 0) + 1);
			if (primarySecondary && primKey != null) {
				if (!primarySecondary[bucket]) primarySecondary[bucket] = {};
				if (!primarySecondary[bucket][cat])
					primarySecondary[bucket][cat] = { primary: 0, secondary: 0 };
				primarySecondary[bucket][cat].primary += Number(r[primKey]) || 0;
				primarySecondary[bucket][cat].secondary += secKey != null ? Number(r[secKey]) || 0 : 0;
			}
			if (details) {
				if (!details[bucket]) details[bucket] = {};
				if (!details[bucket][cat]) details[bucket][cat] = {};
				for (const key of detailKeys) {
					const detailKey = String(key);
					details[bucket][cat][detailKey] =
						(details[bucket][cat][detailKey] ?? 0) + (Number(r[key]) || 0);
				}
			}
		}
		return { group: groupMap, bucketPrimarySecondary: primarySecondary, bucketDetails: details };
	});

	const group = $derived(aggregated.group);
	const bucketPrimarySecondary = $derived(aggregated.bucketPrimarySecondary);
	const bucketDetails = $derived(aggregated.bucketDetails);

	const stackInput = $derived.by((): Iterable<[string, Map<string, number>]> => {
		if (!bucketPrimarySecondary) return group as Iterable<[string, Map<string, number>]>;
		const result = new SvelteMap<string, Map<string, number>>();
		for (const [bucket] of group as Map<string, Map<string, number>>) {
			const valueMap = new SvelteMap<string, number>();
			for (const cat of categoryKeys) {
				const ps = bucketPrimarySecondary[bucket]?.[cat];
				const total = (ps?.primary ?? 0) + (ps?.secondary ?? 0);
				valueMap.set(cat, total);
			}
			result.set(bucket, valueMap);
		}
		return result;
	});

	const series = $derived.by(() => {
		const stacked = stack()
			.keys(categoryKeys)
			.value((d, key) => (d[1] as unknown as Map<string, number>).get(key) ?? 0);

		return stacked(stackInput as Iterable<{ [key: string]: number }>);
	});

	const yDomain = $derived.by(() => {
		const [mn, mx] = extent(series.map((serie) => extent(serie.flat())).flat(), (d) => d);

		return [mn ?? 0, mx ?? 0];
	});

	const yScale = $derived(scaleLinear(yDomain, [innerHeight, 0]).nice(3));
	// Plain counts (no value keys) can't be fractional, so drop ticks like 0.5 on small ranges.
	const countsOnly = $derived(primaryValueKey == null);
	const yAxisTicks = $derived(
		countsOnly ? yScale.ticks(3).filter((t) => Number.isInteger(t)) : yScale.ticks(3)
	);
	const yAxisTickFormat = $derived(
		countsOnly ? (t: number) => t.toLocaleString() : yScale.tickFormat(3)
	);
	const yAxisLongestLabel = $derived(
		yAxisTicks.length
			? yAxisTicks
					.map((t) => String(yAxisTickFormat(t)))
					.reduce((a, b) => (a.length >= b.length ? a : b), '')
			: ''
	);

	$effect(() => {
		const el = yAxisLabelMeasureEl;
		const label = yAxisLongestLabel;
		if (!el || !label) {
			yAxisLabelWidth = yAxisLabelMinWidth;
			return;
		}
		const measure = () => {
			const w = el.getBoundingClientRect().width;
			yAxisLabelWidth = Math.max(yAxisLabelMinWidth, Math.ceil(w) + yAxisLabelPadding);
		};
		measure();
		const ro = new ResizeObserver(measure);
		ro.observe(el);
		return () => ro.disconnect();
	});

	const useSplit = $derived(bucketPrimarySecondary != null && secondaryValueKey != null);

	let legendExpanded = $state(false);
	const legendInitialCount = 8;
	const legendCategories = $derived(
		legendExpanded ? categoryKeys : categoryKeys.slice(0, legendInitialCount)
	);
	const hasMoreLegendCategories = $derived(categoryKeys.length > legendInitialCount);

	let currentItem = $state<TooltipItem>();
	let hoveredBucket = $state<string>();

	const bucketTotals = $derived.by(() => {
		const totals = new SvelteMap<string, number>();
		for (const [bucket, values] of stackInput) {
			let sum = 0;
			for (const v of values.values()) sum += v;
			totals.set(bucket, sum);
		}
		return totals;
	});

	const emptyBuckets = $derived(bands.filter((b) => !(bucketTotals.get(b) ?? 0)));

	const isMainTick = (tick: Date) => {
		const [frame] = timeFrame;

		switch (frame) {
			case 'minute':
				return tick.getMinutes() === 0;
			case 'hour':
				return tick.getHours() === 0;
			case 'day':
				return tick.getDate() === 1 || getDay(tick) === 1;
			case 'month':
				return tick.getMonth() === 0;
			default:
				return false;
		}
	};

	function durationToMonths(duration: Duration) {
		return (duration.years ?? 0) * 12 + (duration.months ?? 0);
	}

	function durationToDays(duration: Duration) {
		return durationToMonths(duration) * 30 + (duration.days ?? 0);
	}

	function durationToHours(duration: Duration) {
		return durationToDays(duration) * 24 + (duration.hours ?? 0);
	}

	function durationToMinutes(duration: Duration) {
		return durationToHours(duration) * 60 + (duration.minutes ?? 0);
	}
</script>

<div class={twMerge('group relative flex h-full w-full flex-col', klass)}>
	<div class="flex min-h-0 flex-1 gap-0">
		<div
			class="y-axis-labels text-muted-content relative shrink-0 self-stretch text-xs"
			style="width: {yAxisLabelWidth}px;"
			aria-hidden="true"
		>
			<span
				bind:this={yAxisLabelMeasureEl}
				class="invisible absolute top-0 left-0 whitespace-nowrap"
				aria-hidden="true"
			>
				{yAxisLongestLabel || '0'}
			</span>
			{#each yAxisTicks as tick (tick)}
				<div
					class="absolute right-0 flex justify-end pr-2"
					style="top: {paddingTop + yScale(tick)}px; transform: translateY(-50%);"
				>
					{yAxisTickFormat(tick)}
				</div>
			{/each}
		</div>
		<div bind:clientHeight bind:clientWidth class="min-h-0 min-w-0 flex-1">
			{#if highlightedRectElement && currentItem}
				<div
					class="tooltip bg-base-100 dark:bg-base-300 pointer-events-none fixed top-0 left-0 z-50 flex flex-col shadow-md"
					{@attach (node) => chartTooltip(highlightedRectElement!, node)}
					in:fade={{ duration: 100, delay: 10 }}
					out:fade={{ duration: 100 }}
				>
					{#if tooltipContent}
						{@render tooltipContent(currentItem)}
					{:else}
						<div class="flex flex-col gap-0 text-xs">
							<div class="text-base-content text-sm">
								{currentItem?.key}
							</div>
							<div class="border-base-200 mb-2 border-b pb-2">
								{currentItem?.date}
							</div>
						</div>
						<div class="text-base-content text-2xl font-bold">{currentItem?.value}</div>
					{/if}
				</div>
			{/if}

			<svg
				width={clientWidth}
				height={clientHeight}
				viewBox={`0 0 ${clientWidth} ${clientHeight}`}
				class="overflow-visible"
			>
				<g transform="translate({paddingLeft}, {paddingTop})">
					{#key userDeviceSettings.timeFormat}
						<g
							class="x-axis text-muted-content"
							transform="translate(0 {innerHeight})"
							{@attach (node: SVGGElement) => {
								const selection = select(node);

								const format = timeFormat;
								const use24h = userDeviceSettings.timeFormat === '24h';

								const formatMillisecond = format('.%L'),
									formatSecond = format(':%S'),
									formatMinute = format(use24h ? '%H:%M' : '%I:%M'),
									formatHour = format(use24h ? '%H:00' : '%I %p'),
									formatDayOfWeek = format('%a %d'),
									formatDayOfMonth = format('%d'),
									formatMonth = format('%B'),
									formatYear = format('%Y');

								function tickFormat(domainValue: Date | NumberValue) {
									const date = domainValue as Date;
									const fn = (() => {
										if (startOfSecond(date) < date) return formatMillisecond;

										if (startOfMinute(date) < date) return formatSecond;

										if (startOfHour(date) < date) {
											if (getHours(date) === 0) {
												return formatDayOfMonth;
											}

											return formatMinute;
										}

										if (startOfDay(date) < date) {
											return formatHour;
										}

										if (startOfMonth(date) < date) {
											if (getDate(date) === 15) {
												return formatDayOfWeek;
											}

											if (timeFrame[0] === 'hour') {
												return formatDayOfWeek;
											}

											if (timeFrame[0] === 'day' && timeFrame[2] <= 90 && getDay(date) === 1) {
												return formatDayOfWeek;
											}

											return formatDayOfMonth;
										}

										if (startOfYear(date) < date) return formatMonth;

										return formatYear;
									})();

									return fn(date);
								}

								const axis = axisBottom(timeScale)
									.tickSizeOuter(0)
									.tickValues(xAxisTicks)
									.tickFormat(tickFormat);

								selection
									.transition()
									.duration(100)
									.call(axis)
									.selectAll('.tick')
									.attr(
										'transform',
										(d) => `translate(${timeScale(d as Date) + xScale.bandwidth() / 2}, 0)`
									)
									.selectAll('line, text')
									.attr('class', function (d) {
										const element = this as SVGElement;

										const add = (...cn: string[]) => {
											for (const name of cn) {
												if (!classNames.includes(name)) {
													classNames.push(name);
												}
											}
										};

										const remove = (...cn: string[]) => {
											for (const name of cn) {
												classNames = classNames.filter((c) => c !== name);
											}
										};

										const isActive = isWithinInterval(d as Date, {
											start,
											end
										});

										let classNames = [...element.classList];
										const baseClassName = ['duration-500', 'transition-all'];
										add(...baseClassName);

										const activeClassName = ['text-muted-content'];
										const inactiveClassName = ['opacity-0', 'duration-500', 'transition-opacity'];

										if (isActive) {
											add(...activeClassName);
											remove(...inactiveClassName);
										} else {
											add(...inactiveClassName);
											remove(...activeClassName);
										}

										const mainTickClassName = ['opacity-100', 'font-medium'];
										const secondaryTickClassName = ['opacity-50', 'font-normal'];

										const isMain = isMainTick(d as Date);

										if (isMain) {
											add(...mainTickClassName);
										} else {
											remove(...mainTickClassName);
											add(...secondaryTickClassName);
										}

										// Keep old class names
										// Filter falsy values and join with a space
										return classNames.join(' ');
									});
							}}
						></g>
					{/key}

					<g
						class="y-axis text-base-content/15 dark:text-base-content/10"
						{@attach (node: SVGGElement) => {
							select(node)
								.transition()
								.duration(100)
								.call(axisLeft(yScale).tickSizeOuter(0).tickValues(yAxisTicks))
								.selectAll('.tick>line')
								.attr('x1', innerWidth);
							select(node).selectAll('.tick text').remove();
							select(node).select('.domain').attr('opacity', 0);
						}}
					></g>

					{#if hoveredBucket !== undefined && xScale(hoveredBucket) !== undefined}
						<rect
							class="pointer-events-none fill-base-content/5"
							x={xScale(hoveredBucket)}
							y={0}
							width={xScale.bandwidth()}
							height={Math.max(0, innerHeight)}
							rx={4}
							transition:fade={{ duration: 100 }}
						/>
					{/if}

					<!-- Quiet stubs mark empty buckets so sparse data still reads as a timeline. -->
					<g class="empty-buckets pointer-events-none fill-base-content/10" aria-hidden="true">
						{#each emptyBuckets as bucket (bucket)}
							<rect
								x={(xScale(bucket) ?? 0) + barOffset}
								y={innerHeight - EMPTY_STUB_HEIGHT}
								width={barWidth}
								height={EMPTY_STUB_HEIGHT}
								rx={EMPTY_STUB_HEIGHT / 2}
							/>
						{/each}
					</g>

					<g
						class="data"
						class:dimmed={hoveredBucket !== undefined}
						{@attach (node: SVGGElement) => {
							// Wait for layout: drawing at size 0 makes bars tween in from the corner.
							if (innerWidth <= 0 || innerHeight <= 0) return;

							const sums = bucketPrimarySecondary;
							const split = useSplit;
							const w = barWidth;
							const offsetX = barOffset;

							type StackSegment = [number, number] & { data: unknown[] };
							type RectDatum = {
								seg: StackSegment;
								part: 'full' | 'primary' | 'secondary';
								category: string;
								isTopOfStack: boolean;
							};

							const topY1ByBucket = new SvelteMap<string, number>();
							for (const serie of series) {
								for (const seg of serie as unknown as StackSegment[]) {
									const bucket = String(seg.data[0] ?? '');
									const y1 = seg[1] as number;
									const cur = topY1ByBucket.get(bucket);
									if (cur === undefined || y1 > cur) topY1ByBucket.set(bucket, y1);
								}
							}

							// Always emit the same path commands (radius may be 0) so d3 can tween between shapes.
							function rectPath(x: number, y: number, w: number, h: number, topRounded: boolean) {
								const r = topRounded ? Math.max(0, Math.min(BAR_RADIUS, w / 2, h)) : 0;
								return `M${x + r},${y}L${x + w - r},${y}Q${x + w},${y} ${x + w},${y + r}L${x + w},${y + h}L${x},${y + h}L${x},${y + r}Q${x},${y} ${x + r},${y}Z`;
							}

							function bucketOf(d: RectDatum) {
								return typeof d.seg.data[0] === 'string'
									? d.seg.data[0]
									: String(d.seg.data[0] ?? '');
							}

							function segmentBounds(d: RectDatum): [number, number] {
								const y0 = d.seg[0] as number;
								const y1 = d.seg[1] as number;
								if (d.part === 'full') return [y0, y1];
								const s = sums?.[bucketOf(d)]?.[d.category];
								const total = (s?.primary ?? 0) + (s?.secondary ?? 0);
								const split = y0 + (y1 - y0) * (total > 0 ? (s?.primary ?? 0) / total : 0);
								return d.part === 'primary' ? [y0, split] : [split, y1];
							}

							function finalPath(d: RectDatum) {
								const x = (xScale(bucketOf(d)) ?? 0) + offsetX;
								const [lo, hi] = segmentBounds(d);
								const yTop = yScale(hi);
								let yBottom = yScale(lo);
								// Surface gap between stacked segments (none at the baseline).
								if (lo > 0 && hi > lo)
									yBottom -= Math.min(STACK_GAP, Math.max(0, yBottom - yTop - 1));
								return rectPath(x, yTop, w, Math.max(0, yBottom - yTop), d.isTopOfStack);
							}

							select(node)
								.selectAll('g')
								.data(series)
								.join('g')
								.attr('class', 'serie')
								.attr('data-type', (d) => d.key)
								.selectAll<SVGPathElement, RectDatum>('path')
								.data(
									(seriesGroup): RectDatum[] => {
										const cat = seriesGroup.key;
										const segs = (seriesGroup as unknown as StackSegment[]).filter(
											(seg) => (seg[1] as number) > (seg[0] as number)
										);
										const isTop = (seg: StackSegment) =>
											(seg[1] as number) === topY1ByBucket.get(String(seg.data[0] ?? ''));
										if (split && sums) {
											return segs.flatMap((seg) => [
												{ seg, part: 'primary' as const, category: cat, isTopOfStack: false },
												{ seg, part: 'secondary' as const, category: cat, isTopOfStack: isTop(seg) }
											]);
										}
										return segs.map((seg) => ({
											seg,
											part: 'full' as const,
											category: cat,
											isTopOfStack: isTop(seg)
										}));
									},
									(d) => `${bucketOf(d)}:${d.part}`
								)
								.join(
									(enter) =>
										enter
											.append('path')
											.attr('d', finalPath)
											.attr('opacity', 0)
											.call((e) => e.transition('enter').duration(300).attr('opacity', 1)),
									(update) => update.call((u) => u.transition().duration(250).attr('d', finalPath))
								)
								.attr('fill', (d) => {
									const base = colorScale(d.category);
									return d.part === 'secondary' ? lightenHex(base, 0.5) : base;
								})
								.attr('cursor', 'pointer')
								.on('pointerenter', function (_ev, d) {
									highlightedRectElement = this as SVGGraphicsElement;
									const bucketKey = bucketOf(d);
									hoveredBucket = bucketKey;
									const baseColor = colorScale(d.category);
									const ps = bucketPrimarySecondary?.[bucketKey]?.[d.category];
									const details = bucketDetails?.[bucketKey]?.[d.category];
									const count = (group.get(bucketKey)?.get(d.category) ?? 0) as number;
									currentItem = {
										key: d.category,
										value: `${(d.seg[1] as number) - (d.seg[0] as number)}`,
										date: formatLogTimestamp(bucketKey, userDeviceSettings.timeFormat),
										count,
										...(ps != null && {
											primaryTotal: ps.primary,
											secondaryTotal: ps.secondary,
											count: ps.primary + ps.secondary
										}),
										...(d.part !== 'full' && { hoveredPart: d.part }),
										primaryColor: baseColor,
										secondaryColor: lightenHex(baseColor, 0.5),
										...(details && { details })
									};
								})
								.on('pointerleave', function () {
									if (this === highlightedRectElement) {
										highlightedRectElement = undefined;
										hoveredBucket = undefined;
									}
								});
						}}
					>
					</g>
				</g>
			</svg>
		</div>
	</div>
</div>

{#if legend}
	<div
		class={twMerge(
			'flex flex-wrap items-center justify-center gap-x-4 gap-y-2 pt-6 text-xs',
			classes?.legend
		)}
	>
		{#each legendCategories as category (category)}
			{@const categoryLabel = legend.hideCategoryLabel ? '' : category}
			{@const primaryColor = colorScale(category)}
			{@const secondaryColor = lightenHex(primaryColor, 0.5)}
			{#if legend.showSecondaryLabel}
				{@render legendItem(primaryColor, categoryLabel, legend.primaryLabel)}
				{@render legendItem(secondaryColor, categoryLabel, legend.secondaryLabel)}
			{:else}
				{@render legendItem(primaryColor, categoryLabel, legend.primaryLabel)}
			{/if}
		{/each}
		{#if hasMoreLegendCategories}
			{#if legendExpanded}
				<button
					class="text-muted-content hover:underline"
					onclick={() => (legendExpanded = !legendExpanded)}>{m.core_show_less()}</button
				>
			{:else}
				<IconButton
					class="btn-xs"
					tooltip={{ text: m.core_show_all_legend_items() }}
					onclick={() => (legendExpanded = !legendExpanded)}
				>
					<Ellipsis class="size-3" />
				</IconButton>
			{/if}
		{/if}
	</div>
{/if}

{#snippet legendItem(color: string, category: string, label?: string)}
	<span class="flex items-center gap-0.5">
		<span class="size-2 shrink-0 rounded-full" style="background-color: {color}"></span>
		<span class="ml-0.5">
			{#if label && category}
				{category} <span class="text-muted-content">{label}</span>
			{:else}
				{label || category}
			{/if}
		</span>
	</span>
{/snippet}

<style>
	.data :global(path) {
		transition: opacity 150ms ease-out;
	}

	.data.dimmed :global(path:not(:hover)) {
		opacity: 0.45;
	}
</style>
