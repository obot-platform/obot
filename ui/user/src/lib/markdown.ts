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
