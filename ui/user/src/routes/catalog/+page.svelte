<script lang="ts">
	import { darkMode, errors } from '$lib/stores';
	import Profile from '$lib/components/navbar/Profile.svelte';
	import { responsive } from '$lib/stores';
	import Notifications from '$lib/components/Notifications.svelte';
	import type { PageProps } from './$types';
	import { q, qIsSet } from '$lib/url';
	import { ChevronLeft } from 'lucide-svelte';
	import { sortByPreferredMcpOrder } from '$lib/sort';
	import McpCatalog from '$lib/components/mcp/McpCatalog.svelte';
	import { EditorService, type MCP } from '$lib/services';
	import { goto } from '$app/navigation';

	let { data }: PageProps = $props();
	const mcps = $derived(data.mcps.sort(sortByPreferredMcpOrder));

	async function createNew(mcp: MCP) {
		try {
			const project = await EditorService.createObot();
			await goto(`/o/${project.id}?mcp=${mcp.id}`);
		} catch (error) {
			errors.append((error as Error).message);
		}
	}
</script>

<div class="flex h-full flex-col items-center">
	<div class="flex h-16 w-full items-center p-4 md:p-5">
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
		<div class="grow"></div>
		<div class="flex items-center gap-1">
			{#if !responsive.isMobile}
				<a href="https://docs.obot.ai" rel="external" target="_blank" class="icon-button">Docs</a>
				<a href="https://discord.gg/9sSf4UyAMC" rel="external" target="_blank" class="icon-button">
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
					class="icon-button"
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

	<main class="colors-background relative flex w-full flex-col items-center justify-center pb-12">
		{#if qIsSet('from')}
			{@const from = decodeURIComponent(q('from'))}
			<div class="mt-8 flex w-full max-w-(--breakpoint-2xl) flex-col justify-start">
				<a
					href={from}
					class="button-text flex w-fit items-center gap-1 pb-0 text-base font-semibold text-black md:text-lg dark:text-white"
				>
					<ChevronLeft class="size-5" /> Back To Chat
				</a>
			</div>
		{/if}

		<div
			class="flex w-full max-w-(--breakpoint-xl) flex-col items-center justify-center gap-2 px-4 py-4"
		>
			{#if qIsSet('new')}
				<h2 class="text-3xl font-semibold md:text-4xl">Welcome To Obot</h2>
			{:else}
				<h2 class="text-3xl font-semibold md:text-4xl">MCP Servers</h2>
			{/if}
			<p class="mb-8 max-w-full text-center text-base font-light md:max-w-md">
				Browse over evergrowing catalog of MCP servers and find the perfect one to set up your Obot
				with.
			</p>
		</div>

		<McpCatalog {mcps} inline onSubmitMcp={createNew} />
	</main>

	<Notifications />
</div>

<svelte:head>
	<title>Obot | Home</title>
</svelte:head>
