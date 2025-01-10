import { PlusIcon } from "lucide-react";
import { useState } from "react";

import { CustomOAuthAppForm } from "~/components/oauth-apps/custom/CustomOAuthAppForm";
import { Button } from "~/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
	DialogTrigger,
} from "~/components/ui/dialog";

export function CreateCustomOAuthApp() {
	const [isOpen, setIsOpen] = useState(false);

	return (
		<Dialog open={isOpen} onOpenChange={setIsOpen}>
			<DialogTrigger asChild>
				<Button variant="outline" className="flex items-center gap-2">
					<PlusIcon className="h-4 w-4" /> Create a Custom OAuth App
				</Button>
			</DialogTrigger>

			<DialogContent>
				<DialogHeader>
					<DialogTitle>Create Custom OAuth App</DialogTitle>
				</DialogHeader>

				<DialogDescription hidden>Create Custom OAuth App</DialogDescription>

				<CustomOAuthAppForm
					onComplete={() => setIsOpen(false)}
					onCancel={() => setIsOpen(false)}
				/>
			</DialogContent>
		</Dialog>
	);
}
