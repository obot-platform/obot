import { EditIcon, PlusIcon } from "lucide-react";
import { useState } from "react";

import { EmailReceiver } from "~/lib/model/email-receivers";

import { EmailReceiverForm } from "~/components/task-triggers/EmailReceiverForm";
import { Button } from "~/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
	DialogTrigger,
} from "~/components/ui/dialog";
import { ScrollArea } from "~/components/ui/scroll-area";

export function TaskEmailDialog({
	taskId,
	emailReceiver,
}: {
	taskId: string;
	emailReceiver?: EmailReceiver;
}) {
	const [open, setOpen] = useState(false);

	return (
		<Dialog open={open} onOpenChange={setOpen}>
			<DialogTrigger asChild>
				{emailReceiver ? (
					<Button variant="ghost" size="icon">
						<EditIcon />
					</Button>
				) : (
					<Button variant="ghost" startContent={<PlusIcon />}>
						Add Email Trigger
					</Button>
				)}
			</DialogTrigger>
			<DialogContent className="gap-0 p-0">
				<DialogHeader className="p-8 pb-0">
					<DialogTitle>
						{emailReceiver
							? "Update Task Email Receiver"
							: "Add Email Receiver To Task"}
					</DialogTitle>

					<DialogDescription>
						Email Receivers are used to run the task when an email is received.
					</DialogDescription>
				</DialogHeader>

				<ScrollArea className="max-h-[60vh]">
					<EmailReceiverForm
						onContinue={() => setOpen(false)}
						emailReceiver={emailReceiver ?? { workflowName: taskId }}
						hideTitle
					/>
				</ScrollArea>
			</DialogContent>
		</Dialog>
	);
}
