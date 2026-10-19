<script lang="ts">
	import { m } from '$lib/i18n';
	import type { ClaudeResources } from '$lib/services';

	let {
		resources = $bindable({}),
		inherit = false
	}: { resources: ClaudeResources; inherit?: boolean } = $props();
</script>

<div class="grid gap-3 sm:grid-cols-2">
	<label class="flex flex-col gap-1 text-sm"
		>{m.hosted_agents_actor_cpu_limit()}<input
			class="text-input-filled"
			bind:value={resources.actorCPU}
			placeholder={inherit ? m.hosted_agents_inherit_environment() : '1'}
			required={!inherit}
		/></label
	>
	<label class="flex flex-col gap-1 text-sm"
		>{m.hosted_agents_actor_memory_limit()}<input
			class="text-input-filled"
			bind:value={resources.actorMemory}
			placeholder={inherit ? m.hosted_agents_inherit_environment() : '2Gi'}
			required={!inherit}
		/></label
	>
	<label class="flex flex-col gap-1 text-sm"
		>{m.hosted_agents_idle_timeout_minutes()}<input
			class="text-input-filled"
			type="number"
			min="1"
			max="10080"
			bind:value={resources.idleTimeoutMinutes}
			placeholder={inherit ? m.hosted_agents_inherit_environment() : '5'}
			required={!inherit}
		/></label
	>
	<label class="flex flex-col gap-1 text-sm"
		>{m.hosted_agents_session_retention_days()}<input
			class="text-input-filled"
			type="number"
			min="-1"
			max="36500"
			bind:value={resources.sessionRetentionDays}
			placeholder={inherit ? m.hosted_agents_inherit_environment() : '30'}
			required={!inherit}
		/></label
	>
</div>
<p class="text-xs text-muted-content">
	{m.hosted_agents_actor_resources_help()}
</p>
