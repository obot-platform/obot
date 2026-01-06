<script lang="ts">
	import { stickToBottom, type StickToBottomControls } from '$lib/actions/div.svelte';
	import Input from '$lib/components/messages/Input.svelte';
	import Message from '$lib/components/messages/Message.svelte';
	import { Thread } from '$lib/services/chat/thread.svelte';
	import {
		ChatService,
		EditorService,
		type Messages,
		type Project,
		type ProjectMCP
	} from '$lib/services';
	import { fade } from 'svelte/transition';
	import { onDestroy, onMount, tick } from 'svelte';
	import { toHTMLFromMarkdown } from '$lib/markdown';
	import { closeAll, getLayout } from '$lib/context/chatLayout.svelte';
	import Files from '$lib/components/edit/Files.svelte';
	import type { UIEventHandler } from 'svelte/elements';
	import { responsive } from '$lib/stores';
	import { Bug, LoaderCircle, X } from 'lucide-svelte';
	import { autoHeight } from '$lib/actions/textarea';
	import { twMerge } from 'tailwind-merge';
	import { getProjectDefaultModel, getThread } from '$lib/services/chat/operations';
	import type {
		CreateProjectForm,
		Assistant,
		MCPServerPrompt,
		Thread as ThreadType
	} from '$lib/services/chat/types';
	import ThreadModelSelector from '$lib/components/edit/ThreadModelSelector.svelte';
	import McpPrompts from './mcp/McpPrompts.svelte';
	import { HELPER_TEXTS } from '$lib/context/helperMode.svelte';
	import { clickOutside } from '$lib/actions/clickoutside';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { browser } from '$app/environment';
	import { goto } from '$lib/url';

	interface Props {
		id?: string;
		project: Project;
		createProject?: CreateProjectForm;
		assistant?: Assistant;
		isNew?: boolean;
		onInputChange?: (input: string) => void;
		inputValue?: string;
	}

	let {
		id = $bindable(),
		project = $bindable(),
		createProject = $bindable(),
		assistant,
		isNew = $bindable(),
		onInputChange,
		inputValue
	}: Props = $props();

	let messagesDiv = $state<HTMLDivElement>();
	let nameInput = $state<HTMLInputElement>();
	let messages = $state<Messages>({ messages: [], inProgress: false });
	let thread = $state<Thread>();
	let scrollSmooth = $state(false);
	let threadContainer = $state<HTMLDivElement>();
	let fadeBarWidth = $state<number>(0);
	let loadingOlderMessages = $state(false);
	let showLoadOlderButton = $state(false);
	let input = $state<ReturnType<typeof Input>>();
	let savingNewProject = $state(false);

	let promptDialogRef = $state<ReturnType<typeof McpPrompts>>();
	let promptPending = $state(false);
	let globalPromptIndex = $state(0);
	let globalPromptCount = $state(0);

	// Model selector state
	let threadDetails = $state<ThreadType | null>(null);
	let hasModelSelected = $state(false);

	let centerInput = $derived(!createProject && (!id || isNew));
	let imagePreviewDialog = $state<HTMLDialogElement>();
	let imagePreviewSrc = $state<string>();

	$effect(() => {
		if (threadContainer) {
			const resizeObserver = new ResizeObserver((entries) => {
				fadeBarWidth = entries[0].contentRect.width - 20; // scrollbar width
			});

			resizeObserver.observe(threadContainer);

			return () => {
				resizeObserver.disconnect();
			};
		}
	});

	$effect(() => {
		// Close and recreate thread if id changes
		if (thread && thread.threadID !== id) {
			scrollControls?.stickToBottom();
			scrollSmooth = false;
			thread?.close?.();
			thread = undefined;
			messages = {
				messages: [],
				inProgress: false
			};

			layout.input = '';
			input?.setValue('');
		}

		scrollSmooth = false;

		if (id && !thread) {
			constructThread();
		}
	});

	$effect(() => {
		if (createProject) {
			setTimeout(() => nameInput?.focus(), 0);
		}
	});

	$effect(() => {
		// Only update if messages change
		const messages_copy = messages; // Create a local reference

		if (messages_copy.messages.length === 0) {
			if (showLoadOlderButton) showLoadOlderButton = false;
			return;
		}

		const shouldShow = !!messages_copy.parentRunID;

		// Only update state if it needs to change
		if (shouldShow !== showLoadOlderButton) {
			showLoadOlderButton = shouldShow;
		}
	});

	let scrollControls = $state<StickToBottomControls>();

	onDestroy(() => {
		thread?.close?.();
	});

	const layout = getLayout();
	function onLoadFile(filename: string) {
		EditorService.load(layout.items, project, filename, {
			threadID: id
		});
		layout.fileEditorOpen = true;
	}

	async function ensureThread(params?: { model?: string; modelProvider?: string }) {
		if (thread && thread.closed && id) {
			await constructThread();
		}
		if (!id) {
			const body: { model?: string; modelProvider?: string } = {};
			if (params?.model) {
				body.model = params.model;
				body.modelProvider = params.modelProvider;
			}
			// Note: Model fallback is now handled in ThreadModelSelector
			// via the thread default model endpoint
			id = (await ChatService.createThread(project.assistantID, project.id, body)).id;
			await constructThread();
		}
	}

	async function constructThread() {
		const newThread = new Thread(project, {
			threadID: id,
			onError: () => {
				// ignore errors they are rendered as messages
			},
			onClose: () => {
				// false means don't reconnect
				return false;
			},
			items: layout.items,
			onItemsChanged: (items) => {
				layout.items = items;
			},
			onMemoryCall: () => {
				layout.sidebarMemoryUpdateAvailable = true;
			}
		});

		messages = {
			messages: [],
			inProgress: false
		};
		newThread.onMessages = (newMessages) => {
			messages = newMessages;
		};

		thread = newThread;
	}

	const onScrollEnd: UIEventHandler<HTMLDivElement> = (e) => {
		const isAtBottom =
			e.currentTarget.scrollHeight - e.currentTarget.scrollTop - e.currentTarget.clientHeight <= 0;

		if (isAtBottom) {
			scrollSmooth = true;
		}
	};

	function onSendCredentials(id: string, credentials: Record<string, string>) {
		thread?.sendCredentials(id, credentials);
	}

	async function loadOlderMessages() {
		if (!messages.lastRunID || !messages.messages.length || loadingOlderMessages) return;

		// Use the parentRunID from the messages object if available
		const previousRunID = messages.parentRunID;
		if (!previousRunID) {
			// No older messages, bail out
			return;
		}

		loadingOlderMessages = true;

		// Store current scroll position to anchor the view when older messages are loaded
		const scrollTop = threadContainer?.scrollTop || 0;
		const scrollHeight = threadContainer?.scrollHeight || 0;

		try {
			// Load older messages
			const oldThread = new Thread(project, {
				threadID: id,
				runID: previousRunID,
				follow: false,
				onError: () => {
					// Ignore errors
				}
			});

			// Wait for the thread to load the previous messages
			const prevMessages = await new Promise<Messages>((resolve) => {
				let resolved = false;
				oldThread.onMessages = (newMessages) => {
					if (oldThread.replayComplete && !resolved) {
						resolved = true;
						resolve(newMessages);
					}
				};

				// Set a timeout in case replayComplete is never triggered
				setTimeout(() => {
					if (!resolved) {
						resolved = true;
						resolve({ messages: [], inProgress: false });
					}
				}, 10000);
			});

			// Close the temporary thread
			oldThread.close();

			// Merge the previous messages with the current ones
			if (prevMessages.messages.length > 0) {
				const existingRunIDs = new Set(messages.messages.map((msg) => msg.runID));
				const newMessages = prevMessages.messages.filter((msg) => !existingRunIDs.has(msg.runID));

				// Update messages
				messages = {
					...messages,
					parentRunID: prevMessages.parentRunID,
					messages: [...newMessages, ...messages.messages]
				};

				// After the DOM updates, adjust the scroll position based on the actual height change
				scrollSmooth = false;
				requestAnimationFrame(() => {
					if (threadContainer) {
						const newScrollHeight = threadContainer.scrollHeight;
						const addedHeight = newScrollHeight - scrollHeight;
						threadContainer.scrollTop = scrollTop + addedHeight;
					}
				});
			} else {
				messages = {
					...messages,
					parentRunID: undefined
				};
			}
		} catch (error) {
			console.error('Error loading older messages:', error);
			messages = {
				...messages,
				parentRunID: undefined
			};
		} finally {
			loadingOlderMessages = false;
		}
	}

	// Function to fetch thread details
	async function fetchThreadDetails() {
		if (!id) return;

		try {
			const thread = await getThread(project.assistantID, project.id, id);
			threadDetails = thread;
		} catch (err) {
			console.error('Error fetching thread details:', err);
		}
	}

	onMount(() => {
		if (id) {
			fetchThreadDetails();
		}
	});

	$effect(() => {
		if (id && !threadDetails) {
			fetchThreadDetails();
		}
	});

	let projectDefaultModelProvider = $state<string>();
	let projectDefaultModel = $state<string>();

	let projectModelProvider = $derived(project.defaultModelProvider ?? projectDefaultModelProvider);
	let projectModel = $derived(project.defaultModel ?? projectDefaultModel);
	let lastMessageWithFile = $derived(messages.messages.findLastIndex((msg) => msg.file));

	// Detect Safari for browser-specific styling
	let isSafari = $derived(browser && /^((?!chrome|android).)*safari/i.test(navigator.userAgent));

	$effect(() => {
		if (!project.defaultModelProvider || !project.defaultModel) {
			getProjectDefaultModel(project.assistantID, project.id).then((res) => {
				projectDefaultModelProvider = res.modelProvider;
				projectDefaultModel = res.model;
			});
		}
	});

	// Handle model change in the thread
	function handleModelChanged() {
		ensureThread();
	}

	// Create a new Thread with model & model provider
	async function handleCreateThread(model?: string, modelProvider?: string) {
		await ensureThread({ model, modelProvider });

		// open created thread
		if (id) {
			handleNavigateToThread?.();
		}
	}

	// Select a thread by id
	function handleNavigateToThread() {
		if (responsive.isMobile) {
			layout.sidebarOpen = false;
		}

		layout.items = [];
		closeAll(layout);
		focusChat();
	}

	function focusChat() {
		const e = window.document.querySelector('#main-input textarea');
		if (e instanceof HTMLTextAreaElement) {
			e.focus();
		}
	}

	async function handleMcpPromptSelect(
		prompt: MCPServerPrompt,
		mcp: ProjectMCP,
		params?: Record<string, string>
	) {
		promptPending = true;
		const result = await ChatService.generateProjectMcpServerPrompt(
			project.assistantID,
			project.id,
			mcp.id,
			prompt.name,
			params
		);

		let promptContent = '';
		for (const message of result.messages) {
			if (message.content.type === 'text') {
				promptContent += message.content.text;
			} else if (message.content.type === 'resource') {
				promptContent += `\n\n${JSON.stringify(message.content.resource)}`;
			}
		}

		input?.setValue(promptContent);
		await tick();
		input?.focus();
		promptPending = false;
	}

	async function handleSaveNewProject() {
		if (!createProject) return;
		savingNewProject = true;
		const { name, description, prompt, icons } = createProject;
		const response = await EditorService.createObot({
			name,
			description,
			prompt,
			icons
		});
		createProject = undefined;
		savingNewProject = false;
		goto(`/o/${response.id}`);
	}
