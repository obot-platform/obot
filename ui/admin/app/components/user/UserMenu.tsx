import { LogOutIcon, User } from "lucide-react";
import React from "react";

import { AuthDisabledUsername, CommonAuthProviderIds } from "~/lib/model/auth";
import { User as UserModel, roleLabel } from "~/lib/model/users";
import { BootstrapApiService } from "~/lib/service/api/bootstrapApiService";
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

	const displayName = getDisplayName(me);

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<ClickableDiv
					className={cn(
						"flex items-center gap-4 rounded-l-3xl border-0 p-2 hover:bg-secondary focus:outline-none data-[state=open]:bg-secondary",
						className
					)}
				>
					<Avatar className={cn({ "w-full": avatarOnly })}>
						<AvatarImage src={me?.iconURL} />
						<AvatarFallback>
							<User className="h-5 w-5" />
						</AvatarFallback>
					</Avatar>
					{!avatarOnly && (
						<div className="max-w-full truncate">
							<p className="truncate text-sm font-medium">{displayName}</p>
							<p className="truncate text-left text-xs text-muted-foreground">
								{roleLabel(me?.role)}
							</p>
						</div>
					)}
				</ClickableDiv>
			</DropdownMenuTrigger>
			<DropdownMenuContent
				className="w-auto min-w-56"
				side="bottom"
				align="end"
			>
				<DropdownMenuGroup>
					<DropdownMenuItem
						className="flex items-center gap-2"
						onClick={async () => {
							await BootstrapApiService.bootstrapLogout();

							window.location.href = "/oauth2/sign_out?rd=/admin/";
						}}
					>
						<LogOutIcon className="size-4" />
						Sign Out
					</DropdownMenuItem>
				</DropdownMenuGroup>
			</DropdownMenuContent>
		</DropdownMenu>
	);

	function getDisplayName(user?: UserModel) {
		if (user?.currentAuthProvider === CommonAuthProviderIds.GITHUB) {
			return user.username;
		}

		return user?.email;
	}
};
