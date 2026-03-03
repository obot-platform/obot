<script lang="ts">
	import { darkMode, errors } from '$lib/stores';
	import { initLayout } from '$lib/context/nanobotLayout.svelte';
	import 'devicon/devicon.min.css';
	import { onMount } from 'svelte';
	import { NanobotService } from '$lib/services';
	import { ChatAPI } from '$lib/services/nanobot/chat/index.svelte';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import { get } from 'svelte/store';

	let { children, data } = $props();
	let loading = $state(true);
	let projects = $derived(data.projects);
	let agent = $derived(data.agent);
	let isNewAgent = $derived(data.isNewAgent);
	const chatApi = $derived(new ChatAPI(agent.connectURL));

	// Initialize layout context for all nanobot child routes
	initLayout();

	async function initNanobotStore() {
		const storedChat = get(nanobotChat);
		if (storedChat && !storedChat.isThreadsLoading) {
			return;
		}

		if (!storedChat) {
			nanobotChat.set({
				isThreadsLoading: true,
				projectId: projects[0].id,
				sessionId: undefined,
				sessions: [],
				resources: [],
				api: chatApi
			});
		}

		const sessions = await chatApi.listSessions();
		const resources = await chatApi.listResources();

		nanobotChat.update((data) => {
			if (data) {
				data.sessions = sessions ?? [];
				data.resources = resources ?? [];
				data.isThreadsLoading = false;
			}
			return data;
		});
	}

	onMount(async () => {
		loading = true;
		if (isNewAgent) {
			try {
				await NanobotService.launchProjectV2Agent(projects[0].id, agent.id);
			} catch (error) {
				console.error(error);
				errors.append(error);
			} finally {
				loading = false;
			}
		}

		try {
			await initNanobotStore();
		} finally {
			loading = false;
		}
	});
</script>

<div class="nanobot" data-theme={darkMode.isDark ? 'nanobotdark' : 'nanobotlight'}>
	{#if loading}
		<div class="h-[100dvh] w-full px-4">
			<div class="absolute top-1/2 left-1/2 w-full -translate-x-1/2 -translate-y-1/2 md:w-4xl">
				<div class="flex flex-col items-center gap-4 px-5 pb-5 md:pb-0">
					<div class="flex w-full flex-col items-center gap-1">
						<div class="h-8 w-xs"></div>
						<p class="text-md skeleton skeleton-text text-center font-light">
							Just a moment, setting up your agent...
						</p>
					</div>
					<div class="flex w-full flex-col items-center justify-center gap-4 md:flex-row">
						<div class="rounded-field skeleton h-[132px] w-full md:w-70"></div>
						<div class="rounded-field skeleton h-[132px] w-full md:w-70"></div>
					</div>

					<div class="skeleton mb-6 h-[124px] w-full"></div>
				</div>
			</div>
		</div>
	{:else}
		{@render children()}
	{/if}
</div>
