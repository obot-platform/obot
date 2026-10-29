<script lang="ts">
	import { m } from '$lib/i18n';
	import {
		CAPABILITY_LABELS,
		CONTENT_LABELS,
		EFFECTS,
		effectLabel
	} from '$lib/services/provenance';
	import { ChevronDown, ChevronUp } from '@lucide/svelte';

	// The CEL vocabulary a policy condition can use, mirrored from pkg/provenance/policy.go.

	let open = $state(false);

	const variables = [
		{
			name: 'subject',
			description: () => m.supervision_cheat_subject(),
			fields: ['id', 'email', 'groups', 'agent', 'device', 'environment', 'api_key', 'delegation']
		},
		{
			name: 'action',
			description: () => m.supervision_cheat_action(),
			fields: [
				'surface',
				'kind',
				'tool',
				'server',
				'server_id',
				'model',
				'provider',
				'host',
				'method',
				'path',
				'command',
				'file',
				'capabilities',
				'sink.external',
				'sink.destination',
				'sink.writes',
				'sink.exec',
				'source.private',
				'source.untrusted',
				'labels',
				'lineage.hit',
				'lineage.exact',
				'lineage.count',
				'lineage.labels',
				'honeytoken',
				'trifecta',
				'text'
			]
		},
		{
			name: 'session',
			description: () => m.supervision_cheat_session(),
			fields: [
				'id',
				'labels',
				'risk',
				'events',
				'tool_calls',
				'denied',
				'duration_seconds',
				'surfaces',
				'trifecta',
				'warrant.present',
				'warrant.task',
				'warrant.scopes',
				'warrant.drifted',
				'warrant.covers',
				'killed'
			]
		},
		{
			name: 'org',
			description: () => m.supervision_cheat_org(),
			fields: ['approved_destinations', 'trusted_hosts']
		}
	];

	const examples = [
		'action.honeytoken',
		'action.trifecta',
		'"secret" in action.lineage.labels && action.sink.external',
		'"untrusted" in session.labels && ("destructive" in action.capabilities || action.sink.exec)',
		'session.warrant.present && !session.warrant.covers',
		'action.kind == "net.egress" && !host_matches(action.host, org.approved_destinations)'
	];
</script>

<div class="bg-base-200 dark:bg-base-300 rounded-md">
	<button
		type="button"
		class="flex w-full items-center justify-between px-4 py-3 text-sm font-medium"
		aria-expanded={open}
		aria-controls="policy-cheat-sheet"
		onclick={() => (open = !open)}
	>
		{m.supervision_policy_cheatsheet()}
		{#if open}<ChevronUp class="size-4" />{:else}<ChevronDown class="size-4" />{/if}
	</button>
	{#if open}
		<div id="policy-cheat-sheet" class="flex flex-col gap-4 px-4 pb-4 text-xs">
			{#each variables as v (v.name)}
				<div class="flex flex-col gap-1">
					<span
						><code class="text-base-content font-semibold">{v.name}</code> — {v.description()}</span
					>
					<div class="flex flex-wrap gap-1">
						{#each v.fields as f (f)}
							<code class="bg-base-100 dark:bg-base-200 rounded px-1 py-0.5">{v.name}.{f}</code>
						{/each}
					</div>
				</div>
			{/each}

			<div class="flex flex-col gap-1">
				<span class="font-semibold">{m.supervision_cheat_functions()}</span>
				<span>
					<code class="bg-base-100 dark:bg-base-200 rounded px-1 py-0.5"
						>host_matches(host, patterns)</code
					>
					— {m.supervision_cheat_host_matches()}
				</span>
			</div>

			<div class="flex flex-col gap-1">
				<span class="font-semibold">{m.supervision_cheat_effects()}</span>
				<div class="flex flex-wrap gap-1">
					{#each EFFECTS as e (e)}
						<code class="bg-base-100 dark:bg-base-200 rounded px-1 py-0.5" title={effectLabel(e)}
							>{e}</code
						>
					{/each}
				</div>
			</div>

			<div class="flex flex-col gap-1">
				<span class="font-semibold">{m.supervision_cheat_labels()}</span>
				<div class="flex flex-wrap gap-1">
					{#each CONTENT_LABELS as l (l)}
						<code class="bg-base-100 dark:bg-base-200 rounded px-1 py-0.5">"{l}"</code>
					{/each}
				</div>
			</div>

			<div class="flex flex-col gap-1">
				<span class="font-semibold">{m.supervision_cheat_capabilities()}</span>
				<div class="flex flex-wrap gap-1">
					{#each CAPABILITY_LABELS as l (l)}
						<code class="bg-base-100 dark:bg-base-200 rounded px-1 py-0.5">"{l}"</code>
					{/each}
				</div>
			</div>

			<div class="flex flex-col gap-1">
				<span class="font-semibold">{m.supervision_cheat_examples()}</span>
				<ul class="flex flex-col gap-1">
					{#each examples as ex (ex)}
						<li><code class="bg-base-100 dark:bg-base-200 rounded px-1 py-0.5">{ex}</code></li>
					{/each}
				</ul>
			</div>
		</div>
	{/if}
</div>
