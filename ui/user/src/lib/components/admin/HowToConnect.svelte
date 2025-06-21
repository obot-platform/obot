<script lang="ts">
	import { ChevronLeft, ChevronRight } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fade, fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';
	import CopyButton from '../CopyButton.svelte';

	interface Props {
		url: string;
	}

	let { url }: Props = $props();
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
		},
		cline: {
			label: 'Cline',
			icon: '/user/images/assistant/cline-mark.svg'
		},
		highlight: {
			label: 'Highlight AI',
			icon: '/user/images/assistant/highlightai-mark.svg'
		},
		augment: {
			label: 'Augment Code',
			icon: '/user/images/assistant/augmentcode-mark.svg'
		}
	};

	const options = Object.keys(optionMap).map((key) => ({ key, value: optionMap[key] }));
	let selected = $state(options[0].key);
	let previousSelected = $state(options[0].key);
	let isAnimating = $state(false);
	let flyDirection = $state(100); // 100 for right, -100 for left

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
		{#each options as option}
			<li class="w-36 flex-shrink-0">
				<button
					class={twMerge(
						'dark:hover:bg-surface3 relative flex w-full items-center justify-center gap-1.5 rounded-t-xs border-b-2 border-transparent py-2 text-[13px] font-light transition-all duration-200 hover:bg-gray-50',
						selected === option.key &&
							'dark:bg-surface2 bg-white hover:bg-transparent dark:hover:bg-transparent'
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
								'absolute right-0 bottom-0 left-0 h-0.5 origin-left bg-blue-500',
								isAnimating && selected === option.key ? 'border-slide-in' : ''
							)}
						></div>
					{:else if isAnimating && previousSelected === option.key}
						<div
							class="border-slide-out absolute right-0 bottom-0 left-0 h-0.5 origin-left bg-blue-500"
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
	<div class="flex min-h-[400px] w-[200%]">
		{#each options as option}
			{#if selected === option.key}
				<div
					in:fly={{ x: flyDirection, duration: 200, delay: 200 }}
					out:fade={{ duration: 150 }}
					class="w-1/2 p-4"
				>
					{#if option.key === 'cursor'}
						<p>
							To add this MCP server to Cursor, update your <span class="snippet"
								>~/.cursor/mcp/json</span
							>
						</p>
						{@render codeSnippet(`
    {
        "mcpServers": {
            "obot": {
                "url": "${url}"
            }
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
            "obot": {
                "command": "npx",
                "args": [
                    "mcp-remote",
                    "${url}"
                ]
            }
        }
    }                    
    `)}
					{:else if option.key === 'vscode'}
						<p>
							To add this MCP server to VSCode, update your <span class="snippet"
								>.vscode/mcp.json</span
							>
						</p>
						{@render codeSnippet(`
    {
        "servers": {
            "obot": {
                "type": "sse",
                "url": "${url}"
            }
        }
    }
    `)}
					{:else if option.key === 'cline'}
						<p>
							To add this MCP server to Cline, update your <span class="snippet"
								>~/Library/Application
								Support/Code/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json</span
							>
						</p>

						{@render codeSnippet(`
    {
        "mcpServers": {
            "obot": {
                "url": "${url}",
                "disabled": false,
                "autoApprove": []
            }
        }
    `)}
					{:else if option.key === 'highlight'}
						<p>
							To add this MCP server to Highlight AI, click the plugins icon in the sidebar (@
							symbol). Then proceed with the following:
						</p>
						<ul class="my-2 list-inside list-disc space-y-2">
							<li>Click <b>Installed Plugins</b> at the top of the sidebar</li>
							<li>Select <b>Custom Plugins</b></li>
							<li>Click <b>Add a plugin using a custom SEE URL</b></li>
							<li>Enter your plugin name: <span class="snippet">obot</span></li>
							<li>Enter the URL as SSE URL: <span class="snippet">{url}</span></li>
						</ul>
					{:else if option.key === 'augment'}
						<p>
							To add this MCP server to Augment Code, go to Settings & MCP Section. Add the
							following configuration:
						</p>
						{@render codeSnippet(`
    {
        "mcpServers": {
            "git-mcp obot": {
                "command": "npx",
                "args": [
                        "mcp-remote",
                        "${url}"
                ]
            }
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
				classes={{ button: 'flex gap-1 flex-shrink-0 items-center' }}
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

	.border-slide-out {
		animation: slideOut 0.3s ease-out forwards;
	}

	.border-slide-in {
		animation: slideIn 0.3s ease-out forwards;
	}
</style>
