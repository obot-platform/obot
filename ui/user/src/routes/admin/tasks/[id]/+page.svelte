<script lang="ts">
	import { browser } from '$app/environment';
	import BackLink from '$lib/components/BackLink.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Task from '$lib/components/tasks/Task.svelte';
	import { initLayout } from '$lib/context/chatLayout.svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();

	initLayout({
		sidebarOpen: false,
		fileEditorOpen: false,
		editTaskID: data.task?.id,
		items: []
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
				{#if data.task}
					{@const currentLabel = data.task.name || 'Unnamed Task'}
					<BackLink fromURL="tasks" {currentLabel} />
				{/if}
			</div>
			<div class="flex w-full grow justify-center">
				{#if data.task && data.project && browser}
					<Task project={data.project} bind:task={data.task} readonly skipFetchOnMount />
				{/if}
			</div>
		</div>
	</div>
</Layout>
