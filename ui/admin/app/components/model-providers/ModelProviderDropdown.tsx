import { EllipsisVerticalIcon } from "lucide-react";

import { ModelProvider } from "~/lib/model/modelProviders";

import { ModelProviderDeconfigure } from "~/components/model-providers/ModelProviderDeconfigure";
import { Button } from "~/components/ui/button";
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuGroup,
    DropdownMenuLabel,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
} from "~/components/ui/dropdown-menu";

export function ModelProviderMenu({
    modelProvider,
}: {
    modelProvider: ModelProvider;
}) {
    return (
        <DropdownMenu>
            <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="icon">
                    <EllipsisVerticalIcon />
                </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent className="w-56" side="bottom" align="start">
                <DropdownMenuGroup>
                    <DropdownMenuLabel>{modelProvider.name}</DropdownMenuLabel>
                    <DropdownMenuSeparator />
                    <ModelProviderDeconfigure modelProvider={modelProvider} />
                </DropdownMenuGroup>
            </DropdownMenuContent>
        </DropdownMenu>
    );
}
