import { EllipsisIcon } from "lucide-react";
import { useState } from "react";

import { User } from "~/lib/model/users";

import { Button } from "~/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from "~/components/ui/dialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "~/components/ui/dropdown-menu";
import { UserUpdateForm } from "~/components/user/UserUpdateForm";

export function UserActionsDropdown({ user }: { user: User }) {
	const [editOpen, setEditOpen] = useState(false);

	return (
		<>
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button variant="ghost" size="icon">
						<EllipsisIcon />
					</Button>
				</DropdownMenuTrigger>

				<DropdownMenuContent side="top" align="end">
					<DropdownMenuItem
						onClick={() => setEditOpen(true)}
						disabled={user.explicitAdmin}
					>
						Update Role
					</DropdownMenuItem>
				</DropdownMenuContent>
			</DropdownMenu>

			<Dialog open={editOpen} onOpenChange={setEditOpen}>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>Update Role</DialogTitle>

						<DialogDescription hidden>
							Update the user&apos;s role and other details.
						</DialogDescription>
					</DialogHeader>

					<UserUpdateForm
						user={user}
						onSuccess={() => setEditOpen(false)}
						onCancel={() => setEditOpen(false)}
					/>
				</DialogContent>
			</Dialog>
		</>
	);
}
