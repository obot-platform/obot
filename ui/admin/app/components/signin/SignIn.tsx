import { useState } from "react";

import { cn } from "~/lib/utils";

import { BootstrapForm } from "~/components/auth-and-model-providers/BootstrapForm";
import { ProviderIcon } from "~/components/auth-and-model-providers/ProviderIcon";
import { ObotLogo } from "~/components/branding/ObotLogo";
import { Button } from "~/components/ui/button";
import {
	Card,
	CardDescription,
	CardHeader,
	CardTitle,
} from "~/components/ui/card";
import { useAuthProviders } from "~/hooks/auth-providers/useAuthProviders";
import { useAuthStatus } from "~/hooks/auth/useAuthStatus";

interface SignInProps {
	className?: string;
}

export function SignIn({ className }: SignInProps) {
	const { authProviders, isLoading } = useAuthProviders();
	const configuredAuthProviders = authProviders.filter((p) => p.configured);

	const [bootstrapSelected, setBootstrapSelected] = useState(false);

	const { bootstrapEnabled } = useAuthStatus();

	if (isLoading) {
		return null;
	}

	const showBootstrapForm =
		(bootstrapEnabled && bootstrapSelected) || !configuredAuthProviders.length;

	return (
		<div className="flex min-h-screen w-full items-center justify-center p-4">
			<Card
				className={cn(
					"flex max-w-96 flex-col justify-between px-8 pb-4",
					className
				)}
			>
				<CardHeader>
					<CardTitle className="flex items-center justify-center">
						<ObotLogo />
					</CardTitle>
					{configuredAuthProviders.length > 0 && (
						<CardDescription className="mx-auto w-3/4 pt-4 text-center">
							Please sign in using an option below.
						</CardDescription>
					)}
				</CardHeader>

				{configuredAuthProviders.map((provider) => (
					<Button
						key={provider.id}
						variant="secondary"
						className="mb-4 w-full"
						onClick={() => {
							window.location.href = `/oauth2/start?rd=/admin/&obot-auth-provider=default/${provider.id}`;
						}}
					>
						<ProviderIcon provider={provider} size="md" />
						Sign in with {provider.name}
					</Button>
				))}

				{bootstrapEnabled && !showBootstrapForm && (
					<Button
						variant="secondary"
						className="mb-4 w-full"
						onClick={() => {
							setBootstrapSelected(true);
						}}
					>
						Sign in with Bootstrap Token
					</Button>
				)}

				{showBootstrapForm && <BootstrapForm />}
			</Card>
		</div>
	);
}
