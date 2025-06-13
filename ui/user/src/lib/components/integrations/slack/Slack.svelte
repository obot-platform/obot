<script lang="ts">
	import type { Project } from '$lib/services';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import {
		configureProjectSlack,
		disableProjectSlack,
		getProjectSlack
	} from '$lib/services/chat/operations';
	import { ChatService, type AssistantTool } from '$lib/services';
	import { X } from 'lucide-svelte';
	import { closeSidebarConfig, getLayout, openTask } from '$lib/context/chatLayout.svelte';
	import { responsive } from '$lib/stores';
	import { getProjectTools } from '$lib/context/projectTools.svelte';
	import { clickOutside } from '$lib/actions/clickoutside';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import type { Task } from '$lib/services';
	import type { ProjectCredential } from '$lib/services/chat/types';
	import CredentialAuth from '$lib/components/edit/CredentialAuth.svelte';

	interface Props {
		project: Project;
	}

	const projectTools = getProjectTools();
	function getSelectionMap() {
		return projectTools.tools
			.filter((t) => !t.builtin)
			.reduce<Record<string, AssistantTool>>((acc, tool) => {
				acc[tool.id] = { ...tool };
				return acc;
			}, {});
	}
	const layout = getLayout();

	let toolSelection = $state<Record<string, AssistantTool>>({});

	toolSelection = getSelectionMap();

	let { project = $bindable() }: Props = $props();
	let dialog: HTMLDialogElement;
	let confirmDisable: HTMLDialogElement;
	let addSlackBotTool: HTMLDialogElement;
	let redirectUrl = $state('');
	let eventUrl = $state('');
	let config = $state({
		appId: '',
		clientId: '',
		clientSecret: '',
		signingSecret: ''
	});
	let slackEnabled = $derived(project.capabilities?.onSlackMessage);
	let taskDialog: HTMLDialogElement | undefined = $state();
	let task = $state<Task | undefined>();
	let authDialog: ReturnType<typeof CredentialAuth> | undefined = $state();
	let credToAuth = $state<ProjectCredential | undefined>();
	let credentials = $state<ProjectCredential[]>([]);

	$effect(() => {
		ChatService.listProjectLocalCredentials(project.assistantID, project.id).then((creds) => {
			credentials = creds.items;
			credToAuth = credentials.find((c) => c.toolID === 'slack-bot-bundle');
		});
	});

	$effect(() => {
		redirectUrl = `${window.location.protocol}//${window.location.host}/api/app-oauth/callback/oa1t1${project.id.slice(2, 8)}`;
		eventUrl = `${window.location.protocol}//${window.location.host}/api/slack/events`;
	});

	async function getSlackConfig() {
		if (slackEnabled) {
			try {
				const response = await getProjectSlack(project.assistantID, project.id);
				config.appId = response.appId;
				config.clientId = response.clientId;
			} catch (error) {
				console.error('Failed to get Slack config:', error);
			}
		}
	}

	$effect(() => {
		if (project) {
			getSlackConfig();
		}
	});

	async function handleSubmit() {
		let slackReceiver = await configureProjectSlack(
			project.assistantID,
			project.id,
			config,
			slackEnabled ? 'PUT' : 'POST'
		);
		project = await ChatService.getProject(project.id);
		dialog.close();
		addSlackBotTool.showModal();
		config.appId = slackReceiver.appId;
		config.clientId = slackReceiver.clientId;
		config.clientSecret = '';
		config.signingSecret = '';
	}

	async function disableSlack() {
		await disableProjectSlack(project.assistantID, project.id);
		project = await ChatService.getProject(project.id);
		project.capabilities = { onSlackMessage: false };
		confirmDisable.close();
		config.appId = '';
		config.clientId = '';
		config.clientSecret = '';
		config.signingSecret = '';
	}

	async function configureSlackTool() {
		addSlackBotTool?.close();
		if (toolSelection['slack-bot-bundle'] && !toolSelection['slack-bot-bundle'].enabled) {
			toolSelection['slack-bot-bundle'].enabled = true;
			projectTools.tools = Object.values(toolSelection);
			await ChatService.updateProjectTools(project.assistantID, project.id, {
				items: Object.values(toolSelection)
			});
		}

		// Wait for the Slack workflow to be created
		let maxAttempts = 30;
		let attempts = 0;

		while (attempts < maxAttempts) {
			attempts++;
			project = await ChatService.getProject(project.id);

			if (project.workflowNamesFromIntegration?.slackWorkflowName) {
				layout.tasks = (await ChatService.listTasks(project.assistantID, project.id)).items;
				task = layout.tasks.find(
					(t) => t.id === project.workflowNamesFromIntegration?.slackWorkflowName
				);
				if (task) {
					authDialog?.show();
				}

				if (!project.sharedTasks) {
					project.sharedTasks = [];
				}
				if (
					!project.sharedTasks.includes(project.workflowNamesFromIntegration?.slackWorkflowName)
				) {
					project.sharedTasks.push(project.workflowNamesFromIntegration?.slackWorkflowName);
					project = await ChatService.updateProject(project);
				}
				break;
			}

			await new Promise((resolve) => setTimeout(resolve, 1000));
		}
	}
