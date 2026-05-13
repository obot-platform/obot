import type { DonutLegendItem } from '$lib/components/graph/DonutGraph.svelte';

export const DEPLOYMENT_STATUS_ORDER = [
	'Available',
	'Progressing',
	'Unavailable',
	'Needs Attention',
	'Shutdown',
	'Unknown'
] as const;

export const ENTRY_TYPE_GRAPH_META: {
	key: 'multi' | 'single' | 'remote' | 'composite';
	label: string;
	baseColor: string;
}[] = [
	{ key: 'multi', label: 'Multi-User', baseColor: '#fee090' },
	{ key: 'single', label: 'Single-User', baseColor: '#f46d43' },
	{ key: 'remote', label: 'Remote', baseColor: '#4575b4' },
	{ key: 'composite', label: 'Composite', baseColor: '#BFB4ACFF' }
];

export const entryTypeDonutLegend: DonutLegendItem[] = ENTRY_TYPE_GRAPH_META.map(
	({ label, baseColor }) => ({ label, color: baseColor })
);
