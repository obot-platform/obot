<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { Check, X } from 'lucide-svelte';
	import { ChatService, type ProjectInvitation } from '$lib/services';
	import { formatTimeAgo } from '$lib/time';
	import Notifications from '$lib/components/Notifications.svelte';
	import { darkMode } from '$lib/stores';
	import { getProjectImage } from '$lib/image';

	interface PageData {
		invitation: ProjectInvitation;
	}

	let props = $props<{ data: PageData }>();
	const invitation: ProjectInvitation = props.data.invitation;

	let isProcessing = $state(false);
	let responseMessage = $state('');
	let responseError = $state(false);
	let view = $state<'rejected' | 'joined'>();
	// Format the invitation date nicely
	let invitationDate = $derived(
		invitation.created ? formatTimeAgo(invitation.created).fullDate : 'Unknown date'
	);

	// Get project details
	let projectName = $derived(invitation.project?.name || 'My Agent');
	let projectDescription = $derived(invitation.project?.description || 'No description available');

	async function acceptInvitation() {
		if (isProcessing) return;

		isProcessing = true;
		responseMessage = '';
		responseError = false;

		try {
			const result = await ChatService.acceptProjectInvitation(page.params.code);
			responseMessage = 'Invitation accepted! Redirecting to project...';
			if (result.project) {
				goto(`/o/${result.project.id}`);
			} else {
				goto('/');
			}
		} catch (error) {
			if (
				error instanceof Error &&
				error.message.includes('you are already a member of this project')
			) {
				view = 'joined';
			} else {
				console.error('Error accepting invitation:', error);
				responseError = true;
				responseMessage = 'Failed to accept invitation. Please try again.';
			}
		} finally {
			isProcessing = false;
		}
	}

	async function rejectInvitation() {
		if (isProcessing) return;

		isProcessing = true;
		responseMessage = '';
		responseError = false;

		try {
			await ChatService.rejectProjectInvitation(page.params.code);
			responseMessage = 'Invitation rejected.';
			view = 'rejected';
		} catch (error) {
			console.error('Error rejecting invitation:', error);
			responseError = true;
			responseMessage = 'Failed to reject invitation. Please try again.';
		} finally {
			isProcessing = false;
		}
	}
</script>

