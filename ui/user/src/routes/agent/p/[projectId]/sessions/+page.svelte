<script lang="ts">
	import Threads from '$lib/components/nanobot/Threads.svelte';
	import { errors } from '$lib/stores';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import { goto } from '$lib/url';

	let selectedSessionId = $state<string | undefined>(undefined);

	async function handleRenameSession(sessionId: string, newTitle: string) {
		if (!$nanobotChat?.api) {
			errors.append(new Error('Nanobot API not found'));
			return;
		}
		try {
			await $nanobotChat.api.renameSession(sessionId, newTitle);
			nanobotChat.update((data) => {
				if (!data) return data;
				const sessionIndex = data.sessions.findIndex((s) => s.id === sessionId);
				if (sessionIndex === -1) return data;
				return {
					...data,
					sessions: data.sessions.map((s, i) =>
						i === sessionIndex ? { ...s, title: newTitle } : s
					)
				};
			});
		} catch (error) {
			console.error('Failed to rename thread:', error);
		}
	}

	async function handleDeleteSession(sessionId: string) {
		if (!$nanobotChat?.api) {
			errors.append(new Error('Nanobot API not found'));
			return;
		}
		const isCurrentViewedSession = selectedSessionId === sessionId;
		try {
			await $nanobotChat.api.deleteSession(sessionId);
			nanobotChat.update((data) => {
				if (data) {
					data.sessions = data.sessions.filter((s) => s.id !== sessionId);
					if (data.sessionId === sessionId) {
						data.sessionId = undefined;

						if (data.chat) {
							data.chat.close();
							data.chat = undefined;
						}
					}
				}
				return data;
			});

			if (isCurrentViewedSession) {
				goto(`/agent`, { replaceState: true });
			}
		} catch (error) {
			console.error('Failed to delete thread:', error);
		}
	}

	function handleCreateSession() {
		nanobotChat.update((data) => {
			if (data) {
				data.sessionId = undefined;
			}
			return data;
		});
		goto(`/agent`);
	}
</script>

<div class="mx-auto flex w-full max-w-4xl flex-col gap-6 overflow-x-hidden px-2">
	<div class="flex items-center gap-1 px-2">
		<h2 class="text-xl font-semibold md:text-2xl">Sessions</h2>
	</div>
	<Threads
		sessions={$nanobotChat?.sessions ?? []}
		onRename={handleRenameSession}
		onDelete={handleDeleteSession}
		onCreateSession={handleCreateSession}
		isLoading={$nanobotChat?.isThreadsLoading ?? false}
		{selectedSessionId}
	/>
</div>

<svelte:head>
	<title>Obot | Sessions</title>
</svelte:head>
