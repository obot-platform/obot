<script lang="ts">
	import { darkMode } from '$lib/stores';
	let { children } = $props();
</script>

<div data-theme={darkMode.isDark ? 'nanobotdark' : 'nanobotlight'}>
	{@render children()}
</div>
