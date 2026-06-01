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
	key: 'single' | 'multi' | 'local' | 'remote' | 'composite';
	label: string;
	baseColor: string;
}[] = [
	{ key: 'single', label: 'Hosted (Single-tenant)', baseColor: '#fee090' },
	{ key: 'multi', label: 'Hosted (Multi-tenant)', baseColor: '#f46d43' },
	{ key: 'remote', label: 'Remote', baseColor: '#4575b4' },
	{ key: 'composite', label: 'Composite', baseColor: '#BFB4ACFF' }
];

export const entryTypeDonutLegend: DonutLegendItem[] = ENTRY_TYPE_GRAPH_META.map(
	({ label, baseColor }) => ({ label, color: baseColor })
);
