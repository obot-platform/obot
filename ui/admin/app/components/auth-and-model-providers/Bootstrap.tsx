import React, { useState } from "react";

import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { cn } from "~/lib/utils";

import { Button } from "~/components/ui/button";

interface BootstrapProps {
	className?: string;
}

export function Bootstrap({ className }: BootstrapProps) {
	const [token, setToken] = useState("");
	const [error, setError] = useState("");

	const handleSubmit = async (event: React.FormEvent) => {
		event.preventDefault();
		try {
			const result = await fetch(ApiRoutes.bootstrap.login().url, {
				method: "POST",
				headers: {
					Authorization: `Bearer ${token}`,
				},
			});

			if (result.status === 401) {
				setError("Invalid token");
				return;
			} else if (result.status !== 200) {
				setError("Failed to login: " + result.statusText);
				return;
			}

			setError("");
			window.location.href = "/admin/auth-providers";
		} catch (e) {
			setError("Failed to login: " + e);
		}
	};

	return (
		<form
			onSubmit={handleSubmit}
			className={cn("flex flex-col space-y-4", className)}
		>
			<h4>Enter Bootstrap Token</h4>
			<input
				type="password"
				value={token}
				onChange={(e) => setToken(e.target.value)}
				placeholder="token"
				className="rounded border p-2"
				required
			/>
			<Button type="submit" className="w-full">
				Submit
			</Button>
			{error && <small className="text-red-500">{error}</small>}
		</form>
	);
}
