<script lang="ts">
	import { Trash2, Plus, Clock, X } from 'lucide-svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { browser } from '$app/environment';
	import { ChatService, type Project, type ProjectInvitation } from '$lib/services';
	import { profile } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import Confirm from '$lib/components/Confirm.svelte';

	interface Props {
		project: Project;
		inline?: boolean;
	}

	let { project, inline = false }: Props = $props();
	let invitations = $state<ProjectInvitation[]>([]);
	let invitation = $state<ProjectInvitation | null>(null);
	let isLoading = $state(false);
	let isCreating = $state(false);
	let ownerID = $state<string>('');
	let isOwnerOrAdmin = $derived(profile.current.id === ownerID || profile.current.role === 1);
	let invitationUrl = $derived(
		browser && invitation?.code
			? `${window.location.protocol}//${window.location.host}/i/${invitation.code}`
			: ''
	);
	let deleteInvitationCode = $state('');

	async function createInvitation() {
		if (!isOwnerOrAdmin || isCreating) return;

		isCreating = true;
		try {
			invitation = await ChatService.createProjectInvitation(project.assistantID, project.id);
			await loadInvitations();
		} catch (error) {
			console.error('Error creating invitation:', error);
		} finally {
			isCreating = false;
		}
	}

	async function loadInvitations() {
		if (!isOwnerOrAdmin) {
			invitations = [];
			return;
		}

		isLoading = true;
		try {
			invitations = await ChatService.listProjectInvitations(project.assistantID, project.id);
		} catch (error) {
			console.error('Error loading invitations:', error);
			invitations = [];
		} finally {
			isLoading = false;
		}
	}

	async function deleteInvitation(code: string) {
		if (!isOwnerOrAdmin) return;
		try {
			await ChatService.deleteProjectInvitation(project.assistantID, project.id, code);
			await loadInvitations();
		} catch (error) {
			console.error('Error deleting invitation:', error);
		}
	}

	function getStatusStyle(status: string): { backgroundColor: string; color: string } {
		const isDarkMode = document.documentElement.classList.contains('dark');

		switch (status.toLowerCase()) {
			case 'pending':
				return isDarkMode
					? { backgroundColor: '#713f12', color: '#fef08a' } // dark mode: yellow-900, yellow-300
					: { backgroundColor: '#fef9c3', color: '#854d0e' }; // light mode: yellow-100, yellow-800
			case 'accepted':
				return isDarkMode
					? { backgroundColor: '#14532d', color: '#86efac' } // dark mode: green-900, green-300
					: { backgroundColor: '#dcfce7', color: '#166534' }; // light mode: green-100, green-800
			case 'rejected':
				return isDarkMode
					? { backgroundColor: '#7f1d1d', color: '#fca5a5' } // dark mode: red-900, red-300
					: { backgroundColor: '#fee2e2', color: '#991b1b' }; // light mode: red-100, red-800
			case 'expired':
			default:
				return isDarkMode
					? { backgroundColor: '#374151', color: '#d1d5db' } // dark mode: gray-700, gray-300
					: { backgroundColor: '#f3f4f6', color: '#1f2937' }; // light mode: gray-100, gray-800
		}
	}

	$effect(() => {
		if (project) {
			ownerID = project.userID;
			if (isOwnerOrAdmin) {
				loadInvitations();
			}
		}
	});
</script>

