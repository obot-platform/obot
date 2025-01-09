import { CheckIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

import { Role, User, roleFromString, roleLabel } from "~/lib/model/users";
import { UserService } from "~/lib/service/api/userService";
import { cn } from "~/lib/utils";

import { Button } from "~/components/ui/button";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "~/components/ui/select";
import { useAsync } from "~/hooks/useAsync";
import { useWatcher } from "~/hooks/useWatcher";

export function UserRoleForm({ user }: { user: User }) {
	const revalidate = useAsync(UserService.getUsers.revalidate);

	const updateUser = useAsync(UserService.updateUser, {
		onSuccess: async () => toast.success("Role Updated Successfully"),
	});

	const [updatedRole, setUpdatedRole] = useState<string>(user.role.toString());

	const watcher = useWatcher(user);
	if (watcher.changed) {
		setUpdatedRole(user.role.toString());

		watcher.update();
	}

	const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
		e.preventDefault();

		if (!updatedRole) return;

		const [error] = await updateUser.executeAsync(user.username, {
			role: roleFromString(updatedRole),
		});

		if (error) return;

		await revalidate.execute();
	};

	const hasChange = updatedRole != null && updatedRole !== user.role.toString();

	return (
		<form onSubmit={handleSubmit} className="flex items-center gap-2">
			<Select value={updatedRole} onValueChange={setUpdatedRole}>
				<SelectTrigger className="w-36">
					<SelectValue />
				</SelectTrigger>

				<SelectContent>
					{Object.values(Role).map((role) => (
						<SelectItem key={role} value={role.toString()}>
							{roleLabel(role)}
						</SelectItem>
					))}
				</SelectContent>
			</Select>

			<Button
				type="submit"
				size="icon"
				variant="ghost"
				loading={updateUser.isLoading}
				disabled={!hasChange || updateUser.isLoading || revalidate.isLoading}
				className={cn({ invisible: !hasChange || revalidate.isLoading })}
			>
				<CheckIcon className={cn("text-success")} />
			</Button>
		</form>
	);
}
