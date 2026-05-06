<script lang="ts">
	import CopyButton from '../CopyButton.svelte';
	import { ChevronLeft, ChevronRight, Plus } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fade, fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		servers: {
			url: string;
			name: string;
		}[];
	}

	let { servers }: Props = $props();
	let scrollContainer: HTMLUListElement;
	let showLeftChevron = $state(false);
	let showRightChevron = $state(false);

	const optionMap: Record<string, { label: string; icon: string }> = {
		cursor: {
			label: 'Cursor',
			icon: '/user/images/assistant/cursor-mark.svg'
		},
		claude: {
			label: 'Claude',
			icon: '/user/images/assistant/claude-mark.svg'
		},
		vscode: {
			label: 'VSCode',
			icon: '/user/images/assistant/vscode-mark.svg'
		}
	};

	const options = Object.keys(optionMap).map((key) => ({ key, value: optionMap[key] }));
	let selected = $state(options[0].key);
	let previousSelected = $state(options[0].key);
	let isAnimating = $state(false);
	let flyDirection = $state(100); // 100 for right, -100 for left

	/** Base64-encodes a UTF-8 string (browser-safe; replaces Node's Buffer). */
	function utf8ToBase64(value: string): string {
		const bytes = new TextEncoder().encode(value);
		const chunkSize = 0x8000;
		let binary = '';
		for (let i = 0; i < bytes.length; i += chunkSize) {
			binary += String.fromCharCode(...bytes.subarray(i, i + chunkSize));
		}
		return btoa(binary);
	}

	function generateMcpLinks(name: string, config: Record<string, unknown>) {
		const configString = JSON.stringify(config);

		const cursorBase64 = utf8ToBase64(configString);
		const cursorLink = `cursor://anysphere.cursor-deeplink/mcp/install?name=${name}&config=${cursorBase64}`;

		const vscodePayload = JSON.stringify({ name, ...config });
		const vscodeEncoded = encodeURIComponent(vscodePayload);
		const vscodeLink = `vscode:mcp/install?${vscodeEncoded}`;

		return { cursorLink, vscodeLink };
	}

	function getFlyDirection(newSelection: string, oldSelection: string): number {
		const newIndex = options.findIndex((option) => option.key === newSelection);
		const oldIndex = options.findIndex((option) => option.key === oldSelection);

		// If new selection is before old selection, fly from left to right
		// If new selection is after old selection, fly from right to left
		return newIndex < oldIndex ? -100 : 100;
	}

	function checkScrollPosition() {
		if (!scrollContainer) return;

		const { scrollLeft, scrollWidth, clientWidth } = scrollContainer;
		showLeftChevron = scrollLeft > 0;
		showRightChevron = scrollLeft < scrollWidth - clientWidth - 1; // -1 for rounding errors
	}

	function scrollLeft() {
		if (scrollContainer) {
			scrollContainer.scrollBy({ left: -200, behavior: 'smooth' });
		}
	}

	function scrollRight() {
		if (scrollContainer) {
			scrollContainer.scrollBy({ left: 200, behavior: 'smooth' });
		}
	}

	function handleSelectionChange(newSelection: string) {
		if (newSelection !== selected) {
			previousSelected = selected;
			selected = newSelection;
			flyDirection = getFlyDirection(newSelection, previousSelected);
			isAnimating = true;

			// Reset animation state after animation completes
			setTimeout(() => {
				isAnimating = false;
			}, 300); // Match the CSS animation duration
		}
	}

	onMount(() => {
		checkScrollPosition();
		scrollContainer?.addEventListener('scroll', checkScrollPosition);
		window.addEventListener('resize', checkScrollPosition);

		return () => {
			scrollContainer?.removeEventListener('scroll', checkScrollPosition);
			window.removeEventListener('resize', checkScrollPosition);
		};
	});

	const mcpLinks = $derived(
		new Map(
			servers.map((server) => [
				server.name,
				generateMcpLinks(server.name, {
					command: 'npx',
					args: ['mcp-remote@latest', server.url]
				})
			])
		)
	);
