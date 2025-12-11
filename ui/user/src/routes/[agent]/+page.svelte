<script lang="ts">
	import { profile } from '$lib/stores';
	import { type PageProps } from './$types';
	import { initLayout } from '$lib/context/chatLayout.svelte';
	import { goto } from '$lib/url';

	let { data }: PageProps = $props();
	let title = $derived(data.project?.name || 'Obot');

	initLayout({
		items: []
	});

	$effect(() => {
		if (profile.current.unauthorized) {
			// Redirect to the main page to log in.
			window.location.href = `/?rd=${window.location.pathname}`;
		} else if (data.project?.id) {
			goto(`/o/${data.project.id}`, { replaceState: true });
		}
	});
</script>

<svelte:head>
	{#if title}
		<title>{title}</title>
	{/if}
</svelte:head>
