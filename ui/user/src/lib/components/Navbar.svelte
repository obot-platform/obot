<script lang="ts">
	import Profile from '$lib/components/navbar/Profile.svelte';
	import { fade } from 'svelte/transition';
	import Logo from '$lib/components/navbar/Logo.svelte';
	import type { Snippet } from 'svelte';

	interface Props {
		children?: Snippet;
	}

	let { children }: Props = $props();
</script>

<nav class="w-full via-80%" in:fade|global>
	<div class="bg-white p-3 dark:bg-black">
		<div class="flex items-center justify-between">
			{#if children}
				{@render children()}
			{:else}
				<Logo />
			{/if}
			<div class="grow"></div>
			<Profile />
		</div>
	</div>
</nav>
