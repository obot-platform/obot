import { BoxesIcon } from "lucide-react";

import { ModelProvider } from "~/lib/model/modelProviders";
import { cn } from "~/lib/utils";

import { CommonModelProviderIds } from "~/components/model-providers/constants";

export function ModelProviderIcon({
    modelProvider,
    size = "md",
}: {
    modelProvider: ModelProvider;
    size?: "md" | "lg";
}) {
    return modelProvider.icon ? (
        <img
            src={modelProvider.icon}
            alt={modelProvider.name}
            className={cn({
                "w-6 h-6": size === "md",
                "w-16 h-16": size === "lg",
                "dark:invert":
                    modelProvider.id !== CommonModelProviderIds.AZURE_OPENAI,
            })}
        />
    ) : (
        <BoxesIcon className="w-16 h-16 color-primary" />
    );
}
