<script lang="ts">
	import { browser } from '$app/environment';
	import Confirm from '$lib/components/Confirm.svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import {
		ChatService,
		type Project,
		type ProjectInvitation,
		type ProjectMember
	} from '$lib/services';
	import { profile, responsive } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import { Trash2, Plus, Clock, X, Crown } from 'lucide-svelte';
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
	let invitationDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let members = $state<ProjectMember[]>([]);
	let toDelete = $state('');

	async function createInvitation() {
		if (!isOwnerOrAdmin || isCreating) return;

		isCreating = true;
		try {
			invitation = await ChatService.createProjectInvitation(project.assistantID, project.id);
			await loadInvitations();
			invitationDialog?.open();
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
						class="group dark:bg-base-200 dark:border-base-400 bg-base-100 flex w-full items-center rounded-md p-2 shadow-sm dark:border"
					>
						<div class="flex grow items-center gap-2">
							<div class="size-10 overflow-hidden rounded-full bg-base-200 dark:bg-base-300">
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
										<span class="text-muted-content text-xs">(Me)</span>
									{/if}
								</p>
								<span class="text-muted-content text-sm font-light">
									{member.isOwner ? 'Owner' : 'Member'}
								</span>
							</div>
						</div>
						{#if isOwnerOrAdmin && profile.current.email !== member.email && !member.isOwner}
							<IconButton
								variant="danger2"
								tooltip={{ text: 'Remove member' }}
								onclick={() => (toDelete = member.email)}
							>
								<Trash2 class="size-4" />
							</IconButton>
						{/if}
					</div>
				{/each}
			</div>

			<div class="mt-8 flex items-center justify-between">
				<h2 class="text-xl font-semibold">Project Invitations</h2>
				{#if isOwnerOrAdmin}
					<button
						class="btn btn-secondary flex items-center gap-1 text-sm"
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
				<p class="text-muted-content p-4 text-center">
					Only project owners can manage invitations.
				</p>
			{:else if isLoading}
				<div class="flex grow items-center justify-center">
					<div
						class="border-t-primary size-6 animate-spin rounded-full border-2 border-gray-300"
					></div>
				</div>
			{:else if invitations.length === 0}
				<p class="text-muted-content p-4 text-center">No invitations found</p>
			{:else}
				<ul class="flex flex-col gap-4">
					{#each invitations as invitation (invitation.code)}
						<li
							class="dark:bg-base-200 dark:border-base-400 bg-base-100 flex items-center justify-between gap-4 rounded-md p-4 shadow-sm dark:border"
						>
							<div class="flex grow flex-col gap-2 md:gap-1">
								<div class="line-clamp-1 overflow-x-auto text-sm font-medium break-all">
									{invitation.code}
								</div>
								<div class="flex shrink-0 gap-4">
									<span
										class={twMerge(
											'inline-flex rounded-lg border px-2 py-0.5 text-xs leading-5 font-semibold whitespace-nowrap capitalize dark:opacity-75',
											invitation.status === 'pending' && 'border-warning text-warning',
											invitation.status === 'accepted' && 'border-success text-success',
											invitation.status === 'rejected' && 'border-error text-error',
											invitation.status === 'expired' && 'border-base-content/40 text-muted-content'
										)}
									>
										{invitation.status}
									</span>
									<div class="bg-base-300 dark:bg-base-400 h-6 w-px"></div>
									<div class="text-muted-content flex items-center gap-2 text-xs">
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
							<div class="flex shrink-0 gap-4 self-start md:self-center">
								{#if invitation.status === 'pending' && !responsive.isMobile}
									<CopyButton
										text={`${window.location.protocol}//${window.location.host}/i/${invitation.code}`}
										buttonText="Copy Invite Link"
									/>
								{/if}
								<button
									class="btn btn-error shrink-0"
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

<ResponsiveDialog bind:this={invitationDialog} class="relative p-4 py-8 md:w-lg" animate="fade">
	<IconButton
		class="relative top-2 right-2 z-40 float-right self-end md:absolute"
		onclick={() => invitationDialog?.close()}
		tooltip={{ disablePortal: true, text: 'Close Project Catalog' }}
	>
		<X class="size-6" />
	</IconButton>

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
		<span class="text-muted-content line-clamp-1 text-xs break-all">{invitationUrl}</span>
	</div>
</ResponsiveDialog>

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
