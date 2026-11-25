<script lang="ts">
	import { m } from '$lib/i18n';
	import { resolve } from '$lib/url';

	interface Props {
		agentId: string;
		agentName: string;
		userName: string;
	}

	let { agentId, agentName, userName }: Props = $props();

	// The message decides the order of the two names, so it is split around placeholders and each name is linked.
	const AGENT = '@@agent@@';
	const USER = '@@user@@';
	const parts = $derived(
		m
			.identity_access_agent_for_user({ agent: AGENT, user: USER })
			.split(/(@@agent@@|@@user@@)/)
			.filter(Boolean)
	);
</script>

<!-- "<agent> for <user>": an agent that acted on behalf of a user, linking to both. A click on a link must not open the row. -->
<span class="truncate">
	{#each parts as part, i (i)}
		{#if part === AGENT}
			<a
				class="link"
				href={resolve(`/identity-access/agent-identities/${agentId}`)}
				onclick={(e) => e.stopPropagation()}>{agentName}</a
			>
		{:else if part === USER}
			<a
				class="link"
				href={resolve(`/identity-access?view=users&query=${encodeURIComponent(userName)}`)}
				onclick={(e) => e.stopPropagation()}>{userName}</a
			>
		{:else}
			{part}
		{/if}
	{/each}
</span>
