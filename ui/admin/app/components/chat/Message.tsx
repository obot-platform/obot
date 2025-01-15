import "@radix-ui/react-tooltip";
import { AlertCircleIcon, WrenchIcon } from "lucide-react";
import React, { useMemo, useState } from "react";
import { useForm } from "react-hook-form";
import Markdown, { defaultUrlTransform } from "react-markdown";
import rehypeExternalLinks from "rehype-external-links";
import remarkGfm from "remark-gfm";

import { AuthPrompt } from "~/lib/model/chatEvents";
import { Message as MessageType } from "~/lib/model/messages";
import { PromptApiService } from "~/lib/service/api/PromptApi";
import { cn } from "~/lib/utils";

import { useChat } from "~/components/chat/ChatContext";
import { MessageDebug } from "~/components/chat/MessageDebug";
import { ToolCallInfo } from "~/components/chat/ToolCallInfo";
import { ControlledInput } from "~/components/form/controlledInputs";
import { CustomMarkdownComponents } from "~/components/react-markdown";
import { ToolIcon } from "~/components/tools/ToolIcon";
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
import { useAsync } from "~/hooks/useAsync";

interface MessageProps {
	message: MessageType;
	isRunning?: boolean;
}

// Allow links for file references in messages if it starts with file://, otherwise this will cause an empty href and cause app to reload when clicking on it
const urlTransformAllowFiles = (u: string) => {
	if (u.startsWith("file://")) {
		return u;
	}
	return defaultUrlTransform(u);
};

const OpenMarkdownLinkRegex = new RegExp(/\[([^\]]+)\]\(https?:\/\/[^)]*$/);

export const Message = React.memo(({ message }: MessageProps) => {
	const isUser = message.sender === "user";

	// note(ryanhopperlowe) we only support one tool call per message for now
	// leaving it in case that changes in the future
	const [toolCall = null] = message.tools || [];

	const { isRunning } = useChat();

	const parsedMessage = useMemo(() => {
		if (OpenMarkdownLinkRegex.test(message.text)) {
			return message.text.replace(
				OpenMarkdownLinkRegex,
				(_, linkText) => `[${linkText}]()`
			);
		}
		return message.text;
	}, [message.text]);

	return (
		<div className="mb-4 w-full">
			<div
				className={cn("flex gap-4", isUser ? "justify-end" : "justify-start")}
			>
				<div
					className={cn({
						"rounded-xl border border-error bg-error-foreground": message.error,
						"max-w-[80%] rounded-2xl bg-accent": isUser,
						"w-full max-w-full": !isUser,
					})}
				>
					<div className="flex max-w-full items-center gap-2 overflow-hidden p-4 pl-[20px]">
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
								className={cn(
									"prose max-w-full flex-auto overflow-x-auto break-words dark:prose-invert prose-pre:whitespace-pre-wrap prose-pre:break-words prose-thead:text-left prose-img:rounded-xl prose-img:shadow-lg",
									{
										"prose-invert text-accent-foreground": isUser,
										"text-muted-foreground": message.aborted,
									}
								)}
								remarkPlugins={[remarkGfm]}
								rehypePlugins={[[rehypeExternalLinks, { target: "_blank" }]]}
								urlTransform={urlTransformAllowFiles}
								components={CustomMarkdownComponents}
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
});

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
							disableTooltip
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
						disableTooltip
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
									disableTooltip
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
							disableTooltip
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
