<script lang="ts">
	import MessageIcon from '$lib/components/messages/MessageIcon.svelte';
	import { FileText, Copy, Edit, Info, X, Brain, FileSymlink, Download } from 'lucide-svelte/icons';
	import { Tween } from 'svelte/motion';
	import { ChatService, type Message, type Project } from '$lib/services';
	import highlight from 'highlight.js';
	import { toHTMLFromMarkdown } from '$lib/markdown.js';
	import { Paperclip } from 'lucide-svelte';
	import { formatTime } from '$lib/time';
	import { fly, slide } from 'svelte/transition';
	import Loading from '$lib/icons/Loading.svelte';
	import { fade } from 'svelte/transition';
	import { overflowToolTip } from '$lib/actions/overflow';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { ABORTED_BY_USER_MESSAGE, ABORTED_THREAD_MESSAGE } from '$lib/constants';
	import { hasTool } from '$lib/tools';
	import { getProjectTools } from '$lib/context/projectTools.svelte';
	import MemoriesDialog from '$lib/components/MemoriesDialog.svelte';
	import { linear } from 'svelte/easing';
	import { twMerge } from 'tailwind-merge';
	import { isTextFile } from '$lib/utils';

	interface Props {
		msg: Message;
		project: Project;
		currentThreadID?: string;
		onLoadFile?: (filename: string) => void;
		onPreviewImage?: (imgSrc: string) => void;
		onSendCredentials?: (id: string, credentials: Record<string, string>) => void;
		onSendCredentialsCancel?: (id: string) => void;
		disableMessageToEditor?: boolean;
		clearable?: boolean;
		expandable?: boolean;
		expanded?: boolean;
		noMemoryTool?: boolean;
		classes?: {
			container?: string;
			messageActions?: string;
			messageBody?: string;
			messageIcon?: string;
			nameAndTime?: string;
			oauth?: string;
			root?: string;
			prompt?: string;
		};
		compactFilePreview?: boolean;
	}

	let {
		msg,
		project,
		currentThreadID,
		onLoadFile = () => {},
		onPreviewImage,
		onSendCredentials = ChatService.sendCredentials,
		onSendCredentialsCancel,
		disableMessageToEditor,
		noMemoryTool,
		clearable = false,
		classes,
		compactFilePreview
	}: Props = $props();

	let content = $derived(
		msg.message
			? msg.message
					.join('')
					.replace(new RegExp(`${ABORTED_BY_USER_MESSAGE}|${ABORTED_THREAD_MESSAGE}`, 'g'), '')
			: ''
	);
	let fullWidth = $derived(!msg.sent && !msg.oauthURL && !msg.tool);
	let showBubble = $derived(msg.sent);
	let isPrompt = msg.fields && msg.promptId;
	let renderMarkdown = !msg.sent && !msg.oauthURL && !msg.tool;
	let shell = $state({
		input: '',
		output: ''
	});

	let promptCredentials = $state<Record<string, string>>({});
	let credentialsSubmitted = $state(false);

	let shouldAnimate = $derived(!msg.done && !msg.toolCall && !msg.promptId && !msg.sent);
	let cursor = new Tween(0);
	let prevContent = $state('');
	let animatedText = $derived(shouldAnimate ? content.slice(0, cursor.current) : content);
	let animating = $state(false);
	let showToolInputDetails = $state(false);
	let showToolOutputDetails = $state(false);

	let showCopied = $state(false);
	let copiedTimeout = $state<ReturnType<typeof setTimeout>>();

	// For file content, we want to animate the content as it's written
	let fileCursor = new Tween(0);
	let prevFileContent = $state('');
	let animatedFileContent = $derived(
		msg.file?.content && !msg.done && !compactFilePreview
			? msg.file.content.slice(0, fileCursor.current)
			: msg.file?.content || ''
	);

	// Check if this is an Memory tool message
	let isMemoryTool = $derived(
		msg.sourceName === 'Create Memory' ||
			msg.sourceName === 'Update Memory' ||
			msg.sourceName === 'Delete Memory'
	);

	$effect(() => {
		if (!shouldAnimate) return;

		if (!content.startsWith(prevContent)) {
			cursor.set(0, { duration: 0 });
		}
		prevContent = content;

		animating = true;
		cursor.set(content.length, { duration: 500 }).then(() => (animating = false));
	});

	$effect(() => {
		if (msg.file?.content && !msg.done && !compactFilePreview) {
			if (!msg.file.content.startsWith(prevFileContent)) {
				fileCursor.set(0, { duration: 0 });
			}
			prevFileContent = msg.file.content;

			fileCursor.set(msg.file.content.length, { duration: 300 });
		}
	});

	$effect(() => {
		if (msg.toolCall && msg.sourceName === 'Shell') {
			try {
				shell.input = JSON.parse(msg.toolCall?.input ?? '').CMD ?? '';
				shell.output = msg.toolCall?.output ?? '';
			} catch {
				return;
			}
		}
	});

	$effect(() => {
		// this is a hack to ensure the effect is run each time animatedText updates
		void animatedText;

		const blocks = document.querySelectorAll('.message-content pre > code');
		blocks.forEach((block) => {
			if (block instanceof HTMLElement && block.dataset.highlighted !== 'yes') {
				highlight.highlightElement(block);
			}
		});

		const links = document.querySelectorAll('.message-content a');
		links.forEach((link) => {
			if (link instanceof HTMLAnchorElement && link.target == '') {
				link.target = '_blank';
			}
		});
	});

	function fileLoad() {
		if (msg.file?.filename) {
			onLoadFile(msg.file?.filename);
		}
	}

	// Citations

	// citation urls starting with knowledge:// mean it's a local file
	const citationKnowledgePrefix = 'knowledge://';

	function citationURL(url: string | undefined) {
		if (!url) return undefined;

		if (url.startsWith(citationKnowledgePrefix)) {
			return `/api/assistants/${project.assistantID}/projects/${project.id}/knowledge/${url.slice(citationKnowledgePrefix.length)}`;
		}

		return url;
	}

	function citationDisplayURL(url: string) {
		if (url.startsWith(citationKnowledgePrefix)) {
			// return only the last path element (file name)
			return decodeURIComponent(url.split('::').pop() ?? url);
		}

		// remove the protocol and www.
		const res = decodeURIComponent(url).replace(/^(.+:\/\/)?(www\.)?/, '');
		return res.length > 25 ? res.slice(0, 25) + '...' : res;
	}

	function citationFavicon(url: string) {
		if (url.startsWith(citationKnowledgePrefix)) {
			return '/favicon';
		}

		const _url = new URL(url);
		return _url.origin + '/favicon.ico';
	}

	function deduplicateCitations(citations: string[]) {
		const seen = new Set<string>();
		return citations.filter((url) => {
			if (seen.has(url)) {
				return false;
			}
			seen.add(url);
			return true;
		});
	}

	function formatJson(jsonString: string) {
		try {
			const parsed = JSON.parse(jsonString);
			// Use null and 2 for consistent indentation, then trim any leading/trailing whitespace
			let formatted = JSON.stringify(parsed, null, 2).trim();

			// Replace decimal numbers (must come before integer replacement)
			formatted = formatted.replace(
				/: (\d+\.\d+)/g,
				': <span class="text-blue-600 dark:text-blue-400">$1</span>'
			);

			// Replace integer numbers
			formatted = formatted.replace(
				/: (\d+)(?!\d*\.)/g,
				': <span class="text-blue-600 dark:text-blue-400">$1</span>'
			);

			// Replace keys
			formatted = formatted.replace(/"([^"]+)":/g, '<span class="text-blue">"$1"</span>:');

			// Replace string values (must come after keys)
			formatted = formatted.replace(/: "([^"]+)"/g, ': <span class="text-gray-500">"$1"</span>');

			// Replace null
			formatted = formatted.replace(/: (null)/g, ': <span class="text-gray-500">$1</span>');

			// Replace brackets and braces
			formatted = formatted.replace(/(".*?")|([{}[\]])/g, (match, stringContent, bracket) => {
				if (stringContent) {
					// If it's part of a string (within quotes), return as-is
					return stringContent;
				}
				// If it's a bracket/brace outside of strings, wrap it
				return `<span class="text-black dark:text-white">${bracket}</span>`;
			});

			return formatted;
		} catch (_error) {
			return jsonString;
		}
	}

	async function copyContentToClipboard(textContent?: string) {
		if (copiedTimeout) {
			clearTimeout(copiedTimeout);
		}

		try {
			await navigator.clipboard.writeText(textContent ?? content);
			showCopied = true;

			copiedTimeout = setTimeout(() => {
				showCopied = false;
			}, 750);
		} catch (err) {
			console.error('Failed to copy message:', err);
		}
	}

	async function checkFileExists(filename: string) {
		try {
			const files = await ChatService.listFiles(project.assistantID, project.id, {
				threadID: currentThreadID
			});

			const fileExists = files.items.some((file) => file.name === filename);
			return fileExists;
		} catch (err) {
			console.error('Failed to check if file exists:', err);
			return false;
		}
	}

	async function openContentInEditor() {
		try {
			const filename = `obot-response-${msg.time?.getTime()}.md`;
			const fileExists = await checkFileExists(filename);
			if (!fileExists) {
				const file = new File([content], filename, { type: 'text/plain' });
				await ChatService.saveFile(project.assistantID, project.id, file, {
					threadID: currentThreadID
				});
			}

			onLoadFile(filename);
		} catch (err) {
			console.error('Failed to create or open file:', err);
		}
	}

	async function addImageContentInEditor({ mimeType, data }: { mimeType: string; data: string }) {
		try {
			const filename = `obot-image-${msg.time?.getTime()}.${mimeType.split('/')[1]}`;
			const fileExists = await checkFileExists(filename);
			if (!fileExists) {
				const blob = await fetch(`data:${mimeType};base64,${data}`).then((r) => r.blob());
				const file = new File([blob], filename, { type: mimeType });
				await ChatService.saveFile(project.assistantID, project.id, file, {
					threadID: currentThreadID
				});
			}

			onLoadFile(filename);
		} catch (err) {
			console.error('Failed to create or open image file:', err);
		}
	}

	const projectTools = noMemoryTool ? { tools: [] } : getProjectTools();
	let memoriesDialog = $state<ReturnType<typeof MemoriesDialog>>();