</script>

<div class="flex min-h-0 w-full grow flex-col items-center">
	<div class="flex w-full items-center">
		<div class="mx-auto flex w-full flex-col gap-4 p-4 md:max-w-[1200px]">
			<div class="flex w-full items-center justify-between">
				<h4 class="text-xl font-semibold">Slack</h4>
				<button
					onclick={() => closeSidebarConfig(layout)}
					class="icon-button"
					use:tooltip={'Exit Slack Configuration'}
				>
					<X class="size-6" />
				</button>
			</div>
		</div>
	</div>
	<div class="dark:bg-gray-980 flex w-full items-center bg-gray-50">
		<div class="mx-auto flex h-full w-full flex-col gap-4 p-4 md:max-w-[1200px]">
			<div class="flex flex-col gap-2">
				<div class="flex flex-col gap-4 rounded-xl bg-white p-4 shadow-sm dark:bg-black">
					<h3 class="text-lg font-semibold">Configure Slack Integration</h3>
					{@render steps()}
				</div>
			</div>
		</div>
	</div>
	<div class="flex w-full items-center bg-white dark:bg-black">
		<div class="mx-auto flex h-full w-full flex-col gap-4 p-4 md:max-w-[1200px]">
			<div class="flex justify-end gap-3">
				{#if project.capabilities?.onSlackMessage}
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
				<button class="button" onclick={handleSubmit}> Configure Now </button>
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
			<h3 class="mb-4 text-lg font-semibold">Configure Slack OAuth App</h3>
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
			<h3 class="text-lg font-medium">Disable Slack Integration</h3>
			<p class="mt-2 text-sm text-gray-500">
				Are you sure you want to disable Slack integration? This will remove the Slack trigger from
				this project.
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
						await disableSlack();
					}}
				>
					Disable
				</button>
			</div>
		</div>
	</div>
</dialog>

<dialog bind:this={addSlackBotTool} class="default-dialog">
	<div class="p-6">
		<h3 class="mb-4 text-lg font-semibold">Next Steps</h3>
		<div class="space-y-4">
			<p class="text-sm text-gray-600">
				We'll add the Slack Bot tool to your project and automatically create a task that can be
				triggered from Slack messages. This will allow your agent to:
			</p>
			<ul class="list-disc pl-6 text-sm text-gray-600">
				<li>Automatically respond when mentioned in Slack</li>
				<li>Process messages and take actions</li>
				<li>Send responses back to the Slack conversation</li>
			</ul>

			<div class="mt-6 flex justify-end gap-3">
				<button class="button-primary" onclick={configureSlackTool}> Continue </button>
			</div>
		</div>
	</div>
</dialog>

<dialog bind:this={taskDialog}>
	<div class="modal-box">
		<div class="p-4">
			<h3 class="text-lg font-medium">Task Created</h3>
			<p class="mt-2 text-sm text-gray-500">
				Task "{task?.name}" has been created from the Slack integration.
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

<CredentialAuth
	bind:this={authDialog}
	credential={credToAuth}
	{project}
	local={true}
	toolID="slack-bot-bundle"
	onClose={() => {
		credToAuth = undefined;
		taskDialog?.showModal();
	}}
/>

