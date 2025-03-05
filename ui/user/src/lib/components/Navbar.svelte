<script lang="ts">
	import DarkModeToggle from '$lib/components/navbar/DarkModeToggle.svelte';
	import Profile from '$lib/components/navbar/Profile.svelte';
	import Files from '$lib/components/edit/Files.svelte';
	import KnowledgeFiles from '$lib/components/navbar/KnowledgeFiles.svelte';
	import Tasks from '$lib/components/navbar/Tasks.svelte';
	import Tables from '$lib/components/navbar/Tables.svelte';
	import Term from '$lib/components/navbar/Term.svelte';
	import Projects from '$lib/components/navbar/Projects.svelte';
	import Clone from '$lib/components/navbar/Clone.svelte';
	import Threads from '$lib/components/navbar/Threads.svelte';
	import type { AssistantTool, Project, Version } from '$lib/services';
	import { hasTool } from '$lib/tools';
	import Tools from '$lib/components/navbar/Tools.svelte';
	import { fade } from 'svelte/transition';
	import type { EditorItem } from '$lib/services/editor/index.svelte';

	interface Props {
		project: Project;
		items: EditorItem[];
		tools: AssistantTool[];
		version: Version;
		currentThreadID?: string;
	}

	let { project, tools, currentThreadID = $bindable(), version, items }: Props = $props();
</script>

<nav class="w-full via-80%" in:fade|global>
	<div class="bg-white p-3 dark:bg-black">
		<div class="flex items-center justify-between">
			{#if hasTool(tools, 'threads')}
				<Threads bind:currentThreadID {project} />
			{/if}
			<Projects {project} />
			<div class="grow"></div>
			{#if hasTool(tools, 'tasks')}
				<Tasks {project} {items} />
			{/if}
			{#if hasTool(tools, 'database')}
				<Tables {project} {items} />
			{/if}
			{#if hasTool(tools, 'knowledge') && currentThreadID}
				<KnowledgeFiles {project} {currentThreadID} />
			{/if}
			{#if hasTool(tools, 'workspace-files') && currentThreadID}
				<Files {project} thread {currentThreadID} {items} />
			{/if}
			{#if hasTool(tools, 'shell')}
				<Term />
			{/if}
			{#if tools.length > 0}
				<Tools {project} {version} {items} {tools} />
			{/if}
			{#if !project.editor}
				<Clone {project} />
			{/if}
			<DarkModeToggle />
			<Profile {project} {tools} />
		</div>
	</div>
</nav>
