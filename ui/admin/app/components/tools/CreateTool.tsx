import { PlusIcon } from "lucide-react";
import { useState } from "react";

import { ErrorDialog } from "~/components/composed/ErrorDialog";
import { CreateToolForm } from "~/components/tools/CreateToolForm";
import { Button } from "~/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
	DialogTrigger,
} from "~/components/ui/dialog";

export function CreateTool() {
	const [isDialogOpen, setIsDialogOpen] = useState(false);
	const [errorDialogError, setErrorDialogError] = useState("");

	const handleSuccess = () => {
		setIsDialogOpen(false);
	};

	const handleError = (error: string) => {
		setIsDialogOpen(false);
		setErrorDialogError(error);
	};

	return (
		<>
			<Dialog open={isDialogOpen} onOpenChange={setIsDialogOpen}>
				<DialogTrigger asChild>
					<Button variant="outline">
						<PlusIcon className="mr-2 h-4 w-4" />
						Register New Tool
					</Button>
				</DialogTrigger>
				<DialogContent className="max-w-2xl">
					<DialogHeader>
						<DialogTitle>Create New Tool Reference</DialogTitle>
						<DialogDescription>
							Register a new tool reference to use in your agents.
						</DialogDescription>
					</DialogHeader>
					<CreateToolForm onSuccess={handleSuccess} onError={handleError} />
				</DialogContent>
			</Dialog>
			<ErrorDialog
				error={errorDialogError}
				isOpen={errorDialogError !== ""}
				onClose={() => setErrorDialogError("")}
			/>
		</>
	);
}
