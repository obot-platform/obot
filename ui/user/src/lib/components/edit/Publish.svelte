<script lang="ts">
	import { ChatService, type Project, type ProjectTemplate } from '$lib/services';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { onDestroy } from 'svelte';
	import { RefreshCw, Trash2 } from 'lucide-svelte';
	import { Check } from 'lucide-svelte/icons';
	import Loading from '$lib/icons/Loading.svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
	let template = $state<ProjectTemplate>();
	let interval = $state(0);
	let progress = $state(false);
	let url = $derived.by(() => {
		if (template?.publicID && typeof window !== 'undefined') {
			return `${window.location.protocol}//${window.location.host}/o/${template.publicID}`;
		}
		return '';
	});

	onDestroy(() => {
		clearInterval(interval);
	});

	function kickPoller() {
		if (interval) {
			return;
		}
		if (template?.ready && template?.publicID) {
			return;
		}
		interval = setInterval(pollTemplate, 1000);
	}

	async function pollTemplate() {
		if (template?.ready && template?.publicID) {
			clearInterval(interval);
			interval = 0;
		} else if (template?.id) {
			template = await ChatService.getProjectTemplate(project.id, template.id);
		} else if (interval) {
			clearInterval(interval);
			interval = 0;
		}
	}

	async function onOpen() {
		const { items } = await ChatService.listProjectTemplates({ projectID: project.id });
		const ready = items.filter((t) => t.ready && t.publicID);
		if (ready.length) {
			template = ready[0];
		} else if (items.length) {
			template = items[0];
		} else {
			template = undefined;
		}
		kickPoller();
	}

	async function publish() {
		template = await ChatService.createProjectTemplate(project.id);
		kickPoller();
	}

	async function unpublish() {
		progress = true;
		try {
			const { items } = await ChatService.listProjectTemplates({ projectID: project.id });
			for (const t of items) {
				await ChatService.deleteProjectTemplate(project.id, t.id);
			}
			template = undefined;
		} finally {
			progress = false;
		}
	}
</script>

<CollapsePane header="Publish" {onOpen}>
	{#if template?.ready && template?.publicID}
		<div class="flex flex-col gap-4">
			<div class="flex items-center gap-2">
				<span>Published</span>
				<div class="grow">
					<Check class="h-4 w-4" />
				</div>
				<button class="button">
					<RefreshCw class="h-4 w-4" />
				</button>
				<button class="button" onclick={() => unpublish()}>
					{#if progress}
						<Loading class="h-4 w-4" />
					{:else}
						<Trash2 class="h-4 w-4" />
					{/if}
				</button>
			</div>
			<div class="flex gap-1">
				<CopyButton text={url} />
				<a href={url} class="text-sm hover:underline" target="_blank">{url.slice(0, 35)}...</a>
			</div>
		</div>
	{:else if template?.id}
		<div class="flex items-center gap-2">
			<Loading />
			<span>Publishing</span>
		</div>
	{:else}
		<div class="flex items-center gap-2">
			<span>Private</span>
			<div class="grow"></div>
			<button class="button self-end" onclick={publish}> Publish </button>
		</div>
	{/if}
</CollapsePane>
