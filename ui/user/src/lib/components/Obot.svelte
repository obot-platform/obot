<script lang="ts">
	import Editor from '$lib/components/Editors.svelte';
	import Navbar from '$lib/components/Navbar.svelte';
	import Sidebar from '$lib/components/Sidebar.svelte';
	import Task from '$lib/components/tasks/Task.svelte';
	import Thread from '$lib/components/Thread.svelte';
	import { ChatService, EditorService, type Project } from '$lib/services';
	import type { EditorItem } from '$lib/services/editor/index.svelte';
	import { errors, responsive } from '$lib/stores';
	import { closeAll, getLayout } from '$lib/context/layout.svelte';
	import { GripVertical, Plus, SidebarOpen } from 'lucide-svelte';
	import { fade, slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';
	import Logo from './navbar/Logo.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { columnResize } from '$lib/actions/resize';
	import { X } from 'lucide-svelte';
	import CredentialAuth from '$lib/components/edit/CredentialAuth.svelte';
	import type { Assistant, ProjectCredential } from '$lib/services';
	import { clickOutside } from '$lib/actions/clickoutside';
	import { goto } from '$app/navigation';
	import SidebarConfig from './SidebarConfig.svelte';

	interface Props {
		assistant?: Assistant;
		project: Project;
		items?: EditorItem[];
		currentThreadID?: string;
	}

	let { project = $bindable(), currentThreadID = $bindable(), assistant }: Props = $props();
	let layout = getLayout();
	let editor: HTMLDivElement | undefined = $state();

	let credentials = $state<ProjectCredential[]>([]);
	let credDialog: HTMLDialogElement;
	let credAuth: ReturnType<typeof CredentialAuth>;
	let configDialog: HTMLDialogElement;

	async function createNewThread() {
		const thread = await ChatService.createThread(project.assistantID, project.id);
		const found = layout.threads?.find((t) => t.id === thread.id);
		if (!found) {
			layout.threads?.splice(0, 0, thread);
		}

		closeAll(layout);
		currentThreadID = thread.id;
	}

	async function createNewAgent() {
		try {
			const project = await EditorService.createObot();
			await goto(`/o/${project.id}`);
		} catch (error) {
			errors.append((error as Error).message);
		}
	}

	$effect(() => {
		ChatService.listProjectLocalCredentials(project.assistantID, project.id).then((creds) => {
			credentials = creds.items;
			if (
				project.capabilities?.onSlackMessage &&
				!credentials.find((c) => c.toolID === 'slack-bot-bundle')?.exists
			) {
				configDialog?.showModal();
				return;
			}
		});
	});
</script>

<div class="colors-background relative flex h-full flex-col overflow-hidden">
	<div
		class="border-surface1 relative flex h-full"
		class:border={layout.sidebarOpen && !layout.fileEditorOpen}
	>
		{#if layout.sidebarOpen && !layout.fileEditorOpen}
			<div
				class="bg-surface1 w-screen min-w-screen md:w-1/6 md:min-w-[275px]"
				transition:slide={{ axis: 'x' }}
			>
				<Sidebar {assistant} bind:project bind:currentThreadID />
			</div>
		{/if}

		<main
			id="main-content"
			class="flex max-w-full grow flex-col overflow-hidden"
			class:hidden={layout.sidebarOpen && responsive.isMobile}
		>
			<div class="w-full">
				<Navbar>
					{#snippet leftContent()}
						{#if !layout.sidebarOpen || layout.fileEditorOpen}
							<Logo class="ml-0" />
							<button
								class="icon-button"
								in:fade={{ delay: 400 }}
								onclick={() => {
									layout.sidebarOpen = true;
									layout.fileEditorOpen = false;
								}}
								use:tooltip={'Open Sidebar'}
							>
								<SidebarOpen class="icon-default" />
							</button>
							<button
								class="icon-button"
								in:fade={{ delay: 400 }}
								use:tooltip={'Start New Thread'}
								onclick={() => createNewThread()}
							>
								<Plus class="icon-default" />
							</button>
						{/if}
					{/snippet}
				</Navbar>
			</div>
			{#if !layout.projectEditorOpen && !layout.fileEditorOpen && !layout.sidebarConfigOpen}
				<div class="absolute top-[76px] right-5 z-30 flex flex-col gap-4" in:fade={{ delay: 300 }}>
					<button
						use:tooltip={'New Agent'}
						class="icon-button border-surface3 border p-2"
						onclick={() => createNewAgent()}
					>
						<Plus class="size-5" />
					</button>
				</div>
			{/if}

			<div class="relative flex h-[calc(100%-76px)] max-w-full grow">
				{#if !responsive.isMobile || (responsive.isMobile && !layout.fileEditorOpen)}
					{#if layout.editTaskID && layout.tasks}
						{#each layout.tasks as task, i}
							{#if task.id === layout.editTaskID}
								{#key layout.editTaskID}
									<Task
										{project}
										bind:task={layout.tasks[i]}
										onDelete={() => {
											layout.editTaskID = undefined;
											layout.tasks?.splice(i, 1);
										}}
									/>
								{/key}
							{/if}
						{/each}
					{:else if layout.displayTaskRun}
						{#key layout.displayTaskRun.id}
							<Task
								{project}
								task={{
									...layout.displayTaskRun.task,
									id: layout.displayTaskRun.taskID
								}}
								runID={layout.displayTaskRun.id}
							/>
						{/key}
					{:else if layout.sidebarConfigOpen}
						<SidebarConfig bind:project />
					{:else}
						<Thread bind:id={currentThreadID} bind:project />
					{/if}
				{/if}

				{#if editor && layout.fileEditorOpen}
					<div
						use:columnResize={{ column: editor, direction: 'right' }}
						class="relative h-full w-8 cursor-grab"
						transition:slide={{ axis: 'x' }}
					>
						<div
							class="text-on-surface1 absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2"
						>
							<GripVertical class="text-surface3 size-3" />
						</div>
					</div>
				{/if}
				<div
					bind:this={editor}
					class={twMerge(
						'border-surface2 absolute right-0 z-30 float-right flex w-full flex-shrink-0 translate-x-full transform border-4 border-r-0 transition-transform duration-300 md:w-3/5 md:max-w-[calc(100%-320px)] md:min-w-[320px] md:rounded-l-3xl',
						layout.fileEditorOpen && 'relative w-full translate-x-0',
						!layout.fileEditorOpen && 'w-0'
					)}
				>
					<Editor {project} {currentThreadID} />
				</div>
			</div>

			<dialog
				bind:this={configDialog}
				class="default-dialog"
				use:clickOutside={() => configDialog?.close()}
			>
				<div class="p-6">
					<button class="absolute top-0 right-0 p-3" onclick={() => configDialog?.close()}>
						<X class="icon-default" />
					</button>
					<h3 class="mb-4 text-lg font-semibold">Configure Slack</h3>
					<p class="text-sm text-gray-600">
						To run this task, you'll need to configure the Slack Bot tool first.
					</p>
					<div class="mt-6 flex justify-end gap-3">
						<button
							class="button"
							onclick={() => {
								configDialog?.close();
								credDialog?.showModal();
								credAuth?.show();
							}}
						>
							Configure Now
						</button>
					</div>
				</div>
			</dialog>

			<dialog
				bind:this={credDialog}
				class="max-h-[90vh] min-h-[300px] w-1/3 min-w-[300px] overflow-visible p-5"
			>
				<div class="flex h-full flex-col">
					<button class="absolute top-0 right-0 p-3" onclick={() => credDialog?.close()}>
						<X class="icon-default" />
					</button>
					<CredentialAuth
						bind:this={credAuth}
						{project}
						local
						toolID="slack-bot-bundle"
						onClose={() => credDialog?.close()}
					/>
				</div>
			</dialog>
		</main>
	</div>
</div>
