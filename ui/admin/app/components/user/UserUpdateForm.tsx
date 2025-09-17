import { HandshakeIcon } from "lucide-react";
import { useEffect, useState } from "react";
import { useForm } from "react-hook-form";

import { BootstrapUsername } from "~/lib/model/auth";
import {
	ExplicitRoleDescription,
	Role,
	User,
	roleFromString,
	roleLabel,
} from "~/lib/model/users";
import { BootstrapApiService } from "~/lib/service/api/bootstrapApiService";
import { UserService } from "~/lib/service/api/userService";

import { useAuth } from "~/components/auth/AuthContext";
import { ControlledCustomInput } from "~/components/form/controlledInputs";
import { Button } from "~/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "~/components/ui/dialog";
import { Form } from "~/components/ui/form";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "~/components/ui/select";
import { useAuthStatus } from "~/hooks/auth/useAuthStatus";
import { useAsync } from "~/hooks/useAsync";

const descriptions = {
	[Role.Owner]: "Owners can manage all aspects of the platform.",
	[Role.Admin]: "Admins can manage all aspects of the platform.",
	[Role.Basic]:
		"Users are restricted to only interacting with agents shared with them. They cannot access the Admin UI",
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
	const { me } = useAuth();
	const { authEnabled } = useAuthStatus();

	const [bootstrapDialogOpen, setBootstrapDialogOpen] = useState(false);
	const [shouldRedirectLogout, setShouldRedirectLogout] = useState(false);

	const form = useForm({ defaultValues: { role: user.role } });
	const [currentRole] = form.watch(["role"]);

	const updateUser = useAsync(UserService.updateUser, {
		onSuccess: async () => {
			if (
				me.username === BootstrapUsername &&
				authEnabled &&
				currentRole === Role.Admin
			) {
				await BootstrapApiService.bootstrapLogout();
				setShouldRedirectLogout(true);
			}

			onSuccess();
			UserService.getUsers.revalidate();
		},
	});

	useEffect(() => {
		if (shouldRedirectLogout) {
			window.location.href = "/oauth2/sign_out?rd=/legacy-admin/";
		}
	}, [shouldRedirectLogout]);

	const handleSubmit = form.handleSubmit((data) => {
		if (
			me.username === BootstrapUsername &&
			authEnabled &&
			data.role === Role.Admin
		) {
			setBootstrapDialogOpen(true);
		} else {
			updateUser.execute(user.id, data);
		}
	});

	const roleDescription = user.explicitRole
		? ExplicitRoleDescription
		: descriptions[currentRole];

	return (
		<>
			<Form {...form}>
				<form
					id={formId}
					onSubmit={handleSubmit}
					className="flex flex-col gap-4"
				>
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
								disabled={user.explicitRole}
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

			<Dialog open={bootstrapDialogOpen} onOpenChange={setBootstrapDialogOpen}>
				<DialogContent>
					<DialogHeader>
						<DialogTitle className="flex items-center gap-2">
							<HandshakeIcon /> Confirm Admin Handoff
						</DialogTitle>
					</DialogHeader>

					<DialogDescription className="flex flex-col gap-4">
						<p>
							Once you&apos;ve established your first admin user, the bootstrap
							user currently being used will be disabled. Upon completing this
							action, you&apos;ll be logged out and asked to log in using your
							auth provider.
						</p>
						<p>Are you sure you want to continue?</p>
					</DialogDescription>

					<DialogFooter>
						<Button variant="secondary" onClick={onCancel}>
							Cancel
						</Button>
						<Button
							loading={updateUser.isLoading}
							disabled={updateUser.isLoading}
							onClick={() => updateUser.execute(user.id, form.getValues())}
						>
							Confirm
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</>
	);
}
