<script lang="ts">
	import { type Project } from '$lib/services';
	import { Settings, SidebarClose, Share, CircleFadingArrowUp } from 'lucide-svelte';
	import { getLayout, openSidebarConfig } from '$lib/context/chatLayout.svelte';
	import { ChatService } from '$lib/services';
	import Tasks from '$lib/components/edit/Tasks.svelte';
	import McpServers from '$lib/components/edit/McpServers.svelte';
	import Threads from '$lib/components/chat/sidebar/Threads.svelte';
	import { responsive } from '$lib/stores';
	import { onDestroy } from 'svelte';
	import { scrollFocus } from '$lib/actions/scrollFocus.svelte';
	import Projects from '../navbar/Projects.svelte';
	import BetaLogo from '../navbar/BetaLogo.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';

	interface Props {
		project: Project;
		currentThreadID?: string;
		shared?: boolean;
		onCreateProject?: () => void;
	}

	let {
		project = $bindable(),
		currentThreadID = $bindable(),
		shared,
		onCreateProject
	}: Props = $props();
	const layout = getLayout();

	let copiedProject = $derived(!!project?.sourceProjectID && project.sourceProjectID.trim() !== '');
	let upgradeAvailable = $derived(
		copiedProject && (project.templateUpgradeAvailable || project.templateUpgradeInProgress)
	);
	let projectPollInterval: number;
	const PROJECT_POLL_INTERVAL_MS = 300_000; // 5 minutes

	function pollProject() {
		// Clear any existing interval
		clearInterval(projectPollInterval);

		projectPollInterval = setInterval(async () => {
			// Only poll for copied projects
			if (!copiedProject) return;
			try {
				project = await ChatService.getProject(project.id, { dontLogErrors: true });
			} catch (error) {
				console.warn('Failed to poll project updates:', error);
				// Restart polling to retry on transient errors
				pollProject();
			}
		}, PROJECT_POLL_INTERVAL_MS);
	}

	$effect(() => {
		if (!copiedProject) {
			clearInterval(projectPollInterval);
			return;
		}

		// Restart polling when the component is mounted or the project is updated elsewhere.
		// The main use case for polling here is to keep the upgradeAvailable state fresh
		// and inform the user of available upgrades when they haven't done anything that
		// caused the project to be fetched for a while.
		pollProject();
	});

	onDestroy(() => {
		clearInterval(projectPollInterval);
	});

	async function openTemplatePanel() {
		openSidebarConfig(layout, 'template');
	}
</script>

<div
	class="border-surface2 dark:bg-gray-990 bg-background relative flex size-full flex-col border-r"
>
	<div class="flex h-16 w-full flex-shrink-0 items-center justify-between px-2 md:justify-start">
		<BetaLogo chat />
		{#if responsive.isMobile}
			{@render closeSidebar()}
		{/if}
	</div>
	<div class="default-scrollbar-thin flex w-full grow flex-col gap-2" use:scrollFocus>
		<Projects {project} {onCreateProject} />
		<div class="flex flex-col gap-8 px-4">
			{#if project.editor && !shared}
				<Threads {project} bind:currentThreadID />
				<Tasks {project} bind:currentThreadID />
				<McpServers {project} />
			{:else}
				<Threads {project} bind:currentThreadID />
				<McpServers {project} />
			{/if}
		</div>
	</div>

	<div class="flex w-full items-center justify-between gap-2 px-2 py-2">
		<div class="flex items-center gap-1">
			<div class="relative">
				<button
					class="icon-button relative"
					onclick={() => (layout.sidebarConfig = 'project-configuration')}
					use:tooltip={upgradeAvailable ? 'Upgrade available' : 'Configure Project'}
				>
					<Settings class="text-on-surface1 size-6" />
					{#if upgradeAvailable}
						<span
							class="absolute top-0 right-0 flex h-4 w-4 animate-[pulse_2s_ease-in-out_5] items-center
                   justify-center rounded-full"
						>
							<CircleFadingArrowUp class="text-primary size-4" />
						</span>
					{/if}
				</button>
			</div>
			{#if !shared}
				<button class="icon-button" onclick={openTemplatePanel} use:tooltip={'Project Sharing'}>
					<Share class="text-on-surface1 size-6" />
				</button>
			{/if}
		</div>
		{#if !responsive.isMobile}
			{@render closeSidebar()}
		{/if}
	</div>
</div>

{#snippet closeSidebar()}
	<button class="icon-button" onclick={() => (layout.sidebarOpen = false)}>
		<SidebarClose class="size-6" />
	</button>
{/snippet}
