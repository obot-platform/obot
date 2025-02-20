import { VariantProps, cva } from "class-variance-authority";
import * as React from "react";
import { forwardRef, useImperativeHandle } from "react";

import { cn } from "~/lib/utils";

// note: use outline instead of ring to avoid overriding the ring from the outline variant
const textareaVariants = cva(
	"group flex w-full rounded-md bg-transparent text-sm placeholder:text-muted-foreground group-disabled:cursor-not-allowed group-disabled:bg-opacity-50 has-[:focus-visible]:outline has-[:focus-visible]:outline-1 has-[:focus-visible]:outline-ring",
	{
		variants: {
			variant: {
				// note: use inset ring instead of border so that the wrapper doesn't add any extra height or width
				outlined: "ring-1 ring-inset ring-input",
				flat: "border-none bg-muted shadow-none",
			},
		},
		defaultVariants: {
			variant: "outlined",
		},
	}
);

type TextAreaWrapperProps = React.HTMLAttributes<HTMLDivElement> &
	VariantProps<typeof textareaVariants>;
const TextAreaWrapper = forwardRef<HTMLDivElement, TextAreaWrapperProps>(
	({ className, variant, ...props }, ref) => {
		return (
			<div
				ref={ref}
				className={cn(textareaVariants({ variant, className }))}
				{...props}
			/>
		);
	}
);
TextAreaWrapper.displayName = "TextAreaWrapper";

type TextAreaBaseProps = React.TextareaHTMLAttributes<HTMLTextAreaElement> &
	VariantProps<typeof textareaVariants>;

const TextAreaBase = forwardRef<HTMLTextAreaElement, TextAreaBaseProps>(
	({ className, variant, ...props }, ref) => {
		return (
			<textarea
				className={cn(
					"disabled:group w-full border-none bg-transparent px-3 py-2 focus-visible:border-none focus-visible:outline-none group-disabled:cursor-not-allowed",
					variant === "flat" && "placeholder:text-muted-foreground",
					className
				)}
				ref={ref}
				{...props}
			/>
		);
	}
);
TextAreaBase.displayName = "TextAreaBase";

export type TextareaProps = TextAreaBaseProps &
	VariantProps<typeof textareaVariants> & {
		resizeable?: boolean;
		endContent?: React.ReactNode;
		bottomContent?: React.ReactNode;
	};

const Textarea = React.forwardRef<HTMLTextAreaElement, TextareaProps>(
	(
		{
			className,
			resizeable = false,
			variant,
			endContent,
			bottomContent,
			...props
		},
		ref
	) => {
		return (
			<TextAreaWrapper
				variant={variant}
				className={cn("flex flex-col", className)}
			>
				<div className="flex w-full">
					<TextAreaBase
						className={cn(
							"w-full border-none bg-transparent px-3 py-2 focus-visible:border-none focus-visible:outline-none",
							!resizeable && "resize-none"
						)}
						variant={variant}
						ref={ref}
						{...props}
					/>
					{endContent}
				</div>
				{bottomContent}
			</TextAreaWrapper>
		);
	}
);
Textarea.displayName = "Textarea";

// note(ryanhopperlowe): AutosizeTextarea taken from (https://shadcnui-expansions.typeart.cc/docs/autosize-textarea)

interface UseAutosizeTextAreaProps {
	textAreaRef: HTMLTextAreaElement | null;
	minHeight?: number;
	maxHeight?: number;
}

const useAutosizeTextArea = ({
	textAreaRef,
	maxHeight = Number.MAX_SAFE_INTEGER,
	minHeight = 0,
}: UseAutosizeTextAreaProps) => {
	const initRef = React.useRef(true);

	const resize = React.useCallback(
		(node: HTMLTextAreaElement) => {
			console.log("resize", node, minHeight, maxHeight, initRef);
			// Reset the height to auto to get the correct scrollHeight
			node.style.height = "auto";

			const offsetBorder = 2;

			if (initRef.current) {
				node.style.minHeight = `${minHeight + offsetBorder}px`;
				if (maxHeight > minHeight) {
					node.style.maxHeight = `${maxHeight}px`;
				}
				node.style.height = `${minHeight + offsetBorder}px`;
				initRef.current = false;
				return;
			}

			const newHeight = Math.min(
				Math.max(node.scrollHeight, minHeight + offsetBorder),
				maxHeight + offsetBorder
			);

			node.style.height = `${newHeight}px`;
		},
		[maxHeight, minHeight]
	);

	const initResizer = React.useCallback(
		(node: HTMLTextAreaElement) => {
			const handleResize = () => resize(node);

			node.addEventListener("input", handleResize);
			node.addEventListener("focus", handleResize);
			node.addEventListener("change", handleResize);
			node.addEventListener("keyup", handleResize);
			node.addEventListener("resize", handleResize);
			if (initRef.current) {
				resize(node);
			}

			// Cleanup function to remove event listeners
			return () => {
				node.removeEventListener("input", handleResize);
				node.removeEventListener("focus", handleResize);
				node.removeEventListener("change", handleResize);
				node.removeEventListener("keyup", handleResize);
				node.removeEventListener("resize", handleResize);
			};
		},
		[resize]
	);

	React.useEffect(() => {
		if (textAreaRef) {
			const cleanup = initResizer(textAreaRef);
			return cleanup;
		}
	}, [initResizer, textAreaRef]);

	return { initResizer };
};

export type AutosizeTextAreaRef = {
	textArea: HTMLTextAreaElement | null;
	maxHeight: number;
	minHeight: number;
};

export type AutosizeTextAreaProps = TextareaProps & {
	maxHeight?: number;
	minHeight?: number;
};

const AutosizeTextarea = React.forwardRef<
	AutosizeTextAreaRef,
	AutosizeTextAreaProps
>(
	(
		{
			maxHeight = Number.MAX_SAFE_INTEGER,
			minHeight = 52,
			className,
			onChange,
			...props
		}: AutosizeTextAreaProps,
		ref: React.Ref<AutosizeTextAreaRef>
	) => {
		const [textAreaEl, setTextAreaEl] =
			React.useState<HTMLTextAreaElement | null>(null);

		useImperativeHandle(ref, () => ({
			textArea: textAreaEl,
			focus: textAreaEl?.focus,
			maxHeight,
			minHeight,
		}));

		const { initResizer } = useAutosizeTextArea({
			textAreaRef: textAreaEl,
			maxHeight,
			minHeight,
		});

		const initRef = React.useCallback(
			(node: HTMLTextAreaElement | null) => {
				setTextAreaEl(node);

				if (node) initResizer(node);
			},
			[initResizer]
		);

		return (
			<Textarea
				{...props}
				rows={props.rows || 1}
				ref={initRef}
				className={cn("resize-none", className)}
				onChange={onChange}
			/>
		);
	}
);
AutosizeTextarea.displayName = "AutosizeTextarea";

export { AutosizeTextarea, Textarea, useAutosizeTextArea };
