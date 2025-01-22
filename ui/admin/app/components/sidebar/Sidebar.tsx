import {
	BotIcon,
	BoxesIcon,
	InfoIcon,
	LockIcon,
	MessageSquare,
	PuzzleIcon,
	User,
	WebhookIcon,
	Wrench,
} from "lucide-react";
import { Link, useLocation } from "react-router";
import { $path } from "safe-routes";
import useSWR from "swr";

import { VersionApiService } from "~/lib/service/api/versionApiService";
import { cn } from "~/lib/utils";

import { ObotLogo } from "~/components/branding/ObotLogo";
import { Button } from "~/components/ui/button";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "~/components/ui/popover";
import { Separator } from "~/components/ui/separator";
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
		title: "Workflows",
		url: $path("/workflows"),
		icon: PuzzleIcon,
	},
	{
		title: "Model Providers",
		url: $path("/model-providers"),
		icon: BoxesIcon,
	},
	{
		title: "Workflow Triggers",
		url: $path("/workflow-triggers"),
		icon: WebhookIcon,
	},
	{
		title: "Auth Providers",
		url: $path("/auth-providers"),
		icon: LockIcon,
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
				<div className={cn("flex h-full items-center justify-center")}>
					<ObotLogo hideText={state === "collapsed"} />
				</div>
			</SidebarHeader>
			<SidebarContent>
				<SidebarGroup>
					<SidebarGroupContent>
						<SidebarMenu className="w-full">
							{items.map((item) => (
								<SidebarMenuItem key={item.title} className="w-full">
									<SidebarMenuButton
										asChild
										className="w-full"
										isActive={location.pathname.startsWith(item.url)}
									>
										<Link to={item.url} className="flex w-full items-center">
											<item.icon
												className={cn(
													"mr-2",
													location.pathname.startsWith(item.url)
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
			<SidebarFooter>
				<VersionInfo />
			</SidebarFooter>
		</Sidebar>
	);
}

function VersionInfo() {
	const { state } = useSidebar();
	const getVersion = useSWR(VersionApiService.getVersion.key(), () =>
		VersionApiService.getVersion()
	);

	const { data: version } = getVersion;
	const versionEntries = Object.entries(version ?? {});
	return version?.obot ? (
		<Popover>
			<PopoverTrigger asChild>
				<Button
					variant="ghost"
					size="sm"
					startContent={<InfoIcon />}
					className="text-muted-foreground"
				>
					{state !== "collapsed" ? version.obot : null}
				</Button>
			</PopoverTrigger>
			<PopoverContent className="w-fit min-w-44 p-2">
				<div>
					{versionEntries.map(([key, value], index) =>
						value ? (
							<div key={key}>
								<small className="text-muted-foreground">{key}:</small>
								<small>{value}</small>
								{index !== versionEntries.length - 1 && (
									<Separator className="my-2" />
								)}
							</div>
						) : null
					)}
				</div>
			</PopoverContent>
		</Popover>
	) : null;
}
