<script lang="ts">
	import { m } from '$lib/i18n';
	import type { RedTeamResult } from '$lib/services/provenance';
	import SurfaceBadge from './SurfaceBadge.svelte';
	import { Check, X } from '@lucide/svelte';

	interface Props {
		result: RedTeamResult;
	}

	let { result }: Props = $props();

	let blocked = $derived(result.scenarios.filter((s) => s.blocked).length);
	const columns = ['observed', 'detected', 'blocked', 'remediated'] as const;
	const columnLabels = {
		observed: () => m.supervision_redteam_col_observed(),
		detected: () => m.supervision_redteam_col_detected(),
		blocked: () => m.supervision_redteam_col_blocked(),
		remediated: () => m.supervision_redteam_col_remediated()
	};
</script>

<section class="paper gap-3" aria-label={m.supervision_redteam_title()}>
	<div class="flex flex-wrap items-baseline justify-between gap-2">
		<div class="flex flex-col">
			<h3 class="text-base font-semibold">{m.supervision_redteam_title()}</h3>
			<p class="text-muted-content text-xs">{m.supervision_redteam_description()}</p>
		</div>
		{#if result.scenarios.length}
			<span class="text-sm font-medium">
				{m.supervision_redteam_score({ blocked, total: result.scenarios.length })}
			</span>
		{/if}
	</div>

	{#if result.scenarios.length === 0}
		<p class="text-muted-content text-sm">{m.supervision_redteam_empty()}</p>
	{:else}
		<div class="overflow-x-auto">
			<table class="table table-sm">
				<thead>
					<tr>
						<th>{m.supervision_redteam_col_scenario()}</th>
						<th>{m.supervision_redteam_col_surface()}</th>
						{#each columns as c (c)}
							<th class="text-center">{columnLabels[c]()}</th>
						{/each}
						<th>{m.supervision_redteam_col_notes()}</th>
					</tr>
				</thead>
				<tbody>
					{#each result.scenarios as s, i (`${s.scenario}-${i}`)}
						<tr>
							<td class="font-medium">{s.scenario}</td>
							<td><SurfaceBadge surface={s.surface} /></td>
							{#each columns as c (c)}
								<td class="text-center">
									{#if s[c]}
										<Check class="text-success inline size-4" aria-label="yes" />
									{:else}
										<X class="text-error inline size-4" aria-label="no" />
									{/if}
								</td>
							{/each}
							<td class="text-muted-content text-xs">{s.notes ?? ''}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</section>
