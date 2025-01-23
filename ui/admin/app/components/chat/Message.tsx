import "@radix-ui/react-tooltip";
import { AlertCircleIcon, WrenchIcon } from "lucide-react";
import React, { useMemo, useState } from "react";
import { useForm } from "react-hook-form";

import { AgentIcons } from "~/lib/model/agents";
import { AuthPrompt } from "~/lib/model/chatEvents";
import { Message as MessageType } from "~/lib/model/messages";
import { PromptApiService } from "~/lib/service/api/PromptApi";
import { cn, formatTime } from "~/lib/utils";

import { MessageDebug } from "~/components/chat/MessageDebug";
import { ToolCallInfo } from "~/components/chat/ToolCallInfo";
import { ControlledInput } from "~/components/form/controlledInputs";
import { ToolIcon } from "~/components/tools/ToolIcon";
import { Avatar, AvatarImage } from "~/components/ui/avatar";
import { Button } from "~/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
	DialogTrigger,
} from "~/components/ui/dialog";
import { Form } from "~/components/ui/form";
import { Link } from "~/components/ui/link";
import { Markdown } from "~/components/ui/markdown";
import { useAnimatedText } from "~/hooks/messages/useAnimatedText";
import { useAsync } from "~/hooks/useAsync";

interface MessageProps {
	message: MessageType;
	isRunning?: boolean;
	icons?: AgentIcons;
	isDarkMode?: boolean;
	isMostRecent?: boolean;
	agentName?: string;
}

const OpenMarkdownLinkRegex = new RegExp(/\[([^\]]+)\]\(https?:\/\/[^)]*$/);

export const Message = React.memo(
	({ message, isRunning, icons, isDarkMode, agentName }: MessageProps) => {
		const isUser = message.sender === "user";

		// note(ryanhopperlowe) we only support one tool call per message for now
		// leaving it in case that changes in the future
		const [toolCall = null] = message.tools || [];

		// prevent animation for messages that never run
		// only calculate on mount because we don't want to stop animation when the message finishes streaming
		const [shouldAnimate] = useState(isRunning);
		const animatedText = useAnimatedText(
			message.text,
			!shouldAnimate || isUser
		);

		const parsedMessage = useMemo(() => {
			if (OpenMarkdownLinkRegex.test(animatedText)) {
				return animatedText.replace(
					OpenMarkdownLinkRegex,
					(_, linkText) => `[${linkText}]()`
				);
			}
			return animatedText;
		}, [animatedText]);

		const icon = isDarkMode ? icons?.iconDark : icons?.icon;
		const showIcon = icon && !isUser && !message.prompt && !toolCall;
		return (
			<div className="mb-4 w-full">
				{showIcon && (
					<div className="flex items-center gap-2">
						<Avatar className="h-6 w-6">
							<AvatarImage src={icon} />
						</Avatar>
						<p className="text-sm font-semibold">{agentName}</p>
						<small className="text-muted-foreground">
							{message.time && formatTime(message.time)}
						</small>
					</div>
				)}
				<div
					className={cn("flex gap-4", {
						"justify-end": isUser,
						"justify-start pl-8": !isUser,
					})}
				>
					<div
						className={cn({
							"rounded-xl border border-error bg-error-foreground":
								message.error,
							"max-w-[80%] rounded-2xl bg-accent p-4": isUser,
							"w-full max-w-full": !isUser,
						})}
					>
						<div
							className={cn(
								"flex max-w-full items-center gap-2 overflow-hidden"
							)}
						>
							{message.aborted && (
								<AlertCircleIcon className="h-5 w-5 text-muted-foreground" />
							)}

							{toolCall?.metadata?.icon && (
								<ToolIcon
									icon={toolCall.metadata.icon}
									category={toolCall.metadata.category}
									name={toolCall.name}
									className="h-5 w-5"
								/>
							)}

							{message.prompt ? (
								<PromptMessage prompt={message.prompt} isRunning={isRunning} />
							) : (
								<Markdown
									className={cn({
										"prose-invert text-accent-foreground": isUser,
										"text-muted-foreground": message.aborted,
									})}
								>
									{parsedMessage || "Waiting for more information..."}
								</Markdown>
							)}

							{toolCall && (
								<ToolCallInfo tool={toolCall}>
									<Button variant="secondary" size="icon">
										<WrenchIcon className="h-4 w-4" />
									</Button>
								</ToolCallInfo>
							)}

							{message.runId && !isUser && (
								<div className="self-start">
									<MessageDebug runId={message.runId} />
								</div>
							)}

							{/* this is a hack to take up space for the debug button */}
							{!toolCall && !message.runId && !isUser && (
								<div className="invisible">
									<Button size="icon" />
								</div>
							)}
						</div>
					</div>
				</div>
			</div>
		);
	}
);

