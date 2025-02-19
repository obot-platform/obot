<script lang="ts">
	import Logo from '$lib/components/navbar/Logo.svelte';
	import DarkModeToggle from '$lib/components/navbar/DarkModeToggle.svelte';
	import Profile from '$lib/components/navbar/Profile.svelte';
	import Tools from '$lib/components/navbar/Tools.svelte';
	import Files from '$lib/components/navbar/Files.svelte';
	import KnowledgeFile from '$lib/components/navbar/KnowledgeFiles.svelte';
	import Tasks from '$lib/components/navbar/Tasks.svelte';
	import Tables from '$lib/components/navbar/Tables.svelte';
	import { tools } from '$lib/stores';
	import Term from '$lib/components/navbar/Term.svelte';
	import Projects from '$lib/components/navbar/Projects.svelte';
	import New from '$lib/components/New.svelte';
	import Settings from '$lib/components/navbar/Settings.svelte';
	import { context } from '$lib/stores';
	import Threads from '$lib/components/navbar/Threads.svelte';

	function hasOptionalTools() {
		for (const tool of tools.items) {
			if (!tool.builtin) {
				return true;
			}
		}
		return false;
	}
</script>

<New />

<nav class="w-full via-80%">
	<div class="bg-white p-3 dark:bg-black">
		<div class="flex items-center justify-between">
			{#if tools.hasTool('projects')}
				<Projects />
			{:else}
				<Logo />
			{/if}
			{#if tools.hasTool('threads')}
				<Threads />
			{/if}
			<div class="grow"></div>
			{#if tools.hasTool('tasks')}
				<Tasks />
			{/if}
			{#if tools.hasTool('database')}
				<Tables />
			{/if}
			{#if !tools.hasTool('projects') && tools.hasTool('knowledge')}
				<KnowledgeFile />
			{/if}
			{#if tools.hasTool('workspace-files')}
				<Files />
			{/if}
			{#if tools.hasTool('shell')}
				<Term />
			{/if}
			{#if !tools.hasTool('projects') && hasOptionalTools()}
				<Tools />
			{/if}
			{#if !context.project?.locked && tools.hasTool('projects')}
				<Settings />
			{/if}
			<DarkModeToggle />
			<Profile />
		</div>
	</div>
</nav>
