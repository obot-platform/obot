<script lang="ts">
	import type { Project } from '$lib/services';
	import ChatService from '$lib/services/chat';
	import type { ProjectCredential } from '$lib/services';
	import { X } from 'lucide-svelte/icons';
	import CredentialAuth from '$lib/components/edit/CredentialAuth.svelte';
	import { getProjectTools } from '$lib/context/projectTools.svelte';
	import type { AssistantTool } from '$lib/services';
	import { closeSidebarConfig, getLayout } from '$lib/context/layout.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { CheckCircle } from 'lucide-svelte/icons';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
	let credentials = $state<ProjectCredential[]>([]);
	let authDialog: ReturnType<typeof CredentialAuth> | undefined = $state();
	let credToAuth = $state<ProjectCredential | undefined>();
	let toolSelection = $state<Record<string, AssistantTool>>({});
	let discordEnabled = $state(false);
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
		ChatService.listProjectCredentials(project.assistantID, project.id).then((creds) => {
			credentials = creds.items;
			credToAuth = credentials.find((c) => c.toolID === 'discord-bundle');
			if (
				credentials.find((c) => c.toolID === 'discord-bundle')?.exists &&
				toolSelection['discord-bundle']?.enabled
			) {
				discordEnabled = true;
			}
		});
	});
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
							Go to the Discord Developer Portal and create a new application if you haven't
							already.
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
						<h4 class="mb-2 font-medium">Step 5: Invite Bot to Server</h4>
						<p class="text-sm text-gray-600">
							Generate an invite URL in the OAuth2 section. Put the url in the browser and it will
							open a new tab to invite the bot to your server. Use `Add to Server` button to invite
							the bot to your server.
						</p>
					</div>
				</div>

				<div class="mt-6 flex justify-end gap-3">
					{#if !discordEnabled}
						<button
							class="button"
							onclick={async () => {
								if (toolSelection['discord-bundle'] && !toolSelection['discord-bundle'].enabled) {
									toolSelection['discord-bundle'].enabled = true;
									projectTools.tools = Object.values(toolSelection);
									await ChatService.updateProjectTools(project.assistantID, project.id, {
										items: Object.values(toolSelection)
									});
								}
								authDialog?.show();
							}}
						>
							Configure Now
						</button>
					{/if}
					{#if discordEnabled}
						<CheckCircle class="size-6 text-green-500" />
						<span class="text-sm text-gray-600">Configured</span>
					{/if}
				</div>
			</div>

			<CredentialAuth
				bind:this={authDialog}
				credential={credToAuth}
				{project}
				local={false}
				toolID="discord-bundle"
				onClose={() => {
					credToAuth = undefined;
				}}
			/>
		</div>
	</div>
</div>
