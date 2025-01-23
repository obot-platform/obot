import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { useLocation } from "react-router";
import { $path } from "safe-routes";
import { z } from "zod";

import { BaseUrl } from "~/lib/routers/baseRouter";
import { BootstrapApiService } from "~/lib/service/api/bootstrapApiService";
import { cn } from "~/lib/utils";

import { Description } from "~/components/composed/typography";
import { ControlledInput } from "~/components/form/controlledInputs";
import { Button } from "~/components/ui/button";
import { Form } from "~/components/ui/form";
import { useAsync } from "~/hooks/useAsync";

interface BootstrapProps {
	className?: string;
}

const formSchema = z.object({
	token: z.string().min(1),
});

function handleNavigate() {
	window.location.href = BaseUrl($path("/auth-providers"));
}

export function BootstrapForm({ className }: BootstrapProps) {
	const location = useLocation();

	useEffect(() => {}, [location.key]);

	const login = useAsync(BootstrapApiService.bootstrapLogin, {
		shouldThrow: () => false,
	});

	const form = useForm<z.infer<typeof formSchema>>({
		resolver: zodResolver(formSchema),
		defaultValues: { token: "" },
	});

	const handleSubmit = form.handleSubmit(async ({ token }) => {
		const [error] = await login.executeAsync(token);

		if (error) {
			form.setError("token", { message: error.message });
			return;
		}

		handleNavigate();
	});

	return (
		<Form {...form}>
			<form
				onSubmit={handleSubmit}
				className={cn("flex flex-col gap-4", className)}
			>
				<h4>Authenticate with Bootstrap Token</h4>

				<Description>
					If this is your first time logging in, you will need to provide a
					bootstrap token.
				</Description>

				<ControlledInput
					control={form.control}
					name="token"
					label="Bootstrap Token"
					description="You can find the bootstrap token in the server logs when starting Obot by searching for 'Bootstrap Token', or configure it directly through environment variables at startup."
					type="password"
				/>

				<Button
					type="submit"
					className="w-full"
					loading={login.isLoading}
					disabled={login.isLoading}
				>
					Login
				</Button>
			</form>
		</Form>
	);
}
