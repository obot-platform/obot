<script lang="ts">
	import DarkModeToggle from '$lib/components/navbar/DarkModeToggle.svelte';
	import Profile from '$lib/components/navbar/Profile.svelte';
	import Projects from '$lib/components/navbar/Projects.svelte';
	import type { Project } from '$lib/services';
	import { fade } from 'svelte/transition';
	import Logo from '$lib/components/navbar/Logo.svelte';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
</script>

<nav class="w-full via-80%" in:fade|global>
	<div class="bg-white p-3 dark:bg-black">
		<div class="flex items-center justify-between">
			<Logo />
			<Projects {project} />
			<div class="grow"></div>
			<DarkModeToggle />
			<Profile />
		</div>
	</div>
</nav>