{#snippet steps()}
	<div class="space-y-6">
		<p class="text-sm font-light text-gray-500">
			To message your agent from Slack, you'll need to create a Slack App and complete this
			configuration. You must be an administrator in your Slack Workspace.
		</p>
		<div>
			<h4 class="font-semibold">Step 1: Create a Slack App</h4>
			<ul class="my-2 list-disc space-y-3 pl-4 text-sm font-light text-gray-500">
				<li>
					From the <a
						href="https://api.slack.com/apps"
						target="_blank"
						rel="external"
						class="text-link">Slack App page</a
					>, click the Create New App button and select "From Scratch" in Create an app modal.
				</li>
				<li>Name the app and select the workspace you want your app to be available in.</li>
			</ul>
		</div>

		<div>
			<h4 class="font-medium">Step 2: Copy App Credentials</h4>
			<ul class="my-2 list-disc space-y-3 pl-4 text-sm font-light text-gray-500">
				<li>
					After completing the previous step, you'll be presented with your App Credentials under
					the Basic Information section. Copy the values to the corresponding fields below.
				</li>
			</ul>
			<div class="border-surface2 dark:border-surface3 rounded-lg border p-4">
				{@render form()}
			</div>
		</div>

		<div>
			<h4 class="font-medium">Step 3: Configure OAuth</h4>
			<ul class="my-2 list-disc space-y-3 pl-4 text-sm font-light text-gray-500">
				<li>
					From the left navigation of your Slack App, navigate to the "OAuth & Permissions" section.
				</li>
				<li>
					In the "Redirect URLs" section, click "Add New Redirect URL" and add the follow URL:
					<div class="copy-link">
						<CopyButton text={redirectUrl} />
						{redirectUrl}
					</div>
				</li>

				<li>
					Next, locate the "Bot Token Scopes" section and add the following scopes:
					<div class="mt-2 flex flex-wrap gap-2">
						<div class="copy-pill">
							<CopyButton text="channels:history" />
							channels:history
						</div>
						<div class="copy-pill">
							<CopyButton text="groups:history" />
							groups:history
						</div>
						<div class="copy-pill">
							<CopyButton text="im:history" />
							im:history
						</div>
						<div class="copy-pill">
							<CopyButton text="mpim:history" />
							mpim:history
						</div>
						<div class="copy-pill">
							<CopyButton text="channels:read" />
							channels:read
						</div>
						<div class="copy-pill">
							<CopyButton text="files:read" />
							files:read
						</div>
						<div class="copy-pill">
							<CopyButton text="im:read" />
							im:read
						</div>
						<div class="copy-pill">
							<CopyButton text="team:read" />
							team:read
						</div>
						<div class="copy-pill">
							<CopyButton text="users:read" />
							users:read
						</div>
						<div class="copy-pill">
							<CopyButton text="groups:read" />
							groups:read
						</div>
						<div class="copy-pill">
							<CopyButton text="chat:write" />
							chat:write
						</div>
						<div class="copy-pill">
							<CopyButton text="groups:write" />
							groups:write
						</div>
						<div class="copy-pill">
							<CopyButton text="mpim:write" />
							mpim:write
						</div>
						<div class="copy-pill">
							<CopyButton text="im:write" />
							im:write
						</div>
						<div class="copy-pill">
							<CopyButton text="assistant:write" />
							assistant:write
						</div>
					</div>
				</li>
			</ul>
		</div>

		<div>
			<h4 class="font-medium">Step 3: Enable Events</h4>
			<ul class="my-2 list-disc space-y-3 pl-4 text-sm font-light text-gray-500">
				<li>
					From the left navigation of your Slack App, navigate to the "Event Subscriptions" section.
				</li>
				<li class="text-sm text-gray-600">
					Enable events and add the Request URL below:
					<div class="copy-link">
						<CopyButton text={eventUrl} />
						{eventUrl}
					</div>
				</li>

				<li class="mt-2 text-sm text-gray-600">
					Next, expand the "Subscribe to bot events" section, click the "Add Bot User Event" button
					and add the following event:
					<div class="copy-pill">
						<CopyButton text={'app_mention'} />
						app_mention
					</div>
				</li>
			</ul>
		</div>
	</div>
{/snippet}

{#snippet form()}
	<div class="space-y-4">
		<div>
			<label for="appIdLabel" class="text-sm font-medium">App ID</label>
			<input
				type="text"
				id="appId"
				class="text-input-filled mt-1 text-sm"
				placeholder="Enter App ID"
				bind:value={config.appId}
				oninput={(e) => (config.appId = e.currentTarget.value)}
			/>
		</div>

		<div>
			<label for="clientIdLabel" class="text-sm font-medium">Client ID</label>
			<input
				type="text"
				id="clientId"
				class="text-input-filled mt-1 text-sm"
				placeholder="Enter Client ID"
				bind:value={config.clientId}
				oninput={(e) => (config.clientId = e.currentTarget.value)}
			/>
		</div>

		<form>
			<label for="clientSecretLabel" class="text-sm font-medium">Client Secret</label>
			<input
				type="password"
				id="clientSecret"
				class="text-input-filled mt-1 text-sm"
				placeholder={slackEnabled ? '***********' : 'Enter Client Secret'}
				autocomplete="off"
				bind:value={config.clientSecret}
				oninput={(e) => (config.clientSecret = e.currentTarget.value)}
			/>
		</form>

		<form>
			<label for="signingSecretLabel" class="text-sm font-medium">Signing Secret</label>
			<input
				type="password"
				id="signingSecret"
				class="text-input-filled mt-1 text-sm"
				placeholder={slackEnabled ? '***********' : 'Enter Signing Secret'}
				autocomplete="off"
				bind:value={config.signingSecret}
				oninput={(e) => (config.signingSecret = e.currentTarget.value)}
			/>
		</form>
	</div>
{/snippet}

<style lang="postcss">
	.copy-link {
		margin-top: 0.5rem;
		display: flex;
		max-width: fit-content;
		align-items: center;
		gap: 0.5rem;
		border-radius: 0.25rem;
		background-color: var(--color-surface1);
		color: var(--color-black);
		padding: 0.5rem;
	}

	:global(.dark) .copy-link {
		background-color: var(--color-surface2);
		color: var(--color-white);
	}

	.copy-pill {
		margin-top: 0.5rem;
		display: flex;
		max-width: fit-content;
		align-items: center;
		gap: 0.5rem;
		border-radius: calc(infinity * 1px);
		background-color: var(--color-surface1);
		padding: 0.5rem 1rem;
		font-size: 0.75rem;
		color: var(--color-black);
	}

	:global(.dark) .copy-pill {
		background-color: var(--color-surface2);
		color: var(--color-white);
	}
</style>
