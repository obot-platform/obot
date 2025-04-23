<script lang="ts">
	import { ChatService, type Project, type ProjectShare } from '$lib/services';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { Trash2 } from 'lucide-svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import { fade } from 'svelte/transition';
	import { profile } from '$lib/stores';
	import { Check } from 'lucide-svelte/icons';

	interface Props {
		project: Project;
		dialog?: boolean;
	}

	let { project, dialog }: Props = $props();
	let share = $state<ProjectShare>();
	let url = $derived.by(() => {
		if (share?.publicID && typeof window !== 'undefined') {
			return `${window.location.protocol}//${window.location.host}/s/${share.publicID}`;
		}
		return '';
	});

	async function onOpen() {
		share = await ChatService.getProjectShare(project.assistantID, project.id);
	}

	async function doShare() {
		share = await ChatService.createProjectShare(project.assistantID, project.id);
	}

	async function unShare() {
		await ChatService.deleteProjectShare(project.assistantID, project.id);
		share = undefined;
	}
</script>

<CollapsePane header={dialog ? '' : 'Share'} open={dialog} {onOpen}>
	{#if share?.publicID && share.public}
		<div class="flex flex-col gap-4" in:fade>
			<div class="flex items-center gap-2">
				<span>Shared</span>
				<Check class="size-5" />
				<button class="button" onclick={() => unShare()}>
					<Trash2 class="size-4" />
				</button>
			</div>
			{#if profile.current.isAdmin?.()}
				<div class="flex gap-1">
					<input
						type="checkbox"
						checked={share.featured}
						onchange={async (e) => {
							if (e.target instanceof HTMLInputElement) {
								share = await ChatService.setFeatured(
									project.assistantID,
									project.id,
									e.target.checked
								);
							}
						}}
					/>
					<span class="text-sm">Featured</span>
				</div>
			{/if}
			<p class="text-sm">
				<b>Anyone with this link</b> can use this agent, which includes <b>any credentials</b> assigned
				to this agent.
			</p>
			<div class="flex gap-1">
				<CopyButton text={url} />
				<a href={url} class="overflow-hidden text-sm text-ellipsis hover:underline">{url}</a>
			</div>
		</div>
	{:else}
		<div class="flex items-center gap-2" in:fade>
			<span>Private</span>
			<div class="grow"></div>
			<button class="button self-end" onclick={doShare}>Share</button>
		</div>
	{/if}
</CollapsePane>
