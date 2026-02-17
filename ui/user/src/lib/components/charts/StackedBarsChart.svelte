<!--
	StackedBarsChart - A reusable stacked bar chart component for time-series data
	
	@component
	
	@example
	Example 1: AuditLog data
		const auditLogs = [
			{ createdAt: '2024-01-01T10:00:00Z', callType: 'initialize' },
			{ createdAt: '2024-01-01T11:00:00Z', callType: 'tools/call' }
		];
		
		Use with:
		- dateAccessor={(item) => item.createdAt}
		- categoryAccessor={(item) => item.callType}
		- colorScheme={{ 'initialize': '#254993', 'tools/call': '#47A3D1' }} (object mapping)
		- OR colorScheme={['#254993', '#47A3D1', '#635DB6']} (array of colors)
		
	Example 2: Custom sales data
		const salesData = [
			{ timestamp: new Date('2024-01-01'), product: 'Widget A' },
			{ timestamp: new Date('2024-01-02'), product: 'Widget B' }
		];
		
		Use with:
		- dateAccessor={(item) => item.timestamp}
		- categoryAccessor={(item) => item.product}
		- colorScheme={['#254993', '#D65C7C', '#635DB6']} (array for multiple categories)
-->

<script module>
	export type TooltipArg = {
		date?: Date;
		category?: string;
		value: number;
		data: any;
		group: any[];
	};

	export type StackTooltipArg = {
		date?: Date;
		segments: Array<{
			category: string;
			value: number;
			data: any;
			color: string;
			group: any[];
		}>;
		total: number;
	};

	export interface StackedBarsChartProps<T> {
		start: Date;
		end: Date;
		data: T[];
		padding?:
			| number
			| { top?: number; right?: number; bottom?: number; left?: number }
			| [top?: number, right?: number, bottom?: number, left?: number];
		/** Function to extract the date/time value from each data item */
		dateAccessor: (item: T) => Date | string;
		/** Function to extract the category/stack key from each data item */
		categoryAccessor: (item: T) => string;
		/** Optional function to determine the value for each item. Defaults to counting items in each category */
		groupAccessor?: (items: T[]) => number;
		/** Optional color mapping for categories. Can be an object mapping category names to colors, or an array of colors to use in order. If not provided, default colors will be used */
		colorScheme?: Record<string, string> | string[];
		/** Optional snippet to render segment tooltip (shows when hovering a specific segment) */
		segmentTooltip?: Snippet<[TooltipArg]>;
		/** Optional snippet to render stack tooltip (shows when hovering anywhere on the stack) */
		stackTooltip?: Snippet<[StackTooltipArg]>;
		/** Optional snippet to render when data is empty. If not provided, shows "No data available" message */
		emptyYAxisContent?: Snippet;
		/** Optional legend configuration. undefined = no legend (default), 'internal' = use built-in legend, Snippet = custom legend */
		legend?: 'internal' | Snippet<[{ category: string; color: string }[]]>;
	}

	function compilePadding(input?: StackedBarsChartProps<any>['padding']) {
		/** Declare default padding values */
		const defaultTop = 8;
		const defaultRight = 8;
		const defaultBottom = 16;
		const defaultLeft = 32;

		/** Compile padding input into a consistent object format */
		if (typeof input === 'number') {
			return { top: input, right: input, bottom: input, left: input };
		} else if (Array.isArray(input)) {
			const [top, right, bottom, left] = [
				input[0] ?? defaultTop,
				input[1] ?? defaultRight,
				input[2] ?? defaultBottom,
				input[3] ?? defaultLeft
			];

			return { top, right, bottom, left };

			/** Compile padding input into a consistent object format */
		} else if (typeof input === 'object' && input !== null) {
			const top = input.top ?? defaultTop;
			const right = input.right ?? defaultRight;
			const bottom = input.bottom ?? defaultBottom;
			const left = input.left ?? defaultLeft;

			return { top, right, bottom, left };
		}

		/** Return default padding if no input is provided */
		return { top: defaultTop, right: defaultRight, bottom: defaultBottom, left: defaultLeft };
	}
