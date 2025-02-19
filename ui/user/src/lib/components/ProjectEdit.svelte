<script lang="ts">
	import { ChevronUp, KeyRound, Trash2, Wrench, X } from 'lucide-svelte';
	import { tick } from 'svelte';
	import {
		type File,
		type AssistantTool,
		type KnowledgeFile,
		ChatService,
		type Project,
		type ProjectAuthorization,
		type ProjectCredential,
		type ProjectTemplate
	} from '$lib/services';
	import { assistants, context } from '$lib/stores';
	import Confirm from '$lib/components/Confirm.svelte';
	import { Brain, Check, ChevronDown, FileText } from 'lucide-svelte/icons';
	import { autoHeight } from '$lib/actions/textarea';
	import { opacityIn } from '$lib/actions/animate';
	import AssistantIcon from '$lib/icons/AssistantIcon.svelte';
	import InfoTooltip from '$lib/components/InfoTooltip.svelte';
	import { type Messages } from '$lib/services';
	import { Thread } from '$lib/services/chat/thread.svelte';
	import Message from '$lib/components/messages/Message.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';

	let dialog = $state<HTMLDialogElement>();
	let project = $state<Project>({
		id: '',
		name: '',
		created: ''
	});
	let toDelete = $state(false);
	let openPane = $state('');

	let authorizations = $state<ProjectAuthorization[]>();
	let credentials = $state<ProjectCredential[]>();
	let credentialsFiltered = $derived.by(() => {
		return credentials?.filter((cred) => {
			return (tools ?? []).find((tool) => {
				return tool.enabled && cred.toolID === tool.id;
			});
		});
	});
	let credentialsExists = $derived.by(() => {
		return credentials?.filter((cred) => {
			return (tools ?? []).find((tool) => {
				return tool.enabled && cred.toolID === tool.id && cred.exists;
			});
		});
	});
	let tools = $state<AssistantTool[]>();
	let customTools = $derived.by(() => {
		return tools?.filter((tool) => tool.enabled && tool.id.startsWith('tl1'));
	});
	let authMessages = $state<Messages>();
	let thread = $state<Thread>();
	let pendingAuthToolID = $state<string>();
	let published = $state(false);
	let toPublish = $state(false);
	let shareCredentials = $state(false);
	let files = $state<File[]>();
	let knowledge = $state<KnowledgeFile[]>();

	export async function show() {
		toDelete = false;
		project = await ChatService.getProject(context.projectID);
		openPane = '';
		dialog?.showModal();
		await tick();
		const e = dialog?.querySelector('#project-name');
		if (e instanceof HTMLInputElement) {
			e.focus();
		}
	}

	async function deAuth(toolID: string) {
		try {
			pendingAuthToolID = toolID;
			await ChatService.deleteProjectCredential(toolID);
			await loadCredentials(true);
		} finally {
			pendingAuthToolID = undefined;
		}
	}

	function auth(toolID: string) {
		const t = new Thread({
			authenticate: {
				tools: [toolID]
			},
			onError: () => {
				// ignore the error. This is so it doesn't get globally printed
			},
			onClose: () => {
				authCancel();
				return false;
			}
		});
		t.onMessages = (messages) => {
			authMessages = messages;
		};
		thread = t;
		pendingAuthToolID = toolID;
	}

	function authCancel() {
		thread?.abort();
		thread?.close();
		// only clear message if nothing failed
		if (!(authMessages?.messages ?? []).find((msg) => msg.icon === 'Error')) {
			authMessages = undefined;
		}
		thread = undefined;
		pendingAuthToolID = undefined;
		loadCredentials(true);
	}

	function togglePane(pane: string) {
		if (pane !== 'publish' && toPublish) {
			toPublish = false;
		}
		if (openPane === pane) {
			openPane = '';
		} else {
			openPane = pane;
		}
	}

	async function toggleMembers() {
		if (!authorizations && project?.id) {
			authorizations = (await ChatService.listProjectAuthorizations(project.id)).items;
		}
		togglePane('members');
	}

	async function loadCredentials(force?: boolean) {
		if (force || (!credentials && project?.id)) {
			credentials = (await ChatService.listProjectCredentials(project.id)).items;
		}
	}

	async function toggleCredentials() {
		await loadTools();
		await loadCredentials();
		togglePane('credentials');
	}

	async function loadTools() {
		if (!tools && project?.id) {
			tools = (await ChatService.listTools()).items;
			tools = tools.filter((tool) => !tool.builtin);
		}
	}

	async function toggleTools() {
		await loadTools();
		togglePane('tools');
	}

	async function toggleInstructions() {
		togglePane('instructions');
	}

	async function doDelete() {
		if (project?.id) {
			await ChatService.deleteProject(project.id);
			window.location.href = `/${assistants.current().id}`;
		}
		dialog?.close();
	}

	export async function close() {
		dialog?.close();
	}

	async function save() {
		project = await ChatService.updateProject(project);
		context.project = project;
		if (project?.id && authorizations) {
			await ChatService.updateProjectAuthorizations(project.id, {
				items: authorizations
			});
		}
		if (project?.id && tools) {
			tools = (
				await ChatService.updateProjectTools(project.id, {
					items: tools
				})
			).items;
		}
		dialog?.close();
	}

	async function preparePublish() {
		if (!files) {
			files = (await ChatService.listFiles()).items;
		}
		if (!knowledge) {
			knowledge = (await ChatService.listKnowledgeFiles()).items;
		}
		shareCredentials = false;
		toPublish = true;
	}

	async function togglePublish() {
		await loadTools();
		await loadCredentials();
		if (!files) {
			files = (await ChatService.listFiles()).items;
		}
		togglePane('publish');
	}

	function pollTemplate(template: ProjectTemplate) {
		ChatService.getProjectTemplate(project.id, template.id).then((template) => {
			if (template.ready) {
				published = true;
				return;
			}
			setTimeout(() => pollTemplate(template), 1000);
		});
	}

	async function publish() {
		let template = await ChatService.createProjectTemplate(project.id, {
			shareCredentials
		});
		pollTemplate(template);
	}

	$effect(() => {
		if (project.id && (project.starterMessages ?? []).length === 0) {
			project.starterMessages = [''];
		} else if (
			project.id &&
			project.starterMessages &&
			project.starterMessages[project.starterMessages.length - 1] !== ''
		) {
			project.starterMessages.push('');
		}
		if (!authorizations) {
			return;
		}
		if (authorizations.length == 0 || authorizations[authorizations.length - 1].target !== '') {
			authorizations.push({ target: '', accepted: true });
		}
	});
