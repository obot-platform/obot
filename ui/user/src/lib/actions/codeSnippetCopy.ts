const CODE_BLOCK_CLASS = 'markdown-code-block';
const COPY_BUTTON_CLASS = 'markdown-copy-btn';
const COPIED_CLASS = 'markdown-copy-btn--copied';

interface EnhancedCodeBlock {
	button: HTMLButtonElement;
	code: HTMLElement;
	onClick: (event: MouseEvent) => void;
	resetTimer?: number;
	wrapper: HTMLDivElement;
}

function createCopyButton(): HTMLButtonElement {
	const button = document.createElement('button');
	button.type = 'button';
	button.className = COPY_BUTTON_CLASS;
	button.setAttribute('aria-label', 'Copy code');
	return button;
}

function fallbackCopyText(text: string): boolean {
	const previousActiveElement = document.activeElement;
	const textArea = document.createElement('textarea');
	textArea.value = text;
	textArea.setAttribute('readonly', '');
	textArea.style.position = 'fixed';
	textArea.style.left = '-9999px';
	textArea.style.top = '0';
	document.body.appendChild(textArea);
	textArea.focus();
	textArea.select();

	try {
		return document.execCommand('copy');
	} catch {
		return false;
	} finally {
		document.body.removeChild(textArea);
		(previousActiveElement as HTMLElement | null)?.focus?.();
	}
}

async function copyTextToClipboard(text: string): Promise<boolean> {
	if (navigator.clipboard) {
		try {
			await navigator.clipboard.writeText(text);
			return true;
		} catch {
			return fallbackCopyText(text);
		}
	}
	return fallbackCopyText(text);
}

export function codeSnippetCopy(node: HTMLElement) {
	const enhancedBlocks = new Set<EnhancedCodeBlock>();
	const createdWrappers = new WeakSet<HTMLDivElement>();

	function removeBlock(block: EnhancedCodeBlock) {
		block.button.removeEventListener('click', block.onClick);
		if (block.resetTimer !== undefined) {
			window.clearTimeout(block.resetTimer);
		}
		enhancedBlocks.delete(block);
	}

	function markCopied(block: EnhancedCodeBlock) {
		if (block.resetTimer !== undefined) {
			window.clearTimeout(block.resetTimer);
		}
		block.button.classList.add(COPIED_CLASS);
		block.button.setAttribute('aria-label', 'Copied!');
		block.resetTimer = window.setTimeout(() => {
			block.button.classList.remove(COPIED_CLASS);
			block.button.setAttribute('aria-label', 'Copy code');
			block.resetTimer = undefined;
		}, 750);
	}

	function enhancePre(pre: HTMLPreElement) {
		if (
			(pre.parentElement instanceof HTMLDivElement && createdWrappers.has(pre.parentElement)) ||
			!node.contains(pre)
		) {
			return;
		}

		const code = pre.querySelector<HTMLElement>('code');
		if (!code?.textContent) {
			return;
		}

		const wrapper = document.createElement('div');
		const button = createCopyButton();
		wrapper.className = CODE_BLOCK_CLASS;
		createdWrappers.add(wrapper);

		const block: EnhancedCodeBlock = {
			button,
			code,
			onClick: (event) => {
				event.preventDefault();
				const text = block.code.textContent ?? '';
				if (!text) {
					return;
				}
				void copyTextToClipboard(text).then((success) => {
					if (success && node.contains(block.button)) {
						markCopied(block);
					}
				});
			},
			wrapper
		};

		button.addEventListener('click', block.onClick);
		pre.replaceWith(wrapper);
		wrapper.append(button, pre);
		enhancedBlocks.add(block);
	}

	function enhanceWithin(element: Element) {
		if (element instanceof HTMLPreElement) {
			enhancePre(element);
		}
		for (const pre of element.querySelectorAll<HTMLPreElement>('pre')) {
			enhancePre(pre);
		}
	}

	enhanceWithin(node);

	const observer = new MutationObserver((mutations) => {
		for (const block of enhancedBlocks) {
			if (!node.contains(block.wrapper)) {
				removeBlock(block);
			}
		}
		for (const mutation of mutations) {
			for (const addedNode of mutation.addedNodes) {
				if (addedNode instanceof Element) {
					enhanceWithin(addedNode);
				}
			}
		}
	});
	observer.observe(node, { childList: true, subtree: true });

	return {
		destroy() {
			observer.disconnect();
			for (const block of Array.from(enhancedBlocks)) {
				removeBlock(block);
			}
		}
	};
}
