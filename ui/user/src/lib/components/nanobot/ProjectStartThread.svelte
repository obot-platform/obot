<script lang="ts">
	import type { ChatService } from '$lib/services/nanobot/chat/index.svelte';
	import Thread from '$lib/components/nanobot/Thread.svelte';
	import FileEditor from './FileEditor.svelte';
	import type { Snippet } from 'svelte';

	interface Props {
		chat: ChatService;
		onToggleSidebar: (open: boolean) => void;
		initialContent?: Snippet;
	}

	let { chat, onToggleSidebar, initialContent }: Props = $props();

	let selectedFile = $state('');
	let drawerInput = $state<HTMLInputElement | null>(null);
</script>

<div class="flex h-full w-full">
	<div class="h-full min-w-0 flex-1">
		{#key chat.chatId}
			<Thread
				messages={chat.messages}
				prompts={chat.prompts}
				resources={chat.resources}
				elicitations={chat.elicitations}
				agents={chat.agents}
				selectedAgentId={chat.selectedAgentId}
				onAgentChange={chat.selectAgent}
				onElicitationResult={chat.replyToElicitation}
				onSendMessage={chat.sendMessage}
				onFileUpload={chat.uploadFile}
				onFileOpen={(filename) => {
					onToggleSidebar(false);
					drawerInput?.click();
					selectedFile = filename;
				}}
				cancelUpload={chat.cancelUpload}
				uploadingFiles={chat.uploadingFiles}
				uploadedFiles={chat.uploadedFiles}
				isLoading={chat.isLoading}
				agent={chat.agent}
			>
				{#snippet emptyStateContent()}
					{@render initialContent?.()}
				{/snippet}
			</Thread>
		{/key}
	</div>

	{#if selectedFile}
		<FileEditor
			filename={selectedFile}
			{chat}
			onClose={() => {
				selectedFile = '';
				onToggleSidebar(true);
			}}
		/>
	{/if}
</div>
