<script lang="ts">
	import { ChatService, type Project } from '$lib/services';
	import { MessageCirclePlus, SidebarClose } from 'lucide-svelte';
	import { closeAll, getLayout } from '$lib/context/chatLayout.svelte';
	import McpServers from '$lib/components/edit/McpServers.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Threads from '$lib/components/chat/sidebar/Threads.svelte';

	import { responsive } from '$lib/stores';
	import Logo from '$lib/components/navbar/Logo.svelte';
	import { scrollFocus } from '$lib/actions/scrollFocus.svelte';

	interface Props {
		project: Project;
		currentThreadID?: string;
		shared?: boolean;
	}

	let { project = $bindable(), currentThreadID = $bindable(), shared }: Props = $props();
	const layout = getLayout();

	async function createNewThread() {
		const thread = await ChatService.createThread(project.assistantID, project.id);
		const found = layout.threads?.find((t) => t.id === thread.id);
		if (!found) {
			layout.threads?.splice(0, 0, thread);
		}

		closeAll(layout);
		currentThreadID = thread.id;
	}
</script>

<div class="bg-surface1 dark:bg-surface2 relative flex size-full flex-col">
	<div class="flex h-16 w-full flex-shrink-0 items-center px-3">
		<Logo class="ml-0" />
		<div class="flex grow"></div>
		{#if !shared}
			<button
				class="icon-button p-0.5"
				use:tooltip={'Start New Thread'}
				onclick={() => createNewThread()}
			>
				<MessageCirclePlus class="size-6" />
			</button>
		{/if}
		{#if responsive.isMobile}
			{@render closeSidebar()}
		{/if}
	</div>
	<div class="default-scrollbar-thin flex w-full grow flex-col" use:scrollFocus>
		<Threads {project} bind:currentThreadID />
		<McpServers {project} />
	</div>

	<div class="flex items-center justify-end px-3 py-2">
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