</script>

<script lang="ts">
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
		rollup,
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
	import { autoUpdate, computePosition, flip, offset } from '@floating-ui/dom';
	import { fade } from 'svelte/transition';
	import type { Snippet } from 'svelte';

	type FrameName = 'minute' | 'hour' | 'day' | 'month';
	type Frame = [name: FrameName, step: number, duration: number];

	let {
		start,
		end,
		data,
		padding,
		dateAccessor,
		categoryAccessor,
		groupAccessor = (d) => d.length,
		colorScheme,
		segmentTooltip,
		stackTooltip,
		emptyYAxisContent,
		legend
	}: StackedBarsChartProps<any> = $props();

	let highlightedRectElement = $state<SVGRectElement>();
	let tooltipData = $state<TooltipArg>();
	let stackTooltipData = $state<StackTooltipArg>();

	const compiledPadding = $derived(compilePadding(padding));

	let paddingLeft = $derived(compiledPadding.left);
	let paddingRight = $derived(compiledPadding.right);
	let paddingTop = $derived(compiledPadding.top);
	let paddingBottom = $derived(compiledPadding.bottom);

	let clientWidth = $state(0);
	let innerWidth = $derived(clientWidth - paddingLeft - paddingRight);

	let clientHeight = $state(0);
	let innerHeight = $derived(clientHeight - paddingTop - paddingBottom);

	const viewportWidth = viewport();

	const categories = $derived(union(data.map((d) => categoryAccessor(d))));

	const durationInterval = $derived(intervalToDuration({ start, end }));

	const timeFrame: Frame = $derived.by(() => {
		const durationInMonths =
			durationToMonths(durationInterval) + (durationInterval?.days ?? 0) / 30;

		if (durationInMonths > 4) {
			return ['month', 1, durationInMonths];
		}

		const durationInDays = durationToDays(durationInterval) + (durationInterval?.hours ?? 0) / 24;

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

		const durationInHours =
			durationToHours(durationInterval) + (durationInterval?.minutes ?? 0) / 60;

		if (durationInHours > 16) {
			return ['hour', 1, durationInHours];
		}

		const durationInMinutes =
			durationToMinutes(durationInterval) + (durationInterval?.seconds ?? 0) / 60;

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
		const width = viewportWidth.current;

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

		return (d: any) => {
			const dateValue = dateAccessor(d);
			const date = typeof dateValue === 'string' ? new Date(dateValue) : dateValue;
			return round(date).toISOString();
		};
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

		return union(generator(start, end, step).map((d) => d.toISOString()));
	});

	const xRange = $derived([0, innerWidth]);

	const timeScale = $derived(scaleTime(timeFrameDomain, xRange));

	const xScale = $derived(scaleBand(xRange).domain(bands).paddingInner(0.1).paddingOuter(0.1));

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

	const defaultColors = [
		'#254993',
		'#D65C7C',
		'#635DB6',
		'#D6A95C',
		'#2EB88A',
		'#47A3D1',
		'#D0CE43',
		'#E85D75',
		'#6C5CE7',
		'#00B894',
		'#FDCB6E',
		'#E17055',
		'#74B9FF',
		'#A29BFE',
		'#FD79A8',
		'#FFEAA7',
		'#55EFC4',
		'#81ECEC',
		'#FAB1A0',
		'#FF7675',
		'#636E72',
		'#999999'
	];

	const categoriesArray = $derived(categories.values().toArray());

	const colorScale = $derived(
		scaleOrdinal(
			categoriesArray,
			categoriesArray.map((d, i) => {
				if (!colorScheme) {
					return defaultColors[i % defaultColors.length];
				}
				if (Array.isArray(colorScheme)) {
					return colorScheme[i % colorScheme.length];
				}
				return colorScheme[d] ?? defaultColors[i % defaultColors.length];
			})
		)
	);

	const group = $derived.by(() => {
		return rollup(
			$state.snapshot(data),
			(items) => [groupAccessor(items), items] as const,
			xAccessor,
			categoryAccessor
		);
	});

	const series = $derived.by(() => {
		const stacked = stack()
			.keys(categories)
			.value((d, key) => {
				const grouped = (d[1] as unknown as Map<string, number>).get(key) as unknown as
					| [number, any]
					| undefined;
				return grouped?.[0] ?? 0;
			});

		return stacked(group as Iterable<{ [key: string]: number }>);
	});

	const yDomain = $derived.by(() => {
		const [mn, mx] = extent(series.map((serie) => extent(serie.flat())).flat(), (d) => d);

		return [mn ?? 0, mx ?? 0];
	});

	const hideYAxis = $derived(Math.abs(yDomain[1] - yDomain[0]) === 0);

	const yScale = $derived(scaleLinear(yDomain, [innerHeight, 0]));

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

	function viewport() {
		return {
			get current() {
				return clientWidth;
			}
		};
	}

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

	function createTooltip(reference: Element, floating: HTMLElement) {
		const compute = async () => {
			const position = await computePosition(reference, floating, {
				placement: 'top',
				middleware: [
					offset(8),
					flip({
						padding: {
							top: 0,
							right: 40,
							left: 40,
							bottom: 0
						},
						boundary: document.documentElement,
						fallbackPlacements: ['top', 'top-end', 'top-start', 'left-start', 'right-start']
					})
				]
			});

			const { x, y } = position;

			floating.style.transform = `translate(${x}px, ${y}px)`;
		};

		return autoUpdate(reference, floating, compute, {
			animationFrame: true,
			ancestorScroll: true,
			ancestorResize: true
		});
	}

	const legendData = $derived(
		categoriesArray.map((category) => ({
			category,
			color: colorScale(category)
		}))
	);
