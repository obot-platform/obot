<script lang="ts">
	import { fade } from 'svelte/transition';
	import { Crown, Plus, ChevronRight, ChevronLeft, Globe, Trash2, Star } from 'lucide-svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { ChatService, type Project, type ProjectMember } from '$lib/services';
	import { profile } from '$lib/stores';
	import { HELPER_TEXTS } from '$lib/context/helperMode.svelte';
	import {
		openTemplate,
		openSidebarConfig,
		closeSidebarConfig,
		getLayout
	} from '$lib/context/chatLayout.svelte';
	import {
		listProjectTemplates,
		createProjectTemplate,
		deleteProjectTemplate,
		type ProjectTemplate
	} from '$lib/services';

	let toDelete = $state('');
	let ownerID = $state<string>('');
	let isOwnerOrAdmin = $derived(profile.current.id === ownerID || profile.current.role === 1);
	let templateToDelete = $state<ProjectTemplate>();
	let templates = $state<ProjectTemplate[]>([]);

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
	let members = $state<ProjectMember[]>([]);
	const layout = getLayout();

	async function loadMembers() {
		members = await ChatService.listProjectMembers(project.assistantID, project.id);
	}

	async function deleteMember(memberId: string) {
		if (!isOwnerOrAdmin) return;
		await ChatService.deleteProjectMember(project.assistantID, project.id, memberId);
		await loadMembers();
	}

	function manageInvitations() {
		if (!isOwnerOrAdmin) return;
		openSidebarConfig(layout, 'invitations');
	}

	function openChatbotConfig() {
		openSidebarConfig(layout, 'chatbot');
	}

	$effect(() => {
		if (project) {
			ownerID = project.userID;
			loadMembers();
			loadTemplates();
		}
	});

	async function loadTemplates() {
		try {
			const result = await listProjectTemplates(project.assistantID, project.id);
			// Sort by newest first
			templates = (result.items || []).sort((a, b) => {
				return new Date(b.created).getTime() - new Date(a.created).getTime();
			});
		} catch (error) {
			console.error('Failed to load templates:', error);
			templates = [];
		}
	}

	async function createTemplate() {
		try {
			const newTemplate = await createProjectTemplate(project.assistantID, project.id);
			templates = [newTemplate, ...templates];
			openTemplate(layout, newTemplate);
		} catch (error) {
			console.error('Failed to create template:', error);
		}
	}

	async function handleDeleteTemplate() {
		if (!templateToDelete || !project?.assistantID || !project?.id) return;

		try {
			await deleteProjectTemplate(project.assistantID, project.id, templateToDelete.id);
			templates = templates.filter((t) => t.id !== templateToDelete?.id);

			if (layout.template?.id === templateToDelete.id) {
				closeSidebarConfig(layout);
			}
		} catch (error) {
			console.error('Failed to delete template:', error);
		} finally {
			templateToDelete = undefined;
		}
	}

	$effect(() => {
		if (layout.template) {
			// Find and update the template in the templates array
			const index = templates.findIndex((t) => t.id === layout.template?.id);
			if (index !== -1) {
				templates[index] = layout.template;
			}
		}
	});
</script>

<CollapsePane
	classes={{ header: 'pl-3 py-2 text-md', content: 'p-0' }}
	iconSize={5}
	header="Sharing"
	helpText={HELPER_TEXTS.sharing}
