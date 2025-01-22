import { BrainCircuit, Compass, Wrench } from "lucide-react";

import { useChat } from "~/components/chat/ChatContext";
import { Button } from "~/components/ui/button";
import { Markdown } from "~/components/ui/markdown";

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
			<div className="text-gray-500">
				<Markdown>
					{introductionMessage ||
						"Looking for a starting point? Try one of these options."}
				</Markdown>
			</div>
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
