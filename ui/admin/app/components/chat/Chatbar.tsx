import { ArrowUpIcon, SquareIcon } from "lucide-react";
import { useState } from "react";

import { cn } from "~/lib/utils";

import { ModelProviderTooltip } from "~/components/auth-and-model-providers/ModelProviderTooltip";
import { ChatActions } from "~/components/chat/ChatActions";
import { useChat } from "~/components/chat/ChatContext";
import { ChatRunInfo } from "~/components/chat/ChatRunInfo";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { Button } from "~/components/ui/button";
import { AutosizeTextarea } from "~/components/ui/textarea";
import { useModelProviders } from "~/hooks/model-providers/useModelProviders";

type ChatbarProps = {
	className?: string;
};

export function Chatbar({ className }: ChatbarProps) {
	const [input, setInput] = useState("");
	const {
		abortRunningThread,
		processUserMessage,
		isRunning,
		isInvoking,
		messages,
	} = useChat();
	const { configured: modelProviderConfigured } = useModelProviders();

	const disabled =
		(!input && !isRunning) || isInvoking || !modelProviderConfigured;

	const handleAbort = () => {
		if (isRunning) {
			abortRunningThread();
		}
	};

	const handleSubmit = (e: React.FormEvent) => {
		e.preventDefault();

		if (disabled) return;

		if (isRunning) {
			return;
		}

		if (input.trim()) {
			processUserMessage(input);
		}

		setInput("");
	};

	return (
		<form
			onSubmit={handleSubmit}
			className={cn("flex items-end gap-2", className)}
		>
			<div className="relative flex-grow">
				<AutosizeTextarea
					className="rounded-3xl p-2"
					variant="flat"
					value={input}
					onKeyDown={(e) => {
						if (e.key === "Enter" && !e.shiftKey) {
							e.preventDefault();
							handleSubmit(e);
						}
					}}
					maxHeight={200}
					minHeight={0}
					onChange={(e) => setInput(e.target.value)}
					placeholder="Type your message..."
					bottomContent={
						<div className="flex flex-row-reverse items-center justify-between">
							<div className="flex flex-row-reverse items-center gap-2">
								<ModelProviderTooltip enabled={modelProviderConfigured}>
									<Button
										size="icon-sm"
										className="m-2"
										color="primary"
										type="submit"
										disabled={disabled}
										onClick={handleAbort}
									>
										{isInvoking ? (
											<LoadingSpinner />
										) : isRunning ? (
											<SquareIcon className="!h-3 !w-3 fill-primary-foreground text-primary-foreground" />
										) : (
											<ArrowUpIcon />
										)}
									</Button>
								</ModelProviderTooltip>
								<ChatRunInfo messages={messages} isRunning={isRunning} />
							</div>
							<ChatActions className="p-2" />
						</div>
					}
				/>
			</div>
		</form>
	);
}
