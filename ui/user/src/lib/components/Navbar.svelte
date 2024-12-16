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

	function hasOptionalTools() {
		for (const tool of $tools.items) {
			if (!tool.builtin) {
				return true;
			}
		}
		return false;
	}

	function hasTool(tool: string) {
		for (const t of $tools.items) {
			if (t.id === tool) {
				return t.enabled || t.builtin;
			}
		}
		return false;
	}
</script>

<nav
	class="fixed z-30
w-full
via-80%"
>
	<div class="bg-white px-3 py-3 dark:bg-black lg:px-5 lg:pl-3">
		<div class="flex items-center justify-between">
			<Logo />
			<div class="flex items-center gap-1 pr-2">
				{#if hasTool('tasks')}
					<Tasks />
				{/if}
				{#if hasTool('database')}
					<Tables />
				{/if}
				{#if hasTool('knowledge')}
					<KnowledgeFile />
				{/if}
				{#if hasTool('workspace-files')}
					<Files />
				{/if}
				{#if hasOptionalTools()}
					<Tools />
				{/if}
				<DarkModeToggle />
				<Profile />
			</div>
		</div>
	</div>
</nav>