<div class="flex min-h-svh flex-col">
	<header class="bg-surface1 border-surface2 sticky top-0 z-40 border-b">
		<div class="colors-background sticky top-0 z-30 flex h-16 w-full items-center">
			<div class="relative flex items-end p-5">
				{#if darkMode.isDark}
					<img src="/user/images/obot-logo-blue-white-text.svg" class="h-12" alt="Obot logo" />
				{:else}
					<img src="/user/images/obot-logo-blue-black-text.svg" class="h-12" alt="Obot logo" />
				{/if}
				<div class="ml-1.5 -translate-y-1">
					<span
						class="rounded-full border-2 border-blue-400 px-1.5 py-[1px] text-[10px] font-bold text-blue-400 dark:border-blue-400 dark:text-blue-400"
					>
						BETA
					</span>
				</div>
			</div>
		</div>
	</header>

	<main
		class="bg-surface1 dark:bg-gray-980 flex w-full grow flex-col items-center justify-center px-4 py-8"
	>
		{#if invitation.status !== 'pending'}
			<div
				class="dark:bg-surface1 dark:border-surface3 w-full max-w-lg rounded-xl bg-white p-8 shadow-md dark:border"
			>
				<div class="flex flex-col items-center gap-4">
					<img src="/user/images/sharing-agent-expired.webp" alt="invitation" />
					<h1 class="text-2xl font-semibold"><i>Expired</i> Invitation Link</h1>
					<h2 class="w-sm text-center text-lg font-extralight">
						Oh no! Looks like this invitation can no longer be accepted.
					</h2>
					<p class="text-md leading-6 font-light">
						Please contact the project owner to get access to the project. If you think this is an
						error, please contact support.
					</p>
					<div class="mt-4 flex w-full justify-center">
						<a href="/" class="button-primary w-full rounded-full p-2 px-6 text-center">
							Go Home
						</a>
					</div>
				</div>
			</div>
		{:else if view === 'rejected'}
			<div
				class="dark:bg-surface1 dark:border-surface3 w-full max-w-lg rounded-xl bg-white p-8 shadow-md dark:border"
			>
				<div class="flex flex-col items-center gap-4">
					<img src="/user/images/sharing-agent-expired.webp" alt="invitation" />
					<h1 class="text-2xl font-semibold">Rejected <i>Invitation</i> Link</h1>
					<h2 class="w-sm text-center text-lg font-extralight">
						You've rejected the invitation to join <strong class="font-semibold"
							>{projectName}</strong
						>.
					</h2>
					<p class="text-md text-center leading-6 font-light">Thank you for your response!</p>
					<div class="mt-4 flex w-full justify-center">
						<a href="/" class="button-primary w-full rounded-full p-2 px-6 text-center">
							Go Home
						</a>
					</div>
				</div>
			</div>
		{:else if view === 'joined'}
			<div
				class="dark:bg-surface1 dark:border-surface3 w-full max-w-lg rounded-xl bg-white p-8 shadow-md dark:border"
			>
				<div class="flex flex-col items-center gap-4">
					<img src="/user/images/sharing-agent.webp" alt="invitation" />
					<h1 class="text-2xl font-semibold">Your Agent <i>Invitation</i> Link</h1>
					<h2 class="w-sm text-center text-lg font-extralight">
						You're already a member of <strong class="font-semibold">{projectName}</strong>!
					</h2>
					<p class="text-md text-center leading-6 font-light">
						Good news! You already have access! Click the link below to get started on or continue
						collaborating on this agent.
					</p>
					<div class="mt-4 flex w-full justify-center">
						<a
							href="/o/{invitation.project?.id}"
							class="button-primary w-full rounded-full p-2 px-6 text-center"
						>
							Go To Agent
						</a>
					</div>
					<div class="flex w-full justify-center">
						<a href="/home" class="button w-full rounded-full p-2 px-6 text-center"> Go Home </a>
					</div>
				</div>
			</div>
		{:else}
			<div
				class="dark:bg-surface1 dark:border-surface3 w-full max-w-lg rounded-xl bg-white p-8 text-center shadow-md dark:border"
			>
				<div class="flex flex-col items-center gap-4">
					<img src="/user/images/sharing-agent.webp" alt="invitation" />
					<h1 class="text-2xl font-semibold">Your Agent <i>Invitation</i> Link</h1>
					<h2 class="max-w-sm text-center text-lg font-extralight">
						You've been invited to join <strong class="font-semibold">{projectName}</strong>!
					</h2>
					{#if invitation.project}
						<div
							class="bg-surface1 dark:bg-surface2 flex w-full max-w-xs flex-col items-center gap-4 rounded-xl p-4 text-center"
						>
							<img
								src={getProjectImage(invitation.project, darkMode.isDark)}
								alt="project icon"
								class="size-16 rounded-full"
							/>
							{#if projectDescription}
								<p class="text-md text-gray-600 dark:text-gray-400">
									{projectDescription}
								</p>
							{/if}
						</div>
					{/if}
					<p class="text-xs text-gray-500">
						Invitation sent on {invitationDate}
					</p>
					<div class="mt-6 flex w-full justify-center gap-4">
						<button
							class="button-destructive text-md w-full justify-center rounded-full p-4 px-6 disabled:opacity-50"
							disabled={isProcessing}
							onclick={rejectInvitation}
						>
							<X class="size-5" />
							Reject
						</button>
						<button
							class="button dark:hover:bg-surface2 hover:bg-surface1 flex w-full items-center justify-center gap-1 rounded-full bg-transparent p-4 px-6 disabled:opacity-50"
							disabled={isProcessing}
							onclick={acceptInvitation}
						>
							<Check class="size-5" />
							Accept
						</button>
					</div>

					{#if responseMessage}
						<p class="mt-4 text-center {responseError ? 'text-red-500' : 'text-green-500'}">
							{responseMessage}
						</p>
					{/if}
				</div>
			</div>
		{/if}
	</main>

	<Notifications />
</div>

<svelte:head>
	<title>Obot | Agent Invitation</title>
</svelte:head>
