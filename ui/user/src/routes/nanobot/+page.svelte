<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import * as nanobotLayout from '$lib/context/nanobotLayout.svelte';
	import ProjectSidebar from './ProjectSidebar.svelte';
	import { MessageCircle, Sparkles } from 'lucide-svelte';
	import { goto } from '$app/navigation';
	import { ChatAPI, ChatService } from '$lib/services/nanobot/chat/index.svelte';
	import { untrack } from 'svelte';
	import ProjectStartThread from '$lib/components/nanobot/ProjectStartThread.svelte';

	let { data } = $props();
	let projects = $derived(data.projects);
	let chat = $state<ChatService | null>(null);
	// let projects = $derived(data.projects);
	let sidebarRef: { refreshThreads: () => Promise<void> } | undefined = $state();

	const layout = nanobotLayout.getLayout();
	layout.sidebarOpen = false;
	const chatApi = $derived(new ChatAPI(data.agent.connectURL));

	function handleThreadCreated() {
		console.log('handleThreadCreated');
		sidebarRef?.refreshThreads();
		layout.sidebarOpen = true;
	}

	$effect(() => {
		const newChat = new ChatService({
			api: chatApi,
			onThreadCreated: handleThreadCreated
		});

		untrack(() => {
			chat = newChat;
		});

		return () => {
			newChat.close();
		};
	});
</script>

<Layout
	title=""
	layoutContext={nanobotLayout}
	classes={{
		container: 'px-0 py-0 md:px-0',
		childrenContainer: 'max-w-full h-[calc(100dvh-4rem)]',
		collapsedSidebarHeaderContent: 'pb-0'
	}}
	whiteBackground
>
	{#snippet overrideSidebarContent()}
		<ProjectSidebar {chatApi} projectId={projects[0].id} bind:this={sidebarRef} />
	{/snippet}

	<div class="flex w-full grow">
		{#if chat}
			{#key chat.chatId}
				<ProjectStartThread
					{chat}
					onToggleSidebar={(open: boolean) => {
						layout.sidebarOpen = open;
					}}
				>
					{#snippet emptyStateContent()}
						<div class="flex flex-col items-center gap-4">
							<div class="flex flex-col items-center gap-1">
								<h1 class="w-xs text-center text-3xl font-semibold md:w-full">
									What would you like to work on?
								</h1>
								<p class="text-base-content/50 text-md text-center font-light">
									Choose an entry point or pick up where you left off.
								</p>
							</div>
							<div class="grid grid-cols-1 items-stretch gap-4 md:grid-cols-2">
								<button
									class="bg-base-200 dark:border-base-300 rounded-field col-span-1 h-full p-4 text-left shadow-sm"
									onclick={() => {
										goto('/nanobot/p?planner=true');
									}}
								>
									<Sparkles class="mb-4 size-5" />
									<h3 class="text-lg font-semibold">Create a workflow</h3>
									<p class="text-base-content/50 text-sm font-light">
										Design an AI agent workflow through conversation
									</p>
								</button>
								<button
									class="bg-base-200 dark:border-base-300 rounded-field col-span-1 h-full p-4 text-left shadow-sm"
									onclick={() => {
										goto('/nanobot/p');
									}}
								>
									<MessageCircle class="mb-4 size-5" />
									<h3 class="text-lg font-semibold">Just explore</h3>
									<p class="text-base-content/50 min-h-[2lh] text-sm font-light">
										Chat and see where it goes
									</p>
								</button>
							</div>
							<!-- 
					<div class="w-full">
						<ul class="list">
							<li class="flex items-center justify-between p-4 pb-2 text-sm tracking-wide">
								<span class="text-base-content/50">Most recently created projects</span>
								<button class="btn btn-primary btn-sm">
									<Plus class="size-4" /> New Project
								</button>
							</li>
							{#if projects.length > 0}
								{#each projects as project, index}
									<li class="list-row">
										<div class="text-4xl font-thin tabular-nums opacity-30">
											{String(index + 1).padStart(2, '0')}
										</div>
										<div class="py-1">
											<FolderRoot class="size-8" />
										</div>
										<div class="list-col-grow">
											<div>{project.displayName}</div>
											<div class="text-xs font-semibold uppercase opacity-60">
												{formatTimeAgo(project.created).relativeTime}
											</div>
										</div>
										<div>
											<div class="dropdown dropdown-left">
												<div tabindex="0" role="button" class="btn btn-square btn-ghost m-1">
													<MoreVerticalIcon class="size-6" />
												</div>
												<ul
													tabindex="-1"
													class="dropdown-content menu bg-base-100 rounded-box z-1 w-52 p-2 shadow-sm"
												>
													<li>
														<button class="text-sm">
															<Edit class="size-4" />
															Rename
														</button>
													</li>
													<li>
														<button class="text-sm">
															<Trash2 class="size-4" />
															Delete
														</button>
													</li>
												</ul>
											</div>
											<button class="btn btn-square btn-ghost">
												<Play class="size-6" />
											</button>
										</div>
									</li>
								{/each}
							{:else}
								<li class="flex items-center justify-between p-4 pb-2 text-xs tracking-wide">
									<span class="text-base-content/50 font-light"
										>No projects yet. Create one to get started.</span
									>
								</li>
							{/if}
						</ul>
					</div> -->
						</div>
					{/snippet}
				</ProjectStartThread>
			{/key}
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Nanobot</title>
</svelte:head>
