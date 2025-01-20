import ReactMarkdown, { defaultUrlTransform } from "react-markdown";
import rehypeExternalLinks from "rehype-external-links";
import rehypeRaw from "rehype-raw";
import rehypeSanitize from "rehype-sanitize";
import remarkGfm from "remark-gfm";

import { cn } from "~/lib/utils/cn";

import { CustomMarkdownComponents } from "~/components/react-markdown";

// Allow links for file references in messages if it starts with file://, otherwise this will cause an empty href and cause app to reload when clicking on it
export const urlTransformAllowFiles = (u: string) => {
	if (u.startsWith("file://")) {
		return u;
	}
	return defaultUrlTransform(u);
};

export function Markdown({
	children,
	className,
}: {
	children?: string | null;
	className?: string;
}) {
	return (
		<ReactMarkdown
			className={cn(
				"prose max-w-full flex-auto overflow-x-auto break-words dark:prose-invert prose-pre:whitespace-pre-wrap prose-pre:break-words prose-thead:text-left prose-img:rounded-xl prose-img:shadow-lg",
				className
			)}
			remarkPlugins={[remarkGfm]}
			rehypePlugins={[
				[rehypeExternalLinks, { target: "_blank" }],
				rehypeRaw,
				rehypeSanitize,
			]}
			urlTransform={urlTransformAllowFiles}
			components={CustomMarkdownComponents}
		>
			{children}
		</ReactMarkdown>
	);
}
