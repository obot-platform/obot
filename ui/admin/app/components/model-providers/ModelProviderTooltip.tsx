import {
    Tooltip,
    TooltipContent,
    TooltipTrigger,
} from "~/components/ui/tooltip";

export function ModelProviderTooltip({
    children,
    enabled,
}: {
    children: React.ReactNode;
    enabled: boolean;
}) {
    return enabled ? (
        children
    ) : (
        <Tooltip>
            <TooltipTrigger>{children}</TooltipTrigger>
            <TooltipContent className="bg-warning">
                Set up a model provider to enable this feature.
            </TooltipContent>
        </Tooltip>
    );
}