</script>

{#snippet upDown(up: boolean)}
	{#if up}
		<ChevronUp class="h-5 w-5" />
	{:else}
		<ChevronDown class="h-5 w-5" />
	{/if}
{/snippet}

{#snippet credIcon(cred: ProjectCredential)}
	{#if cred.icon}
		<img class="h-8 w-8 rounded-md bg-gray-100 p-1" src={cred.icon} alt="credential icon" />
	{:else}
		<KeyRound class="h-8 w-8 rounded-md bg-gray-100 p-1 text-black" />
	{/if}
{/snippet}

<dialog
	bind:this={dialog}
	class=" relative w-[700px] rounded-3xl bg-gray-50 p-5 text-black dark:bg-gray-950 dark:text-gray-50"
>
	<!-- Upper Right Buttons -->
	<div class="absolute right-0 top-0 flex p-2 text-gray">
		<button
			class="icon-button"
			onclick={() => {
				toDelete = true;
				dialog?.close();
			}}
		>
			<Trash2 class="icon-default" />
		</button>
		<button class="icon-button" onclick={() => dialog?.close()}>
			<X class="icon-default" />
		</button>
	</div>

	<!-- Main Content -->
	<div class="flex min-h-96 w-full flex-col gap-6">
		{#if !toPublish}
			<div class="flex items-center gap-3">
				<AssistantIcon />
				<h2 class="text-xl font-semibold">Configure Obot</h2>
			</div>

			<!-- Name and Description -->
			<div class="flex flex-col gap-3">
				<div class="flex items-center gap-2">
					<label for="project-name">Name</label>
					<input
						id="project-name"
						class="grow rounded-lg bg-gray-100 p-2 dark:bg-gray-900"
						type="text"
						bind:value={project.name}
						placeholder="Superfly Obot"
					/>
				</div>
				<label for="project-description">Description</label>
				<textarea
					id="project-description"
					class="grow resize-none rounded-lg bg-gray-100 p-2 dark:bg-gray-900"
					use:autoHeight
					rows="1"
					bind:value={project.description}
					placeholder="I like cake and long walks on the beach"
				></textarea>
			</div>

			<!-- Members -->
			<div class="flex flex-col gap-3">
				<button class="flex items-center gap-2" onclick={toggleMembers}>
					<span class="text-lg">Members</span>
					{@render upDown(openPane === 'members')}
				</button>
				{#if openPane === 'members'}
					<div class="flex flex-col gap-3 rounded-3xl bg-white p-5 dark:bg-black" use:opacityIn>
						<ul class="flex flex-col gap-2">
							{#each authorizations ?? [] as auth, i}
								{@const hide = authorizations?.length === 1 || i + 1 === authorizations?.length}
								<li class="flex flex-col">
									<div class="mb-2 flex items-center gap-2">
										<input
											id="auth-1"
											class="grow rounded-lg bg-gray-100 p-3 dark:bg-gray-900"
											type="text"
											placeholder="user@example.com"
											bind:value={auth.target}
										/>
										<button
											tabindex={hide ? -1 : 0}
											onclick={() => {
												authorizations?.splice(i, 1);
											}}
										>
											<Trash2 class="icon-button {hide ? 'invisible' : ''}" />
										</button>
									</div>
									{#if !auth.accepted}
										<span class="text-sm text-red-400">Invite Pending</span>
									{/if}
								</li>
							{/each}
						</ul>
					</div>
				{/if}
			</div>

			<!-- Instructions -->
			<div class="flex flex-col gap-3">
				<button class="flex items-center gap-2" onclick={toggleInstructions}>
					<span class="text-lg">Instructions</span>
					{@render upDown(openPane === 'instructions')}
				</button>
				{#if openPane === 'instructions'}
					<div class="flex flex-col gap-3 rounded-3xl bg-white p-5 dark:bg-black" use:opacityIn>
						<div class="flex items-center gap-2">
							<label for="instructions">Instructions</label>
							<InfoTooltip>
								Change the behavior, tone, or specific procedures the Obot should follow.
							</InfoTooltip>
						</div>
						<textarea
							id="instructions"
							class="resize-none rounded-lg bg-gray-100 p-2 dark:bg-gray-900"
							rows="2"
							use:autoHeight
							bind:value={project.prompt}
							placeholder="Talk like a pirate..."
						></textarea>
						<div class="flex items-center gap-2">
							<label for="introduction">Introduction</label>
							<InfoTooltip>A message show to the user on a new thread.</InfoTooltip>
						</div>
						<textarea
							id="introduction"
							class="resize-none rounded-lg bg-gray-100 p-2 dark:bg-gray-900"
							rows="2"
							use:autoHeight
							bind:value={project.introductionMessage}
							placeholder="I like cake..."
						></textarea>
						<div class="flex items-center gap-2">
							<span>Starter Messages</span>
							<InfoTooltip>
								Sample conversation starters show to the user on start of a new thread.
							</InfoTooltip>
						</div>
						<ol class="flex flex-col gap-3">
							{#each project.starterMessages?.keys() ?? [] as i}
								{#if project.starterMessages}
									<li>
										<input
											bind:value={project.starterMessages[i]}
											placeholder="Do you like cake?"
											class="w-full rounded-lg bg-gray-100 p-2 dark:bg-gray-900"
										/>
									</li>
								{/if}
							{/each}
						</ol>
					</div>
				{/if}
			</div>

			<!-- Tools -->
			<div class="flex flex-col gap-3">
				<button class="flex items-center gap-2" onclick={toggleTools}>
					<span class="text-lg">Tools</span>
					{@render upDown(openPane === 'tools')}
				</button>
				{#if openPane === 'tools'}
					<div class="flex flex-col gap-3 rounded-3xl bg-white p-5 dark:bg-black" use:opacityIn>
						<ul class="flex flex-col gap-2">
							{#each tools ?? [] as tool}
								<li class="flex items-center gap-3">
									{#if tool.icon}
										<img
											class="h-8 w-8 rounded-md bg-gray-100 p-1"
											src={tool.icon}
											alt="tool icon"
										/>
									{:else}
										<Wrench class="h-8 w-8 rounded-md bg-gray-100 p-1 text-black" />
									{/if}
									<div class="flex grow flex-col">
										<span class="text-sm">{tool.name}</span>
										<span class="text-sm text-gray">{tool.description}</span>
									</div>
									<input type="checkbox" bind:checked={tool.enabled} class="size-4" />
								</li>
							{/each}
						</ul>
					</div>
				{/if}
			</div>

			<!-- Credentials -->
			<div class="flex flex-col gap-3">
				<button class="flex items-center gap-2" onclick={toggleCredentials}>
					<span class="text-lg">Credentials</span>
					{@render upDown(openPane === 'credentials')}
				</button>
				{#if openPane === 'credentials'}
					<div class="flex flex-col gap-3 rounded-3xl bg-white p-5 dark:bg-black" use:opacityIn>
						{#if credentialsFiltered?.length === 0}
							<span class="text-gray-500">No tools require credentials </span>
						{:else}
							<ul class="flex flex-col gap-2">
								{#each credentialsFiltered ?? [] as cred}
									<li class="flex items-center">
										{@render credIcon(cred)}
										<div class="flex flex-1 px-2">
											<span class="text-sm font-medium dark:text-gray-100">{cred.toolName}</span>
										</div>
										{#if cred.exists}
											<Check class="icon-default" />
											<button class="icon-button">
												<Trash2 class="icon-default" onclick={() => deAuth(cred.toolID)} />
											</button>
										{:else if pendingAuthToolID === cred.toolID}
											<Loading class="h-5 w-5" />
										{:else}
											<button class="button-primary" onclick={() => auth(cred.toolID)}>
												Add
											</button>
										{/if}
									</li>
								{/each}
							</ul>
							<span class="text-sm text-gray"
								>These credentials will be shared by all users of this Obot.</span
							>
							{#if authMessages}
								<div class="flex flex-col gap-5 p-5">
									{#each authMessages.messages as msg}
										<Message {msg} onSendCredentialsCancel={() => authCancel()} />
									{/each}
								</div>
							{/if}
						{/if}
					</div>
				{/if}
			</div>
		{/if}

		<!-- Publish -->
		<div class="flex flex-col gap-3">
			{#if toPublish}
				<span class="text-lg">Publish and Share</span>
			{:else}
				<button class="flex items-center gap-2" onclick={togglePublish}>
					<span class="text-lg">Publish and Share</span>
					{@render upDown(openPane === 'publish')}
				</button>
			{/if}
			{#if openPane === 'publish'}
				<div class="flex flex-col gap-3 rounded-3xl bg-white p-5 dark:bg-black" use:opacityIn>
					<div class="flex">
						<div class="flex grow flex-col justify-center gap-1">
							{#if published}
								<div class="mb-2 flex items-center gap-2">
									<h3 class="text-lg">Published</h3>
									<Check class="icon-default" />
								</div>
							{:else if !toPublish}
								<h3 class="text-lg">Unpublished</h3>
								<p class="text-sm">
									Publishing will allow others to create an Obot just like this one.
								</p>
							{/if}
							{#if published}
								<p>Copy and share this link</p>
								<div class="flex gap-2">
									<a href="https://foo.bar" class="hover:underline">https://foo.bar</a>
									<CopyButton text="https://foo.bar" />
								</div>
							{/if}
						</div>
						<div class="flex flex-col gap-2">
							{#if published}
								<button class="button" onclick={() => (published = false)}>
									<span>Unpublish</span>
								</button>
								<button class="button" onclick={() => (published = false)}>
									<span>Update</span>
								</button>
							{:else if !toPublish}
								<button class="button" onclick={() => preparePublish()}>
									<span>Publish</span>
								</button>
							{/if}
						</div>
					</div>

					<div>
						{#if toPublish}
							<div class="flex flex-col gap-4">
								<!-- Files -->
								{#if files && files.length > 0}
									<div class="flex items-center gap-2">
										<FileText class="icon-default-size" />
										<h3 class="text-xl">Attached Files</h3>
									</div>
									<ul class="flex flex-col gap-2 self-end">
										{#each files ?? [] as file}
											<li class="flex items-center">
												<div class="flex flex-1 px-2">
													<span>{file.name}</span>
												</div>
											</li>
										{/each}
									</ul>
								{/if}

								<!-- KnowledgeFiles -->
								{#if knowledge && knowledge.length > 0}
									<div class="flex items-center gap-2">
										<Brain class="icon-default-size" />
										<h3 class="text-xl">Attached Knowledge</h3>
									</div>
									<ul class="flex flex-col gap-2 self-end">
										{#each knowledge ?? [] as file}
											<li class="items center flex">
												<div class="flex flex-1 px-2">
													<span>{file.fileName}</span>
												</div>
											</li>
										{/each}
									</ul>
								{/if}

								<!-- Custom Tools -->
								{#if customTools && customTools.length > 0}
									<div class="mt-2 flex gap-2">
										<Wrench class="icon-default-size" />
										<p class="">
											{customTools.length === 1
												? '1 custom tool'
												: `${customTools.length} custom tools`} will be shared with all users of this
											Obot.
										</p>
									</div>
									<ul class="flex flex-col gap-2">
										{#each customTools ?? [] as tool}
											<li class="items center flex">
												<div class="flex flex-1 px-2">
													<span>{tool.name}</span>
												</div>
											</li>
										{/each}
									</ul>
								{/if}

								<!-- Credentials -->
								{#if credentialsExists && credentialsExists.length > 0}
									<div class="flex items-center gap-2">
										<FileText class="icon-default-size" />
										<h3 class="text-xl">Attached Credentials</h3>
									</div>
									{#if shareCredentials}
										<ul class="flex flex-col gap-2 self-end">
											{#each credentialsExists as cred}
												<li class="mb-2 flex items-center">
													{@render credIcon(cred)}
													<div class="flex flex-1 px-2">
														<span class="text-sm font-medium dark:text-gray-100"
															>{cred.toolName}</span
														>
													</div>
												</li>
											{/each}
										</ul>
									{/if}
									<div class="flex items-center gap-2 self-end">
										<input
											id="share"
											type="checkbox"
											bind:checked={shareCredentials}
											class="size-4"
										/>
										<label for="share">Share Credentials</label>
									</div>
								{/if}

								<p class="mt-5">
									Publishing this Obot will make it's configuration, files, tasks, and other assets
									available to anyone to copy and use.
								</p>

								<div class="flex gap-2 self-end">
									<button class="button-secondary" onclick={() => (toPublish = false)}>
										<span>Cancel</span>
									</button>
									<button class="button-primary" onclick={() => publish()}>
										<span>Publish</span>
									</button>
								</div>
							</div>
						{/if}
					</div>
				</div>
			{/if}
		</div>

		<!-- Spacer -->
		<div class="grow"></div>

		<!-- Buttons -->
		{#if !toPublish}
			<div class="flex justify-end gap-3">
				<button class="button-secondary" onclick={close}>Cancel</button>
				<button class="button-primary" onclick={save}>Save</button>
			</div>
		{/if}
	</div>
</dialog>

<Confirm
	show={toDelete}
	msg="Are you sure you want to delete this project? All associated resources will be permanently deleted."
	onsuccess={doDelete}
	oncancel={() => {
		show();
	}}
/>
