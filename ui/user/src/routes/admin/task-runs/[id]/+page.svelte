<script lang="ts">
	import { browser } from '$app/environment';
	import BackLink from '$lib/components/BackLink.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Task from '$lib/components/tasks/Task.svelte';
	import { initLayout } from '$lib/context/chatLayout.svelte';
	import { initProjectTools } from '$lib/context/projectTools.svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();

	initLayout({
		sidebarOpen: false,
		fileEditorOpen: false,
		editTaskID: data.task?.id,
		items: []
	});

	initProjectTools({
		tools: [],
		maxTools: 5
	});
</script>

<Layout whiteBackground={true}>
	<div
		class="h-dvh w-full"
		in:fly={{ x: 100, duration: 300, delay: 150 }}
		out:fly={{ x: -100, duration: 300 }}
	>
		<div class="flex h-full flex-col">
			<div class="my-6">
				{#if data.taskRun}
					{@const currentLabel = data.taskRun?.name || 'Unnamed Task Run'}
					<BackLink fromURL="task-runs" {currentLabel} />
				{/if}
			</div>
			<div class="flex w-full grow justify-center">
				{#if data.taskRun && data.task && data.project && browser}
					<Task project={data.project} task={data.task} runID={data.taskRun.taskRunID} readonly />
				{/if}
			</div>
		</div>
	</div>
</Layout>