<div class="flex h-full w-full flex-col overflow-hidden p-4 {inline ? '' : 'mx-auto max-w-3xl'}">
	<div class="mb-6 flex items-center justify-between">
		<h2 class="text-xl font-medium">Project Invitations</h2>
		{#if isOwnerOrAdmin}
			<button
				class="bg-surface3 hover:bg-surface4 flex items-center gap-1 rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
				onclick={createInvitation}
				disabled={isCreating}
			>
				<Plus class="size-4" />
				New Invitation
			</button>
		{/if}
	</div>

	{#if !isOwnerOrAdmin}
		<p class="p-4 text-center text-gray-500">Only project owners can manage invitations.</p>
	{:else if isLoading}
		<div class="flex grow items-center justify-center">
			<div
				class="size-6 animate-spin rounded-full border-2 border-gray-300 border-t-blue-600"
			></div>
		</div>
	{:else}
		{#if invitation}
			<div
				class="dark:bg-surface2 mb-6 flex w-full flex-col gap-2 rounded-xl bg-white p-4 shadow-sm"
			>
				<div class="flex items-center justify-between">
					<h3 class="text-sm font-medium">New Invitation Link</h3>
					<button class="text-gray-500 hover:text-gray-700" onclick={() => (invitation = null)}>
						<X class="size-4" />
					</button>
				</div>
				<p class="text-xs text-gray-500">
					Share this link with someone to invite them to join this project:
				</p>
				<div class="flex items-center gap-2">
					<CopyButton text={invitationUrl} />
					<div class="overflow-x-auto py-1 pr-2 text-sm break-all">
						{invitationUrl}
					</div>
				</div>
			</div>
		{/if}

		{#if invitations.length === 0}
			<p class="p-4 text-center text-gray-500">No invitations found</p>
		{:else}
			<div
				class="default-scrollbar-thin flex-1 overflow-y-auto rounded-md border border-gray-200 dark:border-gray-700"
			>
				<table class="w-full">
					<thead class="dark:bg-surface2 sticky top-0 bg-gray-50">
						<tr>
							<th
								class="w-2/3 px-4 py-2 text-left text-xs font-medium tracking-wider text-gray-500 uppercase"
								>Invitation URL</th
							>
							<th
								class="w-1/8 px-4 py-2 text-left text-xs font-medium tracking-wider text-gray-500 uppercase"
								>Status</th
							>
							<th
								class="w-1/8 px-4 py-2 text-left text-xs font-medium tracking-wider text-gray-500 uppercase"
								>Created</th
							>
							<th
								class="w-1/12 px-4 py-2 text-right text-xs font-medium tracking-wider text-gray-500 uppercase"
							></th>
						</tr>
					</thead>
					<tbody class="divide-y divide-gray-200 dark:divide-gray-700">
						{#each invitations as invitation}
							<tr class="dark:hover:bg-surface2 hover:bg-gray-50">
								<td class="px-4 py-3">
									<div class="flex items-center">
										<CopyButton
											text={`${window.location.protocol}//${window.location.host}/i/${invitation.code}`}
										/>
										<div class="ml-2 overflow-x-auto pr-2 text-sm font-medium break-all">
											{`${window.location.protocol}//${window.location.host}/i/${invitation.code}`}
										</div>
									</div>
								</td>
								<td class="px-4 py-3 whitespace-nowrap">
									<span
										class="inline-flex rounded-full px-2 py-1 text-xs leading-5 font-semibold whitespace-nowrap capitalize"
										style={`background-color: ${getStatusStyle(invitation.status).backgroundColor}; color: ${getStatusStyle(invitation.status).color};`}
									>
										{invitation.status}
									</span>
								</td>
								<td class="px-4 py-3 whitespace-nowrap">
									<div
										class="flex items-center gap-1 text-xs text-gray-500"
										title={formatTimeAgo(invitation.created).fullDate}
									>
										<Clock class="size-3.5" />
										<span>{formatTimeAgo(invitation.created).relativeTime}</span>
									</div>
								</td>
								<td class="px-4 py-3 text-right whitespace-nowrap">
									<button
										class="p-1 text-red-600 hover:text-red-900"
										onclick={() => (deleteInvitationCode = invitation.code)}
										use:tooltip={'Delete invitation'}
									>
										<Trash2 class="size-4" />
									</button>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	{/if}
</div>

<Confirm
	msg={`Delete this invitation?`}
	show={!!deleteInvitationCode}
	onsuccess={async () => {
		if (!deleteInvitationCode) return;
		try {
			await deleteInvitation(deleteInvitationCode);
		} finally {
			deleteInvitationCode = '';
		}
	}}
	oncancel={() => (deleteInvitationCode = '')}
/>
