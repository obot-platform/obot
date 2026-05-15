import type { DonutDatum } from '$lib/components/graph/DonutGraph.svelte';

export const DEVICE_SCAN_TOP_N = 10;

export const DEVICE_SCAN_PALETTE = [
	'#4575b4',
	'#74add1',
	'#abd9e9',
	'#e0f3f8',
	'#fee090',
	'#fdae61',
	'#f46d43',
	'#d73027',
	'#a50026',
	'#7f3b08'
];

export const DEVICE_SCAN_OTHER_COLOR = 'var(--color-base-400, #6b7280)';

export type DeviceScanDrilldown = 'mcp' | 'skill' | undefined;

export type DeviceScanTopBucket = {
	key: string;
	label: string;
	value: number;
	color: string;
	isOther: boolean;
	otherCount?: number;
	drilldown: DeviceScanDrilldown;
};

export function buildDeviceScanTopBuckets<T>(
	items: T[] | null | undefined,
	key: (t: T) => string,
	label: (t: T) => string,
	value: (t: T) => number,
	drilldown: DeviceScanDrilldown = undefined,
	topN: number = DEVICE_SCAN_TOP_N
): DeviceScanTopBucket[] {
	const all = (items ?? []).filter((t) => value(t) > 0);
	const sorted = [...all].sort((a, b) => value(b) - value(a));
	const top = sorted.slice(0, topN).map<DeviceScanTopBucket>((t, i) => ({
		key: key(t),
		label: label(t),
		value: value(t),
		color: DEVICE_SCAN_PALETTE[i] ?? DEVICE_SCAN_OTHER_COLOR,
		isOther: false,
		drilldown
	}));
	const tail = sorted.slice(topN);
	const otherSum = tail.reduce((s, t) => s + value(t), 0);
	if (otherSum > 0) {
		top.push({
			key: '__other__',
			label: 'Other',
			value: otherSum,
			color: DEVICE_SCAN_OTHER_COLOR,
			isOther: true,
			otherCount: tail.length,
			drilldown: undefined
		});
	}
	return top;
}

export function deviceScanBucketsToDonut(buckets: DeviceScanTopBucket[]): DonutDatum[] {
	return buckets.map((b) => ({ label: b.label, value: b.value, color: b.color }));
}