</script>

{#snippet editBasicSection()}
	<button
		aria-label="backdrop"
		class="fixed top-0 left-0 z-20 h-full w-full"
		onclick={() => {
			createProject = undefined;
		}}
	></button>
	<div class="relative z-30 mt-4 w-sm self-center border-2 border-transparent pt-4 md:w-md">
		<div class="flex flex-col items-center justify-center text-center">
			{#if createProject}
				<input
					id="project-name"
					type="text"
					placeholder="Project Name"
					class="ghost-input border-b-surface1 mb-2 w-full pt-0 pb-0 text-center text-base font-bold"
					bind:value={createProject.name}
					bind:this={nameInput}
				/>
				<textarea
					id="project-desc"
					class="ghost-input border-b-surface1 text-md scrollbar-none mb-4 w-full grow resize-none pt-0.5 pb-0 text-center font-light"
					rows="1"
					placeholder="A short description of your project"
					use:autoHeight
					bind:value={createProject.description}
				></textarea>

				<div class="mt-2 flex w-full flex-col justify-start text-left">
					<label for="project-prompt" class="text-on-surface1 mb-1 text-sm font-semibold"
						>Instructions</label
					>
					<textarea
						id="project-prompt"
						class="ghost-input bg-surface1 text-md scrollbar-none mb-4 w-full grow resize-none rounded-md p-4 text-left font-light shadow-inner"
						rows="4"
						placeholder={HELPER_TEXTS.prompt}
						use:autoHeight
						bind:value={createProject.prompt}
					></textarea>
				</div>
				<button
					class="button-primary w-full text-sm"
					disabled={savingNewProject}
					onclick={handleSaveNewProject}
				>
					{#if savingNewProject}
						<LoaderCircle class="size-4" />
					{:else}
						Save
					{/if}
				</button>
			{/if}
		</div>

		<button
			class="icon-button absolute top-2 right-2"
			onclick={() => {
				createProject = undefined;
			}}
		>
			<X class="size-6" />
		</button>

		<div
			class="bg-surface1 dark:bg-surface2 m-auto mt-4 h-[1px] w-96 max-w-sm self-center rounded-full"
		></div>
	</div>
{/snippet}

<div
	id="main-input"
	class="default-scrollbar-thin flex w-full grow justify-center overflow-y-auto"
	class:scroll-smooth={scrollSmooth}
	use:stickToBottom={{
		contentEl: messagesDiv,
		setControls: (controls) => (scrollControls = controls)
	}}
	onscrollend={onScrollEnd}
	bind:this={threadContainer}
>
	<div
		class={twMerge(
			'bottom-fade-bar pointer-events-none',
			layout.fileEditorOpen ? 'left-5' : 'left-1/2 -translate-x-1/2'
		)}
		style="width: {fadeBarWidth}px"
	></div>
	<div class="relative flex w-full max-w-[900px] flex-col">
		<div
			in:fade|global
			bind:this={messagesDiv}
			class={twMerge(
				'flex w-full flex-1 flex-col justify-start gap-8 p-5 transition-all',
				!centerInput ? 'grow' : 'h-0 overflow-hidden'
			)}
			class:justify-center={!thread}
		>
			{#if createProject}
				{@render editBasicSection()}
			{:else}
				{#if showLoadOlderButton}
					<div class="mb-4 flex justify-center">
						<button
							class="border-surface3 hover:bg-surface2 bg-background rounded-full border px-4 py-2 text-sm font-light transition-all duration-300"
							onclick={loadOlderMessages}
							disabled={loadingOlderMessages}
						>
							{#if loadingOlderMessages}
								<div
									class="inline-block h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent"
									role="status"
								>
									<span class="sr-only">Loading...</span>
								</div>
								<span class="ml-2">Loading...</span>
							{:else}
								Load older messages
							{/if}
						</button>
					</div>
				{/if}

				{#each messages.messages as msg, i (i)}
					<Message
						{project}
						{msg}
						currentThreadID={id}
						{onLoadFile}
						{onSendCredentials}
						onSendCredentialsCancel={() => thread?.abort()}
						onPreviewImage={(imgSrc) => {
							imagePreviewSrc = imgSrc;
							imagePreviewDialog?.showModal();
						}}
						compactFilePreview={!!layout.fileEditorOpen || i !== lastMessageWithFile}
					/>
				{/each}
			{/if}
			<div class={isSafari ? 'min-h-48' : 'min-h-4'}>
				<!-- Vertical Spacer - needs to be taller (like input size) in Safari due to sticky interaction -->
			</div>
		</div>

		<div
			class={twMerge(
				'bg-background sticky z-30 flex w-full justify-center pb-2 transition-transform duration-300',
				centerInput ? 'absolute top-1/2 -translate-y-[50%]' : 'bottom-0 -translate-y-[0%]'
			)}
		>
			<div class="w-full max-w-[1000px]">
				{#if centerInput && assistant?.introductionMessage}
					<div class="milkdown-content mb-5 max-w-full px-5" in:fade>
						{#if browser}
							{@html toHTMLFromMarkdown(assistant?.introductionMessage)}
						{/if}
					</div>
				{/if}
				<Input
					id="thread-input"
					bind:this={input}
					readonly={messages.inProgress || !hasModelSelected}
					pending={thread?.pending || promptPending}
					onAbort={async () => {
						await thread?.abort();
					}}
					onSubmit={async (i) => {
						const v = input?.getValue() ?? '';

						try {
							// Clear the input immediately to improve responsiveness
							tick().then(() => {
								input?.setValue('');
							});

							if (v?.startsWith('/') && promptDialogRef?.hasPromptHighlighted()) {
								promptDialogRef?.triggerSelectPrompt();
								return;
							}

							await ensureThread();
							scrollSmooth = false;
							await tick();
							scrollControls?.stickToBottom();
							await thread?.invoke(i);
							onInputChange?.('');
							isNew = false;
						} catch (_) {
							// If there's an error, restore the input value
							input?.setValue(v);
							console.error(_);
						}
					}}
					onArrowKeys={(direction) => {
						if (direction === 'up' && globalPromptIndex > 0) {
							globalPromptIndex--;
						} else if (direction === 'down' && globalPromptIndex < globalPromptCount - 1) {
							globalPromptIndex++;
						}
					}}
					{onInputChange}
					initialValue={inputValue}
					bind:items={layout.items}
				>
					<div class="flex w-full items-center justify-between">
						<div class="flex items-center">
							{#key id}
								<div in:fade>
									<Files
										thread
										{project}
										bind:currentThreadID={id}
										helperText="Files"
										classes={{ list: 'max-h-[60vh] space-y-4 overflow-y-auto pt-2 pb-6 text-sm' }}
									/>
								</div>
							{/key}
						</div>
						<ThreadModelSelector
							threadId={id}
							{project}
							projectDefaultModel={projectModel}
							projectDefaultModelProvider={projectModelProvider}
							onModelChanged={handleModelChanged}
							onCreateThread={handleCreateThread}
							bind:hasModelSelected
						/>
					</div>
					{#snippet inputPopover(value: string)}
						<McpPrompts
							bind:this={promptDialogRef}
							{project}
							variant="popover"
							filterText={value}
							onSelect={handleMcpPromptSelect}
							onClickOutside={() => {
								input?.clear();
							}}
							bind:selectedIndex={globalPromptIndex}
							bind:limit={globalPromptCount}
						/>
					{/snippet}
				</Input>
				{#if !centerInput}
					<div
						class="mt-3 grid grid-cols-[auto_auto] items-center justify-center gap-x-2 px-5 text-xs font-light"
					>
						<span class="text-on-surface1">Obot isn't perfect. Double check its work.</span>
						<a
							href="https://github.com/obot-platform/obot/issues/new?template=bug_report.md"
							target="_blank"
							rel="noopener noreferrer"
							class="whitespace-nowrap text-blue-500/50 hover:underline"
						>
							{#if responsive.isMobile}
								<Bug class="h-4 w-4" />
							{:else}
								Report issues here
							{/if}
						</a>
					</div>
				{/if}
			</div>
		</div>
	</div>
</div>

<dialog
	bind:this={imagePreviewDialog}
	use:clickOutside={() => {
		imagePreviewDialog?.close();
		imagePreviewSrc = undefined;
	}}
>
	<div
		class="default-scrollbar-thin relative flex max-h-[95vh] w-full flex-col overflow-y-auto p-4"
	>
		<div class="sticky top-0 left-0 z-10 w-full">
			<button
				class="icon-button absolute top-0 right-0"
				onclick={() => {
					imagePreviewDialog?.close();
					imagePreviewSrc = undefined;
				}}
				use:tooltip={'Close Preview'}
			>
				<X class="size-6" />
			</button>
		</div>
		{#if imagePreviewSrc}
			<img src={imagePreviewSrc} class="mx-auto max-w-full rounded-xl" alt="preview" />
		{/if}
	</div>
</dialog>

<style lang="postcss">
	.bottom-fade-bar {
		z-index: 20;
		position: absolute;
		bottom: 9rem;
		height: 3.5rem;
		max-width: 900px;
		background: linear-gradient(to bottom, transparent, var(--background));
	}

	.top-fade-bar {
		z-index: 20;
		position: absolute;
		top: 0;
		height: 3.5rem;
		max-width: 900px;
		background: linear-gradient(to top, transparent, var(--background));
	}
</style>