</script>

<div class="flex w-full items-center gap-2">
	<div class="size-4">
		{#if showLeftChevron}
			<button onclick={scrollLeft}>
				<ChevronLeft class="size-4" />
			</button>
		{/if}
	</div>

	<ul
		bind:this={scrollContainer}
		class="default-scrollbar-thin scrollbar-none flex overflow-x-auto"
		style="scroll-behavior: smooth;"
	>
		{#each options as option (option.key)}
			<li class="w-49 flex-shrink-0">
				<button
					class={twMerge(
						'dark:hover:bg-surface3 relative flex w-full items-center justify-center gap-1.5 rounded-t-xs border-b-2 border-transparent py-2 text-[13px] font-light transition-all duration-200 hover:bg-gray-50',
						selected === option.key &&
							'dark:bg-surface2 bg-background hover:bg-transparent dark:hover:bg-transparent'
					)}
					onclick={() => {
						handleSelectionChange(option.key);
					}}
				>
					<img
						src={option.value.icon}
						alt={option.value.label}
						class="size-5 rounded-sm p-0.5 dark:bg-gray-600"
					/>
					{option.value.label}

					{#if selected === option.key}
						<div
							class={twMerge(
								'bg-primary absolute right-0 bottom-0 left-0 h-0.5 origin-left',
								isAnimating && selected === option.key ? 'border-slide-in' : ''
							)}
						></div>
					{:else if isAnimating && previousSelected === option.key}
						<div
							class="border-slide-out bg-primary absolute right-0 bottom-0 left-0 h-0.5 origin-left"
						></div>
					{/if}
				</button>
			</li>
		{/each}
	</ul>

	<div class="size-4">
		{#if showRightChevron}
			<button onclick={scrollRight}>
				<ChevronRight class="size-4" />
			</button>
		{/if}
	</div>
</div>

<div class="w-full overflow-hidden">
	<div class="flex min-h-[380px] w-[200%]">
		{#each options as option (option.key)}
			{#if selected === option.key}
				<div
					in:fly={{ x: flyDirection, duration: 200, delay: 200 }}
					out:fade={{ duration: 150 }}
					class="w-1/2 p-4"
				>
					{#if option.key === 'cursor'}
						<div class="flex flex-col gap-2 w-full items-center justify-center mb-4">
							{#each mcpLinks.entries() as [name, links] (name)}
								{@const link = links.cursorLink}
								<a
									href={link}
									rel="noopener noreferrer external"
									target="_blank"
									class="cursor-link group"
								>
									<span class="relative inline-flex size-4 shrink-0 items-center justify-center">
										<img
											src={option.value.icon}
											alt={`${option.value.label} icon`}
											class="size-4 rounded-sm bg-surface3 p-px transition-opacity duration-200 ease-in-out group-hover:opacity-0"
										/>
										<Plus
											class="absolute size-4 text-current opacity-0 transition-opacity duration-200 ease-in-out group-hover:opacity-100"
											strokeWidth={2.5}
											aria-hidden="true"
										/>
									</span>
									Add To Cursor
								</a>
							{/each}
						</div>
						<div class="divider">OR</div>
						<p>
							To add this MCP server to Cursor, update your <span class="snippet"
								>~/.cursor/mcp.json</span
							>
						</p>
						{@render codeSnippet(`
	{
		"mcpServers": {
${servers
	.map(
		(server) => `			"${server.name}": {
				"url": "${server.url}"
			}`
	)
	.join(',\n')}
		}
	}

`)}
					{:else if option.key === 'claude'}
						<p>
							To add this MCP server to Claude Desktop, update your <span class="snippet"
								>claude_desktop_config.json</span
							>
						</p>
						{@render codeSnippet(`
	{
		"mcpServers": {
${servers
	.map(
		(server) => `			"${server.name}": {
				"command": "npx",
				"args": [
					"mcp-remote@latest",
					"${server.url}"
				]
			}`
	)
	.join(',\n')}
		}
	}

`)}
					{:else if option.key === 'vscode'}
						<div class="flex flex-col gap-2 w-full items-center justify-center mb-4">
							{#each mcpLinks.entries() as [name, links] (name)}
								{@const link = links.vscodeLink}
								<a
									href={link}
									rel="noopener noreferrer external"
									target="_blank"
									class="vscode-link group"
								>
									<span class="relative inline-flex size-4 shrink-0 items-center justify-center">
										<img
											src={option.value.icon}
											alt={`${option.value.label} icon`}
											class="size-4 rounded-sm p-px transition-opacity duration-200 ease-in-out group-hover:opacity-0"
										/>
										<Plus
											class="absolute size-4 text-current opacity-0 transition-opacity duration-200 ease-in-out group-hover:opacity-100"
											strokeWidth={2.5}
											aria-hidden="true"
										/>
									</span>
									Add To VSCode
								</a>
							{/each}
						</div>
						<div class="divider">OR</div>
						<p>
							To add this MCP server to VSCode, update your <span class="snippet"
								>.vscode/mcp.json</span
							>
						</p>
						{@render codeSnippet(`
	{
		"servers": {
${servers
	.map(
		(server) => `			"${server.name}": {
				"url": "${server.url}"
			}`
	)
	.join(',\n')}
		}
	}

`)}
					{/if}
				</div>
			{/if}
		{/each}
	</div>
</div>

{#snippet codeSnippet(code: string)}
	<div class="relative">
		<div class="absolute top-4 right-4 flex h-fit w-fit">
			<CopyButton
				text={code}
				showTextLeft
				class="text-white"
				classes={{ button: 'flex gap-1 flex-shrink-0 items-center text-white' }}
			/>
		</div>
		<pre><code>{code}</code></pre>
	</div>
{/snippet}

<style lang="postcss">
	.snippet {
		background-color: var(--surface1);
		border-radius: 0.375rem;
		padding: 0.125rem 0.5rem;
		font-size: 13px;
		font-weight: 300;

		.dark & {
			background-color: var(--surface3);
		}
	}
	@keyframes slideOut {
		from {
			transform: scaleX(1);
			opacity: 1;
		}
		to {
			transform: scaleX(0);
			opacity: 0;
		}
	}

	@keyframes slideIn {
		from {
			transform: scaleX(0);
			opacity: 0;
		}
		to {
			transform: scaleX(1);
			opacity: 1;
		}
	}

	.cursor-link {
		background-color: var(--color-black);
		color: var(--color-white);
		font-size: var(--text-xs);
		font-weight: 300;
		padding: 0.5rem 1rem;
		border-radius: var(--radius-xs);
		transition: all 0.2s ease-in-out;
		transition-property: background-color, border-color, color;
		text-transform: uppercase;
		width: fit-content;
		display: flex;
		align-items: center;
		gap: 0.35rem;

		&:hover {
			background-color: color-mix(in oklab, var(--color-black) 85%, var(--color-white));
		}
	}

	.vscode-link {
		background-color: #0065a9;
		color: var(--color-white);
		font-size: var(--text-xs);
		font-weight: 300;
		padding: 0.5rem 1rem;
		border-radius: var(--radius-xs);
		transition: all 0.2s ease-in-out;
		transition-property: background-color, border-color, color;
		text-transform: uppercase;
		width: fit-content;
		display: flex;
		align-items: center;
		gap: 0.35rem;

		&:hover {
			background-color: color-mix(in oklab, #0065a9 90%, var(--color-white));
		}
	}

	.border-slide-out {
		animation: slideOut 0.3s ease-out forwards;
	}

	.border-slide-in {
		animation: slideIn 0.3s ease-out forwards;
	}
</style>
