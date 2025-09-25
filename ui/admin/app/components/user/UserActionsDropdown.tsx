import { EllipsisIcon } from "lucide-react";
import { useState } from "react";

import { User } from "~/lib/model/users";
import { UserService } from "~/lib/service/api/userService";

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
import { useAsync } from "~/hooks/useAsync";

export function UserActionsDropdown({ user }: { user: User }) {
	const [editOpen, setEditOpen] = useState(false);
	const [deleteOpen, setDeleteOpen] = useState(false);

	const deleteUser = useAsync(UserService.deleteUser, {
		onSuccess: async () => {
			UserService.getUsers.revalidate();
		},
	});

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
						disabled={user.explicitRole}
					>
						Update Role
					</DropdownMenuItem>
					<DropdownMenuItem
						onClick={() => setDeleteOpen(true)}
						disabled={user.explicitRole}
					>
						Delete User
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

			<Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>Delete User</DialogTitle>
					</DialogHeader>
					<DialogHeader>
						<p>Are you sure you want to delete this user?</p>
					</DialogHeader>
					<div className="flex justify-end gap-2">
						<Button
							type="button"
							variant="secondary"
							onClick={() => setDeleteOpen(false)}
						>
							Cancel
						</Button>

						<Button
							type="button"
							variant="destructive"
							onClick={() => deleteUser.execute(user.id)}
						>
							Delete
						</Button>
					</div>
				</DialogContent>
			</Dialog>
		</>
	);
}
