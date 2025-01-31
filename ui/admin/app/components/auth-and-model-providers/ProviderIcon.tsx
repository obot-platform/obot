import { BoxesIcon } from "lucide-react";

import { AuthProvider, ModelProvider } from "~/lib/model/providers";
import { cn } from "~/lib/utils";

export function ProviderIcon({
	provider,
	size = "md",
}: {
	provider: ModelProvider | AuthProvider;
	size?: "md" | "lg";
}) {
	return provider.icon ? (
		<img
			src={provider.icon}
			alt={provider.name}
			className={cn({
				"h-6 w-6": size === "md",
				"h-16 w-16": size === "lg",
				"dark:invert": !provider.iconNoInvert,
			})}
		/>
	) : (
		<BoxesIcon className="color-primary h-16 w-16" />
	);
}
