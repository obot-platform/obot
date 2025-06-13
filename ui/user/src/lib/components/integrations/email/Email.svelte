<script lang="ts">
	import type { Project } from '$lib/services';
	import { ChatService } from '$lib/services';
	import { getLayout, openTask } from '$lib/context/chatLayout.svelte';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import type { Task } from '$lib/services';

	interface Props {
		project: Project;
	}

	const layout = getLayout();

	let { project = $bindable() }: Props = $props();
	let config = $state({
		allowedSenders: [] as string[]
	});
	let showSteps = $state(!project.capabilities?.onEmail);
	let task = $state<Task | undefined>();
	let taskDialog: HTMLDialogElement;

	$effect(() => {
		if (project.capabilities?.onEmail) {
			config.allowedSenders = project.capabilities.onEmail.allowedSenders ?? [];
		}
	});

	async function handleSubmit() {
		if (!project.capabilities) {
			project.capabilities = {};
		}

		if (!project.capabilities.onEmail) {
			try {
				project.capabilities.onEmail = {
					allowedSenders: config.allowedSenders
				};

				project = await ChatService.updateProject(project);
			} catch (err) {
				project.capabilities.onEmail = undefined;
				throw err;
			}
		}

		config.allowedSenders = [];

		let maxAttempts = 30;
		let attempts = 0;

		while (attempts < maxAttempts) {
			attempts++;
			project = await ChatService.getProject(project.id);

			if (project.workflowNamesFromIntegration?.emailWorkflowName) {
				layout.tasks = (await ChatService.listTasks(project.assistantID, project.id)).items;
				task = layout.tasks.find(
					(t) => t.id === project.workflowNamesFromIntegration?.emailWorkflowName
				);
				if (task) {
					taskDialog?.showModal();
				}
				break;
			}

			await new Promise((resolve) => setTimeout(resolve, 1000));
		}
	}

	async function disableEmail() {
		project.capabilities = {
			onEmail: undefined
		};
		project = await ChatService.updateProject(project);
		config.allowedSenders = [];
	}
</script>

<div class="flex w-full flex-col">
	<div class="flex w-full justify-center px-4 py-4 md:px-8">
		<div class="flex w-full flex-col gap-4 md:max-w-[1200px]">
			<div class="flex w-full items-center justify-between">
				<h4 class="text-xl font-semibold">Email</h4>
			</div>

			<div class="pr-2.5">
				<CollapsePane
					header="Configure Email Integration"
					open={showSteps}
					classes={{
						header: 'font-semibold px-0',
						content: 'bg-transparent px-0 shadow-none',
						headerText: 'text-base font-normal'
					}}
				>
					{@render steps()}
				</CollapsePane>
				<div class="w-full">
					{@render form()}
				</div>
			</div>
		</div>
	</div>
</div>

<dialog bind:this={taskDialog}>
	<div class="modal-box">
		<div class="p-4">
			<h3 class="text-lg font-medium">Task Created</h3>
			<p class="mt-2 text-sm text-gray-500">
				Task "{task?.name}" has been created from the Email integration.
			</p>

			<div class="mt-6 flex justify-end gap-3">
				<button
					class="button"
					onclick={() => {
						taskDialog?.close();
						openTask(layout, task?.id);
					}}
				>
					Go to Task
				</button>
			</div>
		</div>
	</div>
</dialog>

{#snippet steps()}
	<div class="space-y-6">
		<p class="text-sm text-gray-600">Configure your email integration below.</p>
	</div>
{/snippet}

{#snippet form()}
	<div class="space-y-3">
		<div>
			<label for="allowedSender" class="text-sm font-medium">Allowed Sender (Optional)</label>
			<p class="text-sm text-gray-600">Add "*" to allow all senders.</p>
			<input
				type="text"
				id="allowedSender"
				class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm"
				placeholder="Enter allowed sender separated by comma"
				value={config.allowedSenders.join(',')}
				oninput={(e) => (config.allowedSenders = e.currentTarget.value.split(','))}
			/>
		</div>
		<div class="mt-6 flex justify-end gap-3">
			{#if project.capabilities?.onEmail}
				<button class="button bg-red-500 text-white hover:bg-red-600" onclick={disableEmail}>
					Remove Configuration
				</button>
			{/if}
			<button class="button" onclick={handleSubmit}> Configure </button>
		</div>
	</div>
{/snippet}