</script>

{#snippet timeAndUsername()}
	{#if msg.time}
		<div
			class="mt-1 flex items-center justify-end gap-2 self-end text-xs whitespace-nowrap text-gray-500"
		>
			<span>{formatTime(msg.time)}</span>
			{#if msg.username}
				<span class="text-gray-400">•</span>
				<span class="max-w-[100px] truncate font-medium">by {msg.username?.split(' ')[0]}</span>
			{/if}
		</div>
	{/if}
{/snippet}

{#snippet nameAndTime()}
	<div class={twMerge('mb-1 flex items-center space-x-2', classes?.nameAndTime)}>
		{#if msg.sourceName}
			<span class="text-sm font-semibold"
				>{msg.sourceName === 'Assistant' ? project?.name || 'Project' : msg.sourceName}</span
			>
		{/if}
		{#if msg.time}
			<span class="text-gray text-sm">{formatTime(msg.time)}</span>
		{/if}
		{#if !msg.done || animating}
			<Loading class="size-4" />
		{/if}

		{#if (msg.toolCall?.input || msg.toolCall?.output) && !msg.file}
			<div class="flex items-center gap-2">
				<button
					class="text-gray cursor-pointer text-xs underline"
					onclick={() => (showToolInputDetails = !showToolInputDetails)}
				>
					{showToolInputDetails ? 'Hide' : 'Show'} Details
				</button>

				{#if isMemoryTool && hasTool(projectTools.tools, 'memory')}
					<button
						class="text-gray flex cursor-pointer items-center gap-1 text-xs underline"
						onclick={() => memoriesDialog?.show()}
						use:tooltip={'Open memories'}
					>
						<Brain class="h-3 w-3" />
						Memories
					</button>
				{/if}
			</div>
		{/if}
	</div>
{/snippet}

{#snippet messageBody()}
	<div
		class:flex={showBubble}
		class:contents={!showBubble}
		class:message-content={renderMarkdown}
		class={twMerge(
			'bg-gray-70 flex w-full flex-col rounded-2xl px-6 py-3 text-black dark:bg-gray-950 dark:text-white',
			classes?.messageBody
		)}
	>
		{#if clearable}
			<button
				class="absolute top-0 right-0 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
				aria-label="Clear message"
				onclick={() => (msg.ignore = true)}
			>
				<X class="icon-default" />
			</button>
		{/if}
		{#if msg.oauthURL}
			{@render oauth()}
		{:else if msg.toolCall || msg.tool}
			{@render toolContent()}
		{:else if content}
			{#if ['Abort Current Task', 'Loop Data', 'Search'].every((d) => d !== msg.sourceName)}
				{@render messageContent()}
			{/if}
		{/if}

		{@render files()}
		{@render citations()}
	</div>
{/snippet}

{#snippet files()}
	{#if msg.file}
		<button
			class={twMerge(
				'w-[750px]cursor-pointer my-2 flex max-w-full flex-col rounded-3xl border border-gray-300 bg-white text-start text-black shadow-lg dark:bg-black dark:text-gray-50',
				!msg.file?.filename && 'cursor-wait',
				compactFilePreview ? 'w-md pb-4' : 'w-md md:w-[750px]'
			)}
			disabled={!msg.file?.filename}
			onclick={fileLoad}
		>
			<div class="text-md flex justify-between gap-2 px-5 pt-4">
				<div class="flex items-center gap-2 truncate">
					<FileText class="size-4" />
					{#if msg.file?.filename}
						<span use:overflowToolTip>{msg.file.filename}</span>
					{:else}
						<span class="text-gray-400 dark:text-gray-600">...</span>
					{/if}
				</div>
			</div>
			{#if !compactFilePreview}
				<div class="relative" transition:slide={{ axis: 'y', duration: 50 }}>
					<div class="font-body text-md p-5 whitespace-pre-wrap text-gray-700 dark:text-gray-300">
						{#if msg.file?.content}
							{animatedFileContent}
						{/if}
					</div>
					{#if !msg.done}
						<div
							class="absolute bottom-0 z-20 h-24 w-full rounded-3xl bg-linear-to-b from-transparent to-white dark:to-black"
						></div>
					{/if}
				</div>
			{/if}
		</button>
		{#if msg.file.content && isTextFile(msg.file.filename)}
			<div class="flex gap-2">
				<div>
					<button
						use:tooltip={'Copy file contents to clipboard'}
						class="icon-button-small"
						onclick={() => copyContentToClipboard(msg.file?.content)}
					>
						<Copy class="h-4 w-4" />
					</button>
				</div>
			</div>
		{/if}
	{/if}
{/snippet}

{#snippet explain()}
	{#if msg.explain}
		<div
			role="none"
			class="-m-6 mt-2 -mb-4 flex flex-col
		 divide-y divide-gray-300
		 rounded-3xl border
		 border-gray-300 bg-white
		 text-black shadow-lg
		   dark:bg-black
		    dark:text-gray-50"
		>
			<div class="flex gap-2 px-5 py-4">
				<Paperclip />
				<span>Selection from</span>
				<button
					class="font-medium"
					onclick={() => {
						if (msg.explain?.filename) {
							onLoadFile(msg.explain.filename);
						}
					}}>{msg.explain.filename}</button
				>
			</div>
			<div class="font-body text-md p-5 whitespace-pre-wrap text-gray-700 dark:text-gray-300">
				{msg.explain.selection}
			</div>
		</div>
	{/if}
{/snippet}

{#snippet toolDetails(stringifiedJson: string, title: string)}
	<div class="flex w-full flex-col gap-1">
		<p class="p-0 text-xs font-semibold">{title}</p>
		<pre
			transition:slide={{ duration: 300 }}
			class="default-scrollbar-thin bg-surface1 mt-0! max-h-[300px] w-fit max-w-full overflow-auto rounded-lg px-4 py-2 text-xs break-all whitespace-pre-wrap text-black dark:text-white">{@html formatJson(
				stringifiedJson ?? ''
			)}</pre>
	</div>
{/snippet}

{#snippet toolContent()}
	{#if msg.toolCall?.input && showToolInputDetails}
		{@const parsedInput = (() => {
			try {
				return JSON.parse(msg.toolCall.input);
			} catch {
				return null;
			}
		})()}
		<div
			transition:slide={{ duration: 300, easing: linear }}
			class="mb-4 flex w-full flex-col justify-start gap-4"
		>
			{#if parsedInput}
				{@render toolDetails(msg.toolCall.input, 'Input')}
			{/if}
			{#if msg.toolCall?.output}
				<button
					class="text-gray w-fit text-xs underline"
					onclick={() => (showToolOutputDetails = !showToolOutputDetails)}
				>
					{showToolOutputDetails ? 'Hide' : 'Show'} Output
				</button>
				{#if showToolOutputDetails}
					{@render toolDetails(msg.toolCall.output, 'Output')}
				{/if}
			{/if}
		</div>
	{/if}
	{#if msg.toolCall?.output}
		{@const parsedOutput = (() => {
			try {
				return JSON.parse(msg.toolCall.output);
			} catch {
				return null;
			}
		})()}
		{#if parsedOutput?.content}
			{#each parsedOutput.content as content, i (i)}
				{#if content.type === 'image' && content.mimeType && content.data}
					{@const imgSrc = `data:${content.mimeType};base64,${content.data}`}
					<div class="flex flex-col gap-2">
						<button class="mt-2 w-full max-w-md" onclick={() => onPreviewImage?.(imgSrc)}>
							<img
								src={imgSrc}
								alt="tool output"
								class="rounded-xl"
								onerror={(e) => {
									const img = e.currentTarget as HTMLImageElement;
									img.style.display = 'none';
									const fallbackDiv = document.createElement('div');
									fallbackDiv.className = 'text-gray-500 text-xs';
									fallbackDiv.textContent = '[Image Failed to Load.]';
									img.parentNode?.insertBefore(fallbackDiv, img.nextSibling);
								}}
							/>
						</button>
						<div class="flex gap-2">
							<button
								class="icon-button-small"
								use:tooltip={'Add to Workspace Files'}
								onclick={() => addImageContentInEditor(content)}
							>
								<FileSymlink class="size-4" />
							</button>
							{@render downloadImageButton(
								`data:${content.mimeType};base64,${content.data}`,
								`image-${Date.now()}.${content.mimeType.split('/')[1]}`
							)}
						</div>
					</div>
				{/if}
			{/each}
		{:else if parsedOutput?.images}
			{#each parsedOutput.images as image, i (i)}
				<div class="flex flex-col gap-2">
					<button class="mt-2 w-full max-w-md" onclick={() => onPreviewImage?.(image?.downloadUrl)}>
						<img src={image?.downloadUrl} alt="tool output" class="rounded-xl" />
					</button>
					<div class="flex gap-2">
						{@render downloadImageButton(image?.downloadUrl, image?.workspaceFilePath)}
					</div>
				</div>
			{/each}
		{/if}
	{/if}
	{#if shell.input && shell.output}
		<div class="mt-1 rounded-3xl bg-gray-100 p-5 dark:bg-gray-900 dark:text-gray-50">
			<div class="pb-1 font-mono">
				> {shell.input}
			</div>
			<div class="font-mono">
				{shell.output}
			</div>
		</div>
	{/if}
{/snippet}

{#snippet messageContent()}
	{#if msg.sent}
		{#each content.split('\n') as line, i (i)}
			<p class="text-md">{line}</p>
		{/each}
		{@render explain()}
	{:else}
		<div class="message-content" transition:fade={{ duration: 1000 }}>
			{@html toHTMLFromMarkdown(animatedText)}
		</div>
	{/if}
{/snippet}

{#snippet oauth()}
	<a
		href={msg.oauthURL}
		class={twMerge(
			'bg-blue rounded-xl p-4 text-center text-white transition-colors duration-300 hover:bg-blue-400 hover:text-white',
			classes?.oauth
		)}
		target="_blank"
	>
		<p>Authentication is required.</p>
		<p><span class="underline">Click here</span> to log-in using OAuth.</p>
	</a>
{/snippet}

{#snippet promptAuth()}
	{#if msg.fields && !credentialsSubmitted}
		{@const isAuthMethodSelectionPrompt =
			msg.fields.length === 1 &&
			msg.fields[0].name === 'Authentication Method' &&
			msg.fields[0].options}
		<form
			class="flex flex-col gap-2"
			onsubmit={(e) => {
				e.preventDefault();
				if (msg.promptId) {
					onSendCredentials(msg.promptId, promptCredentials);
					credentialsSubmitted = true;
				}
			}}
		>
			{@html toHTMLFromMarkdown(msg.message.join('\n'))}

			{#if isAuthMethodSelectionPrompt && msg.fields && msg.fields[0].options}
				<div class="flex flex-col">
					<div class="flex w-full flex-col items-center gap-2">
						<div class="inline-flex flex-col gap-2">
							{#each msg.fields[0].options as option, i (i)}
								<button
									class="button-primary"
									onclick={() => (promptCredentials[msg.fields![0].name] = option)}
								>
									{option}
								</button>
							{/each}
							{#if onSendCredentialsCancel}
								<button
									class="button-secondary hover:bg-surface3 border-transparent"
									type="button"
									onclick={() => onSendCredentialsCancel(msg.promptId ?? '')}
								>
									Never mind, don't authenticate
								</button>
							{/if}
						</div>
					</div>
				</div>
			{:else}
				{#each msg.fields as field, i (i)}
					<div class="flex flex-col gap-1">
						<label for={field.name} class="mt-1 text-sm font-medium">{field.name}</label>
						{#if field.options}
							<div class="flex flex-col gap-2">
								{#each field.options as option, i (i)}
									<button class="button" onclick={() => (promptCredentials[field.name] = option)}>
										{option}
									</button>
								{/each}
							</div>
						{:else}
							<input
								class="rounded-lg bg-white p-2 outline-hidden dark:bg-gray-900"
								type={field.sensitive ? 'password' : 'text'}
								name={field.name}
								bind:value={promptCredentials[field.name]}
							/>
						{/if}
						{#if field.description}
							<p class="text-xs text-gray-500">{field.description}</p>
						{/if}
					</div>
				{/each}

				<span class="text-gray mt-1 flex grow items-end self-end text-sm">
					*The submitted contents are not visible to AI.
				</span>

				<div class="item-center flex gap-2 self-end">
					{#if onSendCredentialsCancel}
						<button
							class="button-secondary"
							type="button"
							onclick={() => onSendCredentialsCancel(msg.promptId ?? '')}
						>
							Cancel
						</button>
					{/if}
					<button class="button-primary" type="submit">Submit</button>
				</div>
			{/if}
		</form>
	{/if}
{/snippet}

{#snippet citations()}
	{#if msg.citations && msg.citations.length > 0}
		<div class="mb-4 flex flex-wrap gap-2">
			{#each deduplicateCitations(msg.citations
					.map((c) => c.url)
					.filter((url) => url !== undefined)) as url, i (i)}
				{#if msg.done}
					<a
						href={citationURL(url)}
						target="_blank"
						class="flex w-fit items-center gap-2 rounded-full bg-gray-100 p-2 text-sm dark:bg-gray-900"
						transition:fly={{ y: 100, delay: 50 * i, duration: 250 }}
					>
						<img
							src={citationFavicon(url)}
							alt="Favicon"
							class="size-4"
							onerror={(e) => ((e.currentTarget as HTMLImageElement).src = '/favicon.ico')}
						/>
						{citationDisplayURL(url)}
					</a>
				{/if}
			{/each}
		</div>
	{/if}
{/snippet}

{#if !msg.ignore}
	{@const isAbortedContent =
		msg.aborted ||
		msg.message.at(-1)?.toLowerCase().endsWith(ABORTED_BY_USER_MESSAGE) ||
		msg.message.at(-1)?.toLowerCase().endsWith(ABORTED_THREAD_MESSAGE)}
	<div
		class={twMerge(
			'group relative flex items-start gap-3',
			isPrompt && '-m-5 rounded-3xl bg-gray-100 p-5 dark:bg-gray-950',
			isPrompt && classes?.prompt,
			classes?.root
		)}
		class:justify-end={msg.sent}
		class:opacity-30={msg.aborted || isAbortedContent}
	>
		{#if !msg.sent}
			<MessageIcon class={classes?.messageIcon} {msg} />
		{/if}

		<div class={twMerge('flex flex-col', classes?.container)} class:w-full={fullWidth}>
			{#if isPrompt}
				{@render promptAuth()}
			{:else}
				{#if !msg.sent}
					{@render nameAndTime()}
				{/if}
				{@render messageBody()}
				{#if msg.sent}
					{@render timeAndUsername()}
				{/if}

				{#if !msg.sent && msg.done && !msg.toolCall && msg.time && content && !animating && content.length > 0}
					<div class={twMerge('mt-2 -ml-1 flex gap-2', classes?.messageActions)}>
						<div>
							<button
								use:tooltip={showCopied ? 'Copied!' : 'Copy message to clipboard'}
								class="icon-button-small"
								onclick={() => copyContentToClipboard()}
							>
								<Copy class="h-4 w-4" />
							</button>
						</div>

						{#if !disableMessageToEditor}
							<div>
								<button
									use:tooltip={'Open message in editor'}
									class="icon-button-small"
									onclick={() => openContentInEditor()}
								>
									<Edit class="h-4 w-4" />
								</button>
							</div>
						{/if}
					</div>
				{/if}

				{#if isAbortedContent}
					<div class="mt-2 flex w-full items-center gap-1" class:justify-end={msg.sent}>
						<div class="flex-shrink-0">
							<Info class="size-3" />
						</div>
						<p class="mb-0 text-xs">
							Aborted. This {msg.toolCall ? 'call' : 'message'} has been discarded.
						</p>
					</div>
				{/if}
			{/if}
		</div>
	</div>
{/if}

<MemoriesDialog bind:this={memoriesDialog} {project} />

{#snippet downloadImageButton(data: string, filename: string)}
	<button
		class="icon-button-small"
		use:tooltip={'Download'}
		onclick={() => {
			const link = document.createElement('a');
			link.href = data;
			link.download = filename;
			document.body.appendChild(link);
			link.click();
			document.body.removeChild(link);
		}}
	>
		<Download class="size-4" />
	</button>
{/snippet}

<style lang="postcss">
	/* The :global is to get rid of warnings about the selector not being found */
	:global {
		.message-content {
			& h1 {
				margin-top: 1rem;
				margin-bottom: 1rem;
				font-size: 1.5rem; /* text-2xl */
				font-weight: 800; /* font-extrabold */
				color: black;
				.dark & {
					color: var(--color-gray-100);
				}
			}

			& h2 {
				margin-top: 1rem;
				margin-bottom: 1rem;
				font-size: 1.25rem; /* text-xl */
				font-weight: 700; /* font-bold */
				color: black;
				.dark & {
					color: var(--color-gray-100);
				}
			}

			& h3,
			& h4 {
				margin-top: 1rem;
				margin-bottom: 1rem;
				font-size: 1rem; /* text-base */
				font-weight: 700; /* font-bold */
				color: black;
				.dark & {
					color: var(--color-gray-100);
				}
			}

			& h5 {
				margin-top: 1rem;
				margin-bottom: 1rem;
				font-size: 1rem; /* text-base */
				font-weight: 600; /* font-semibold */
				color: black;
				.dark & {
					color: var(--color-gray-100);
				}
			}

			& h6 {
				margin-top: 1rem;
				margin-bottom: 1rem;
				font-size: 1rem; /* text-base */
				font-weight: 700; /* font-bold */
				color: black;
				.dark & {
					color: var(--color-gray-100);
				}
			}

			& p {
				margin-bottom: 1rem;
				font-size: var(--text-md);
				color: var(--color-gray-900);
				.dark & {
					color: var(--color-gray-100);
				}

				&:last-child {
					margin-bottom: 0;
				}
			}

			& a {
				font-weight: 500; /* font-medium */
				color: var(--color-blue-600);
				&:hover {
					text-decoration: underline;
				}
				.dark & {
					color: var(--color-gray-400);
				}
			}

			& ul {
				position: relative;
				margin-bottom: 1rem;
				display: flex;
				flex-direction: column;
				list-style-position: outside;
				list-style-type: disc;
				padding-left: 18px;
				padding-right: 18px;
				color: var(--color-gray-900);
				&::marker {
					color: var(--color-gray-900);
				}
				.dark & {
					color: var(--color-gray-100);
					&::marker {
						color: var(--color-gray-100);
					}
				}
			}

			& ol {
				position: relative;
				margin-bottom: 1rem;
				display: flex;
				flex-direction: column;
				list-style-position: outside;
				list-style-type: decimal;
				padding-left: 18px;
				padding-right: 18px;
				color: var(--color-gray-900);
				&::marker {
					color: var(--color-gray-900);
				}
				.dark & {
					color: var(--color-gray-100);
					&::marker {
						color: var(--color-gray-100);
					}
				}
			}

			& ul li {
				font-size: var(--text-md);
				padding-left: 0.5rem; /* ps-2 */
			}

			& code {
				scrollbar-width: none;
				font-size: 0.75rem; /* text-xs */
				@media (min-width: 768px) {
					font-size: var(--text-md);
				}
			}

			& hr {
				margin: 1rem 0;
			}
		}

		span[data-end-indicator] {
			visibility: hidden;
		}

		.loading-container span[data-end-indicator] {
			visibility: visible;
			position: relative;
			margin-top: -2px;
			margin-left: 0.25rem;
			display: inline-block;
			height: 1rem; /* size-4 */
			width: 1rem; /* size-4 */
			border-radius: 9999px;
			background-color: var(--color-gray-400);
			vertical-align: middle;
			color: transparent;
			animation: pulse 2s cubic-bezier(0.4, 0, 0.6, 1) infinite;
		}

		@keyframes pulse {
			0%,
			100% {
				opacity: 1;
			}
			50% {
				opacity: 0.5;
			}
		}
	}
</style>
