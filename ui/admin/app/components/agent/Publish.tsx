import { zodResolver } from "@hookform/resolvers/zod";
import { Eye } from "lucide-react";
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

type PublishProps = {
	className?: string;
	alias: string;
	onPublish: (alias: string) => void;
};

const publishSchema = z.object({
	alias: z
		.string()
		.min(1, "Alias is required.")
		.regex(
			/^[a-zA-Z0-9_-]+$/,
			"Only alphanumeric characters, dashes, and underscores are allowed."
		),
});

type PublishFormValues = z.infer<typeof publishSchema>;

export function Publish({ className, alias: _alias, onPublish }: PublishProps) {
	const form = useForm<PublishFormValues>({
		resolver: zodResolver(publishSchema),
		defaultValues: {
			alias: _alias,
		},
	});

	const handlePublish = (values: PublishFormValues) => {
		onPublish(values.alias);
	};

	return (
		<Dialog>
			<DialogTrigger asChild>
				<Button className={className} variant="secondary" size="sm">
					<Eye className="h-4 w-4" />
					Publish
				</Button>
			</DialogTrigger>
			<DialogContent className="max-w-3xl p-10">
				<Form {...form}>
					<form
						onSubmit={form.handleSubmit(handlePublish)}
						className="space-y-4"
					>
						<div className="flex w-full justify-between gap-2 pt-6">
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
						<div className="space-y-4 py-4">
							<p className="text-muted-foreground">
								This agent will be available at:
							</p>

							<p className="text-primary">
								{`${window.location.protocol}//${window.location.host}/${form.watch("alias")}`}
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
									<Eye className="h-4 w-4" />
									Publish
								</Button>
							</div>
						</DialogFooter>
					</form>
				</Form>
			</DialogContent>
		</Dialog>
	);
}
