import { useForm } from "react-hook-form";

import {
	ExplicitAdminDescription,
	Role,
	User,
	roleFromString,
	roleLabel,
} from "~/lib/model/users";
import { UserService } from "~/lib/service/api/userService";

import { ControlledCustomInput } from "~/components/form/controlledInputs";
import { Button } from "~/components/ui/button";
import { Form } from "~/components/ui/form";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "~/components/ui/select";
import { useAsync } from "~/hooks/useAsync";

const descriptions = {
	[Role.Admin]: "Admins can manage all aspects of the platform.",
	[Role.User]:
		"Users are restricted to only interracting with agents shared with them. They cannot access the Admin UI",
};

export function UserUpdateForm({
	user,
	onSuccess,
	formId,
	onCancel,
}: {
	user: User;
	onSuccess: () => void;
	onCancel: () => void;
	formId?: string;
}) {
	const form = useForm({ defaultValues: { role: user.role } });
	const [currentRole] = form.watch(["role"]);

	const updateUser = useAsync(UserService.updateUser, {
		onSuccess: () => {
			onSuccess();
			UserService.getUsers.revalidate();
		},
	});

	const handleSubmit = form.handleSubmit((data) =>
		updateUser.execute(user.username, data)
	);

	const roleDescription = user.explicitAdmin
		? ExplicitAdminDescription
		: descriptions[currentRole];

	return (
		<Form {...form}>
			<form id={formId} onSubmit={handleSubmit} className="flex flex-col gap-4">
				<ControlledCustomInput
					label="Role"
					description={roleDescription}
					control={form.control}
					name="role"
					classNames={{ description: "h-8" }}
				>
					{({ field, className }) => (
						<Select
							onValueChange={(value) => field.onChange(roleFromString(value))}
							value={field.value.toString()}
							disabled={user.explicitAdmin}
						>
							<SelectTrigger className={className}>
								<SelectValue placeholder="Select Role" />
							</SelectTrigger>

							<SelectContent>
								{Object.values(Role).map((role) => (
									<SelectItem key={role} value={role.toString()}>
										{roleLabel(role)}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					)}
				</ControlledCustomInput>

				<div className="flex justify-end gap-2">
					<Button variant="secondary" onClick={onCancel}>
						Cancel
					</Button>

					<Button
						type="submit"
						loading={updateUser.isLoading}
						disabled={updateUser.isLoading}
					>
						Update
					</Button>
				</div>
			</form>
		</Form>
	);
}
