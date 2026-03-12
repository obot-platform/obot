<script lang="ts">
	import Thread from '$lib/components/nanobot/Thread.svelte';
	import { MessageCircle, Sparkles } from 'lucide-svelte';
	import type { ChatSession } from '$lib/services/nanobot/chat/index.svelte';

	interface Props {
		agentId: string;
		projectId: string;
		chat: ChatSession;
		browserBaseUrl?: string;
		browserViewerOpen?: boolean;
		onFileOpen?: (filename: string) => void;
		suppressEmptyState?: boolean;
		onThreadContentWidth?: (width: number) => void;
	}

	let {
		chat,
		browserBaseUrl = '',
		browserViewerOpen = $bindable(false),
		onFileOpen,
		suppressEmptyState,
		onThreadContentWidth
	}: Props = $props();
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
				onRefreshResources={() => {
					chat.refreshResources();
				}}
				{onFileOpen}
				{browserBaseUrl}
				bind:browserViewerOpen
				onReadResource={chat.readResource}
				{suppressEmptyState}
				onContentWidthChange={onThreadContentWidth}
			>
				{#snippet emptyStateContent()}
					<div class="flex flex-col items-center gap-4 px-5 pb-5 md:pb-0">
						<div class="flex flex-col items-center gap-1">
							<h1 class="w-xs text-center text-3xl font-semibold md:w-full">
								What would you like to work on?
							</h1>
							<p class="text-base-content/50 text-md text-center font-light">
								Choose an entry point or begin a conversation to get started.
							</p>
						</div>
						<div class="flex w-full flex-col items-center justify-center gap-4 md:flex-row">
							<button
								class="bg-base-200 dark:border-base-300 rounded-field hover:bg-base-300 col-span-1 h-full p-4 text-left shadow-sm md:w-70"
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
								class="bg-base-200 dark:border-base-300 rounded-field hover:bg-base-300 col-span-1 h-full p-4 text-left shadow-sm md:w-70"
								onclick={() => {
									chat?.sendMessage(
										'Help me understand what you can do. Explain your capabilities and suggest a few things we could try.'
									);
								}}
							>
								<MessageCircle class="mb-4 size-5" />
								<h3 class="text-base font-semibold">Just explore</h3>
								<p class="text-base-content/50 min-h-[2lh] text-sm font-light">
									Learn what the agent can do and take it from there
								</p>
							</button>
						</div>
					</div>
				{/snippet}
			</Thread>
		{/key}
	</div>
</div>
