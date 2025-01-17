import { User } from "lucide-react";
import React from "react";

import { AuthDisabledUsername } from "~/lib/model/auth";
import { roleLabel } from "~/lib/model/users";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { cn } from "~/lib/utils";

import { useAuth } from "~/components/auth/AuthContext";
import { Avatar, AvatarFallback, AvatarImage } from "~/components/ui/avatar";
import { ClickableDiv } from "~/components/ui/clickable-div";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuGroup,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "~/components/ui/dropdown-menu";

interface UserMenuProps {
	className?: string;
	avatarOnly?: boolean;
}

export const UserMenu: React.FC<UserMenuProps> = ({
	className,
	avatarOnly,
}) => {
	const { me } = useAuth();

	if (me.username === AuthDisabledUsername) {
		return null;
	}

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<ClickableDiv className={cn("flex items-center", className)}>
					<Avatar className={cn("mr-4", { "w-full": avatarOnly })}>
						<AvatarImage src={me?.iconURL} />
						<AvatarFallback>
							<User className="h-5 w-5" />
						</AvatarFallback>
					</Avatar>
					{!avatarOnly && (
						<div className="max-w-full truncate">
							<p className="truncate text-sm font-medium">{me?.email}</p>
							<p className="truncate text-left text-xs text-muted-foreground">
								{roleLabel(me?.role)}
							</p>
						</div>
					)}
				</ClickableDiv>
			</DropdownMenuTrigger>
			<DropdownMenuContent className="w-auto" side="bottom" align="start">
				<DropdownMenuGroup>
					<DropdownMenuItem
						onClick={async () => {
							await fetch(ApiRoutes.bootstrap.logout().url, {
								method: "POST",
							});

							window.location.href = "/oauth2/sign_out?rd=/admin/";
						}}
					>
						Sign Out
					</DropdownMenuItem>
				</DropdownMenuGroup>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};
