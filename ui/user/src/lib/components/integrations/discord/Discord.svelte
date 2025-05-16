<script lang="ts">
	import type { Project } from '$lib/services';
	import ChatService from '$lib/services/chat';
	import type { ProjectCredential } from '$lib/services';
	import { X } from 'lucide-svelte/icons';
	import CredentialAuth from '$lib/components/edit/CredentialAuth.svelte';
	import { getProjectTools } from '$lib/context/projectTools.svelte';
	import type { AssistantTool } from '$lib/services';
	import { closeSidebarConfig, getLayout, openTask } from '$lib/context/layout.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import type { Task } from '$lib/services';

	interface Props {
		project: Project;
	}

	let { project = $bindable() }: Props = $props();
	let authDialog: ReturnType<typeof CredentialAuth> | undefined = $state();
	let credToAuth = $state<ProjectCredential | undefined>();
	let toolSelection = $state<Record<string, AssistantTool>>({});
	let confirmRemove: HTMLDialogElement | undefined = $state();
	let credentials = $state<ProjectCredential[]>([]);
	let task = $state<Task | undefined>();
	let taskDialog: HTMLDialogElement | undefined = $state();
	const discordEnabled = $derived(project.capabilities?.onDiscordMessage);
	const layout = getLayout();

	const projectTools = getProjectTools();
	toolSelection = getSelectionMap();

	function getSelectionMap() {
		return projectTools.tools
			.filter((t) => !t.builtin)
			.reduce<Record<string, AssistantTool>>((acc, tool) => {
				acc[tool.id] = { ...tool };
				return acc;
			}, {});
	}

	$effect(() => {
		ChatService.listProjectLocalCredentials(project.assistantID, project.id).then((creds) => {
			credentials = creds.items;
			credToAuth = credentials.find((c) => c.toolID === 'discord-bundle');
		});
	});

	async function configureDiscord() {
		if (toolSelection['discord-bundle'] && !toolSelection['discord-bundle'].enabled) {
			toolSelection['discord-bundle'].enabled = true;
			projectTools.tools = Object.values(toolSelection);
			await ChatService.updateProjectTools(project.assistantID, project.id, {
				items: Object.values(toolSelection)
			});
		}

		if (!project.capabilities) {
			project.capabilities = {};
		}

		if (!project.capabilities.onDiscordMessage) {
			project.capabilities.onDiscordMessage = true;
			try {
				await ChatService.updateProject(project);
			} catch (error) {
				project.capabilities.onDiscordMessage = false;
				throw error;
			}
		}

		let maxAttempts = 30;
		let attempts = 0;

		while (attempts < maxAttempts) {
			attempts++;
			project = await ChatService.getProject(project.id);
			if (project.workflowNamesFromIntegration?.discordWorkflowName) {
				layout.tasks = (await ChatService.listTasks(project.assistantID, project.id)).items;
				task = layout.tasks.find(
					(t) => t.id === project.workflowNamesFromIntegration?.discordWorkflowName
				);
				if (task && !credToAuth?.exists) {
					authDialog?.show();
				}
				break;
			}

			await new Promise((resolve) => setTimeout(resolve, 1000));
		}
	}
</script>

