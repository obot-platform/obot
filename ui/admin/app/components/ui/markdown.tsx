import {
	BlockTypeSelect,
	BoldItalicUnderlineToggles,
	ChangeCodeMirrorLanguage,
	ConditionalContents,
	CreateLink,
	DiffSourceToggleWrapper,
	InsertCodeBlock,
	InsertImage,
	ListsToggle,
	MDXEditor,
	MDXEditorMethods,
	Separator,
	UndoRedo,
	codeBlockPlugin,
	codeMirrorPlugin,
	diffSourcePlugin,
	headingsPlugin,
	imagePlugin,
	linkDialogPlugin,
	linkPlugin,
	listsPlugin,
	markdownShortcutPlugin,
	quotePlugin,
	tablePlugin,
	thematicBreakPlugin,
	toolbarPlugin,
} from "@mdxeditor/editor";
import "@mdxeditor/editor/style.css";
import { useEffect, useRef, useState } from "react";
import ReactMarkdown, { defaultUrlTransform } from "react-markdown";
import rehypeExternalLinks from "rehype-external-links";
import rehypeRaw from "rehype-raw";
import rehypeSanitize from "rehype-sanitize";
import remarkGfm from "remark-gfm";

import { cn } from "~/lib/utils/cn";

import { CustomMarkdownComponents } from "~/components/react-markdown";
import { useTheme } from "~/components/theme";
import "~/components/ui/markdown.css";

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
	const allowedAttributes = {
		a: ["href", "target", "rel", "name", "title"],
		img: ["src", "alt", "title"],
	};
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
				[
					rehypeSanitize,
					{
						attributes: allowedAttributes,
					},
				],
			]}
			urlTransform={urlTransformAllowFiles}
			components={CustomMarkdownComponents}
		>
			{children}
		</ReactMarkdown>
	);
}

export function MarkdownEditor({
	className,
	markdown,
	onChange,
}: {
	className?: string;
	markdown: string;
	onChange: (markdown: string) => void;
}) {
	const { isDark } = useTheme();
	const ref = useRef<MDXEditorMethods>(null);
	const [isExpanded, setIsExpanded] = useState(false);

	useEffect(() => {
		if (ref.current && ref.current.getMarkdown() !== markdown) {
			ref.current.setMarkdown(markdown);
		}
	}, [markdown]);

	const handlePaste = (event: React.ClipboardEvent<HTMLDivElement>) => {
		event.stopPropagation();
		const text = event.clipboardData.getData("text/plain");
		ref.current?.insertMarkdown(text);
		onChange(`${markdown}\n${text}`);
	};

	return (
		<div
			onFocusCapture={() => setIsExpanded(true)}
			onPasteCapture={handlePaste}
		>
			<MDXEditor
				ref={ref}
				className={cn(
					{
						"dark-theme": isDark,
					},
					"flex flex-col rounded-md p-0.5 ring-1 ring-inset ring-input has-[:focus-visible]:outline has-[:focus-visible]:outline-1 has-[:focus-visible]:outline-ring",
					className
				)}
				contentEditableClassName={cn(
					isExpanded ? "h-[300px] overflow-y-auto" : "h-[54px] overflow-hidden"
				)}
				markdown={markdown}
				onChange={onChange}
				plugins={[
					toolbarPlugin({
						toolbarContents: () => (
							<DiffSourceToggleWrapper>
								<UndoRedo />
								<Separator />
								<BoldItalicUnderlineToggles />
								<ConditionalContents
									options={[
										{
											when: (editor) => editor?.editorType === "codeblock",
											contents: () => <ChangeCodeMirrorLanguage />,
										},
										{
											fallback: () => (
												<>
													<InsertCodeBlock />
												</>
											),
										},
									]}
								/>
								<Separator />
								<ListsToggle />
								<Separator />
								<BlockTypeSelect />
								<Separator />
								<CreateLink />
								<InsertImage />
								<Separator />
							</DiffSourceToggleWrapper>
						),
					}),
					headingsPlugin(),
					imagePlugin(),
					linkPlugin(),
					linkDialogPlugin(),
					tablePlugin(),
					listsPlugin(),
					thematicBreakPlugin(),
					markdownShortcutPlugin(),
					codeBlockPlugin({ defaultCodeBlockLanguage: "js" }),
					codeMirrorPlugin({
						codeBlockLanguages: { js: "JavaScript", css: "CSS" },
					}),
					quotePlugin(),
					diffSourcePlugin({
						readOnlyDiff: true,
					}),
				]}
				suppressHtmlProcessing
			/>
		</div>
	);
}
