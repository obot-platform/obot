import { Key, MessageSquare, User } from "lucide-react";
import { Link } from "react-router-dom";
import { $path } from "remix-routes";

import { Button } from "~/components/ui/button";

export function SidebarCollapsed() {
    return (
        <div className="flex flex-col items-center mt-4 space-y-4">
            <Button asChild variant="ghost" size="icon" title="Agents">
                <Link to={$path("/agents")}>
                    <User className="w-5 h-5" />
                </Link>
            </Button>

            <Button asChild variant="ghost" size="icon" title="Threads">
                <Link to={$path("/threads")}>
                    <MessageSquare className="w-5 h-5" />
                </Link>
            </Button>

            <Button asChild variant="ghost" size="icon" title="OAuth Apps">
                <Link to={$path("/oauth-apps")}>
                    <Key className="w-5 h-5" />
                </Link>
            </Button>
        </div>
    );
}
