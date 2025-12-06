<script lang="ts">
	import Navbar from '$lib/components/Navbar.svelte';
	import { responsive } from '$lib/stores';
	import { GripVertical, Play, Plus, SidebarClose, SidebarOpen, X } from 'lucide-svelte';
	import { fade, slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';
	import { columnResize } from '$lib/actions/resize';
	import BetaLogo from '$lib/components/navbar/BetaLogo.svelte';
	import Projects from '$lib/components/navbar/Projects.svelte';
	import { scrollFocus } from '$lib/actions/scrollFocus.svelte.js';
	import { initProjectMCPs } from '$lib/context/projectMcps.svelte.js';
	import { getLayout, initLayout } from '$lib/context/chatLayout.svelte.js';
	import McpServers from '$lib/components/edit/McpServers.svelte';
	import Runs from './Runs.svelte';
	import WorkflowTasks from './WorkflowTasks.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { tick } from 'svelte';
	import Thread from '$lib/components/Thread.svelte';

	let { data } = $props();
	initProjectMCPs(data.mcps ?? []);
	initLayout({
		items: [],
		sidebarOpen: !responsive.isMobile
	});

	let workflowContainer = $state<HTMLDivElement>();
	let project = $state(data.project);
	let workflow = $state({
		name: 'Onboarding Workflow',
		description: 'This workflow is used to onboard new users to the platform.',
		prompt:
			'You are an assistant responsible for onboarding members to the CNCF without breaking the rules.\n\nRules:\n* You can read from Salesforce.\n* Never add a new record to Salesforce.\n* Follow the Action you are told.',
		arguments: [
			{
				name: 'CompanyName',
				displayLabel: '',
				description: ''
			}
		],
		tasks: [
			{
				name: 'Add New Member Contacts to Google Groups',
				description:
					'This will add the contacts of our new members to the appropriate Google Groups',
				content:
					'1. Get the account record for \$CompanyName in Salesforce. You also need to get all related Contacts including roles and emails.',
				id: '1'
			},
			{
				name: 'Add Logo to site',
				description: 'Create a github PR to add the logo to the site',
				content: `
1. Get the account record for \$CompanyName in Salesforce. Search all opportunities, look for the most recent closed won opportunity for membership level. Get the documents. Documents may be stored as classic Attachments (in the Attachment object, linked by ParentId) or as Salesforce Files (in ContentDocument, linked to the Account via ContentDocumentLink and LinkedEntityId). To review everything created for a company, look for these related records and fields using the Account’s Id.

2. Create a branch in the repo cloudnautique/obot-mcpserver-examples called add-\$CompanyName-logo.

3. Create a file in the workspace called logo.txt and write a story about a robot in markdown.

4. Add the file to the assets/img directory called \$CompanyName-logo.txt, and create a PR back into the main branch.
				`,
				id: '2'
			},
			{
				name: 'Add Member to Google Sheet',
				description: 'This task will add the member to the google sheet',
				content: `
1. Get the account record for $CompanyName in Salesforce. You also need to get all related Contacts including roles and emails. Search all opportunities, look for the most recent closed won opportunity for membership level. Get the documents. Documents may be stored as classic Attachments (in the Attachment object, linked by ParentId) or as Salesforce Files (in ContentDocument, linked to the Account via ContentDocumentLink and LinkedEntityId). Notes or special instructions are typically found in the Account’s Description field. To review everything created for a company, look for these related records and fields using the Account’s Id.

2. Get the Demo Workflow LF Google Sheet. Read the first few rows to understand the sheet and formats used in each column.

3. Follow the formatting and style, add a new row to the google sheet for the member $CompanyName based on the information we got previously in Salesforce. The join date should be the closed won date.
				`,
				id: '3'
			},
			{
				name: 'Send Welcome Email',
				description: 'Used to send welcome email when the org has been onboarded.',
				content: `
1. Get the account record for \$CompanyName in Salesforce. You also need to get the contacts and their email addresses. Also get the most recent opportunity to determine the membership.

2. Using gmail tools, create a draft email using the business owner contact, account, and opportunity info.

\`\`\` Markdown
# CNCF Onboarding Completion 

**Subject:** Welcome to the Cloud Native Computing Foundation (CNCF)!

---

Dear {{FirstName}} {{LastName}},

Congratulations, and welcome to the **Cloud Native Computing Foundation (CNCF)** community!

We’re pleased to let you know that all onboarding steps for **{{CompanyName}}** have been successfully completed. Your organization is now fully set up as a {{membership level}} member and ready to take advantage of CNCF programs, resources, and community benefits.

Congrats!
Dir. CNCF Onboarding Agent
\`\`\`

Send the drafted email.
				`,
				id: '4'
			},
			{
				name: 'Add member contacts to Slack',
				description: '',
				content: `
1. Get the account record for \$CompanyName in Salesforce. You also need to get all related Contacts including roles and emails.

2. List all channels including private ones.

3. Search for the marketing contacts in the slack workspace to see if they are members. If they are in the workspace add them to the private marketing channel.

4. Search for the business owner contacts in the slack workspace to see if they are members. If they are in the workspace, add them to the private business-owners channel.

5. Search for the technical contacts in the slack workspace to see if they are members. If they are members of the workspace, add them to the private technical-leads channel.

6. Report back who is a member of slack already.`,
				id: '5'
			}
		]
	});

	let workflowRunOpen = $state(false);
	let runContainer = $state<HTMLDivElement>();
	let navContainer = $state<HTMLDivElement>();
	let workflowNameContainer = $state<HTMLDivElement>();
	let selectedRun = $state<Thread>();

	let titleVisible = $state(false);

	let observer: IntersectionObserver;
	function setupObserver() {
		// Always disconnect existing observer before setting up new one
		observer?.disconnect();

		observer = new IntersectionObserver(
			([entry]) => {
				titleVisible = entry.isIntersecting;
			},
			{ threshold: 0 }
		);

		if (workflowNameContainer) {
			observer.observe(workflowNameContainer);
		}
	}

	$effect(() => {
		if (workflowNameContainer) {
			setupObserver();
		}
		return () => observer.disconnect();
	});

	const layout = getLayout();

	function handleVariableAddition(variable: string) {
		const argumentToAdd = variable.replace('$', '');
		if (workflow.arguments.some((arg) => arg.name === argumentToAdd)) {
			return;
		}

		workflow.arguments.push({
			name: argumentToAdd,
			description: '',
			displayLabel: ''
		});
	}
</script>

<div class="bg-surface1 dark:bg-background relative flex h-dvh flex-col overflow-hidden">
	<div class="relative flex h-full">
		{#if layout.sidebarOpen}
			<div
				class="bg-surface1 w-screen min-w-screen flex-shrink-0 md:w-1/6 md:min-w-[250px]"
				transition:slide={{ axis: 'x' }}
				bind:this={navContainer}
			>
				<div
					class="border-surface2 dark:bg-gray-990 bg-background relative flex size-full flex-col border-r"
				>
					<div
						class="flex h-16 w-full flex-shrink-0 items-center justify-between px-2 md:justify-start"
					>
						<BetaLogo workflow />
						{#if responsive.isMobile}
							{@render closeSidebar()}
						{/if}
					</div>
					<div class="default-scrollbar-thin flex w-full grow flex-col gap-4" use:scrollFocus>
						{#if project}
							<Projects
								{project}
								onCreateProject={() => {
									//TODO:
								}}
							/>
						{/if}
						<div class="mb-2 flex flex-col gap-8 px-4">
							<!-- todo -->
							{#if project}
								<Runs {project} onSelectRun={() => (workflowRunOpen = true)} />
								<McpServers {project} />
							{/if}
						</div>
					</div>

					<div class="flex w-full items-center justify-end gap-2 px-2 py-2">
						{#if !responsive.isMobile}
							{@render closeSidebar()}
						{/if}
					</div>
				</div>
			</div>
			{#if !responsive.isMobile}
				<div
					role="none"
					class="relative -ml-3 h-full w-3 cursor-col-resize"
					use:columnResize={{ column: navContainer }}
				></div>
			{/if}
		{/if}

		<main
			id="main-content"
			class="bg-surface1 dark:bg-background flex min-h-dvh max-w-full grow flex-col overflow-hidden"
			class:hidden={layout.sidebarOpen && responsive.isMobile}
		>
			<div class="w-full">
				<Navbar workflow class="bg-surface1 dark:bg-background">
					{#snippet leftContent()}
						{#if !layout.sidebarOpen}
							<BetaLogo workflow />
						{/if}
						{#if !layout.sidebarOpen && responsive.isMobile}
							<div class="ml-2">
								{@render openSidebar()}
							</div>
						{/if}
						{#if layout.sidebarOpen && !titleVisible && !workflowRunOpen}
							<h4 in:fade={{ duration: 200 }} class="pl-14 text-xl font-semibold">
								{workflow.name}
							</h4>
						{/if}
					{/snippet}
					{#snippet centerContent()}
						<div class="flex w-full justify-end px-2">
							<button
								class="button-primary flex w-48 flex-shrink-0 items-center justify-center gap-2 text-sm"
							>
								Run <Play class="size-4" />
							</button>
						</div>
					{/snippet}
				</Navbar>
			</div>

			{#if !layout.sidebarOpen && !responsive.isMobile}
				<div class="absolute bottom-2 left-2 z-30" in:fade={{ delay: 300 }}>
					{@render openSidebar()}
				</div>
			{/if}

			<div
				class="default-scrollbar-thin relative h-[calc(100%-64px)] max-w-full overflow-y-auto"
				class:px-16={!workflowRunOpen}
				class:px-8={workflowRunOpen}
				bind:this={workflowContainer}
			>
				<div class="mx-auto min-h-full w-full px-1 pb-4 md:max-w-[1200px]">
					<div class="mb-4 flex w-full flex-col gap-1" bind:this={workflowNameContainer}>
						<input
							class="ghost-input text-2xl font-semibold"
							bind:value={workflow.name}
							placeholder="Workflow title"
						/>
						<input
							class="ghost-input"
							bind:value={workflow.description}
							placeholder="Description (optional)"
						/>
					</div>

					<WorkflowTasks
						bind:tasks={workflow.tasks}
						onVariableAddition={handleVariableAddition}
						onDelete={(task) => {
							workflow.tasks = workflow.tasks.filter((t) => t.id !== task.id);
						}}
					/>
				</div>
				<div class="bg-surface1 dark:bg-background sticky bottom-0 left-0 z-50 w-full py-4">
					<div class="flex w-full items-center justify-center">
						<button
							use:tooltip={'Add Task'}
							class="button-icon-primary bg-background dark:bg-surface2 shadow-xs"
							onclick={async () => {
								workflow.tasks.push({
									id: (workflow.tasks.length + 1).toString(),
									name: '',
									description: '',
									content: ''
								});

								await tick();
								workflowContainer?.scrollTo({
									top: workflowContainer.scrollHeight,
									behavior: 'smooth'
								});
							}}
						>
							<Plus class="size-6" />
						</button>
					</div>
				</div>
			</div>
		</main>

		<div
			bind:this={runContainer}
			class={twMerge(
				'bg-background dark:bg-surface1 absolute right-0 z-30 float-right flex w-full flex-shrink-0 translate-x-full transform transition-transform duration-300 md:w-2/5 md:max-w-4xl md:min-w-[320px]',
				workflowRunOpen && 'relative w-full translate-x-0',
				!workflowRunOpen && 'w-0'
			)}
		>
			{#if workflowRunOpen && runContainer}
				<div
					use:columnResize={{ column: runContainer, direction: 'right' }}
					class="bg-surface1 dark:bg-background relative h-full w-8 cursor-grab"
					transition:slide={{ axis: 'x' }}
				>
					<div class="text-on-surface1 absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2">
						<GripVertical class="text-surface3 size-3" />
					</div>
				</div>
			{/if}
			<button class="icon-button absolute top-2 right-2" onclick={() => (workflowRunOpen = false)}>
				<X class="size-6" />
			</button>
		</div>
	</div>
</div>

{#snippet closeSidebar()}
	<button class="icon-button" onclick={() => (layout.sidebarOpen = false)}>
		<SidebarClose class="size-6" />
	</button>
{/snippet}

<!-- <McpServerRequirements assistantId={assistant?.id || ''} projectId={project.id} /> -->

{#snippet openSidebar()}
	<button class="icon-button" onclick={() => (layout.sidebarOpen = true)}>
		<SidebarOpen class="size-6" />
	</button>
{/snippet}

<svelte:head>
	<title>Obot | Workflow</title>
</svelte:head>
