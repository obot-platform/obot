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
		return DOMPurify.sanitize(html);
	}

	return html;
}

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
