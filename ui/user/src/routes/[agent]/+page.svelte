<script lang="ts">
	import { profile } from '$lib/stores';
	import { type PageProps } from './$types';
	import { initLayout } from '$lib/context/chatLayout.svelte';
	import { goto } from '$app/navigation';

	let { data }: PageProps = $props();
	let project = $state(data.project);
	let title = $derived(project?.name || 'Obot');

	initLayout({
		items: []
	});

	$effect(() => {
		if (profile.current.unauthorized) {
			// Redirect to the main page to log in.
			window.location.href = `/?rd=${window.location.pathname}`;
		} else if (project?.id) {
			goto(`/o/${project.id}`, { replaceState: true });
		}
	});
</script>

<svelte:head>
	{#if title}
		<title>{title}</title>
	{/if}
</svelte:head>
