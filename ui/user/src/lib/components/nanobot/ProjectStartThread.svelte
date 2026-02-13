<script lang="ts">
	import type { ChatService } from '$lib/services/nanobot/chat/index.svelte';
	import Thread from '$lib/components/nanobot/Thread.svelte';
	import { Binoculars, MessageCircle, Sparkles } from 'lucide-svelte';
	import { AdminService, NanobotService } from '$lib/services';
	import { errors } from '$lib/stores';
	import Confirm from '$lib/components/Confirm.svelte';

	interface Props {
		agentId: string;
		projectId: string;
		chat: ChatService;
		onFileOpen?: (filename: string) => void;
		suppressEmptyState?: boolean;
		onThreadContentWidth?: (width: number) => void;
	}

	let { agentId, projectId, chat, onFileOpen, suppressEmptyState, onThreadContentWidth }: Props =
		$props();

	let showRestartConfirm = $state(false);
	let restarting = $state(false);

	async function handleRestart() {
		restarting = true;
		try {
			await AdminService.restartK8sDeployment(`ms1${agentId}`);
			await NanobotService.launchProjectV2Agent(projectId, agentId);
			window.location.reload();
		} catch (error) {
			console.error('Failed to restart agent:', error);
			errors.append(error);
		} finally {
			restarting = false;
			showRestartConfirm = false;
		}
	}
</script>

<div class="flex h-full w-full">
	<div class="h-full min-w-0 flex-1">
		{#key chat.chatId}
			<Thread
				messages={chat.messages}
				prompts={chat.prompts}
				elicitations={chat.elicitations}
				agents={chat.agents}
				selectedAgentId={chat.selectedAgentId}
				onAgentChange={chat.selectAgent}
				onElicitationResult={chat.replyToElicitation}
				onSendMessage={chat.sendMessage}
				onFileUpload={chat.uploadFile}
				onCancel={chat.cancelMessage}
				cancelUpload={chat.cancelUpload}
				uploadingFiles={chat.uploadingFiles}
				uploadedFiles={chat.uploadedFiles}
				isLoading={chat.isLoading}
				isRestoring={chat.isRestoring}
				agent={chat.agent}
				onRestart={() => {
					showRestartConfirm = true;
				}}
				onRefreshResources={() => {
					chat.refreshResources();
				}}
				{onFileOpen}
				{suppressEmptyState}
				onContentWidthChange={onThreadContentWidth}
			>
				{#snippet emptyStateContent()}
					<div class="flex flex-col items-center gap-4 px-5">
						<div class="flex flex-col items-center gap-1">
							<h1 class="w-xs text-center text-3xl font-semibold md:w-full">
								What would you like to work on?
							</h1>
							<p class="text-base-content/50 text-md text-center font-light">
								Choose an entry point or pick up where you left off.
							</p>
						</div>
						<div class="grid grid-cols-1 items-stretch gap-4 md:grid-cols-3">
							<button
								class="bg-base-200 dark:border-base-300 rounded-field col-span-1 h-full p-4 text-left shadow-sm"
								onclick={() => {
									chat?.sendMessage('I want to design an AI workflow. Help me get started.');
								}}
							>
								<Sparkles class="mb-4 size-5" />
								<h3 class="text-base font-semibold">Create a workflow</h3>
								<p class="text-base-content/50 text-sm font-light">
									Design and execute an agentic workflow through conversation
								</p>
							</button>
							<button
								class="bg-base-200 dark:border-base-300 rounded-field col-span-1 h-full p-4 text-left shadow-sm"
								onclick={() => {
									chat?.sendMessage(
										'I want you to perform deep research on a topic. Help me get started.'
									);
								}}
							>
								<Binoculars class="mb-4 size-5" />
								<h3 class="text-base font-semibold">Deep research a topic</h3>
								<p class="text-base-content/50 text-sm font-light">
									Get a thorough, evidence-backed report on complex topics
								</p>
							</button>
							<button
								class="bg-base-200 dark:border-base-300 rounded-field col-span-1 h-full p-4 text-left shadow-sm"
								onclick={() => {
									chat?.sendMessage(
										'Help me understand what you can do. Explain your capabilities and suggest a few things we could try.'
									);
								}}
							>
								<MessageCircle class="mb-4 size-5" />
								<h3 class="text-base font-semibold">Just explore</h3>
								<p class="text-base-content/50 min-h-[2lh] text-sm font-light">
									Learn what Nanobot can do and take it from there
								</p>
							</button>
						</div>
					</div>
				{/snippet}
			</Thread>
		{/key}
	</div>
</div>

<Confirm
	show={showRestartConfirm}
	onsuccess={handleRestart}
	oncancel={() => (showRestartConfirm = false)}
	loading={restarting}
	title="Restart Agent"
	msg="Are you sure you want to restart this agent?"
	type="info"
>
	{#snippet note()}
		This will restart the current agent with the latest available version. Are you sure you want to
		continue?
	{/snippet}
</Confirm>
