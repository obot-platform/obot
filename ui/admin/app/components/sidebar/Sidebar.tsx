import React from "react";

import { cn } from "~/lib/utils";

import { SidebarFull } from "~/components/sidebar/SidebarFull";

import { useLayout } from "../layout/LayoutProvider";
import { SidebarCollapsed } from "./SidebarCollapsed";

type SidebarProps = React.HTMLAttributes<HTMLDivElement>;

export function Sidebar({ className, ...props }: SidebarProps) {
    const { isExpanded, sidebarWidth } = useLayout();

    return (
        <div
            className={cn("h-full overflow-hidden", sidebarWidth, className)}
            {...props}
        >
            <div className="relative h-full">
                <div
                    className={cn(
                        "absolute inset-y-0 left-0 w-64 transition-transform duration-300 ease-in-out",
                        isExpanded
                            ? "translate-x-0 z-20"
                            : "-translate-x-48 z-10"
                    )}
                >
                    <div className="h-full border-r">
                        <SidebarFull />
                    </div>
                </div>

                <div
                    className={cn(
                        "absolute inset-y-0 left-0 w-16 transition-opacity duration-300 ease-in-out",
                        isExpanded
                            ? "opacity-0 pointer-events-none"
                            : "opacity-100 z-20"
                    )}
                >
                    <div className="h-full flex flex-col items-center border-r">
                        <SidebarCollapsed />
                    </div>
                </div>
            </div>
        </div>
    );
}
