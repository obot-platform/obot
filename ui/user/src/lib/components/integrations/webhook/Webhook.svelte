<script lang="ts">
	import type { Project } from '$lib/services';
	import { ChatService } from '$lib/services';
	import { X } from 'lucide-svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import { responsive } from '$lib/stores';
	import { clickOutside } from '$lib/actions/clickoutside';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { openTask } from '$lib/context/layout.svelte';
	import type { Task } from '$lib/services';

	interface Props {
		project: Project;
	}

	const layout = getLayout();

	let { project = $bindable() }: Props = $props();
	let dialog: HTMLDialogElement;
	let confirmDisable: HTMLDialogElement;
	let config = $state({
		headers: [] as string[],
		secret: '',
		validationHeader: ''
	});
	let showSteps = $state(!project.capabilities?.onWebhook);
	let taskDialog: HTMLDialogElement | undefined = $state();
	let task = $state<Task | undefined>();
	let removeSecret = $state(false);

	$effect(() => {
		if (project.capabilities?.onWebhook) {
			config.secret = project.capabilities.onWebhook.secret;
			config.validationHeader = project.capabilities.onWebhook.validationHeader;
			config.headers = project.capabilities.onWebhook.headers;
		}
	});

	async function handleSubmit() {
		if (!project.capabilities) {
			project.capabilities = {};
		}

		if (!project.capabilities.onWebhook) {
			try {
				project.capabilities.onWebhook = {
					headers: config.headers,
					secret: config.secret,
					validationHeader: config.validationHeader
				};

				project = await ChatService.updateProject(project);
			} catch (err) {
				project.capabilities.onWebhook = undefined;
				throw err;
			}
		}

		dialog.close();
		config.headers = [];
		config.secret = '';
		config.validationHeader = '';

		let maxAttempts = 30;
		let attempts = 0;

		while (attempts < maxAttempts) {
			attempts++;
			project = await ChatService.getProject(project.id);

			if (project.workflowNameFromIntegration) {
				layout.tasks = (await ChatService.listTasks(project.assistantID, project.id)).items;
				task = layout.tasks.find((t) => t.id === project.workflowNameFromIntegration);
				if (task) {
					taskDialog?.showModal();
				}
				break;
			}

			await new Promise((resolve) => setTimeout(resolve, 1000));
		}
	}

	async function disableWebhook() {
		project.capabilities = {
			onWebhook: undefined
		};
		project = await ChatService.updateProject(project);
		confirmDisable.close();
		config.headers = [];
		config.secret = '';
		config.validationHeader = '';
	}
</script>

<div class="flex w-full flex-col">
	<div class="flex w-full justify-center px-4 py-4 md:px-8">
		<div class="flex w-full flex-col gap-4 md:max-w-[1200px]">
			<div class="flex w-full items-center justify-between">
				<h4 class="text-xl font-semibold">Webhook</h4>
			</div>

			<div class="pr-2.5">
				<CollapsePane
					header="Configure Webhook Integration"
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

<dialog
	bind:this={dialog}
	use:clickOutside={() => dialog?.close()}
	class="default-dialog md:w-1/2"
	class:mobile-screen-dialog={responsive.isMobile}
>
	<div class="p-6">
		<div class="flex flex-col gap-2">
			<button class="absolute top-0 right-0 p-3" onclick={() => dialog?.close()}>
				<X class="icon-default" />
			</button>
			<h3 class="mb-4 text-lg font-semibold">Configure Webhook</h3>
			{@render steps()}

			<div>
				{@render form()}
			</div>
		</div>
	</div>
</dialog>

<dialog bind:this={confirmDisable} class="modal" use:clickOutside={() => confirmDisable?.close()}>
	<div class="modal-box">
		<div class="p-4">
			<h3 class="text-lg font-medium">Disable Webhook Integration</h3>
			<p class="mt-2 text-sm text-gray-500">
				Are you sure you want to disable webhook integration? This will remove the webhook trigger
				from this project.
			</p>

			<div class="mt-6 flex justify-end gap-3">
				<button
					class="button"
					onclick={() => {
						confirmDisable.close();
					}}
				>
					Cancel
				</button>
				<button
					class="button bg-red-500 text-white hover:bg-red-600"
					onclick={async () => {
						await disableWebhook();
					}}
				>
					Disable
				</button>
			</div>
		</div>
	</div>
</dialog>

<dialog bind:this={taskDialog}>
	<div class="modal-box">
		<div class="p-4">
			<h3 class="text-lg font-medium">Task Created</h3>
			<p class="mt-2 text-sm text-gray-500">
				Task "{task?.name}" has been created from the webhook integration.
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
		<p class="text-sm text-gray-600">Configure your webhook integration below.</p>
	</div>
{/snippet}

{#snippet form()}
	<div class="space-y-3">
		<div class="space-y-2">
			<label for="secret" class="text-sm font-medium">Payload Signature Secret (Optional)</label>
			<p class="text-sm text-gray-600">
				This should match the secret you provide to the webhook provider.
			</p>
			<input
				type="password"
				id="secret"
				class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm"
				disabled={removeSecret}
				bind:value={config.secret}
				oninput={(e) => (config.secret = e.currentTarget.value)}
			/>
		</div>

		<div class={removeSecret ? 'hidden' : ''}>
			<label for="validationHeader" class="text-sm font-medium"
				>Payload Signature Header (Optional)</label
			>
			<p class="text-sm text-gray-600">
				The webhook receiver will calculate an HMAC digest of the payload using the supplied secret
				and compare it to the value sent in this header.
			</p>
			<input
				type="text"
				id="validationHeader"
				class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm"
				placeholder="Enter validation header name"
				bind:value={config.validationHeader}
				oninput={(e) => (config.validationHeader = e.currentTarget.value)}
			/>
		</div>

		<div>
			<label for="headers" class="text-sm font-medium">Headers (Optional)</label>
			<p class="text-sm text-gray-600">Add "*" to include all headers.</p>
			<input
				type="text"
				id="headers"
				class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm"
				placeholder="Enter headers separated by commas"
				value={config.headers.join(',')}
				oninput={(e) => (config.headers = e.currentTarget.value.split(','))}
			/>
		</div>

		<div class="mt-6 flex justify-end gap-3">
			{#if project.capabilities?.onWebhook}
				<button
					class="button bg-red-500 text-white hover:bg-red-600"
					onclick={() => {
						dialog.close();
						confirmDisable?.showModal();
					}}
				>
					Remove Configuration
				</button>
			{/if}
			<button class="button" onclick={handleSubmit}> Configure </button>
		</div>
	</div>
{/snippet}
