<script lang="ts">
	import ProfileIcon from '$lib/components/profile/ProfileIcon.svelte';
	import { profile } from '$lib/stores';
	import Menu from '$lib/components/navbar/Menu.svelte';
	import Credentials from '$lib/components/navbar/Credentials.svelte';
	import type { AssistantTool, Project } from '$lib/services';

	interface Props {
		project?: Project;
		tools?: AssistantTool[];
	}

	let { project, tools }: Props = $props();
	let credentials = $state<ReturnType<typeof Credentials>>();
</script>

<Menu title={profile.current.getDisplayName?.() || 'Anonymous'}>
	{#snippet icon()}
		<ProfileIcon />
	{/snippet}
	{#snippet body()}
		<div class="flex flex-col gap-2 py-2">
			{#if project && tools}
				<button class="text-start" onclick={() => credentials?.show()}>Credentials</button>
			{/if}
			{#if profile.current.role === 1}
				<a href="/admin/" rel="external" role="menuitem" class="text-red-400">Admin</a>
			{/if}
			{#if profile.current.email}
				<a href="/oauth2/sign_out?rd=/" rel="external" role="menuitem">Sign out</a>
			{/if}
		</div>
	{/snippet}
</Menu>

{#if project && tools}
	<Credentials bind:this={credentials} {project} {tools} />
{/if}
