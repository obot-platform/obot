<script lang="ts">
	import Profile from '$lib/components/navbar/Profile.svelte';
	import { fade } from 'svelte/transition';
	import type { Snippet } from 'svelte';
	import { darkMode, errors, responsive } from '$lib/stores';
	import { goto } from '$app/navigation';
	import { EditorService } from '$lib/services';

	interface Props {
		leftContent?: Snippet;
	}

	let { leftContent }: Props = $props();

	async function handleChatLink() {
		const lastVisitedObot = localStorage.getItem('lastVisitedObot');
		if (lastVisitedObot) {
			goto(`/o/${lastVisitedObot}`);
		} else {
			try {
				const project = await EditorService.createObot();
				await goto(`/o/${project.id}`);
			} catch (error) {
				errors.append((error as Error).message);
			}
		}
	}
</script>

<nav class="flex h-16 w-full items-center bg-white px-3 dark:bg-black" in:fade|global>
	<div class="flex w-full items-center justify-between">
		{#if leftContent}
			{@render leftContent()}
		{:else}
			<div class="relative flex items-end">
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
			</div>
		{/if}
		<div class="grow"></div>
		<div class="flex items-center gap-4">
			{#if !responsive.isMobile}
				<button onclick={handleChatLink} class="nav-link">Chat</button>
				<a class="nav-link" href="/agents">Agent Catalog</a>
				<a class="nav-link" href="/catalog">MCP Servers</a>
				<a href="https://docs.obot.ai" rel="external" target="_blank" class="nav-link">Docs</a>
				<a href="https://discord.gg/9sSf4UyAMC" rel="external" target="_blank" class="nav-link">
					{#if darkMode.isDark}
						<img src="/user/images/discord-mark/discord-mark-white.svg" alt="Discord" class="h-6" />
					{:else}
						<img src="/user/images/discord-mark/discord-mark.svg" alt="Discord" class="h-6" />
					{/if}
				</a>
				<a
					href="https://github.com/obot-platform/obot"
					rel="external"
					target="_blank"
					class="nav-link"
				>
					{#if darkMode.isDark}
						<img src="/user/images/github-mark/github-mark-white.svg" alt="GitHub" class="h-6" />
					{:else}
						<img src="/user/images/github-mark/github-mark.svg" alt="GitHub" class="h-6" />
					{/if}
				</a>
			{/if}
			<Profile />
		</div>
	</div>
</nav>
