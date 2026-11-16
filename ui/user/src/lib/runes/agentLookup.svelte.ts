import { AgentIdentitiesService } from '$lib/services';
import type { AgentIdentity, AgentLookup } from '$lib/services/agent-identities';
import { agentLookupWith } from '$lib/subjectResolver';
import { untrack } from 'svelte';

// The names of agents, deleted ones included, loaded once the first time `wanted` is true, such as
// when a list or an editor first shows an agent. Listing agents is for admins, read-only admins and
// auditors, so for anyone else the lookup is empty and agents are shown by their ID. Call it while
// a component initializes.
export function createAgentLookup(wanted: () => boolean) {
	let loaded = $state<Map<string, AgentLookup>>();
	// Agents just picked in an editor, which the loaded lookup may not hold yet.
	let picked = $state<AgentIdentity[]>([]);
	const current = $derived(picked.length > 0 ? agentLookupWith(loaded, picked) : loaded);

	$effect(() => {
		if (!wanted() || untrack(() => loaded)) return;
		const controller = new AbortController();
		// Deleted agents are listed too, so that history keeps their names.
		AgentIdentitiesService.listAgents({
			includeDeleted: true,
			dontLogErrors: true,
			signal: controller.signal
		})
			// Anyone but admins, read-only admins and auditors is refused, and sees agents by their IDs.
			.catch((): AgentIdentity[] => [])
			.then((agents) => {
				const lookup = agentLookupWith(undefined, agents);
				if (!controller.signal.aborted) loaded = lookup;
			});
		return () => controller.abort();
	});

	return {
		get current() {
			return current;
		},
		add(agents: AgentIdentity[]) {
			if (agents.length > 0) picked = [...picked, ...agents];
		}
	};
}
