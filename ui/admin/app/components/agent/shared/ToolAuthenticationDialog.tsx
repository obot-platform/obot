import { CheckIcon, CircleAlert } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";

import { useToolReference } from "~/components/agent/ToolEntry";
import { PromptAuthForm } from "~/components/chat/Message";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { Button } from "~/components/ui/button";
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "~/components/ui/dialog";
import { Link } from "~/components/ui/link";
import { Markdown } from "~/components/ui/markdown";
import { ScrollArea } from "~/components/ui/scroll-area";
import { useInitMessageStore } from "~/hooks/messages/useMessageStore";

type AgentAuthenticationDialogProps = {
	threadId: Nullish<string>;
	onComplete: () => void;
	tool: string;
};

export function ToolAuthenticationDialog({
	onComplete,
	threadId,
	tool,
}: AgentAuthenticationDialogProps) {
	const { icon, label } = useToolReference(tool);

	const { messages: _messages } = useInitMessageStore(threadId);

	type ItemState = {
		isLoading?: boolean;
		isError?: boolean;
		isDone?: boolean;
	};

	const [map, setMap] = useState<Record<number, ItemState>>({});
	const updateItem = useCallback((id: number, state: Partial<ItemState>) => {
		setMap((prev) => ({ ...prev, [id]: { ...prev[id], ...state } }));
	}, []);

	const messages = useMemo(
		() => _messages.filter((m) => m.prompt || m.error || m.text === "DONE"),
		[_messages]
	);

	useEffect(() => {
		// any time a message is added, prevent the last message from being loading
		const isError = messages.at(-1)?.error;

		const i = messages.length - 2;
		setMap((prev) => ({
			...prev,
			[i]: { isLoading: false, isDone: !isError, isError },
		}));
	}, [messages]);

	const done = messages.at(-1)?.text === "DONE";

	useEffect(() => {
		if (messages.length === 1) {
			const message = messages.at(0);
			const authURL = message?.prompt?.metadata?.authURL;
			if (authURL) {
				window.open(authURL);
				updateItem(0, { isLoading: true });
			}
		}
	}, [messages, updateItem]);

	return (
		<Dialog open={!!threadId} onOpenChange={onComplete}>
			<DialogContent className="max-h-[80vh] p-0">
				<ScrollArea className="overflow-auto p-6">
					<DialogHeader>
						<DialogTitle className="flex items-center gap-2">
							{icon} <span>Authorize {label}</span>
						</DialogTitle>

						<DialogDescription hidden={done}></DialogDescription>
					</DialogHeader>

					<div className="flex w-full items-center justify-center [&_svg]:size-4">
						{!messages.length ? (
							<div className="flex items-center gap-2">
								<LoadingSpinner /> Loading...
							</div>
						) : (
							<div className="flex flex-col gap-2">
								{messages.map((message, index) => {
									if (message.error) {
										return (
											<p
												className="flex items-center gap-2 text-destructive"
												key={index}
											>
												<CircleAlert /> Error: {message.text}
											</p>
										);
									}

									if (message.text === "DONE") {
										return (
											<p key={index} className="flex items-center gap-2">
												<CheckIcon className="text-success" />
												Done
											</p>
										);
									}

									if (message.prompt) {
										if (map[index]?.isDone) {
											return (
												<p key={index} className="flex items-center gap-2">
													<CheckIcon className="text-success" />
													Authentication Successful
												</p>
											);
										}

										if (map[index]?.isLoading) {
											return (
												<p key={index} className="flex items-center gap-2">
													<LoadingSpinner /> Authentication Processing
												</p>
											);
										}

										if (message.prompt.metadata?.authURL) {
											return (
												<p key={index} className="flex items-center gap-2">
													<CircleAlert />
													<span>
														Authentication Required{" "}
														<Link
															target="_blank"
															rel="noreferrer"
															to={message.prompt.metadata.authURL}
															onClick={() =>
																updateItem(index, { isLoading: true })
															}
														>
															Click Here
														</Link>
													</span>
												</p>
											);
										}

										if (message.prompt.fields) {
											return (
												<div key={index} className="flex flex-col gap-2">
													{message.prompt.message ? (
														<Markdown>{message.prompt.message}</Markdown>
													) : (
														<p className="flex items-center gap-2">
															Authentication Required
														</p>
													)}

													<PromptAuthForm
														prompt={message.prompt}
														onSubmit={() =>
															updateItem(index, { isLoading: true })
														}
													/>
												</div>
											);
										}
									}
								})}
							</div>
						)}
					</div>

					<DialogFooter>
						{done && (
							<DialogClose asChild>
								<Button>Done</Button>
							</DialogClose>
						)}
					</DialogFooter>
				</ScrollArea>
			</DialogContent>
		</Dialog>
	);
}
