<script lang="ts">
	import { Thread } from '$lib/services/chat/thread.svelte';
	import type { Messages, Project, ProjectCredential } from '$lib/services';
	import Message from '$lib/components/messages/Message.svelte';
	import { X } from 'lucide-svelte';
	import { responsive } from '$lib/stores';
	import ChatService from '$lib/services/chat';
	import { onMount } from 'svelte';

	interface Props {
		toolID: string;
		onClose?: (error?: boolean) => void | Promise<void>;
		project: Project;
		local?: boolean;
		credential?: ProjectCredential;
		inline?: boolean;
	}

	let { toolID, onClose, project, local, credential, inline }: Props = $props();
	let authMessages = $state<Messages>();
	let thread = $state<Thread>();
	let authDialog: HTMLDialogElement | undefined = $state();
	let inProgress = $state(false);

	export function show() {
		authCancel(true);
		auth(toolID);
		authDialog?.showModal();
		authDialog?.addEventListener('close', () => {
			authMessages = undefined;
		});
	}

	onMount(() => {
		if (inline) {
			auth(toolID);
		}
	});

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
				// if last message is an error, don't close the dialog
				let skipClose = (authMessages?.messages ?? []).find((msg) => msg.icon === 'Error');
				authCancel(skipClose ? true : false);

				// false means don't reconnect
				return false;
			}
		});
		t.onMessages = (messages) => {
			authMessages = messages;
		};
		thread = t;
	}

	function authCancel(skipClose?: boolean) {
		thread?.abort();
		thread?.close();
		inProgress = false;
		if (!(authMessages?.messages ?? []).find((msg) => msg.icon === 'Error')) {
			authMessages = undefined;
		}
		thread = undefined;
		if (!skipClose) {
			authDialog?.close();
			onClose?.();
		}
	}
</script>

{#if inline}
	<div class="flex flex-col gap-2">
		{@render content()}
	</div>
{:else}
	<dialog bind:this={authDialog} class="default-dialog w-full sm:max-w-lg">
		{@render content()}
	</dialog>
{/if}

{#snippet content()}
	{#if credential}
		<div class="flex flex-col">
			<h4
				class="default-dialog-title py-2 text-base md:pr-2 md:pl-5"
				class:default-dialog-mobile-title={responsive.isMobile}
			>
				<span class="flex items-center gap-2">
					<img
						src={credential.icon}
						class="size-6 rounded-md bg-white p-1"
						alt="credential {credential.toolName} icon"
					/>
					{credential.toolName}
				</span>
				<button
					class="icon-button"
					class:mobile-header-button={responsive.isMobile}
					onclick={() => authCancel()}
				>
					<X class="size-5" />
				</button>
			</h4>
			{#if inProgress}
				<div class="flex flex-col gap-5 p-4 md:m-5 md:mt-0 md:p-0">
					<p class="text-center">Sending credentials...</p>
				</div>
			{:else if authMessages}
				<div class="flex flex-col gap-5 p-4 md:m-5 md:mt-0 md:p-0">
					{#each authMessages.messages as msg}
						<Message
							{msg}
							{project}
							onSendCredentialsCancel={() => authCancel()}
							onSendCredentials={(id: string, credentials: Record<string, string>) => {
								ChatService.sendCredentials(id, credentials);
								inProgress = true;
							}}
							noMemoryTool
							classes={{
								messageIcon: 'hidden',
								nameAndTime: 'hidden',
								messageActions: 'hidden',
								root: 'w-full',
								container: 'grow',
								oauth: 'border border-blue-500 bg-blue-500/30',
								prompt: 'm-0'
							}}
						/>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
{/snippet}
