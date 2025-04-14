<script lang="ts">
	import { Thread } from '$lib/services/chat/thread.svelte';
	import type { Messages, Project } from '$lib/services';
	import Message from '$lib/components/messages/Message.svelte';

	interface Props {
		toolID: string;
		onClose?: () => void | Promise<void>;
		project: Project;
		local?: boolean;
	}

	let { toolID, onClose, project, local }: Props = $props();
	let authMessages = $state<Messages>();
	let thread = $state<Thread>();

	export function show() {
		authCancel();
		auth(toolID);
	}

	function auth(toolID: string) {
		const t = new Thread(project, {
			authenticate: {
				tools: [toolID],
				local
			},
			onError: () => {
				// ignore the error. This is so it doesn't get globally printed
			},
			onClose: () => {
				authCancel();
				onClose?.();
				// false means don't reconnect
				return false;
			}
		});
		t.onMessages = (messages) => {
			authMessages = messages;
		};
		thread = t;
	}

	function authCancel() {
		thread?.abort();
		thread?.close();
		// only clear message if nothing failed
		if (!(authMessages?.messages ?? []).find((msg) => msg.icon === 'Error')) {
			authMessages = undefined;
		}
		thread = undefined;
	}
</script>

{#if authMessages}
	<div class="flex flex-col gap-5 p-5">
		{#each authMessages.messages as msg}
			<Message {msg} {project} clearable onSendCredentialsCancel={() => authCancel()} />
		{/each}
	</div>
{/if}
