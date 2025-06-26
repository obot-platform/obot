<script lang="ts">
	import Profile from '$lib/components/navbar/Profile.svelte';
	import { fade } from 'svelte/transition';
	import type { Snippet } from 'svelte';
	import { darkMode } from '$lib/stores';
	import { Home } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		leftContent?: Snippet;
		centerContent?: Snippet;
		class?: string;
	}

	let { leftContent, centerContent, class: klass }: Props = $props();
</script>

<nav
	class={twMerge('flex h-16 w-full items-center bg-white px-3 dark:bg-black', klass)}
	in:fade|global
>
	<div class="flex w-full items-center justify-between">
		{#if leftContent}
			{@render leftContent()}
		{:else}
			<a href="/" class="relative flex items-end">
				{#if darkMode.isDark}
					<img src="/user/images/obot-logo-blue-white-text.svg" class="h-12" alt="Obot logo" />
				{:else}
					<img src="/user/images/obot-logo-blue-black-text.svg" class="h-12" alt="Obot logo" />
				{/if}
				<div class="ml-1.5 -translate-y-1">
					<span
						class="rounded-full border-2 border-blue-400 px-1.5 py-[1px] text-[10px] font-bold text-blue-400 dark:border-blue-400 dark:text-blue-400"
					>
						BETA
					</span>
				</div>
			</a>
		{/if}
		<div class="flex grow items-center justify-center">
			{#if centerContent}
				{@render centerContent()}
			{/if}
		</div>
		<div class="flex items-center gap-4">
			<a class="nav-link" href="/" id="navbar-home-link">
				<Home class="size-6" />
			</a>
			<Profile />
		</div>
	</div>
</nav>