>
	<div class="flex flex-col">
		<CollapsePane
			classes={{
				header: 'pl-3 pr-5.5 py-2 border-surface3 border-b',
				content: 'p-3 border-b border-surface3',
				headerText: 'text-sm font-normal'
			}}
			iconSize={4}
			header="Templates"
			helpText={HELPER_TEXTS.agentTemplate}
		>
			<div class="flex flex-col gap-1.5">
				{#each templates as template (template.id)}
					<div
						class="hover:bg-surface3 group flex min-h-9 items-center justify-between rounded-md bg-transparent p-2 pr-3 text-xs transition-colors duration-200"
					>
						<button
							class="flex grow items-center gap-2"
							onclick={() => openTemplate(layout, template)}
						>
							<div class="flex flex-col">
								<div class="flex items-center gap-2">
									<span>{template.name || 'Unnamed Template'}</span>
									<span class="text-[10px] text-gray-500">
										{new Date(template.created).toLocaleString(undefined, {
											year: 'numeric',
											month: 'short',
											day: 'numeric',
											hour: '2-digit',
											minute: '2-digit'
										})}
									</span>
								</div>
								<div class="flex items-center gap-2 text-[10px] text-gray-500">
									{#if template.featured}
										<div class="flex items-center gap-1" use:tooltip={'Featured template'}>
											<Star class="size-3 text-blue-500" />
											<span>Featured</span>
										</div>
									{/if}
									{#if template.public}
										<div class="flex items-center gap-1" use:tooltip={'Public template'}>
											<Globe class="size-3" />
											<span>Public</span>
										</div>
									{/if}
								</div>
							</div>
						</button>
						<div class="flex items-center gap-2">
							<button
								class="text-gray-500 opacity-0 transition-opacity duration-200 group-hover:opacity-100 hover:text-red-600 dark:text-gray-400 dark:hover:text-red-400"
								onclick={() => (templateToDelete = template)}
								use:tooltip={'Delete template'}
							>
								<Trash2 class="size-4" />
							</button>
							{#if layout.template?.id === template.id}
								<ChevronLeft class="size-4" />
							{:else}
								<ChevronRight class="size-4" />
							{/if}
						</div>
					</div>
				{/each}
				<div class="mt-2 flex justify-end" in:fade>
					<button
						class="button flex cursor-pointer items-center justify-end gap-1 text-xs"
						onclick={createTemplate}
					>
						<Plus class="size-4" />
						<span>Create Template</span>
					</button>
				</div>
			</div>
		</CollapsePane>

		<CollapsePane
			classes={{
				header: 'pl-3 pr-5.5 py-2 border-surface3 border-b',
				content: 'p-3 border-b border-surface3 overflow-x-hidden',
				headerText: 'text-sm font-normal'
			}}
			iconSize={4}
			header="ChatBot"
			helpText={HELPER_TEXTS.chatbot}
		>
			<div class="flex flex-col gap-3">
				<p class="text-xs text-gray-500">
					Configure ChatBot to produce a link that allows anyone to use this agent in a read-only
					mode.
				</p>
				<div class="mt-2 flex justify-end" in:fade>
					<button
						class="button flex cursor-pointer items-center justify-end gap-1 text-xs"
						onclick={openChatbotConfig}
					>
						<span>Configure ChatBot</span>
					</button>
				</div>
			</div>
		</CollapsePane>

		<CollapsePane
			classes={{
				header: 'pl-3 pr-5.5 py-2 border-surface3 border-b',
				content: 'p-3 border-b border-surface3',
				headerText: 'text-sm font-normal'
			}}
			iconSize={4}
			header="Members"
			helpText={HELPER_TEXTS.members}
		>
			<div class="flex flex-col gap-2 text-sm">
				<div class="flex flex-col gap-1">
					<div class="flex items-center justify-between">
						<span class="text-sm font-medium">Agent Members</span>
						{#if isOwnerOrAdmin}
							<div class="flex gap-2">
								<button
									class="icon-button"
									onclick={manageInvitations}
									use:tooltip={'Manage invitations'}
								>
									<Plus class="size-4" />
								</button>
							</div>
						{/if}
					</div>
					{#each members as member (member.userID)}
						<div
							class="group flex min-h-9 w-full items-center rounded-md transition-colors duration-300"
						>
							<div class="flex grow items-center gap-2">
								<div class="size-6 overflow-hidden rounded-full bg-gray-50 dark:bg-gray-600">
									<img
										src={member.iconURL}
										class="h-full w-full object-cover"
										alt="agent member icon"
										referrerpolicy="no-referrer"
									/>
								</div>
								<p class="truncate text-left text-sm font-light">
									{member.email}
								</p>
								{#if member.isOwner}
									<span use:tooltip={'Project Owner'}>
										<Crown class="size-4" />
									</span>
								{/if}
							</div>
							{#if isOwnerOrAdmin && profile.current.email !== member.email && !member.isOwner}
								<button
									class="icon-button"
									onclick={() => (toDelete = member.email)}
									use:tooltip={'Remove member'}
								>
									<Trash2 class="size-4" />
								</button>
							{/if}
						</div>
					{/each}
				</div>
			</div>
		</CollapsePane>
	</div>
</CollapsePane>

<Confirm
	msg={`Remove ${toDelete} from your agent?`}
	show={!!toDelete}
	onsuccess={async () => {
		if (!toDelete) return;
		try {
			const memberToDelete = members.find((m) => m.email === toDelete);
			if (memberToDelete) {
				await deleteMember(memberToDelete.userID);
			}
		} finally {
			toDelete = '';
		}
	}}
	oncancel={() => (toDelete = '')}
/>

<Confirm
	msg={`Are you sure you want to delete template: ${templateToDelete?.name || 'Unnamed Template'}?`}
	show={!!templateToDelete}
	onsuccess={handleDeleteTemplate}
	oncancel={() => (templateToDelete = undefined)}
/>
