import { Link, UIMatch, useLocation, useMatches } from "@remix-run/react";
import React from "react";
import { $path } from "remix-routes";

import { RouteHandle } from "~/lib/service/routeHandles";
import { cn } from "~/lib/utils";

import { DarkModeToggle } from "~/components/DarkModeToggle";
import {
    Breadcrumb,
    BreadcrumbItem,
    BreadcrumbLink,
    BreadcrumbList,
    BreadcrumbPage,
    BreadcrumbSeparator,
} from "~/components/ui/breadcrumb";
import { SidebarTrigger } from "~/components/ui/sidebar";
import { UserMenu } from "~/components/user/UserMenu";

export function HeaderNav() {
    const headerHeight = "h-[60px]";

    return (
        <header
            className={cn(
                "flex transition-all duration-300 ease-in-out",
                headerHeight
            )}
        >
            <div className="h-full flex-auto flex">
                <div className="flex flex-grow z-20">
                    <div className="flex-grow flex justify-start items-center p-4">
                        <SidebarTrigger className="h-4 w-4" />
                        <div className="border-r h-4 mx-4" />
                        <RouteBreadcrumbHandles />
                    </div>

                    <div className="flex items-center justify-center p-4 mr-4">
                        <UserMenu className="pr-4 border-r mr-4" />
                        <DarkModeToggle />
                    </div>
                </div>
            </div>
        </header>
    );
}

function RouteBreadcrumbHandles() {
    const matches = useMatches() as UIMatch<unknown, RouteHandle>[];
    const location = useLocation();
    const filtered = matches.filter((match) => match.handle?.breadcrumb);

    const renderItem = (
        match: UIMatch<unknown, RouteHandle>,
        isLeaf: boolean
    ) => {
        if (!match.handle?.breadcrumb) return;

        return match.handle.breadcrumb(location).map((item, i, arr) => {
            const withHref = isLeaf && i === arr.length - 1;

            return (
                <React.Fragment key={`${match.id}-${i}`}>
                    <BreadcrumbSeparator />

                    <BreadcrumbItem>
                        {withHref ? (
                            <BreadcrumbPage>{item.content}</BreadcrumbPage>
                        ) : (
                            <BreadcrumbLink asChild>
                                <Link to={item.href ?? match.pathname}>
                                    {item.content}
                                </Link>
                            </BreadcrumbLink>
                        )}
                    </BreadcrumbItem>
                </React.Fragment>
            );
        });
    };

    return (
        <Breadcrumb>
            <BreadcrumbList>
                <BreadcrumbItem>
                    <BreadcrumbLink asChild>
                        <Link to={$path("/")}>Home</Link>
                    </BreadcrumbLink>
                </BreadcrumbItem>

                {filtered.map((match, i, arr) =>
                    renderItem(match, i === arr.length - 1)
                )}
            </BreadcrumbList>
        </Breadcrumb>
    );
}
