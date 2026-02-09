<script lang="ts">
	import type { ChatService } from '$lib/services/nanobot/chat/index.svelte';
	import Thread from '$lib/components/nanobot/Thread.svelte';
	import FileEditor from './FileEditor.svelte';

	interface Props {
		chat: ChatService;
		onToggleSidebar: (open: boolean) => void;
	}

	let { chat, onToggleSidebar }: Props = $props();

	let selectedFile = $state('');
	let drawerInput = $state<HTMLInputElement | null>(null);

	const onFileOpenRef = $state.raw({ current: (_filename: string) => {} });
	const handleFileOpen = $state.raw((filename: string) => onFileOpenRef.current(filename));
	$effect(() => {
		onFileOpenRef.current = (filename: string) => {
			onToggleSidebar(false);
			drawerInput?.click();
			selectedFile = filename;
		};
	});
</script>

<div class="flex h-full w-full">
	<div class="h-full min-w-0 flex-1">
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
			onFileOpen={handleFileOpen}
			cancelUpload={chat.cancelUpload}
			uploadingFiles={chat.uploadingFiles}
			uploadedFiles={chat.uploadedFiles}
			isLoading={chat.isLoading}
			isRestoring={chat.isRestoring}
			agent={chat.agent}
		/>
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
