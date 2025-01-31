import { BoxesIcon } from "lucide-react";
import { useEffect, useState } from "react";

import { AuthProvider, ModelProvider } from "~/lib/model/providers";
import { cn } from "~/lib/utils";

export function ProviderIcon({
	provider,
	size = "md",
}: {
	provider: ModelProvider | AuthProvider;
	size?: "md" | "lg";
}) {
	const [isDarkMode, setIsDarkMode] = useState(false);

	useEffect(() => {
		const darkModeMediaQuery = window.matchMedia(
			"(prefers-color-scheme: dark)"
		);
		setIsDarkMode(darkModeMediaQuery.matches);

		const handleChange = (e: MediaQueryListEvent) => {
			setIsDarkMode(e.matches);
		};

		darkModeMediaQuery.addEventListener("change", handleChange);
		return () => darkModeMediaQuery.removeEventListener("change", handleChange);
	}, []);

	return provider.icon ? (
		<img
			src={isDarkMode && provider.iconDark ? provider.iconDark : provider.icon}
			alt={provider.name}
			className={cn({
				"h-6 w-6": size === "md",
				"h-16 w-16": size === "lg",
				"dark:invert": !provider.iconDark,
			})}
		/>
	) : (
		<BoxesIcon className="color-primary h-16 w-16" />
	);
}
