import { ReactNode, createContext, useContext, useState } from "react";

import { cn } from "~/lib/utils";

type LayoutContextType = {
    isExpanded: boolean;
    onExpandedChange: (isExpanded?: Nullish<boolean>) => void;
    sidebarWidth: string;
    smallSidebarWidth: string;
    fullSidebarWidth: string;
};

const LayoutContext = createContext<LayoutContextType | null>(null);

const ExpandedService = {
    getExpanded: () => {
        const saved = localStorage.getItem("sidebarExpanded");
        return saved !== null ? (JSON.parse(saved) as boolean) : true;
    },
    setExpanded: (isExpanded: boolean) => {
        localStorage.setItem("sidebarExpanded", JSON.stringify(isExpanded));
    },
};

export function LayoutProvider({ children }: { children: ReactNode }) {
    const [isExpanded, setIsExpanded] = useState(ExpandedService.getExpanded);

    return (
        <LayoutContext.Provider
            value={{
                isExpanded,
                sidebarWidth: cn(
                    "transition-all duration-300 ease-in-out",
                    isExpanded ? "w-64" : "w-16"
                ),
                smallSidebarWidth: "w-16",
                fullSidebarWidth: "w-64",
                onExpandedChange: (expanded) => {
                    if (expanded == null) {
                        // toggle
                        setIsExpanded((prev) => !prev);
                        ExpandedService.setExpanded(!isExpanded);
                    } else {
                        setIsExpanded(expanded);
                        ExpandedService.setExpanded(expanded);
                    }
                },
            }}
        >
            {children}
        </LayoutContext.Provider>
    );
}

export function useLayout() {
    const context = useContext(LayoutContext);
    if (!context) {
        throw new Error("useLayout must be used within a LayoutProvider");
    }
    return context;
}
