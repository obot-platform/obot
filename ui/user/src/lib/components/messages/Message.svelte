<script lang="ts">
	import MessageIcon from '$lib/components/messages/MessageIcon.svelte';
	import { FileText, Pencil, Copy, Edit, Info, X } from 'lucide-svelte/icons';
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

	interface Props {
		msg: Message;
		project: Project;
		currentThreadID?: string;
		onLoadFile?: (filename: string) => void;
		onSendCredentials?: (id: string, credentials: Record<string, string>) => void;
		onSendCredentialsCancel?: (id: string) => void;
		disableMessageToEditor?: boolean;
		clearable?: boolean;
	}

	let {
		msg,
		project,
		currentThreadID,
		onLoadFile = () => {},
		onSendCredentials = ChatService.sendCredentials,
		onSendCredentialsCancel,
		disableMessageToEditor,
		clearable = false
	}: Props = $props();

	let content = $derived(
		msg.message
			? msg.message
					.join('')
					.replace(new RegExp(`${ABORTED_BY_USER_MESSAGE}|${ABORTED_THREAD_MESSAGE}`, 'g'), '')
			: ''
	);
	let fullWidth = !msg.sent && !msg.oauthURL && !msg.tool;
	let showBubble = msg.sent;
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

	async function copyContentToClipboard() {
		try {
			await navigator.clipboard.writeText(content);
		} catch (err) {
			console.error('Failed to copy message:', err);
		}
	}

	async function openContentInEditor() {
		try {
			const filename = `obot-response-${msg.time?.getTime()}.md`;
			const files = await ChatService.listFiles(project.assistantID, project.id, {
				threadID: currentThreadID
			});

			const fileExists = files.items.some((file) => file.name === filename);
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
</script>

{#snippet time()}
	{#if msg.time}
		<span class="text-gray mt-2 self-end text-sm">{formatTime(msg.time)}</span>
	{/if}
{/snippet}

{#snippet nameAndTime()}
	<div class="mb-1 flex items-center space-x-2">
		{#if msg.sourceName}
			<span class="text-sm font-semibold"
				>{msg.sourceName === 'Assistant' ? project?.name || 'Obot' : msg.sourceName}</span
			>
		{/if}
		{#if msg.time}
			<span class="text-gray text-sm">{formatTime(msg.time)}</span>
		{/if}
		{#if !msg.done || animating}
			<Loading class="size-4" />
		{/if}

		{#if (msg.toolCall?.input || msg.toolCall?.output) && !msg.file}
			<button
				class="text-gray cursor-pointer text-xs underline"
				onclick={() => (showToolInputDetails = !showToolInputDetails)}
			>
				{showToolInputDetails ? 'Hide' : 'Show'} Details
			</button>
		{/if}
	</div>
{/snippet}

{#snippet messageBody()}
	<div
		class:flex={showBubble}
		class:contents={!showBubble}
		class:message-content={renderMarkdown}
		class="bg-gray-70 flex w-full flex-col rounded-2xl px-6 py-3 text-black dark:bg-gray-950 dark:text-white"
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
		{:else if msg.toolCall}
			{@render toolContent()}
		{:else if content}
			{#if msg.sourceName !== 'Abort Current Task'}
				{@render messageContent()}
			{/if}
		{/if}

		{@render files()}
		{@render citations()}
	</div>
{/snippet}

{#snippet files()}
	{#if msg.file?.filename}
		<button
			class="my-2 flex max-w-[750px] cursor-pointer flex-col divide-y divide-gray-300 overflow-x-auto rounded-3xl border border-gray-300 bg-white text-start text-black shadow-lg dark:bg-black dark:text-gray-50"
			onclick={fileLoad}
		>
			<div class="text-md flex justify-between gap-2 px-5 py-4">
				<div class="flex items-center gap-2 truncate">
					<FileText class="min-w-fit" />
					<span use:overflowToolTip>{msg.file.filename}</span>
				</div>
				<div>
					<Pencil />
					<span class="sr-only">Open</span>
				</div>
			</div>
			<div class="relative">
				<div class="font-body text-md p-5 whitespace-pre-wrap text-gray-700 dark:text-gray-300">
					{msg.file.content.split('\n').splice(0, 6).join('\n')}
				</div>
				<div
					class="absolute bottom-0 z-20 h-24 w-full rounded-3xl bg-linear-to-b from-transparent to-white dark:to-black"
				></div>
			</div>
		</button>
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
			class="default-scrollbar-thin bg-surface1 max-h-[300px] w-fit max-w-full overflow-auto rounded-lg px-4 py-2 text-xs break-all whitespace-pre-wrap">{@html formatJson(
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
		<div transition:slide={{ duration: 300 }} class="mb-4 flex w-full flex-col justify-start gap-4">
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
		{#each content.split('\n') as line}
			<p class="text-md">{line}</p>
		{/each}
		{@render explain()}
	{:else}
		<div transition:fade={{ duration: 1000 }}>
			{@html toHTMLFromMarkdown(animatedText)}
		</div>
	{/if}
{/snippet}

{#snippet oauth()}
	<a
		href={msg.oauthURL}
		class="bg-blue rounded-3xl
						p-4
						text-white
					  hover:bg-blue-400"
		target="_blank"
		>Authentication is required
		<span class="underline">click here</span> to log-in using OAuth
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
							{#each msg.fields[0].options as option}
								<button
									class="button whitespace-nowrap"
									onclick={() => (promptCredentials[msg.fields![0].name] = option)}
								>
									{option}
								</button>
							{/each}
							{#if onSendCredentialsCancel}
								<button
									class="button-secondary whitespace-nowrap"
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
				{#each msg.fields as field}
					<div class="flex flex-col gap-1">
						<label for={field.name} class="mt-1 text-sm font-medium">{field.name}</label>
						{#if field.options}
							<div class="flex flex-col gap-2">
								{#each field.options as option}
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
							<p class="text-sm text-gray-500">{field.description}</p>
						{/if}
					</div>
				{/each}

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
				<span class="text-gray mt-1 flex grow items-end self-end text-sm">
					*The submitted contents are not visible to AI.
				</span>
			{/if}
		</form>
	{/if}
{/snippet}

{#snippet citations()}
	{#if msg.citations && msg.citations.length > 0}
		<div class="mb-4 flex flex-wrap gap-2">
			{#each deduplicateCitations(msg.citations
					.map((c) => c.url)
					.filter((url) => url !== undefined)) as url, i}
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
		class="group relative flex items-start gap-3 {isPrompt
			? '-m-5 rounded-3xl bg-gray-100 p-5 dark:bg-gray-950'
			: ''}"
		class:justify-end={msg.sent}
		class:opacity-30={msg.aborted || isAbortedContent}
	>
		{#if !msg.sent}
			<MessageIcon {msg} />
		{/if}

		<div class="flex w-full flex-col" class:w-full={fullWidth}>
			{#if isPrompt}
				{@render promptAuth()}
			{:else}
				{#if !msg.sent}
					{@render nameAndTime()}
				{/if}
				{@render messageBody()}
				{#if msg.sent}
					{@render time()}
				{/if}

				{#if !msg.sent && msg.done && !msg.toolCall && msg.time && content && !animating && content.length > 0}
					<div class="mt-2 -ml-1 flex gap-2">
						<div>
							<button
								use:tooltip={'Copy message to clipboard'}
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
