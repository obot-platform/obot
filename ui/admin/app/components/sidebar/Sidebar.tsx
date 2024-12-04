import { Link, useLocation } from "@remix-run/react";
import {
    BotIcon,
    BoxesIcon,
    BrainIcon,
    KeyIcon,
    MessageSquare,
    PuzzleIcon,
    SettingsIcon,
    User,
    WebhookIcon,
    Wrench,
} from "lucide-react";
import { $path } from "remix-routes";

import { cn } from "~/lib/utils";

import { OttoLogo } from "~/components/branding/OttoLogo";
import { Button } from "~/components/ui/button";
import {
    Popover,
    PopoverContent,
    PopoverTrigger,
} from "~/components/ui/popover";
import {
    Sidebar,
    SidebarContent,
    SidebarFooter,
    SidebarGroup,
    SidebarGroupContent,
    SidebarHeader,
    SidebarMenu,
    SidebarMenuButton,
    SidebarMenuItem,
    SidebarRail,
    useSidebar,
} from "~/components/ui/sidebar";

// Menu items.
const items = [
    {
        title: "Agents",
        url: $path("/agents"),
        icon: BotIcon,
    },
    {
        title: "Threads",
        url: $path("/threads"),
        icon: MessageSquare,
    },
    {
        title: "Tools",
        url: $path("/tools"),
        icon: Wrench,
    },
    {
        title: "Users",
        url: $path("/users"),
        icon: User,
    },
    {
        title: "OAuth Apps",
        url: $path("/oauth-apps"),
        icon: KeyIcon,
    },
    {
        title: "Workflows",
        url: $path("/workflows"),
        icon: PuzzleIcon,
    },
    {
        title: "Models",
        url: $path("/models"),
        icon: BrainIcon,
    },
    {
        title: "Model Providers",
        url: $path("/model-providers"),
        icon: BoxesIcon,
    },
    {
        title: "Webhooks",
        url: $path("/webhooks"),
        icon: WebhookIcon,
    },
];

export function AppSidebar() {
    const { state } = useSidebar();
    const location = useLocation();
    return (
        <Sidebar collapsible="icon">
            <SidebarRail />
            <SidebarHeader
                className={cn("h-[60px]", state === "collapsed" ? "" : "px-4")}
            >
                <div className={cn("flex items-center justify-center h-full")}>
                    <OttoLogo hideText={state === "collapsed"} />
                </div>
            </SidebarHeader>
            <SidebarContent>
                <SidebarGroup>
                    <SidebarGroupContent>
                        <SidebarMenu className="w-full">
                            {items.map((item) => (
                                <SidebarMenuItem
                                    key={item.title}
                                    className="w-full"
                                >
                                    <SidebarMenuButton
                                        asChild
                                        className="w-full"
                                        isActive={location.pathname.startsWith(
                                            item.url
                                        )}
                                    >
                                        <Link
                                            to={item.url}
                                            className="w-full flex items-center"
                                        >
                                            <item.icon
                                                className={cn(
                                                    "mr-2",
                                                    location.pathname.startsWith(
                                                        item.url
                                                    )
                                                        ? "text-primary"
                                                        : ""
                                                )}
                                            />
                                            <span>{item.title}</span>
                                        </Link>
                                    </SidebarMenuButton>
                                </SidebarMenuItem>
                            ))}
                        </SidebarMenu>
                    </SidebarGroupContent>
                </SidebarGroup>
            </SidebarContent>
        </Sidebar>
    );
}

// disabling this because this will inevitably be used in the future
// eslint-disable-next-line @typescript-eslint/no-unused-vars
function AppSidebarFooter() {
    const { state } = useSidebar();
    return (
        <SidebarFooter
            className={cn(
                "pb-4 bg-background",
                state === "collapsed" ? "" : "px-2"
            )}
        >
            <Popover>
                <PopoverTrigger asChild>
                    <SidebarMenuButton className="w-full flex items-center">
                        <SettingsIcon className="mr-2" /> Settings
                    </SidebarMenuButton>
                </PopoverTrigger>
                <PopoverContent side="right" align="end">
                    <Button variant="secondary" asChild className="w-full">
                        <Link
                            to={$path("/oauth-apps")}
                            className="flex items-center p-2 hover:bg-accent rounded-md"
                        >
                            <KeyIcon className="mr-2 h-4 w-4" />
                            <span>Manage OAuth Apps</span>
                        </Link>
                    </Button>
                </PopoverContent>
            </Popover>
        </SidebarFooter>
    );
}
