import * as React from "react";
import { useImperativeHandle } from "react";

import { cn } from "~/lib/utils";

export type TextareaProps = React.TextareaHTMLAttributes<HTMLTextAreaElement>;

const Textarea = React.forwardRef<HTMLTextAreaElement, TextareaProps>(
    ({ className, ...props }, ref) => {
        return (
            <textarea
                className={cn(
                    "flex min-h-[60px] w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50",
                    className
                )}
                ref={ref}
                {...props}
            />
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

export type AutosizeTextAreaProps = {
    maxHeight?: number;
    minHeight?: number;
} & React.TextareaHTMLAttributes<HTMLTextAreaElement>;

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
            value,
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
                value={value}
                ref={initRef}
                className={cn("resize-none", className)}
                onChange={onChange}
            />
        );
    }
);
AutosizeTextarea.displayName = "AutosizeTextarea";

export { Textarea, AutosizeTextarea, useAutosizeTextArea };
