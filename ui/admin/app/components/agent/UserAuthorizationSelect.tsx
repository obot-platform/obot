import { UserIcon, UsersIcon, XIcon } from "lucide-react";
import { toast } from "sonner";
import useSWR, { mutate } from "swr";

import { AgentAuthorization } from "~/lib/model/agents";
import { User } from "~/lib/model/users";
import { AgentService } from "~/lib/service/api/agentService";

import { ComboBox } from "~/components/composed/ComboBox";
import { Avatar, AvatarFallback, AvatarImage } from "~/components/ui/avatar";
import { Button } from "~/components/ui/button";
import { Checkbox } from "~/components/ui/checkbox";
import { ScrollArea } from "~/components/ui/scroll-area";
import { Separator } from "~/components/ui/separator";
import { useAsync } from "~/hooks/useAsync";

type UserOption = {
	id: string;
	name: string;
	value: string;
};

export function UserAuthorizationSelect({
	users,
	agentId,
}: {
	users: User[];
	agentId: string;
}) {
	const { data: authorizations = [] } = useSWR(
		AgentService.getAgentAuthorizations.key(agentId),
		({ agentId }) => AgentService.getAgentAuthorizations(agentId)
	);

	const selectedUsers = new Set(authorizations.map((a) => a.userID));
	const allUsers = collateUsersOptions(users, authorizations);

	const addAuthorizationToAgent = useAsync(AgentService.addAgentAuthorization, {
		onSuccess: () => {
			mutate(AgentService.getAgentAuthorizations.key(agentId));
		},
		onError: () => toast.error("Failed to add user to agent."),
	});

	const removeAuthorizationFromAgent = useAsync(
		AgentService.removeAgentAuthorization,
		{
			onSuccess: () => {
				mutate(AgentService.getAgentAuthorizations.key(agentId));
			},
			onError: () => toast.error("Failed to remove user from agent."),
		}
	);

	const handleChange = (option: UserOption | null) => {
		if (!option) return;

		const checked = selectedUsers.has(option.value);
		if (checked) {
			removeAuthorizationFromAgent.executeAsync(agentId, option.value);
		} else {
			addAuthorizationToAgent.executeAsync(agentId, option.value);
		}
	};

	const handleCreate = (email: string) => {
		addAuthorizationToAgent.executeAsync(agentId, email);
	};

	const handleDelete = (userID: string) => {
		removeAuthorizationFromAgent.executeAsync(agentId, userID);
	};

	const validateCreate = (str: string) => {
		const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
		return emailRegex.test(str);
	};

	// add Everyone option if it's not already in the list
	const options = (
		allUsers.has("*")
			? Array.from(allUsers.values())
			: [
					{ id: "everyone", name: "Everyone", value: "*" },
					...Array.from(allUsers.values()),
				]
	).sort(sortByUserIdentifier);
	const sortedAuthorizations = authorizations.sort(sortByUserIdentifier);

	return (
		<div className="flex flex-col gap-2">
			<ComboBox
				allowCreate
				closeOnSelect={false}
				emptyLabel="No Users Found."
				placeholder="Select or Add Users..."
				onChange={handleChange}
				onCreate={handleCreate}
				options={options}
				renderOption={renderOption}
				validateCreate={validateCreate}
				value={null}
			/>
			<Separator />
			<ScrollArea className="max-h-[30vh]">
				<div className="flex flex-col gap-2">
					{sortedAuthorizations.map(renderAuthorizationRow)}
				</div>
			</ScrollArea>
		</div>
	);

	function renderOption(option: UserOption) {
		return (
			<span className="flex w-full items-center justify-between gap-2">
				<Checkbox checked={selectedUsers.has(option.value)} />
				{option.name}
			</span>
		);
	}

	function renderAuthorizationRow(authorization: AgentAuthorization) {
		return (
			<div key={authorization.userID}>
				<div className="flex w-full items-center justify-between gap-2 p-2">
					<div className="flex items-center gap-2">
						{authorization.userID === "*" ? (
							<UsersIcon className="h-4 w-4" />
						) : authorization.user ? (
							<Avatar className="h-4 w-4">
								<AvatarImage src={authorization.user.iconURL} />
								<AvatarFallback>
									<UserIcon />
								</AvatarFallback>
							</Avatar>
						) : (
							<UserIcon className="h-4 w-4" />
						)}
						<p className="text-sm">
							{authorization.userID === "*"
								? "Everyone"
								: (authorization.user?.email ?? authorization.userID)}
						</p>
					</div>
					<Button
						onClick={() => handleDelete(authorization.userID)}
						variant="ghost"
						className="px-2"
					>
						<XIcon className="h-4 w-4" />
					</Button>
				</div>
				<Separator />
			</div>
		);
	}
}

const sortByUserIdentifier = (
	a: { value?: string; userID?: string },
	b: { value?: string; userID?: string }
) => {
	const aId = a.value ?? a.userID ?? "";
	const bId = b.value ?? b.userID ?? "";

	if (aId === "*") return -1;
	if (bId === "*") return 1;
	return aId.localeCompare(bId);
};

const collateUsersOptions = (
	users: User[],
	authorizations: AgentAuthorization[]
) => {
	return new Map(
		[...authorizations, ...users].map((item) => {
			if ("userID" in item) {
				return [
					item.user?.email ?? item.userID,
					{
						id: item.userID,
						value: item.userID,
						name:
							item.userID === "*"
								? "Everyone"
								: (item.user?.email ?? item.userID),
					},
				];
			}

			return [
				item.email,
				{
					id: item.id,
					value: item.id,
					name: item.email,
				},
			];
		})
	);
};
