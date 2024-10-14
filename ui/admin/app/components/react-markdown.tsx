import { CSSProperties, ComponentProps, memo, useMemo } from "react";
import { Components } from "react-markdown";

type MarkdownComponentProps<TKey extends keyof Components> = ComponentProps<
    Exclude<Components[TKey], keyof JSX.IntrinsicElements | undefined>
>;

const MarkdownOrderedList = memo(
    ({ node, ...props }: MarkdownComponentProps<"ol">) => {
        const styleVal = useMemo<CSSProperties>(() => {
            const liElements =
                node?.children.filter(
                    (child) =>
                        child.type === "element" && child.tagName === "li"
                ).length || 0;

            const liCount = String(liElements).length;

            const listInside = liCount > 4;
            const paddingLeft = listInside ? "0px" : `${liCount * 12 + 8}px`;

            return {
                listStylePosition: listInside ? "inside" : undefined,
                paddingLeft,
            };
        }, [node?.children]);

        return <ol style={styleVal} {...props} />;
    }
);

MarkdownOrderedList.displayName = "MarkdownOrderedList";

export const CustomMarkdownComponents: Partial<Components> = {
    ol: MarkdownOrderedList as Components["ol"],
};
