<script lang="ts">
	import { m } from '$lib/i18n';
	import {
		HEADER_VISIBILITIES,
		setHeaderVisibility,
		type DashboardConfig,
		type HeaderRow,
		type HeaderVisibility
	} from './dashboardLayout';
	import { twMerge } from 'tailwind-merge';

	type Props = {
		row: HeaderRow;
		config: DashboardConfig;
		onChange: (config: DashboardConfig) => void;
		scope: string;
		class?: string;
	};

	let { row, config, onChange, scope, class: className }: Props = $props();
	const name = $derived(row.text || m.dashboard_header());

	function visibilityLabel(visibility: HeaderVisibility) {
		switch (visibility) {
			case 'device-scans':
				return m.dashboard_header_only_with_scans();
			case 'mcp-stats':
				return m.dashboard_header_only_with_mcp_stats();
			case 'llm-stats':
				return m.dashboard_header_only_with_llm_stats();
			default:
				return m.dashboard_header_always_show();
		}
	}
</script>

<div class={twMerge('flex flex-col gap-1', className)} role="radiogroup" aria-label={name}>
	{#each HEADER_VISIBILITIES as visibility (visibility)}
		<label class="flex items-center gap-2 px-1 py-1 text-xs">
			<input
				type="radio"
				class="radio radio-xs radio-primary"
				name="{scope}-{row.id}"
				value={visibility}
				checked={row.visibility === visibility}
				aria-label="{visibilityLabel(visibility)}: {name}"
				onchange={() => onChange(setHeaderVisibility(config, row.id, visibility))}
			/>
			{visibilityLabel(visibility)}
		</label>
	{/each}
</div>
