import { VariantProps, cva } from "class-variance-authority";
import * as React from "react";
import { forwardRef, useImperativeHandle } from "react";

import { cn } from "~/lib/utils";

const textareaVariants = cva(
    "flex w-full rounded-md bg-transparent text-sm placeholder:text-muted-foreground has-[:focus-visible]:ring-1 has-[:focus-visible]:ring-ring group group-disabled:cursor-not-allowed group-disabled:bg-opacity-50",
    {
        variants: {
            variant: {
                outlined: "border border-input shadow-sm",
                flat: "border-none shadow-none bg-muted",
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
                    "w-full px-3 py-2 bg-transparent border-none focus-visible:border-none focus-visible:outline-none disabled:group group-disabled:cursor-not-allowed",
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
                <div className="w-full flex">
                    <TextAreaBase
                        className={cn(
                            "w-full px-3 py-2 bg-transparent border-none focus-visible:border-none focus-visible:outline-none",
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
    const [init, setInit] = React.useState(true);

    const resize = React.useCallback(
        (node: HTMLTextAreaElement) => {
            // Reset the height to auto to get the correct scrollHeight
            node.style.height = "auto";

            const offsetBorder = 2;

            if (init) {
                node.style.minHeight = `${minHeight + offsetBorder}px`;
                if (maxHeight > minHeight) {
                    node.style.maxHeight = `${maxHeight}px`;
                }
                node.style.height = `${minHeight + offsetBorder}px`;
                setInit(false);
            }

            node.style.height = `${
                Math.min(Math.max(node.scrollHeight, minHeight), maxHeight) +
                offsetBorder
            }px`;
        },
        // disable exhaustive deps because we don't want to rerun this after init is set to false
        // eslint-disable-next-line react-hooks/exhaustive-deps
        [maxHeight, minHeight]
    );

    const initResizer = React.useCallback(
        (node: HTMLTextAreaElement) => {
            node.onkeyup = () => resize(node);
            node.onfocus = () => resize(node);
            node.oninput = () => resize(node);
            node.onresize = () => resize(node);
            node.onchange = () => resize(node);
            resize(node);
        },
        [resize]
    );

    React.useEffect(() => {
        if (textAreaRef) {
            initResizer(textAreaRef);
            resize(textAreaRef);
        }
    }, [resize, initResizer, textAreaRef]);

    return { initResizer };
};

export type AutosizeTextAreaRef = {
    textArea: HTMLTextAreaElement;
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
        const textAreaRef = React.useRef<HTMLTextAreaElement | null>(null);

        useImperativeHandle(ref, () => ({
            textArea: textAreaRef.current as HTMLTextAreaElement,
            focus: textAreaRef?.current?.focus,
            maxHeight,
            minHeight,
        }));

        const { initResizer } = useAutosizeTextArea({
            textAreaRef: textAreaRef.current,
            maxHeight,
            minHeight,
        });

        const initRef = React.useCallback(
            (node: HTMLTextAreaElement | null) => {
                textAreaRef.current = node;

                if (!node) return;

                initResizer(node);
            },
            [initResizer]
        );

        return (
            <Textarea
                {...props}
                ref={initRef}
                className={cn("resize-none", className)}
                onChange={onChange}
            />
        );
    }
);
AutosizeTextarea.displayName = "AutosizeTextarea";

export { Textarea, AutosizeTextarea, useAutosizeTextArea };
