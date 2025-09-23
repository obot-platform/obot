<script lang="ts">
	import { Trash2, Plus, Clock, X, Crown } from 'lucide-svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { browser } from '$app/environment';
	import {
		ChatService,
		type Project,
		type ProjectInvitation,
		type ProjectMember
	} from '$lib/services';
	import { profile, responsive } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import { clickOutside } from '$lib/actions/clickoutside';
	import { dialogAnimation } from '$lib/actions/dialogAnimation';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
	let invitations = $state<ProjectInvitation[]>([]);
	let invitation = $state<ProjectInvitation | null>(null);
	let isLoading = $state(false);
	let isCreating = $state(false);
	let ownerID = $state<string>('');
	let isOwnerOrAdmin = $derived(profile.current.id === ownerID || profile.current.isAdmin?.());
	let invitationUrl = $derived(
		browser && invitation?.code
			? `${window.location.protocol}//${window.location.host}/i/${invitation.code}`
			: ''
	);
	let deleteInvitationCode = $state('');
	let invitationDialog = $state<HTMLDialogElement>();
	let members = $state<ProjectMember[]>([]);
	let toDelete = $state('');

	async function createInvitation() {
		if (!isOwnerOrAdmin || isCreating) return;

		isCreating = true;
		try {
			invitation = await ChatService.createProjectInvitation(project.assistantID, project.id);
			await loadInvitations();
			invitationDialog?.showModal();
		} catch (error) {
			console.error('Error creating invitation:', error);
		} finally {
			isCreating = false;
		}
	}

	async function loadMembers() {
		members = await ChatService.listProjectMembers(project.assistantID, project.id);
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

	$effect(() => {
		if (project) {
			ownerID = project.userID;
			if (isOwnerOrAdmin) {
				loadInvitations();
				loadMembers();
			}
		}
	});
</script>

<div class="flex w-full flex-col items-center">
	<div class="flex w-full items-center p-4">
		<div class="mx-auto flex w-full flex-col gap-4 md:max-w-[1200px]">
			<h1 class="text-2xl font-semibold">Manage Project Members</h1>

			<h2 class="text-xl font-semibold">Members</h2>
			<div class="dark:bg-gray-980 flex flex-col gap-2 rounded-md bg-gray-50 p-2 shadow-inner">
				{#each members as member (member.userID)}
					<div
						class="group dark:bg-surface1 dark:border-surface3 flex w-full items-center rounded-md bg-white p-2 shadow-sm dark:border"
					>
						<div class="flex grow items-center gap-2">
							<div class="size-10 overflow-hidden rounded-full bg-gray-50 dark:bg-gray-600">
								<img
									src={member.iconURL}
									class="h-full w-full object-cover"
									alt="agent member icon"
									referrerpolicy="no-referrer"
								/>
							</div>
							<div class="flex flex-col">
								<p class="flex items-center gap-1 truncate text-left text-base font-light">
									{member.email}
									{#if member.isOwner}
										<Crown class="size-4" />
									{/if}
									{#if member.email === profile.current.email}
										<span class="text-xs text-gray-500">(Me)</span>
									{/if}
								</p>
								<span class="text-sm font-light text-gray-500">
									{member.isOwner ? 'Owner' : 'Member'}
								</span>
							</div>
						</div>
						{#if isOwnerOrAdmin && profile.current.email !== member.email && !member.isOwner}
							<button
								class="button-destructive"
								onclick={() => (toDelete = member.email)}
								use:tooltip={'Remove member'}
							>
								<Trash2 class="size-4" />
							</button>
						{/if}
					</div>
				{/each}
			</div>

			<div class="mt-8 flex items-center justify-between">
				<h2 class="text-xl font-semibold">Project Invitations</h2>
				{#if isOwnerOrAdmin}
					<button
						class="button flex items-center gap-1 text-sm"
						onclick={createInvitation}
						disabled={isCreating}
					>
						<Plus class="size-4" />
						New Invite
					</button>
				{/if}
			</div>
		</div>
	</div>
	<div class="dark:bg-gray-980 flex w-full grow items-center bg-gray-50 p-4">
		<div class="mx-auto flex w-full flex-col self-start md:max-w-[1200px]">
			{#if !isOwnerOrAdmin}
				<p class="p-4 text-center text-gray-500">Only project owners can manage invitations.</p>
			{:else if isLoading}
				<div class="flex grow items-center justify-center">
					<div
						class="size-6 animate-spin rounded-full border-2 border-gray-300 border-t-blue-600"
					></div>
				</div>
			{:else if invitations.length === 0}
				<p class="p-4 text-center text-gray-500">No invitations found</p>
			{:else}
				<ul class="flex flex-col gap-4">
					{#each invitations as invitation (invitation.code)}
						<li
							class="dark:bg-surface1 dark:border-surface3 flex items-center justify-between gap-4 rounded-md bg-white p-4 shadow-sm dark:border"
						>
							<div class="flex grow flex-col gap-2 md:gap-1">
								<div class="line-clamp-1 overflow-x-auto text-sm font-medium break-all">
									{invitation.code}
								</div>
								<div class="flex flex-shrink-0 gap-4">
									<span
										class={twMerge(
											'inline-flex rounded-lg border px-2 py-0.5 text-xs leading-5 font-semibold whitespace-nowrap capitalize dark:opacity-75',
											invitation.status === 'pending' && 'border-yellow-500 text-yellow-500',
											invitation.status === 'accepted' && 'border-green-500 text-green-500',
											invitation.status === 'rejected' && 'border-red-500 text-red-500',
											invitation.status === 'expired' && 'border-gray-500 text-gray-500'
										)}
									>
										{invitation.status}
									</span>
									<div class="bg-surface2 dark:bg-surface3 h-6 w-[1px]"></div>
									<div class="flex items-center gap-2 text-xs text-gray-500">
										<Clock class="size-3.5" />
										<span>{formatTimeAgo(invitation.created).relativeTime}</span>
									</div>
								</div>
								{#if invitation.status === 'pending' && responsive.isMobile}
									<CopyButton
										text={`${window.location.protocol}//${window.location.host}/i/${invitation.code}`}
										buttonText="Copy Invite Link"
										classes={{ button: 'w-fit' }}
									/>
								{/if}
							</div>
							<div class="flex flex-shrink-0 gap-4 self-start md:self-center">
								{#if invitation.status === 'pending' && !responsive.isMobile}
									<CopyButton
										text={`${window.location.protocol}//${window.location.host}/i/${invitation.code}`}
										buttonText="Copy Invite Link"
									/>
								{/if}
								<button
									class="button-destructive flex-shrink-0"
									onclick={() => (deleteInvitationCode = invitation.code)}
								>
									<Trash2 class="size-4" />
								</button>
							</div>
						</li>
					{/each}
				</ul>
			{/if}
		</div>
	</div>
</div>

<Confirm
	msg={`Remove ${toDelete} from your project?`}
	show={!!toDelete}
	onsuccess={async () => {
		if (!toDelete) return;
		try {
			const memberToDelete = members.find((m) => m.email === toDelete);
			if (memberToDelete && isOwnerOrAdmin) {
				await ChatService.deleteProjectMember(
					project.assistantID,
					project.id,
					memberToDelete.userID
				);
				await loadMembers();
			}
		} finally {
			toDelete = '';
		}
	}}
	oncancel={() => (toDelete = '')}
/>

<dialog
	use:dialogAnimation={{ type: 'fade' }}
	bind:this={invitationDialog}
	use:clickOutside={() => invitationDialog?.close()}
	class="default-dialog relative w-lg p-4 py-8"
	class:mobile-screen-dialog={responsive.isMobile}
>
	<button
		class="icon-button absolute top-2 right-2 z-40 float-right self-end"
		onclick={() => invitationDialog?.close()}
		use:tooltip={{ disablePortal: true, text: 'Close Project Catalog' }}
	>
		<X class="size-6" />
	</button>

	<div class="flex flex-col items-center gap-4">
		<img src="/user/images/sharing-agent.webp" alt="invitation" />
		<h4 class="text-2xl font-semibold">Your Project <i>Invite</i> Link</h4>
		<p class="text-md max-w-md text-center leading-6 font-light">
			Copy the invitation link below and share with your colleagues to get started collaborating on
			this project!
		</p>
		<CopyButton
			text={invitationUrl}
			buttonText="Copy Invite Link"
			classes={{ button: 'text-md px-6 gap-2' }}
		/>
		<span class="line-clamp-1 text-xs break-all text-gray-500">{invitationUrl}</span>
	</div>
</dialog>

<Confirm
	msg="Delete this invitation?"
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
