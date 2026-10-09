<script lang="ts">
	import TabLayout from '$lib/components/TabLayout.svelte';
	import ClaudeConnection from '$lib/components/claude/ClaudeConnection.svelte';
	import ClaudeSessions from '$lib/components/claude/ClaudeSessions.svelte';
	import { m } from '$lib/i18n';
	import { profile, version } from '$lib/stores';
</script>

<svelte:head><title>Obot | {m.nav_claude_environments()}</title></svelte:head>

<TabLayout
	title={m.nav_claude_environments()}
	defaultView="environments"
	views={[
		{ label: m.nav_claude_environments(), value: 'environments', content: environments },
		...(version.current.engine === 'docker' || !version.current.claudeEnvironmentsEnabled
			? []
			: [{ label: m.hosted_agents_my_claude_sessions(), value: 'sessions', content: sessions }]),
		...(version.current.engine !== 'docker' &&
		version.current.claudeEnvironmentsEnabled &&
		profile.current.isAdmin?.()
			? [
					{
						label: m.hosted_agents_all_claude_sessions(),
						value: 'all-sessions',
						content: allSessions
					}
				]
			: [])
	]}
/>

{#snippet environments()}
	<div class="w-full max-w-4xl">
		{#if version.current.engine === 'docker'}
			<p class="text-sm text-muted-content">{m.hosted_agents_kubernetes_required()}</p>
		{:else}
			<ClaudeConnection />
		{/if}
	</div>
{/snippet}

{#snippet sessions()}
	<ClaudeSessions />
{/snippet}

{#snippet allSessions()}
	<ClaudeSessions all />
{/snippet}
