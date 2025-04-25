<script lang="ts">
	import { ChatService, type Project, type ProjectShare } from '$lib/services';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import { browser } from '$app/environment';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
	let share = $state<ProjectShare>();
	let url = $derived(
		browser && share?.publicID
			? `${window.location.protocol}//${window.location.host}/s/${share.publicID}`
			: ''
	);

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

<div class="flex w-full flex-col">
	<div
		class="dark:border-surface2 flex w-full justify-center border-b border-transparent px-4 py-4 md:px-8"
	>
		<div class="flex w-full items-start md:max-w-[1200px]">
			<h4 class="text-xl font-semibold">ChatBot</h4>
		</div>
	</div>
	<div class="flex w-full justify-center px-4 py-8 md:px-8">
		<div class="flex w-full flex-col items-start gap-4 md:max-w-[1200px]">
			<div class="mb-1 flex w-full items-center justify-between">
				<p class="text-md font-semibold">Enable ChatBot</p>
				<Toggle label="Toggle ChatBot" checked={!!share?.publicID} onChange={handleChange} />
			</div>

			{#if share?.publicID}
				<div class="bg-surface2 flex w-full flex-col gap-2 rounded-xl p-3">
					<p class="text-sm text-gray-500">
						<b>Anyone with this link</b> can use this agent, which includes <b>any credentials</b> assigned
						to this agent.
					</p>
					<div class="flex gap-1">
						<CopyButton text={url} />
						<a href={url} class="text-md overflow-hidden text-ellipsis hover:underline">{url}</a>
					</div>
				</div>
			{/if}
		</div>
	</div>
</div>
