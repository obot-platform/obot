<script lang="ts">
	import { type Task } from '$lib/services';
	import type { Project } from '$lib/services';
	import ChatService from '$lib/services/chat';
	import type { ProjectCredential } from '$lib/services';
	import { X } from 'lucide-svelte/icons';
	import CredentialAuth from '$lib/components/edit/CredentialAuth.svelte';
	import { tools } from '$lib/stores';
	import type { AssistantTool } from '$lib/services';

	interface Props {
		task?: Task;
		project: Project;
	}

	let { task = $bindable(), project }: Props = $props();
	let credentials = $state<ProjectCredential[]>([]);
	let configDialog: HTMLDialogElement;
	let credDialog: HTMLDialogElement;
	let credAuth: ReturnType<typeof CredentialAuth>;
	let toolSelection = $state<Record<string, AssistantTool>>({});

	toolSelection = getSelectionMap();

	function getSelectionMap() {
		return tools.current.tools
			.filter((t) => !t.builtin)
			.reduce<Record<string, AssistantTool>>((acc, tool) => {
				acc[tool.id] = { ...tool };
				return acc;
			}, {});
	}

	$effect(() => {
		ChatService.listProjectLocalCredentials(project.assistantID, project.id).then((creds) => {
			credentials = creds.items;
			if (!credentials.find((c) => c.toolID === 'discord-bundle')?.exists) {
				configDialog?.showModal();
				return;
			}
		});
	});
</script>

<div class="flex grow flex-col overflow-visible rounded-2xl bg-gray-50 p-5 dark:bg-gray-950">
	<dialog bind:this={configDialog} class="default-dialog">
		<div class="p-6">
			<button class="absolute right-0 top-0 p-3" onclick={() => configDialog?.close()}>
				<X class="icon-default" />
			</button>
			<h3 class="mb-4 text-lg font-semibold">Configure Discord Bot</h3>
			<div class="space-y-6">
				<p class="text-sm text-gray-600">
					All steps will be performed on the Discord Developer Portal.
				</p>

				<div class="space-y-4">
					<div>
						<h4 class="font-medium">Step 1: Create a Discord Application</h4>
						<p class="text-sm text-gray-600">
							Go to the Discord Developer Portal and create a new application if you haven't
							already.
						</p>
					</div>

					<div>
						<h4 class="font-medium">Step 2: Create a Bot</h4>
						<p class="text-sm text-gray-600">
							In your application settings, go to the "Bot" section and create a new bot. In token
							section, you'll see the bot token by clicking on `Reset Token``. Keep this token
							secure as we'll need it later.
						</p>
					</div>

					<div>
						<h4 class="font-medium">Step 3: Enable Required Intents</h4>
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
						<h4 class="font-medium">Step 4: Set Bot Permissions</h4>
						<p class="text-sm text-gray-600">
							In the Installations section, under "Default Install Settings", select "bot" and
							enable these permissions:
						</p>
						<div class="mt-2 space-y-1">
							<div class="text-sm text-gray-600">• View Channels</div>
							<div class="text-sm text-gray-600">• Send Messages</div>
							<div class="text-sm text-gray-600">• Send Messages in Threads</div>
							<div class="text-sm text-gray-600">• Read Message History</div>
						</div>
					</div>

					<div>
						<h4 class="font-medium">Step 5: Invite Bot to Server</h4>
						<p class="text-sm text-gray-600">
							Generate an invite URL in the OAuth2 section. Put the url in the browser and it will
							open a new tab to invite the bot to your server. Use `Add to Server` button to invite
							the bot to your server.
						</p>
					</div>
				</div>

				<div class="mt-6 flex justify-end gap-3">
					<button
						class="button"
						onclick={async () => {
							if (toolSelection['discord-bundle'] && !toolSelection['discord-bundle'].enabled) {
								toolSelection['discord-bundle'].enabled = true;
								tools.setTools(Object.values(toolSelection));
								await ChatService.updateProjectTools(project.assistantID, project.id, {
									items: Object.values(toolSelection)
								});
							}
							configDialog?.close();
							credDialog?.showModal();
							credAuth?.show();
						}}
					>
						Configure Now
					</button>
				</div>
			</div>
		</div>
	</dialog>

	<dialog
		bind:this={credDialog}
		class="max-h-[90vh] min-h-[300px] w-1/3 min-w-[300px] overflow-visible p-5"
	>
		<div class="flex h-full flex-col">
			<button class="absolute right-0 top-0 p-3" onclick={() => credDialog?.close()}>
				<X class="icon-default" />
			</button>
			<CredentialAuth
				bind:this={credAuth}
				{project}
				local
				toolID="discord-bundle"
				onClose={() => {
					credDialog?.close();
				}}
			/>
		</div>
	</dialog>
</div>
