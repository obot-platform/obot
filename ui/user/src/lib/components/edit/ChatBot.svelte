<script lang="ts">
	import { ChatService, type Project, type ProjectShare } from '$lib/services';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import Toggle from '../Toggle.svelte';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
	let share = $state<ProjectShare>();
	let url = $derived.by(() => {
		if (share?.publicID && typeof window !== 'undefined') {
			return `${window.location.protocol}//${window.location.host}/s/${share.publicID}`;
		}
		return '';
	});

	async function updateShare() {
		share = await ChatService.getProjectShare(project.assistantID, project.id);
	}

	$effect(() => {
		if (project) {
			updateShare();
		}
	});

	async function handleChange(checked: boolean) {
		if (checked) {
			share = await ChatService.createProjectShare(project.assistantID, project.id);
		} else {
			await ChatService.deleteProjectShare(project.assistantID, project.id);
			share = undefined;
		}
	}
</script>

<div class="flex flex-col gap-2">
	<div class="mb-1 flex items-center justify-between">
		<p class="grow text-sm font-semibold">ChatBot</p>
		<Toggle label="Toggle ChatBot" checked={!!share} onChange={handleChange} />
	</div>

	{#if share}
		<div class="bg-surface2 flex flex-col gap-2 rounded-xl p-3">
			<p class="text-xs text-gray-500">
				<b>Anyone with this link</b> can use this agent, which includes <b>any credentials</b> assigned
				to this agent.
			</p>
			<div class="flex gap-1">
				<CopyButton text={url} />
				<a href={url} class="overflow-hidden text-sm text-ellipsis hover:underline">{url}</a>
			</div>
		</div>
	{/if}
</div>
