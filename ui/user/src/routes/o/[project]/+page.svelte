<script lang="ts">
	import { profile } from '$lib/stores';
	import EditMode from '$lib/components/EditMode.svelte';
	import { type PageProps } from './$types';
	import { initLayout } from '$lib/context/layout.svelte';
	import Obot from '$lib/components/Obot.svelte';

	let { data }: PageProps = $props();
	let project = $state(data.project);
	let tools = $state(data.tools);
	let title = $derived(project?.name || 'Obot');

	initLayout({});

	$effect(() => {
		// This happens on page transitions
		if (data.project.id !== project.id) {
			project = data.project;
			tools = data.tools;
		}
	});

	$effect(() => {
		if (profile.current.unauthorized) {
			// Redirect to the main page to log in.
			window.location.href = `/?rd=${window.location.pathname}`;
		}
	});
</script>

<svelte:head>
	{#if title}
		<title>{title}</title>
	{/if}
</svelte:head>

<div class="h-svh">
	{#key project.id}
		{#if project.editor}
			<EditMode bind:project bind:tools />
		{:else}
			<Obot {project} {tools} />
			<p>Project not found.</p>
		{/if}
	{/key}
</div>
