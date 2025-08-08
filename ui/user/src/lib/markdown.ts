import DOMPurify from 'dompurify';
import { micromark } from 'micromark';
import { gfm, gfmHtml } from 'micromark-extension-gfm';

export function toHTMLFromMarkdown(markdown: string): string {
	const html = micromark(markdown, {
		extensions: [gfm()],
		htmlExtensions: [gfmHtml()],
		allowDangerousHtml: true
	});

	if (typeof window !== 'undefined') {
		// DOMPurify requires browser, errors in SSR
		const sanitized = DOMPurify.sanitize(html);

		const processedHtml = processVideoAndIframeTags(sanitized);
		return processedHtml;
	}

	return html;
}

const processVideoAndIframeTags = (html: string): string => {
	// Create a temporary DOM element to parse and manipulate the HTML
	const tempDiv = document.createElement('div');
	tempDiv.innerHTML = html;

	// Find all text nodes that contain iframe or video tags as strings
	const walker = document.createTreeWalker(tempDiv, NodeFilter.SHOW_TEXT, {
		acceptNode: (node) => {
			const text = node.textContent || '';
			if (text.includes('<iframe') || text.includes('<video')) {
				return NodeFilter.FILTER_ACCEPT;
			}
			return NodeFilter.FILTER_REJECT;
		}
	});

	const textNodesToProcess: Text[] = [];
	let node;
	while ((node = walker.nextNode()) !== null) {
		textNodesToProcess.push(node as Text);
	}

	textNodesToProcess.forEach((textNode) => {
		const text = textNode.textContent || '';

		// Find iframe tags in the text
		const iframeRegex = /<iframe[^>]*src="([^"]*)"[^>]*><\/iframe>/g;
		let match;
		let newText = text;

		while ((match = iframeRegex.exec(text)) !== null) {
			const fullMatch = match[0];
			const src = match[1];

			const iframe = document.createElement('iframe');
			iframe.src = src;
			iframe.setAttribute('frameborder', '0');
			iframe.setAttribute('allowfullscreen', 'true');

			newText = newText.replace(fullMatch, iframe.outerHTML);
		}

		const videoRegex = /<video[^>]*src="([^"]*)"[^>]*><\/video>/g;
		while ((match = videoRegex.exec(text)) !== null) {
			const fullMatch = match[0];
			const src = match[1];

			const video = document.createElement('video');
			video.src = src;
			video.setAttribute('controls', 'true');

			newText = newText.replace(fullMatch, video.outerHTML);
		}

		if (newText !== text) {
			const container = document.createElement('div');
			container.innerHTML = newText;

			const parent = textNode.parentNode;
			if (parent) {
				while (container.firstChild) {
					parent.insertBefore(container.firstChild, textNode);
				}
				parent.removeChild(textNode);
			}
		}
	});

	return tempDiv.innerHTML;
};

const updateLinksWithTargetBlank = (html: string) => {
	return html.replace(/<a href=/g, '<a target="_blank" rel="noopener" href=');
};

export function toHTMLFromMarkdownWithNewTabLinks(markdown: string): string {
	return updateLinksWithTargetBlank(toHTMLFromMarkdown(markdown));
}

export function stripMarkdownToText(markdown: string): string {
	// First convert markdown to HTML
	const html = toHTMLFromMarkdown(markdown);

	// Create a temporary DOM element to parse the HTML
	const tempDiv = typeof document !== 'undefined' ? document.createElement('div') : null;

	if (tempDiv) {
		// Set the HTML content
		tempDiv.innerHTML = html;

		// Get the text content, which automatically strips all HTML tags
		const text = (tempDiv.textContent || tempDiv.innerText || '')
			.replace(/\s+/g, ' ') // Clean up extra whitespace
			.trim();

		return text;
	} else {
		// Fallback for SSR: use regex to strip HTML tags
		const text = html
			.replace(/<[^>]*>/g, '') // Remove all HTML tags
			.replace(/&nbsp;/g, ' ') // Replace &nbsp; with space
			.replace(/&amp;/g, '&') // Replace &amp; with &
			.replace(/&lt;/g, '<') // Replace &lt; with <
			.replace(/&gt;/g, '>') // Replace &gt; with >
			.replace(/&quot;/g, '"') // Replace &quot; with "
			.replace(/&#39;/g, "'") // Replace &#39; with '
			.replace(/\s+/g, ' ') // Replace multiple whitespace with single space
			.trim();

		return text;
	}
}
