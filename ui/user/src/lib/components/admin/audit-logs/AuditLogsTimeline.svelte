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
	import type { AuditLog } from '$lib/services';
	import { debounce } from 'es-toolkit';
	import { autoUpdate, computePosition, flip, offset } from '@floating-ui/dom';
	import { fade } from 'svelte/transition';

	interface Props<T> {
		start: Date;
		end: Date;
		data: T[];
		padding?: number;
	}

	type FrameName = 'minute' | 'hour' | 'day' | 'month';
	type Frame = [name: FrameName, step: number, duration: number];

	let { start, end, data }: Props<AuditLog> = $props();

	let highlightedRectElement = $state<SVGRectElement>();

	let paddingLeft = $state(24);
	let paddingRight = $state(8);
	let paddingTop = $state(8);
	let paddingBottom = $state(16);

	let clientWidth = $state(0);
	let innerWidth = $derived(clientWidth - paddingLeft - paddingRight);

	let clientHeight = $state(0);
	let innerHeight = $derived(clientHeight - paddingTop - paddingBottom);

	const vpWidth = viewport();

	const callTypes = $derived(union(data.map((d) => d.callType)));

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
		const width = vpWidth.current;

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

		return (d: AuditLog) => round(new Date(d.createdAt)).toISOString();
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

	const colorByCallType: Record<string, string> = {
		initialize: '#254993',
		'notifications/initialized': '#D65C7C',
		'notifications/message': '#635DB6',
		'prompts/list': '#D6A95C',
		'resources/list': '#2EB88A',
		'tools/call': '#47A3D1',
		'tools/list': '#D0CE43'
	};

	const callTypesArray = $derived(callTypes.values().toArray());

	const colorScale = $derived(
		scaleOrdinal(
			callTypesArray,
			callTypesArray.map((d) => colorByCallType[d] ?? '#999999')
		)
	);

	const group = $derived.by(() => {
		return rollup(
			$state.snapshot(data),
			(d) => d.length,
			xAccessor,
			(d) => d.callType
		);
	});

	const series = $derived.by(() => {
		const stacked = stack()
			.keys(callTypes)
			.value((d, key) => (d[1] as unknown as Map<string, number>).get(key) ?? 0);

		return stacked(group as Iterable<{ [key: string]: number }>);
	});

	const yDomain = $derived.by(() => {
		const [mn, mx] = extent(series.map((serie) => extent(serie.flat())).flat(), (d) => d);

		return [mn ?? 0, mx ?? 0];
	});

	const yScale = $derived(scaleLinear(yDomain, [innerHeight, 0]));

	let currentItem = $state<{ key: string; value: string; date: string }>();

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
		const getViewportWidth = () => {
			if (typeof window !== 'undefined') {
				return (
					window.visualViewport?.width ||
					window.innerWidth ||
					document.documentElement.clientWidth ||
					document.body.clientWidth ||
					0
				);
			}

			return 0;
		};

		let width = $state(getViewportWidth());

		const onResize = debounce(() => {
			width = getViewportWidth();
		}, 1000 / 60);

		$effect(() => {
			window.addEventListener('resize', onResize);

			return () => {
				window.removeEventListener('resize', onResize);
			};
		});

		return {
			get current() {
				return width;
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

	function tooltip(reference: Element, floating: HTMLElement) {
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
</script>

<div bind:clientHeight bind:clientWidth class="group relative h-full w-full">
	{#if highlightedRectElement && currentItem}
		<div
			class="tooltip pointer-events-none fixed top-0 left-0 flex flex-col shadow-md"
			{@attach (node) => tooltip(highlightedRectElement!, node)}
			in:fade={{ duration: 100, delay: 10 }}
			out:fade={{ duration: 100 }}
		>
			<div class="flex flex-col gap-0 text-xs">
				<div>
					{currentItem?.date}
				</div>
				<div class="text-sm">
					{currentItem?.key}
				</div>
			</div>
			<div class="text-2xl font-bold">{currentItem?.value}</div>
		</div>
	{/if}

	<svg width={clientWidth} height={clientHeight} viewBox={`0 0 ${clientWidth} ${clientHeight}`}>
		<g transform="translate({paddingLeft}, {paddingTop})">
			<g
				class="x-axis text-on-surface3/20 dark:text-on-surface1/10"
				transform="translate(0 {innerHeight})"
				{@attach (node: SVGGElement) => {
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
									classNames.add(name);
								}
							};

							const remove = (...cn: string[]) => {
								for (const name of cn) {
									classNames.delete(name);
								}
							};

							const isActive = isWithinInterval(d as Date, {
								start,
								end
							});

							const classNames = new Set(element.classList);
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
							return classNames.values().toArray().join(' ');
						});
				}}
			></g>

			<g
				class="y-axis text-on-surface3/20 dark:text-on-surface1/10"
				{@attach (node: SVGGElement) => {
					select(node)
						.transition()
						.duration(100)
						.call(axisLeft(yScale).tickSizeOuter(0).ticks(3))
						.selectAll('.tick>line')
						.attr('x1', innerWidth);

					select(node).select('.domain').attr('opacity', 0);
				}}
			></g>

			<g
				class="data"
				{@attach (node: SVGGElement) => {
					select(node)
						.selectAll('g')
						.data(series)
						.join('g')
						.attr('class', 'serie')
						.attr('data-type', (d) => d.key)
						.attr('fill', (d) => colorScale(d.key))
						.selectAll('rect')
						.data((d) => d)
						.join('rect')
						.attr('x', (d) => xScale((d.data[0] ?? '') as unknown as string) ?? 0)
						.attr('y', (d) => yScale(d[1]))
						.attr('height', (d) => Math.abs(yScale(d[0]) - yScale(d[1])))
						.attr('width', xScale.bandwidth())
						.attr('cursor', 'pointer')
						.attr('class', 'text-on-surface1')
						.on('pointerenter', function (ev, d) {
							highlightedRectElement = this as SVGRectElement;

							const item: { key?: string; value?: string; date?: string } = {};

							const parentData = select(
								highlightedRectElement.parentNode as SVGElement
							).datum() as {
								key: string;
							};

							// Update tooltip content and position
							item.key = parentData.key;

							// The actual value of this segment
							item.value = d[1] - d[0] + '';

							item.date = new Date(d.data[0]).toLocaleString();

							currentItem = { ...item } as {
								key: string;
								value: string;
								date: string;
							};

							select(this).attr('stroke', 'currentColor').attr('stroke-width', 2);
						})
						.on('pointerleave', function () {
							if (this === highlightedRectElement) {
								highlightedRectElement = undefined;
							}

							select(this).attr('stroke-width', 0);
						});
				}}
			>
			</g>
		</g>
	</svg>
</div>
