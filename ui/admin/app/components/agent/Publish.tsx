import { zodResolver } from "@hookform/resolvers/zod";
import { PencilIcon } from "lucide-react";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { ControlledInput } from "~/components/form/controlledInputs";
import { Button } from "~/components/ui/button";
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogFooter,
	DialogTitle,
	DialogTrigger,
} from "~/components/ui/dialog";
import { Form } from "~/components/ui/form";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";

type PublishProps = {
	className?: string;
	alias: string;
	id: string;
	onPublish: (alias: string) => void;
};

const publishSchema = z.object({
	alias: z
		.string()
		.regex(
			/^[a-zA-Z0-9_-]*$/,
			"Only alphanumeric characters, dashes, and underscores are allowed."
		),
});

type PublishFormValues = z.infer<typeof publishSchema>;

export function Publish({
	className,
	alias: _alias,
	onPublish,
	id,
}: PublishProps) {
	const form = useForm<PublishFormValues>({
		resolver: zodResolver(publishSchema),
		defaultValues: {
			alias: _alias,
		},
	});
	const [open, setOpen] = useState(false);

	const handlePublish = (values: PublishFormValues) => {
		onPublish(values.alias);
		setOpen(false);
	};

	const changedAlias = form.watch("alias");
	return (
		<Dialog open={open} onOpenChange={setOpen}>
			<Tooltip>
				<TooltipTrigger asChild>
					<DialogTrigger asChild>
						<Button className={className} variant="ghost" size="icon-sm">
							<PencilIcon />
						</Button>
					</DialogTrigger>
				</TooltipTrigger>
				<TooltipContent>Edit Agent URL Alias</TooltipContent>
			</Tooltip>

			<DialogContent className="max-w-3xl p-10">
				<Form {...form}>
					<form
						onSubmit={form.handleSubmit(handlePublish)}
						className="space-y-4 py-4"
					>
						<div className="flex w-full justify-between gap-2">
							<DialogTitle className="!text-md font-normal">
								Enter a handle for this agent:
							</DialogTitle>
							<ControlledInput
								classNames={{
									wrapper: "relative top-[-0.5rem] w-1/2",
								}}
								autoComplete="off"
								control={form.control}
								name="alias"
							/>
						</div>
						<div className="space-y-4">
							<p className="text-muted-foreground">
								This agent will be available at:
							</p>

							<p className="text-primary">
								{`${window.location.protocol}//${window.location.host}/${changedAlias || id}`}
							</p>

							<p className="text-muted-foreground">
								If you have another agent with this handle, you will need to
								unpublish it before this agent can be accessed at the above URL.
							</p>
						</div>
						<DialogFooter>
							<div className="flex w-full items-center justify-center gap-10 pt-4">
								<DialogClose asChild>
									<Button className="w-1/2" variant="outline">
										Cancel
									</Button>
								</DialogClose>
								<Button className="w-1/2" type="submit">
									Confirm
								</Button>
							</div>
						</DialogFooter>
					</form>
				</Form>
			</DialogContent>
		</Dialog>
	);
}