Message.displayName = "Message";

export function PromptMessage({
	prompt,
	isRunning = false,
}: {
	prompt: AuthPrompt;
	isRunning?: boolean;
}) {
	const [open, setOpen] = useState(false);
	const [isSubmitted, setIsSubmitted] = useState(false);

	const getMessage = () => {
		if (prompt.metadata?.authURL || prompt.metadata?.authType)
			return `${prompt.metadata.category || "Tool call"} requires Authentication`;

		return prompt.message;
	};

	const getCtaText = () => {
		if (prompt.metadata?.authURL || prompt.metadata?.authType)
			return ["Authenticate", prompt.metadata.category]
				.filter(Boolean)
				.join(" with ");

		return "Submit Parameters";
	};

	const getSubmittedText = () => {
		if (prompt.metadata?.authURL || prompt.metadata?.authType) {
			let str = "Authenticated";

			if (prompt.metadata.category) {
				str += ` with ${prompt.metadata.category}`;
			}

			if (prompt.metadata.icon) {
				return (
					<div className="flex items-center gap-2">
						<ToolIcon
							name={prompt.name}
							category={prompt.metadata.category}
							icon={prompt.metadata.icon}
							className="h-5 w-5"
						/>
						{str}
					</div>
				);
			}

			return str;
		}

		return "Parameters Submitted";
	};

	if (isSubmitted) {
		return (
			<div className="flex w-fit flex-auto flex-col flex-wrap gap-2">
				<p className="min-w-fit">{getSubmittedText()}</p>
			</div>
		);
	}

	return (
		<div className="flex w-fit flex-auto flex-col flex-wrap gap-2">
			<p className="min-w-fit">{getMessage()}</p>

			{isRunning && prompt.metadata?.authURL && (
				<Link
					as="button"
					rel="noreferrer"
					target="_blank"
					onClick={() => setIsSubmitted(true)}
					to={prompt.metadata.authURL}
				>
					<ToolIcon
						icon={prompt.metadata.icon}
						category={prompt.metadata.category}
						name={prompt.name}
					/>

					{getCtaText()}
				</Link>
			)}

			{isRunning && prompt.fields && (
				<Dialog open={open} onOpenChange={setOpen}>
					<DialogTrigger disabled={isSubmitted} asChild>
						<Button
							startContent={
								<ToolIcon
									icon={prompt.metadata?.icon}
									category={prompt.metadata?.category}
									name={prompt.name}
								/>
							}
						>
							{getCtaText()}
						</Button>
					</DialogTrigger>

					<DialogContent>
						<DialogHeader>
							<DialogTitle>{getCtaText()}</DialogTitle>
						</DialogHeader>

						<DialogDescription>{prompt.message}</DialogDescription>

						<PromptAuthForm
							prompt={prompt}
							onSuccess={() => {
								setOpen(false);
								setIsSubmitted(true);
							}}
						/>
					</DialogContent>
				</Dialog>
			)}

			{!isRunning && (
				<Button
					disabled
					startContent={
						<ToolIcon
							icon={prompt.metadata?.icon}
							category={prompt.metadata?.category}
							name={prompt.name}
						/>
					}
				>
					{getCtaText()}
				</Button>
			)}
		</div>
	);
}

export function PromptAuthForm({
	prompt,
	onSuccess,
	onSubmit,
}: {
	prompt: AuthPrompt;
	onSuccess?: () => void;
	onSubmit?: () => void;
}) {
	const authenticate = useAsync(PromptApiService.promptResponse, {
		onSuccess,
	});

	const form = useForm<Record<string, string>>({
		defaultValues: prompt.fields?.reduce(
			(acc, field) => {
				acc[field] = "";
				return acc;
			},
			{} as Record<string, string>
		),
	});

	const handleSubmit = form.handleSubmit(async (values) => {
		authenticate.execute({ id: prompt.id, response: values });
		onSubmit?.();
	});

	return (
		<Form {...form}>
			<form onSubmit={handleSubmit} className="flex flex-col gap-4">
				{prompt.fields?.map((field) => (
					<ControlledInput
						key={field}
						control={form.control}
						name={field}
						label={field}
						type={prompt.sensitive ? "password" : "text"}
					/>
				))}

				<Button
					disabled={authenticate.isLoading}
					loading={authenticate.isLoading}
					type="submit"
				>
					Submit
				</Button>
			</form>
		</Form>
	);
}
