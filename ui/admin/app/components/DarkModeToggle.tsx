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
                return <Sun className="w-6 h-6" />;
            case AppTheme.Dark:
                return <Moon className="w-6 h-6" />;
            case AppTheme.System:
                return <Monitor className="w-6 h-6" />;
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
                    "w-10 h-10 p-0 justify-center ring-0 focus:ring-0 border-0 shadow-none"
                )}
            >
                <SelectValue aria-label={theme}>{getIcon(theme)}</SelectValue>
            </SelectTrigger>
            <SelectContent>
                <SelectItem value={AppTheme.System}>
                    <Monitor className="w-4 h-4 mr-2 inline-block" /> System
                </SelectItem>
                <SelectItem value={AppTheme.Dark}>
                    <Moon className="w-4 h-4 mr-2 inline-block" /> Dark
                </SelectItem>
                <SelectItem value={AppTheme.Light}>
                    <Sun className="w-4 h-4 mr-2 inline-block" /> Light
                </SelectItem>
            </SelectContent>
        </Select>
    );
}
