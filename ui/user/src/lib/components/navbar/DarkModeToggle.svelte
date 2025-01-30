<script>
	import { Moon, Sun } from 'lucide-svelte/icons';
	import { darkMode } from '$lib/stores';
</script>

<button
	type="button"
	onclick={() => {
		darkMode.setDark(!darkMode.isDark);
	}}
	class="icon-button"
>
	{#if darkMode.isDark}
		<Sun class="h-5 w-5" />
	{:else}
		<Moon class="h-5 w-5" />
	{/if}
</button>
