<script lang="ts">
	import { MessageCirclePlus, SidebarOpen } from 'lucide-svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import { ChatService, type Project } from '$lib/services';

	interface Props {
		currentThreadID?: string;
		project: Project;
	}

	let { currentThreadID = $bindable(), project }: Props = $props();
	const layout = getLayout();

	async function newThread() {
		const thread = await ChatService.createThread(project.assistantID, project.id);
		currentThreadID = thread.id;
	}
</script>

{#if !layout.threadsOpen}
	<button class="icon-button" onclick={() => (layout.threadsOpen = !layout.threadsOpen)}>
		<SidebarOpen class="icon-default" />
	</button>
	<button class="icon-button" onclick={() => newThread()}>
		<MessageCirclePlus class="icon-default" />
	</button>
{/if}
