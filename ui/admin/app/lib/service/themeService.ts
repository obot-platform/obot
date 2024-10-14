export const AppTheme = {
    System: "system",
    Dark: "dark",
    Light: "light",
} as const;
export type AppTheme = (typeof AppTheme)[keyof typeof AppTheme];

export function getTheme() {
    return (
        (localStorage.getItem("theme") as AppTheme | null) || AppTheme.System
    );
}

export function setTheme(theme: AppTheme) {
    localStorage.setItem("theme", theme);
}
