<script lang="ts">
	import { profile } from '$lib/stores';
	import { type PageProps } from './$types';
	import { goto } from '$app/navigation';
	import { ChatService } from '$lib/services';
	import { onMount } from 'svelte';

	let { data }: PageProps = $props();

	onMount(async () => {
		// check if url has ?create in it
		const urlParams = new URLSearchParams(window.location.search);
		const project = await ChatService.createProjectFromShare(data.id, {
			create: urlParams.has('create')
		});

		if (profile.current.unauthorized) {
			// Redirect to the main page to log in.
			window.location.href = `/?rd=${window.location.pathname}`;
		} else if (project?.id) {
			goto(`/o/${project.id}`, { replaceState: true });
		}
	});
</script>

<svelte:head>
	<title>Obot</title>
</svelte:head>
