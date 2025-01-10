import { SelectValue } from "@radix-ui/react-select";
import { Monitor, Moon, Sun } from "lucide-react";

import { AppTheme } from "~/lib/service/themeService";
import { cn } from "~/lib/utils";

import { useTheme } from "~/components/theme";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
} from "~/components/ui/select";

export function DarkModeToggle({ className = "" }: { className?: string }) {
	const { theme, updateTheme } = useTheme();

	const getIcon = (currentTheme: AppTheme) => {
		switch (currentTheme) {
			case AppTheme.Light:
				return <Sun className="h-6 w-6" />;
			case AppTheme.Dark:
				return <Moon className="h-6 w-6" />;
			case AppTheme.System:
				return <Monitor className="h-6 w-6" />;
		}
	};

	return (
		<Select
			value={theme}
			onValueChange={(value: AppTheme) => {
				updateTheme(value);
			}}
		>
			<SelectTrigger
				className={cn(
					className,
					"h-10 w-10 justify-center border-0 p-0 shadow-none ring-0 focus:ring-0"
				)}
			>
				<SelectValue aria-label={theme}>{getIcon(theme)}</SelectValue>
			</SelectTrigger>
			<SelectContent>
				<SelectItem value={AppTheme.System}>
					<Monitor className="mr-2 inline-block h-4 w-4" /> System
				</SelectItem>
				<SelectItem value={AppTheme.Dark}>
					<Moon className="mr-2 inline-block h-4 w-4" /> Dark
				</SelectItem>
				<SelectItem value={AppTheme.Light}>
					<Sun className="mr-2 inline-block h-4 w-4" /> Light
				</SelectItem>
			</SelectContent>
		</Select>
	);
}
