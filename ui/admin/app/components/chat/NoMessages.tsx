import { BrainCircuit, Compass, Wrench } from "lucide-react";

import { useChat } from "~/components/chat/ChatContext";
import { Button } from "~/components/ui/button";

export function NoMessages() {
	const { processUserMessage, isInvoking } = useChat();

	return (
		<div className="flex h-full flex-col items-center justify-center space-y-4 p-4 text-center">
			<h2 className="text-2xl font-semibold">Start the conversation!</h2>
			<p className="text-gray-500">
				Looking for a starting point? Try one of these options.
			</p>
			<div className="flex flex-wrap justify-center gap-2">
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
			</div>
		</div>
	);
}
