<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { Check, X } from 'lucide-svelte';
	import { ChatService, type ProjectInvitation } from '$lib/services';
	import { formatTimeAgo } from '$lib/time';
	import Notifications from '$lib/components/Notifications.svelte';
	import { darkMode } from '$lib/stores';

	interface PageData {
		invitation: ProjectInvitation;
	}

	let props = $props<{ data: PageData }>();
	const invitation: ProjectInvitation = props.data.invitation;

	let isProcessing = $state(false);
	let responseMessage = $state('');
	let responseError = $state(false);
	let processed = $state(false);

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
			processed = true;
			responseMessage = 'Invitation accepted! Redirecting to project...';

			// Redirect to the project after a short delay
			setTimeout(() => {
				if (result.project) {
					goto(`/o/${result.project.id}`);
				} else {
					goto('/');
				}
			}, 1500);
		} catch (error) {
			console.error('Error accepting invitation:', error);
			responseError = true;
			responseMessage = 'Failed to accept invitation. Please try again.';
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
			processed = true;
			responseMessage = 'Invitation rejected.';
		} catch (error) {
			console.error('Error rejecting invitation:', error);
			responseError = true;
			responseMessage = 'Failed to reject invitation. Please try again.';
		} finally {
			isProcessing = false;
		}
	}
</script>

<div class="flex h-full flex-col">
	<header class="bg-surface1 border-surface2 sticky top-0 z-40 border-b">
		<div class="mx-auto flex h-16 max-w-7xl items-center justify-between px-4 sm:px-6 lg:px-8">
			<a href="/" class="flex items-center gap-2 text-xl font-semibold">
				{#if darkMode.isDark}
					<img src="/user/images/obot-logo-blue-white-text.svg" class="h-12" alt="Obot logo" />
				{:else}
					<img src="/user/images/obot-logo-blue-black-text.svg" class="h-12" alt="Obot logo" />
				{/if}
			</a>
		</div>
	</header>

	<main
		class="colors-background mx-auto flex w-full max-w-3xl grow flex-col items-center justify-center px-4 py-8"
	>
		{#if invitation.status !== 'pending'}
			<div class="dark:bg-surface1 w-full max-w-lg rounded-xl bg-white p-8 shadow-md">
				<h1 class="mb-4 text-2xl font-bold">Invitation {invitation.status}</h1>
				<p class="mb-6">This invitation has already been {invitation.status}.</p>
				<div class="flex justify-center">
					<a href="/" class="rounded-md bg-blue-600 px-4 py-2 text-white hover:bg-blue-700">
						Go Home
					</a>
				</div>
			</div>
		{:else if processed}
			<div class="dark:bg-surface1 w-full max-w-lg rounded-xl bg-white p-8 shadow-md">
				<h1 class="mb-4 text-2xl font-bold">
					{#if responseError}
						Error Processing Invitation
					{:else}
						Invitation Processed
					{/if}
				</h1>
				<p class="mb-6 {responseError ? 'text-red-500' : ''}">{responseMessage}</p>
				<div class="flex justify-center">
					<a href="/" class="rounded-md bg-blue-600 px-4 py-2 text-white hover:bg-blue-700">
						Go Home
					</a>
				</div>
			</div>
		{:else}
			<div class="dark:bg-surface1 w-full max-w-lg rounded-xl bg-white p-8 shadow-md">
				<h1 class="mb-4 text-2xl font-bold">Project Invitation</h1>
				<div class="mb-6">
					<p class="mb-2">
						You've been invited to join <strong>{projectName}</strong>
					</p>
					{#if projectDescription}
						<p class="mb-4 text-sm text-gray-600 dark:text-gray-400">
							{projectDescription}
						</p>
					{/if}
					<p class="text-xs text-gray-500">
						Invitation sent on {invitationDate}
					</p>
				</div>
				<div class="flex justify-center gap-4">
					<button
						class="bg-primary text-primary-foreground hover:bg-primary/90 flex items-center gap-2 rounded-md px-4 py-2 disabled:opacity-50"
						disabled={isProcessing}
						onclick={acceptInvitation}
					>
						<Check class="size-5" />
						Accept
					</button>
					<button
						class="flex items-center gap-2 rounded-md bg-red-600 px-4 py-2 text-white hover:bg-red-700 disabled:opacity-50"
						disabled={isProcessing}
						onclick={rejectInvitation}
					>
						<X class="size-5" />
						Reject
					</button>
				</div>
				{#if responseMessage}
					<p class="mt-4 text-center {responseError ? 'text-red-500' : 'text-green-500'}">
						{responseMessage}
					</p>
				{/if}
			</div>
		{/if}
	</main>

	<Notifications />
</div>
