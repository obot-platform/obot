import {
	EraserIcon,
	LinkIcon,
	PaintbrushIcon,
	PaletteIcon,
	PencilIcon,
	SlashIcon,
} from "lucide-react";
import { useState } from "react";

import { AgentIcons } from "~/lib/model/agents";
import { AppTheme } from "~/lib/service/themeService";
import { cn } from "~/lib/utils/cn";

import { AgentImageUrl } from "~/components/agent/icon/AgentImageUrl";
import { useTheme } from "~/components/theme";
import { Avatar, AvatarFallback, AvatarImage } from "~/components/ui/avatar";
import { Button } from "~/components/ui/button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuPortal,
	DropdownMenuSub,
	DropdownMenuSubContent,
	DropdownMenuSubTrigger,
	DropdownMenuTrigger,
} from "~/components/ui/dropdown-menu";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";

const iconOptions = [
	"obot_alt_1",
	"obot_alt_2",
	"obot_alt_3",
	"obot_alt_4",
	"obot_alt_5",
	"obot_alt_6",
	"obot_alt_7",
	"obot_alt_8",
	"obot_alt_9",
	"obot_alt_10",
];

const colors = [
	{
		name: "purple",
		value: "#380067",
	},
	{
		name: "blue",
		value: "#4f73f3",
	},
	{
		name: "teal",
		value: "#2ddcec",
	},
	{
		name: "green",
		value: "#06eaa7",
	},
	{
		name: "red",
		value: "#ff4044",
	},
	{
		name: "yellow",
		value: "#fdcc11",
	},
	{
		name: "orange",
		value: "#ff7240",
	},
];

type AgentIconProps = {
	icons: AgentIcons | null;
	onChange: (icons: AgentIcons | null) => void;
	name?: string;
};

export function AgentIcon({ icons, onChange, name }: AgentIconProps) {
	const { theme } = useTheme();
	const [imageUrlDialogOpen, setImageUrlDialogOpen] = useState(false);

	const { icon = "", iconDark = "" } = icons ?? {};
	const isDarkMode = theme === AppTheme.Dark;
	const obotIconIndex = iconOptions.findIndex((option) =>
		icon.includes(option)
	);
	return (
		<>
			<DropdownMenu>
				<Tooltip>
					<TooltipTrigger asChild>
						<DropdownMenuTrigger asChild>
							<Button variant="ghost" size="icon-xl" className="group relative">
								<Avatar className="size-20">
									<AvatarImage src={iconDark && isDarkMode ? iconDark : icon} />
									<AvatarFallback className="text-[3.5rem] font-semibold">
										{name?.charAt(0) ?? ""}
									</AvatarFallback>
								</Avatar>
								<div className="absolute -right-1 top-0 items-center justify-center rounded-full bg-primary-foreground p-2 opacity-0 drop-shadow-md transition group-hover:opacity-100 group-focus:opacity-100">
									<PencilIcon className="!size-4" />
								</div>
							</Button>
						</DropdownMenuTrigger>
					</TooltipTrigger>
					<TooltipContent>Change Agent Icon</TooltipContent>
				</Tooltip>
				<DropdownMenuContent className="w-52" align="start">
					<DropdownMenuSub>
						<DropdownMenuSubTrigger className="flex items-center gap-2">
							<PaintbrushIcon size={16} /> Select Icon
						</DropdownMenuSubTrigger>
						<DropdownMenuPortal>
							<DropdownMenuSubContent>
								{renderIconOptions()}
							</DropdownMenuSubContent>
						</DropdownMenuPortal>
					</DropdownMenuSub>
					<DropdownMenuSub>
						<DropdownMenuSubTrigger
							className={cn("flex items-center gap-2", {
								"opacity-50": obotIconIndex === -1,
							})}
							disabled={obotIconIndex === -1}
						>
							<PaletteIcon size={16} /> Choose Color
						</DropdownMenuSubTrigger>
						<DropdownMenuPortal>
							<DropdownMenuSubContent>
								{renderColorOptions()}
							</DropdownMenuSubContent>
						</DropdownMenuPortal>
					</DropdownMenuSub>
					<DropdownMenuItem
						className="flex items-center gap-2"
						onClick={() => setImageUrlDialogOpen(true)}
					>
						<LinkIcon size={16} /> Use Image URL
					</DropdownMenuItem>
					<DropdownMenuItem
						className="flex items-center gap-2"
						onClick={() => {
							onChange(null);
						}}
					>
						<EraserIcon size={16} /> Clear
					</DropdownMenuItem>
				</DropdownMenuContent>
			</DropdownMenu>
			<AgentImageUrl
				open={imageUrlDialogOpen}
				onOpenChange={setImageUrlDialogOpen}
				icons={icons}
				onChange={onChange}
			/>
		</>
	);

	function renderIconOptions() {
		return (
			<div className="grid grid-cols-5 gap-2 p-2">
				{iconOptions.map((icon) => (
					<DropdownMenuItem
						key={icon}
						onClick={() => {
							onChange({
								icon: generateIconUrl(icon),
								iconDark: generateIconUrl(icon, true),
								collapsed: "",
								collapsedDark: "",
							});
						}}
					>
						<img
							src={generateIconUrl(icon)}
							alt="Agent Icon"
							className={cn("h-8 w-8", {
								"dark:invert": isDarkMode,
							})}
						/>
					</DropdownMenuItem>
				))}
			</div>
		);
	}

	function renderColorOptions() {
		return (
			<div className="grid grid-cols-4 gap-2 p-2">
				{colors.map((color) => (
					<DropdownMenuItem
						key={color.name}
						onClick={() => {
							onChange({
								icon: generateIconUrl(
									iconOptions[obotIconIndex],
									false,
									color.name
								),
								iconDark: generateIconUrl(
									iconOptions[obotIconIndex],
									false,
									color.name
								),
								collapsed: "",
								collapsedDark: "",
							});
						}}
					>
						<div
							className={cn("h-8 w-8 rounded-sm")}
							style={{ backgroundColor: color.value }}
						/>
					</DropdownMenuItem>
				))}
				<DropdownMenuItem
					onClick={() => {
						onChange({
							icon: generateIconUrl(iconOptions[obotIconIndex], false),
							iconDark: generateIconUrl(iconOptions[obotIconIndex], true),
							collapsed: "",
							collapsedDark: "",
						});
					}}
				>
					<div
						className={cn(
							"flex h-8 w-8 items-center justify-center rounded-sm border border-foreground"
						)}
					>
						<SlashIcon />
					</div>
				</DropdownMenuItem>
			</div>
		);
	}

	function generateIconUrl(icon: string, dark = false, color = "") {
		return `/agent/images/${icon}${dark ? "_dark" : ""}${color ? `_${color}` : ""}.svg`;
	}
}
