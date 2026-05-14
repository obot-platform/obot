<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { Clock3, Folders, MessageCircleMore, Workflow } from 'lucide-svelte';

	interface Props {
		projectId: string;
	}

	let { projectId }: Props = $props();
	let activeView = $derived(page.url.pathname.split('/').pop());
</script>

<div class="dock dock-sm bg-base-200 rounded-t-md">
	<a
		href={resolve(`/agent/p/${projectId}/workflows`)}
		class={activeView === 'workflows' ? 'dock-active text-primary' : ''}
	>
		<Workflow class="text-base-content/65" />
	</a>

	<a
		href={resolve(`/agent/p/${projectId}/scheduler`)}
		class={activeView === 'scheduler' ? 'dock-active text-primary' : ''}
	>
		<Clock3 class="text-base-content/65" />
	</a>

	<a
		href={resolve(`/agent/p/${projectId}/files`)}
		class={activeView === 'files' ? 'dock-active text-primary' : ''}
	>
		<Folders class="text-base-content/65" />
	</a>

	<a
		href={resolve(`/agent/p/${projectId}/sessions`)}
		class={activeView === 'sessions' ? 'dock-active text-primary' : ''}
	>
		<MessageCircleMore class="text-base-content/65" />
	</a>
</div>
