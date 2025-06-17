<script lang="ts">
	import { Origami, Scroll } from 'lucide-svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { ChatService, EditorService } from '$lib/services';
	import { goto } from '$app/navigation';

	async function handleChat() {
		const projects = await ChatService.listProjects();
		const lastVisitedObot = localStorage.getItem('lastVisitedObot');
		const lastProject =
			(lastVisitedObot && projects.items.find((p) => p.id === lastVisitedObot)) ??
			projects.items[projects.items.length - 1];
		if (lastProject) {
			goto(`/o/${lastProject.id}`);
		} else {
			const newProject = await EditorService.createObot();
			goto(`/o/${newProject.id}`);
		}
	}
</script>

<Layout>
	<div
		class="dark:border-surface3 dark:bg-surface1 w-full max-w-(--breakpoint-xl) rounded-md bg-white px-8 py-12 text-center shadow-sm dark:border"
	>
		<h1 class="mb-2 text-2xl font-bold md:text-3xl">Welcome To Obot!</h1>
		<p class="font-md mb-8 text-gray-500">
			It looks like it's your first time here. Let's get started! What would you like to do?
		</p>

		<div class="grid grid-cols-1 gap-8 md:grid-cols-2 lg:grid-cols-2">
			<button class="flex flex-col justify-start gap-2 text-left" onclick={handleChat}>
				<img
					src="/agent/images/create-a-chat.webp"
					alt="Create a template"
					class="aspect-video rounded-md"
				/>
				<p class="flex items-center gap-1 text-base font-semibold">
					<Scroll class="size-4" /> Start Chatting
				</p>
				<span class="text-sm text-gray-500">
					Utilize existing extensions with AI to meet your needs.
				</span>
			</button>

			<a href="/mcp-servers" class="flex flex-col justify-start gap-2 text-left">
				<img
					src="/agent/images/create-from-mcp.webp"
					alt="Create a template"
					class="aspect-video rounded-md"
				/>
				<p class="flex items-center gap-1 text-base font-semibold">
					<Origami class="size-4" /> Discover & Connect MCP Servers
				</p>
				<span class="text-sm text-gray-500">
					I have an existing client I want to connect one of our MCP servers to.
				</span>
			</a>
		</div>
	</div>
</Layout>

<svelte:head>
	<title>Obot | Home</title>
</svelte:head>
