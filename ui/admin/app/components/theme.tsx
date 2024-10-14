import { createContext, useContext, useEffect, useState } from "react";

import { AppTheme, getTheme } from "~/lib/service/themeService";

type ThemeContext = {
    theme: AppTheme;
    updateTheme: (theme: AppTheme) => void;
    isDark: boolean;
    systemEmoji: string;
};

const ThemeContext = createContext<ThemeContext | null>(null);

export function ThemeProvider({ children }: { children: React.ReactNode }) {
    const [theme, setTheme] = useState<AppTheme>(AppTheme.System);
    const [systemEmoji, setSystemEmoji] = useState<string>("🌐");
    const [isDark, setIsDark] = useState(false);

    useEffect(() => {
        const initialTheme = getTheme();
        setTheme(initialTheme);
        updateTheme(initialTheme);

        const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
        const handleSystemThemeChange = (event: MediaQueryListEvent) => {
            if (theme === AppTheme.System) {
                setSystemEmoji(event.matches ? "🌙" : "☀️");
                document.documentElement.classList.toggle(
                    "dark",
                    event.matches
                );
            }
        };

        mediaQuery.addEventListener("change", handleSystemThemeChange);

        // Initial system theme check
        if (initialTheme === AppTheme.System) {
            setSystemEmoji(mediaQuery.matches ? "🌙" : "☀️");
        }

        return () => {
            mediaQuery.removeEventListener("change", handleSystemThemeChange);
        };
    }, [theme]);

    const updateTheme = (newTheme: AppTheme) => {
        if (newTheme === AppTheme.System) {
            const isDark = window.matchMedia(
                "(prefers-color-scheme: dark)"
            ).matches;
            document.documentElement.classList.toggle("dark", isDark);
            setSystemEmoji(isDark ? "🌙" : "☀️");
            setIsDark(isDark);
        } else {
            document.documentElement.classList.toggle(
                "dark",
                newTheme === AppTheme.Dark
            );
            setIsDark(newTheme === AppTheme.Dark);
        }
        setTheme(newTheme);
        localStorage.setItem("theme", newTheme);
    };

    return (
        <ThemeContext.Provider
            value={{ theme, updateTheme, systemEmoji, isDark }}
        >
            {children}
        </ThemeContext.Provider>
    );
}

export function useTheme() {
    const context = useContext(ThemeContext);

    if (!context) {
        throw new Error("useTheme must be used within a ThemeProvider");
    }

    return context;
}
