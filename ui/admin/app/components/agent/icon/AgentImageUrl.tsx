import { zodResolver } from "@hookform/resolvers/zod";
import { GearIcon } from "@radix-ui/react-icons";
import { ImagePlusIcon } from "lucide-react";
import { Control, useForm } from "react-hook-form";
import { z } from "zod";

import { AgentIcons } from "~/lib/model/agents";

import { ControlledInput } from "~/components/form/controlledInputs";
import {
	Accordion,
	AccordionContent,
	AccordionItem,
	AccordionTrigger,
} from "~/components/ui/accordion";
import { Avatar, AvatarFallback, AvatarImage } from "~/components/ui/avatar";
import { Button } from "~/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "~/components/ui/dialog";
import { ScrollArea } from "~/components/ui/scroll-area";

type AgentImageUrlProps = {
	icons: AgentIcons | null;
	onChange: (icons: AgentIcons) => void;
	onOpenChange: (open: boolean) => void;
	open: boolean;
};

const formSchema = z.object({
	collapsed: z.string(),
	collapsedDark: z.string(),
	icon: z.string(),
	iconDark: z.string(),
});

type AgentIconFormValues = z.infer<typeof formSchema>;

export function AgentImageUrl({
	icons,
	onChange,
	onOpenChange,
	open,
}: AgentImageUrlProps) {
	const form = useForm<AgentIconFormValues>({
		resolver: zodResolver(formSchema),
		mode: "onChange",
		defaultValues: {
			collapsed: getDefaultUrl(icons?.collapsed),
			collapsedDark: getDefaultUrl(icons?.collapsedDark),
			icon: getDefaultUrl(icons?.icon),
			iconDark: getDefaultUrl(icons?.iconDark),
		},
	});

	const handleOpenChange = (open: boolean) => {
		onOpenChange(open);
		form.reset();
	};

	const handleApply = () => {
		onChange(form.getValues());
		onOpenChange(false);
	};

	const icon = form.watch("icon");

	return (
		<Dialog open={open} onOpenChange={handleOpenChange}>
			<DialogContent className="p-0">
				<DialogHeader className="px-6 pt-6">
					<DialogTitle>Use Image URL</DialogTitle>
				</DialogHeader>

				<ScrollArea className="max-h-[calc(100vh-14rem)] px-6">
					<DialogDescription>
						Include the full URL to the image you want to use as your agent
						icon.
					</DialogDescription>
					<div className="flex w-full gap-4 pt-4">
						<IconInput
							value={icon}
							name="icon"
							label="Icon URL"
							control={form.control}
						/>
					</div>
					<Accordion type="multiple">
						<AccordionItem value="model">
							<AccordionTrigger className="border-b">
								<h4 className="flex items-center gap-2">
									<GearIcon className="size-5" />
									Extra Customization
								</h4>
							</AccordionTrigger>

							<AccordionContent className="space-y-8 py-4">
								{["iconDark", "collapsed", "collapsedDark"].map((key) => (
									<IconInput
										key={key}
										value={form.watch(key as keyof AgentIconFormValues)}
										name={key}
										label={`${getIconLabel(key)} URL`}
										control={form.control}
									/>
								))}
							</AccordionContent>
						</AccordionItem>
					</Accordion>
				</ScrollArea>
				<DialogFooter className="px-6 py-4">
					<Button variant="ghost" onClick={() => handleOpenChange(false)}>
						Cancel
					</Button>
					<Button onClick={handleApply}>Apply</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);

	function getIconLabel(key: string) {
		switch (key) {
			case "icon":
				return "Icon";
			case "iconDark":
				return "Icon (Dark)";
			case "collapsed":
				return "Collapsed";
			case "collapsedDark":
				return "Collapsed (Dark)";
			default:
				return "";
		}
	}

	function getDefaultUrl(url?: string) {
		if (!url) return "";

		const isDefaultAsset = url.toLowerCase().startsWith("/agent/images/obot_");
		return isDefaultAsset ? "" : url;
	}
}

function IconInput({
	control,
	label,
	name,
	value,
}: {
	value: string;
	name: string;
	label: string;
	control: Control<AgentIconFormValues>;
}) {
	return (
		<div className="flex w-full items-center gap-4">
			<Avatar className="size-24">
				<AvatarImage src={value} className="bg-muted" />
				<AvatarFallback>
					<ImagePlusIcon />
				</AvatarFallback>
			</Avatar>
			<ControlledInput
				classNames={{
					wrapper: "w-[calc(100%-6rem)]",
				}}
				autoComplete="off"
				control={control}
				name={name as keyof AgentIconFormValues}
				label={label}
				placeholder="https://example.com/image.png"
			/>
		</div>
	);
}
