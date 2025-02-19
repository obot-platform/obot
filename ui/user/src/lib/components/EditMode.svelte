<script lang="ts">
	import { Trash2, X } from 'lucide-svelte';
	import General from '$lib/components/edit/General.svelte';
	import { context } from '$lib/stores';
	import { type Project, ChatService } from '$lib/services';
	import { onDestroy, type Snippet } from 'svelte';
	import Instructions from '$lib/components/edit/Instructions.svelte';
	import Interface from '$lib/components/edit/Interface.svelte';
	import Tools from '$lib/components/edit/Tools.svelte';
	import Knowledge from '$lib/components/edit/Knowledge.svelte';
	import Credentials from '$lib/components/edit/Credentials.svelte';
	import Publish from '$lib/components/edit/Publish.svelte';
	import { opacityIn } from '$lib/actions/animate';
	import Confirm from '$lib/components/Confirm.svelte';
	import { columnResize } from '$lib/actions/resize';

	interface Props {
		children: Snippet;
	}

	let { children }: Props = $props();

	let project = $state<Project>({
		id: '',
		name: '',
		created: ''
	});
	let projectSaved = '';
	let timer: number = 0;
	let show = $derived(context.editMode);
	let nav = $state<HTMLDivElement>();
	let toDelete = $state(false);

	async function updateProject() {
		if (JSON.stringify(project) === projectSaved) {
			return;
		}
		const oldProject = JSON.stringify(project);
		const newProject = await ChatService.updateProject(project);
		projectSaved = JSON.stringify(newProject);
		if (oldProject === JSON.stringify(project)) {
			project = newProject;
		}
	}

	async function loadProject() {
		project = await ChatService.getProject(context.projectID);
		projectSaved = JSON.stringify(project);
	}

	onDestroy(() => clearInterval(timer));

	$effect(() => {
		if (
			context.valid &&
			project.id === context.projectID &&
			context.project &&
			JSON.stringify(context.project) != JSON.stringify(project)
		) {
			context.project = project;
		}
	});

	$effect(() => {
		if (context.valid && project.id === '') {
			loadProject().then(() => {
				timer = setInterval(updateProject, 1000);
			});
		}
	});
</script>

<div class="colors-surface1 flex size-full flex-col">
	<!-- Header -->
	{#if show}
		<div class="flex h-16 w-full items-center gap-2 p-5" use:opacityIn>
			<img src="/user/images/obot-icon-blue.svg" class="h-8" alt="Obot icon" />
			<h1 class="text-xl font-semibold">Edit Mode</h1>
			<div class="grow"></div>
			<button
				class="button"
				onclick={() => {
					context.editMode = false;
				}}
			>
				<X class="icon-default" />
			</button>
		</div>
	{/if}

	<div class="flex h-full" style={show ? 'height: calc(100% - 64px);' : ''}>
		{#if show}
			<!-- Left Nav -->
			<div
				bind:this={nav}
				class="flex h-full w-[320px] flex-col overflow-auto pt-5"
				class:flex={show}
				class:hidden={!show}
			>
				<General {project} />
				<Instructions {project} />
				<Interface {project} />
				<Tools {project} />
				<Knowledge {project} />
				<Credentials {project} />
				<Publish {project} />
				<div class="grow"></div>
				<div class="flex justify-end p-2">
					<button
						class="button flex gap-1 text-gray"
						onclick={() => {
							toDelete = true;
						}}
					>
						<Trash2 class="icon-default" />
						<span>Remove</span>
					</button>
				</div>
			</div>
			<div role="none" class="w-2 cursor-col-resize" use:columnResize={nav}></div>
		{/if}
		<div class="colors-background h-full grow {show ? 'rounded-3xl p-5' : ''}">
			{@render children()}
		</div>
	</div>
</div>

<Confirm
	msg="Delete the current Obot?"
	show={toDelete}
	onsuccess={async () => {
		await ChatService.deleteProject(project.id);
		window.location.href = '/';
	}}
	oncancel={() => (toDelete = false)}
/>
