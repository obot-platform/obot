<script lang="ts">
	import { darkMode } from '$lib/stores';
	import { initLayout } from '$lib/context/nanobotLayout.svelte';

	let { children } = $props();

	// Initialize layout context for all nanobot child routes
	initLayout();
</script>

<div class="nanobot" data-theme={darkMode.isDark ? 'nanobotdark' : 'nanobotlight'}>
	{@render children()}
</div>
