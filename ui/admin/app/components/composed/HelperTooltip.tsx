import { CircleHelpIcon } from "lucide-react";

import { Button } from "~/components/ui/button";
import { Link } from "~/components/ui/link";
import {
    Tooltip,
    TooltipContent,
    TooltipTrigger,
} from "~/components/ui/tooltip";

export function HelperTooltipLabel({
    label,
    tooltip,
}: {
    label: string;
    tooltip?: string;
}) {
    return (
        <div className="flex items-center">
            {label}
            {tooltip && (
                <Tooltip>
                    <TooltipTrigger asChild>
                        <Button
                            size="icon"
                            variant="ghost"
                            onClick={(e) => e.preventDefault()}
                        >
                            <CircleHelpIcon className="text-muted-foreground" />
                        </Button>
                    </TooltipTrigger>

                    <TooltipContent side="right" variant="secondary">
                        {tooltip}
                    </TooltipContent>
                </Tooltip>
            )}
        </div>
    );
}

export function HelperTooltipLink({ link }: { link: string }) {
    return (
        <Tooltip>
            <TooltipTrigger asChild>
                <Link to={link} size="icon" variant="ghost" as="button">
                    <CircleHelpIcon className="text-muted-foreground" />
                </Link>
            </TooltipTrigger>

            <TooltipContent side="right" variant="secondary">
                This model provider supports additional environment variable
                configurations. Click to learn more.
            </TooltipContent>
        </Tooltip>
    );
}
