<script lang="ts">
	import type { ChatSession } from '$lib/services/nanobot/chat/index.svelte';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import { onDestroy, type Snippet } from 'svelte';

	interface Props {
		publishedArtifactId: string;
		onClose: () => void;
		onSuccess: () => void;
		children?: Snippet;
		title: string;
	}

	let { publishedArtifactId, onClose, onSuccess, children, title }: Props = $props();
	let chat = $state<ChatSession | undefined>(undefined);
	let loading = $state(false);
	let action = $state<'update' | 'cancel'>();

	let elicitation = $derived(chat?.elicitations[chat?.elicitations.length - 1] ?? undefined);

	$effect(() => {
		if (!$nanobotChat?.api || chat) {
			return;
		}
		$nanobotChat.api.getSession($nanobotChat.api.sessionId).then((existingSession) => {
			chat = existingSession;
			chat.installArtifact(publishedArtifactId).then((response) => {
				if (response.message.includes('Installed')) {
					onSuccess?.();
				}
			});
		});
	});

	onDestroy(() => {
		chat?.close();
	});

	$effect(() => {
		if (chat && !chat.isLoading && !elicitation && loading) {
			loading = false;
			if (action === 'update') {
				onSuccess?.();
			} else {
				onClose();
			}
		}
	});

	async function cancel() {
		loading = true;
		action = 'cancel';
		if (elicitation) {
			await chat?.replyToElicitation(elicitation, { action: 'decline' });
		}
	}

	async function update() {
		loading = true;
		action = 'update';
		if (elicitation) {
			await chat?.replyToElicitation(elicitation, { action: 'accept' });
		}
	}
</script>

<dialog class="modal-open modal">
	<div class="modal-box dialog-container w-full max-w-md">
		<form method="dialog">
			<button class="btn btn-circle btn-ghost btn-sm absolute top-2 right-2" onclick={cancel}
				>✕</button
			>
		</form>
		<h3 class="text-lg font-bold">{title}</h3>
		{#if elicitation}
			{#if children}
				{@render children?.()}
			{:else}
				<p class="my-4 text-sm">
					{elicitation.message}
				</p>
			{/if}
			<div class="modal-action">
				<button class="btn btn-primary" disabled={loading} onclick={update}>
					{#if loading}
						<div class="loading loading-spinner loading-xs text-white"></div>
					{:else}
						Update
					{/if}
				</button>
				<button class="btn btn-error" disabled={loading} onclick={cancel}>Cancel</button>
			</div>
		{:else}
			<div class="my-4 flex w-full items-center justify-center">
				<div class="loading loading-spinner loading-lg text-primary"></div>
			</div>
		{/if}
	</div>
</dialog>