</script>

<div bind:clientHeight bind:clientWidth class="group relative flex w-full flex-1 flex-col">
	<!-- Empty state content -->
	{#if hideYAxis}
		<div class="absolute inset-0 flex items-center justify-center">
			{#if emptyYAxisContent}
				{@render emptyYAxisContent()}
			{:else}
				<div
					class="text-on-surface1 flex h-full w-full items-center justify-center text-sm font-light"
				>
					No data available
				</div>
			{/if}
		</div>
	{/if}

	<!-- Segment tooltip (shows when hovering a specific segment) -->
	{#if highlightedRectElement && tooltipData && !stackTooltipData}
		<div
			class="tooltip pointer-events-none fixed top-0 left-0 flex flex-col shadow-md"
			{@attach (node) => createTooltip(highlightedRectElement!, node)}
			in:fade={{ duration: 100, delay: 10 }}
			out:fade={{ duration: 100 }}
		>
			{#if segmentTooltip}
				{@render segmentTooltip(tooltipData)}
			{:else}
				<div class="flex flex-col">
					<div class="flex flex-col gap-0 text-xs">
						{#if tooltipData?.date}
							<div>
								{tooltipData.date.toLocaleDateString(undefined, {
									year: 'numeric',
									month: 'short',
									day: 'numeric',
									hour: '2-digit',
									minute: '2-digit'
								})}
							</div>
						{/if}
						{#if tooltipData?.category}
							<div class="text-sm">
								{tooltipData.category}
							</div>
						{/if}
					</div>
					<div class="text-2xl font-bold">{tooltipData?.value}</div>
				</div>
			{/if}
		</div>
	{/if}

	<!-- Stack tooltip (shows when hovering anywhere on the stack background) -->
	{#if highlightedRectElement && stackTooltipData}
		<div
			class="tooltip pointer-events-none fixed top-0 left-0 flex flex-col shadow-md"
			{@attach (node) => createTooltip(highlightedRectElement!, node)}
			in:fade={{ duration: 100, delay: 10 }}
			out:fade={{ duration: 100 }}
		>
			{#if stackTooltip}
				{@render stackTooltip(stackTooltipData)}
			{:else}
				<div class="flex flex-col gap-2">
					{#if stackTooltipData?.date}
						<div class="text-xs">
							{stackTooltipData.date.toLocaleDateString(undefined, {
								year: 'numeric',
								month: 'short',
								day: 'numeric',
								hour: '2-digit',
								minute: '2-digit'
							})}
						</div>
					{/if}
					<div class="flex flex-col gap-1">
						{#each stackTooltipData.segments as segment}
							<div class="flex items-center gap-2">
								<div
									class="h-3 w-3 rounded-sm"
									style="background-color: {colorScale(segment.category)}"
								></div>
								<div class="text-sm">{segment.category}</div>
								<div class="ml-auto font-semibold">{segment.value.toLocaleString()}</div>
							</div>
						{/each}
						<div class="flex items-center gap-2 border-t pt-1">
							<div class="text-sm font-semibold">Total</div>
							<div class="ml-auto text-lg font-bold">{stackTooltipData.total.toLocaleString()}</div>
						</div>
					</div>
				</div>
			{/if}
		</div>
	{/if}

	<svg class="flex-1 absolute inset-0" width={clientWidth} height={clientHeight} viewBox={`0 0 ${clientWidth} ${clientHeight}`}>
		<g transform="translate({paddingLeft}, {paddingTop})">
			<g
				class="x-axis text-on-surface3/20 dark:text-on-surface1/10"
				transform="translate(0 {innerHeight})"
				{@attach (node: SVGGElement) => {
					console.log('Updating x-axis');
					const selection = select(node);

					const format = timeFormat;

					const formatMillisecond = format('.%L'),
						formatSecond = format(':%S'),
						formatMinute = format('%I:%M'),
						formatHour = format('%I %p'),
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
							const baseClassName = ['duration-500', 'transiton-all'];
							add(...baseClassName);

							const activeClassName = ['text-on-surface3', 'dark:text-on-surface1'];
							const inactiveClassName = ['opacity-0', 'duration-500', 'transiton-opacity'];

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

			<!-- Y Axis -->
			<g style:opacity={+!hideYAxis}>
				<g
					class="y-axis text-on-surface3 dark:text-on-surface1"
					{@attach (node: SVGGElement) => {
						const selection = select(node);

						selection.transition().duration(100).call(axisLeft(yScale).tickSizeOuter(0).ticks(3));

						selection
							.selectAll('.tick')
							.attr('stroke', 'currentColor')
							.attr('stroke-opacity', 0.5)
							.selectAll('line')
							.attr('x1', innerWidth)
							.attr('stroke-opacity', 0.1);

						selection.select('.domain').attr('opacity', 0);
					}}
				></g>

				<!-- Background rects for stack-level tooltips -->
				<g
					class="stack-backgrounds"
					{@attach (node: SVGGElement) => {
						// Create one background rect per time bucket
						type BackgroundData = {
							dateKey: string;
							segments: Array<{
								category: string;
								value: number;
								data: { dateKey: string };
								color: string;
								group: any[];
							}>;
							total: number;
						};

						const backgroundData: BackgroundData[] = bands
							.values()
							.toArray()
							.map((band) => {
								const dateKey = band;
								const categoryMap = group.get(String(dateKey));

								if (!categoryMap) return null;

								const segments = categoriesArray
									.map((category) => {
										const groupedData = categoryMap.get(category);
										if (!groupedData) return null;

										const [value, items] = groupedData;
										return {
											category,
											value,
											data: { dateKey },
											color: colorScale(category),
											group: items
										};
									})
									.filter((s): s is NonNullable<typeof s> => s !== null);

								const total = segments.reduce((sum, seg) => sum + seg.value, 0);

								return {
									dateKey,
									segments,
									total
								};
							})
							.filter((d): d is BackgroundData => d !== null && d.segments.length > 0);

						select(node)
							.selectAll('rect')
							.data(backgroundData)
							.join('rect')
							.attr('x', (d) => xScale(d.dateKey) ?? 0)
							.attr('y', -paddingTop)
							.attr('width', Math.max(xScale.bandwidth(), 0))
							.attr('height', Math.max(innerHeight + paddingTop, 0))
							.attr('fill', 'transparent')
							.attr('cursor', 'pointer')
							.attr(
								'class',
								'stack-background text-background/0 hover:text-background/50 duration-200 transition-colors'
							)
							.attr('fill', 'currentColor')
							.on('pointerenter', function (ev, d) {
								highlightedRectElement = this as SVGRectElement;

								stackTooltipData = $state.snapshot({
									date: new Date(d.dateKey),
									segments: d.segments,
									total: d.total
								});

								// Clear segment tooltip when showing stack tooltip
								tooltipData = undefined;
							})
							.on('pointerleave', function () {
								if (this === highlightedRectElement) {
									highlightedRectElement = undefined;
									stackTooltipData = undefined;
								}
							});
					}}
				>
				</g>

				<!-- Segment rects -->
				<g
					class="data"
					{@attach (node: SVGGElement) => {
						select(node)
							.selectAll('g')
							.data(series)
							.join('g')
							.attr('class', 'serie')
							.attr('data-type', (d) => d.key)
							.attr('color', (d) => colorScale(d.key))
							.selectAll('rect')
							.data((d) => d)
							.join('rect')
							.attr('x', (d) => xScale((d.data[0] ?? '') as unknown as string) ?? 0)
							.attr('y', (d) => yScale(d[1]))
							.attr('height', (d) => Math.abs(yScale(d[0]) - yScale(d[1])))
							.attr('width', xScale.bandwidth())
							.attr('cursor', 'pointer')
							.attr(
								'class',
								'segment-rect stroke-current fill-current hover:fill-opacity-50 stroke-0 hover:stroke-2 duration-100 transition-colors'
							)
							.attr('pointer-events', 'all')
							.on('pointerenter', function (ev, d) {
								highlightedRectElement = this as SVGRectElement;

								const parentData = select(
									highlightedRectElement.parentNode as SVGElement
								).datum() as {
									key: string;
								};

								const category = parentData.key;
								const value = d[1] - d[0];
								const dateKey = d.data[0];
								const date = new Date(dateKey);

								// Get the original items from the grouped data
								const groupedData = group.get(String(dateKey))?.get(category);
								const items = groupedData?.[1] ?? [];

								tooltipData = $state.snapshot({
									date: date,
									category: category,
									value: value,
									data: d.data,
									group: items
								});

								// Clear stack tooltip when showing segment tooltip
								stackTooltipData = undefined;
							})
							.on('pointerleave', function () {
								if (this === highlightedRectElement) {
									highlightedRectElement = undefined;
									tooltipData = undefined;
								}
							});
					}}
				>
				</g>
			</g>
		</g>
	</svg>
</div>

<!-- Legend -->
{#if legend}
	{#if legend === 'internal'}
		<div class="legend-container shrink-0 flex justify-center max-h-48 overflow-y-auto">
			<div class="flex flex-wrap items-center justify-center gap-x-4 gap-y-2 px-2 py-1">
				{#each legendData as item (item.category)}
					<div class="flex items-center gap-1" style:color={item.color}>
						<div class="h-3 w-3 rounded-sm bg-current"></div>
						<span class="text-sm">{item.category}</span>
					</div>
				{/each}
			</div>
		</div>
	{:else}
		{@render legend(legendData)}
	{/if}
{/if}