<div class="flex w-full flex-col">
	<div class="flex w-full justify-center px-4 py-4 md:px-8">
		<div class="flex w-full flex-col gap-4 md:max-w-[1200px]">
			<div class="flex w-full items-center justify-between">
				<h4 class="text-xl font-semibold">Discord</h4>
				<button
					onclick={() => closeSidebarConfig(layout)}
					class="icon-button"
					use:tooltip={'Exit Discord Configuration'}
				>
					<X class="size-6" />
				</button>
			</div>

			<h3 class="mb-4 text-lg font-semibold">Configure Discord Bot</h3>
			<div class="space-y-6">
				<p class="text-sm text-gray-600">
					All steps will be performed on the Discord Developer Portal.
				</p>

				<div class="space-y-4">
					<div>
						<h4 class="mb-2 font-medium">Step 1: Create a Discord Application</h4>
						<p class="text-sm text-gray-600">
							Go to the Discord Developer Portal <a
								href="https://discord.com/developers/applications/"
								target="_blank"
								class="text-blue-500">here</a
							> and create a new application if you haven't already.
						</p>
					</div>

					<div>
						<h4 class="mb-2 font-medium">Step 2: Create a Bot</h4>
						<p class="text-sm text-gray-600">
							In your application settings, go to the "Bot" section and create a new bot. In token
							section, you'll see the bot token by clicking on `Reset Token``. Keep this token
							secure as we'll need it later.
						</p>
					</div>

					<div>
						<h4 class="mb-2 font-medium">Step 3: Enable Required Intents</h4>
						<p class="text-sm text-gray-600">
							In the Bot section, enable these Privileged Gateway Intents:
						</p>
						<div class="mt-2 space-y-1">
							<div class="text-sm text-gray-600">• Message Content Intent</div>
							<div class="text-sm text-gray-600">• Server Members Intent</div>
							<div class="text-sm text-gray-600">• Presence Intent</div>
						</div>
					</div>

					<div>
						<h4 class="mb-2 font-medium">Step 4: Set Bot Permissions</h4>
						<p class="text-sm text-gray-600">
							In the Installations section, under "Default Install Settings", Add "bot" and enable
							these permissions:
						</p>
						<div class="mt-2 space-y-1">
							<div class="text-sm text-gray-600">• View Channels</div>
							<div class="text-sm text-gray-600">• Send Messages</div>
							<div class="text-sm text-gray-600">• Send Messages in Threads</div>
							<div class="text-sm text-gray-600">• Read Message History</div>
						</div>
					</div>

					<div>
						<h4 class="mb-2 font-medium">Step 5: Invite Bot to Server</h4>
						<p class="text-sm text-gray-600">
							Go to installation section and copy the installation link from "Install link". Use the
							discord provided one.
						</p>
					</div>
				</div>

				<div class="mt-6 flex justify-end gap-3">
					{#if !discordEnabled}
						<button class="button" onclick={configureDiscord}> Configure Now </button>
					{/if}
					{#if discordEnabled}
						<div class="flex items-center gap-2">
							<button
								class="button bg-red-500 text-white hover:bg-red-600"
								onclick={() => {
									confirmRemove?.showModal();
								}}
							>
								Remove Configuration
							</button>
							<button class="button" onclick={configureDiscord}> Configure </button>
						</div>
					{/if}
				</div>
			</div>

			<dialog bind:this={confirmRemove}>
				<div class="modal-box">
					<div class="p-4">
						<h3 class="text-lg font-medium">Remove Discord Configuration</h3>
						<p class="mt-2 text-sm text-gray-500">
							Are you sure you want to remove the Discord configuration? This will also remove the
							associated task.
						</p>

						<div class="mt-6 flex justify-end gap-3">
							<button
								class="button"
								onclick={() => {
									confirmRemove?.close();
								}}
							>
								Cancel
							</button>
							<button
								class="button bg-red-500 text-white hover:bg-red-600"
								onclick={async () => {
									if (project.capabilities) {
										project.capabilities.onDiscordMessage = false;
										project = await ChatService.updateProject(project);
									}
									confirmRemove?.close();
								}}
							>
								Remove
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
				toolID="discord-bundle"
				onClose={() => {
					credToAuth = undefined;
					taskDialog?.showModal();
				}}
			/>

			<dialog bind:this={taskDialog}>
				<div class="modal-box">
					<div class="p-4">
						<h3 class="text-lg font-medium">Task Created</h3>
						<p class="mt-2 text-sm text-gray-500">
							Task "{task?.name}" has been created from the Discord integration.
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
		</div>
	</div>
</div>
