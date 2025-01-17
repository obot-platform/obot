import { BrainCircuit, Compass, Wrench } from "lucide-react";
import Markdown from "react-markdown";
import rehypeExternalLinks from "rehype-external-links";
import remarkGfm from "remark-gfm";

import { cn } from "~/lib/utils";

import { useChat } from "~/components/chat/ChatContext";
import { urlTransformAllowFiles } from "~/components/chat/Message";
import { CustomMarkdownComponents } from "~/components/react-markdown";
import { Button } from "~/components/ui/button";

export function NoMessages() {
	const {
		processUserMessage,
		isInvoking,
		starterMessages,
		introductionMessage,
	} = useChat();

	return (
		<div className="flex h-full flex-col items-center justify-center space-y-4 p-4 text-center">
			<h2 className="text-2xl font-semibold">Start the conversation!</h2>
			<p className="text-gray-500">
				<Markdown
					className={cn(
						"prose max-w-full flex-auto overflow-x-auto break-words text-muted-foreground dark:prose-invert prose-pre:whitespace-pre-wrap prose-pre:break-words prose-thead:text-left prose-img:rounded-xl prose-img:shadow-lg"
					)}
					remarkPlugins={[remarkGfm]}
					rehypePlugins={[[rehypeExternalLinks, { target: "_blank" }]]}
					urlTransform={urlTransformAllowFiles}
					components={CustomMarkdownComponents}
				>
					{introductionMessage ||
						"Looking for a starting point? Try one of these options."}
				</Markdown>
			</p>
			<div className="flex flex-wrap justify-center gap-2">
				{starterMessages && starterMessages.length > 0
					? starterMessages.map((starterMessage, index) => (
							<Button
								key={`starter-message-${index}`}
								variant="outline"
								shape="pill"
								disabled={isInvoking}
								onClick={() => processUserMessage(starterMessage)}
							>
								{starterMessage}
							</Button>
						))
					: renderDefaultStarterMessages()}
			</div>
		</div>
	);

	function renderDefaultStarterMessages() {
		return (
			<>
				<Button
					variant="outline"
					shape="pill"
					disabled={isInvoking}
					onClick={() =>
						processUserMessage(
							"Tell me who you are and what your objectives are."
						)
					}
				>
					<Compass className="mr-2 h-4 w-4" />
					Objectives
				</Button>
				<Button
					variant="outline"
					shape="pill"
					disabled={isInvoking}
					onClick={() =>
						processUserMessage("Tell me what tools you have available.")
					}
				>
					<Wrench className="mr-2 h-4 w-4" />
					Tools
				</Button>
				<Button
					variant="outline"
					shape="pill"
					disabled={isInvoking}
					onClick={() =>
						processUserMessage(
							"Using your knowledge tools, tell me about your knowledge set."
						)
					}
				>
					<BrainCircuit className="mr-2 h-4 w-4" />
					Knowledge
				</Button>
			</>
		);
	}
}
