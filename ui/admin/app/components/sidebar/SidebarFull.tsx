import { Key, MessageSquare, User } from "lucide-react";
import { $path } from "remix-routes";

import { cn } from "~/lib/utils";

import { SidebarSection } from "~/components/sidebar/SidebarSection";
import { ScrollArea } from "~/components/ui/scroll-area";

type SidebarFullProps = React.HTMLAttributes<HTMLDivElement>;

export function SidebarFull({ className }: SidebarFullProps) {
    return (
        <div className={cn("h-full flex flex-col", className)}>
            <ScrollArea className="flex-grow overflow-y-auto">
                <div className="w-64 pb-20 h-full">
                    <SidebarSection
                        title="Agents"
                        linkTo={$path("/agents")}
                        icon={<User className="w-5 h-5" />}
                    />
                    <SidebarSection
                        title="Threads"
                        linkTo={$path("/threads")}
                        icon={<MessageSquare className="w-5 h-5" />}
                    />
                    <SidebarSection
                        title="OAuth Apps"
                        linkTo={$path("/oauth-apps")}
                        icon={<Key className="w-5 h-5" />}
                    />
                </div>
            </ScrollArea>
        </div>
    );
}
