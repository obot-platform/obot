<script lang="ts">
	import Thread from '$lib/components/nanobot/Thread.svelte';
	import { m } from '$lib/i18n';
	import type { ChatSession } from '$lib/services/nanobot/chat/index.svelte';
	import { MessageCircle, Sparkles } from '@lucide/svelte';

	interface Props {
		agentId: string;
		projectId: string;
		chat: ChatSession;
		browserBaseUrl?: string;
		browserAvailable?: boolean;
		browserViewerOpen?: boolean;
		onFileOpen?: (filename: string) => void;
		suppressEmptyState?: boolean;
		onThreadContentWidth?: (width: number) => void;
		classes?: {
			root?: string;
		};
	}

	let {
		chat,
		browserBaseUrl = '',
		browserAvailable = false,
		browserViewerOpen = $bindable(false),
		onFileOpen,
		suppressEmptyState,
		onThreadContentWidth,
		classes
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
				{browserAvailable}
				bind:browserViewerOpen
				onReadResource={chat.readResource}
				{suppressEmptyState}
				onContentWidthChange={onThreadContentWidth}
				{classes}
			>
				{#snippet emptyStateContent()}
					<div class="flex flex-col items-center gap-4 px-5 pb-5 md:pb-0">
						<div class="flex flex-col items-center gap-1">
							<h1 class="w-full text-center text-xl font-semibold md:text-3xl">
								{m.chat_start_heading()}
							</h1>
							<p class="text-muted-content md:text-md text-center text-sm font-light">
								{m.chat_start_subheading()}
							</p>
						</div>
						<div class="flex w-full flex-col items-center justify-center gap-4 md:flex-row">
							<button
								class="bg-base-200 dark:border-base-300 rounded-field hover:bg-base-300 col-span-1 h-full w-full p-4 text-left shadow-sm md:w-70"
								onclick={() => {
									chat?.sendMessage('I want to design an AI workflow. Help me get started.');
								}}
							>
								<Sparkles class="mb-4 size-5" />
								<h3 class="text-base font-semibold">{m.chat_start_create_workflow()}</h3>
								<p class="text-muted-content text-sm font-light">
									{m.chat_start_create_workflow_desc()}
								</p>
							</button>
							<button
								class="bg-base-200 dark:border-base-300 rounded-field hover:bg-base-300 col-span-1 h-full w-full p-4 text-left shadow-sm md:w-70"
								onclick={() => {
									chat?.sendMessage(
										'Help me understand what you can do. Explain your capabilities and suggest a few things we could try.'
									);
								}}
							>
								<MessageCircle class="mb-4 size-5" />
								<h3 class="text-base font-semibold">{m.chat_start_just_explore()}</h3>
								<p class="text-muted-content min-h-[2lh] text-sm font-light">
									{m.chat_start_just_explore_desc()}
								</p>
							</button>
						</div>
					</div>
				{/snippet}
			</Thread>
		{/key}
	</div>
</div>
