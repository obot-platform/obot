<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { darkMode, errors, profile } from '$lib/stores';
	import { initLayout } from '$lib/context/nanobotLayout.svelte';
	import 'devicon/devicon.min.css';
	import { onMount, untrack } from 'svelte';
	import { AdminService, NanobotService } from '$lib/services';
	import { ChatAPI } from '$lib/services/nanobot/chat/index.svelte';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import { get } from 'svelte/store';
	import type { Chat, Resource } from '$lib/services/nanobot/types';

	let { children, data } = $props();
	let projects = $derived(data.projects);
	let agent = $derived(data.agent);
	let isNewAgent = $derived(data.isNewAgent);
	const chatApi = $derived(new ChatAPI(agent.connectURL));

	// Detect impersonation by comparing agent owner to current user.
	const impersonating = $derived(
		!!profile.current?.id && !!agent.userID && agent.userID !== profile.current.id
	);

	let ownerEmail = $state('');
	$effect(() => {
		if (impersonating && agent.userID) {
			AdminService.getUser(agent.userID)
				.then((owner) => {
					ownerEmail = owner.email || owner.username || agent.userID;
				})
				.catch(() => {
					ownerEmail = agent.userID;
				});
		}
	});

	const initialChat = get(nanobotChat);
	let loading = $state(untrack(() => !initialChat || data.isNewAgent));

	const tid = $derived(page.url.searchParams.get('tid'));
	const projectIdFromPath = $derived(page.params.projectId);
	const skipLoadingForStoredThread = $derived(
		!!(
			tid &&
			projectIdFromPath &&
			$nanobotChat?.projectId === projectIdFromPath &&
			$nanobotChat?.sessionId === tid
		)
	);
	const showLoading = $derived(loading && !skipLoadingForStoredThread);

	// Initialize layout context for all nanobot child routes
	initLayout();

	async function initNanobotStore() {
		nanobotChat.set({
			isThreadsLoading: true,
			projectId: projects[0].id,
			sessionId: undefined,
			sessions: [],
			resources: [],
			api: chatApi
		});

		let sessions: Chat[] = [];
		let resources: Resource[] = [];

		try {
			sessions = await chatApi.listSessions();
			resources = await chatApi.listResources();
		} catch (error) {
			console.error(`Error listing sessions or resources`, error);
			errors.append(error);
		} finally {
			nanobotChat.update((data) => {
				if (data) {
					data.sessions = sessions;
					data.resources = resources;
					data.isThreadsLoading = false;
				}
				return data;
			});
		}
	}

	onMount(async () => {
		const storedChat = get(nanobotChat);
		// Re-initialize when there's no stored chat, it's a new agent, or
		// the project changed (e.g. switching between own agent and impersonation).
		const projectChanged = storedChat && storedChat.projectId !== projects[0].id;
		if (!storedChat || isNewAgent || projectChanged) {
			loading = true;
			if (isNewAgent) {
				try {
					await NanobotService.launchProjectV2Agent(projects[0].id, agent.id);
				} catch (error) {
					console.error(error);
					errors.append(error);
				}
			}

			try {
				await initNanobotStore();
			} catch (error) {
				console.error(`Error initializing nanobot store`, error);
			}
			loading = false;
		}
	});
</script>

<div class="nanobot" data-theme={darkMode.isDark ? 'nanobotdark' : 'nanobotlight'}>
	{#if impersonating}
		<div
			class="bg-warning/20 border-warning flex items-center justify-center gap-2 border-b px-4 py-2 text-sm font-medium"
		>
			Impersonating: Viewing another user's agent (owner: {ownerEmail})
			<a href={resolve('/admin/user-impersonation')} class="text-accent underline">Back to list</a>
		</div>
	{/if}
	{#if showLoading}
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
