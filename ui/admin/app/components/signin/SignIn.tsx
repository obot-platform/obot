import { cn } from "~/lib/utils";

import { Bootstrap } from "~/components/auth-and-model-providers/Bootstrap";
import { ProviderIcon } from "~/components/auth-and-model-providers/ProviderIcon";
import { CommonAuthProviderFriendlyNames } from "~/components/auth-and-model-providers/constants";
import { ObotLogo } from "~/components/branding/ObotLogo";
import { Button } from "~/components/ui/button";
import {
	Card,
	CardDescription,
	CardFooter,
	CardHeader,
	CardTitle,
} from "~/components/ui/card";
import { useAuthProviders } from "~/hooks/auth-providers/useAuthProviders";

interface SignInProps {
	className?: string;
}

export function SignIn({ className }: SignInProps) {
	const { authProviders } = useAuthProviders();
	const configuredAuthProviders = authProviders.filter((p) => p.configured);

	return (
		<div className="flex min-h-screen w-full items-center justify-center p-4">
			<Card className={cn("flex w-96 flex-col justify-between", className)}>
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
				<CardFooter className="flex flex-col border-t pt-4">
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
							Sign in with {CommonAuthProviderFriendlyNames[provider.id]}
						</Button>
					))}
					{configuredAuthProviders.length === 0 && <Bootstrap />}
				</CardFooter>
			</Card>
		</div>
	);
}
