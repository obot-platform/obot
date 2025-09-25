import { ReactNode, createContext, useContext } from "react";
import useSWR from "swr";

import { AuthDisabledUsername } from "~/lib/model/auth";
import { Role, User } from "~/lib/model/users";
import { UserService } from "~/lib/service/api/userService";

interface AuthContextType {
	me: User;
	isLoading: boolean;
	isSignedIn: boolean;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function AuthProvider({ children }: { children: ReactNode }) {
	const { data: me, isLoading } = useSWR(
		UserService.getMe.key(),
		() => UserService.getMe(),
		{ fallbackData: { role: Role.Basic } as User }
	);

	const isSignedIn = !!me.username && me.username !== AuthDisabledUsername;

	return (
		<AuthContext.Provider value={{ me, isLoading, isSignedIn }}>
			{children}
		</AuthContext.Provider>
	);
}

export function useAuth() {
	const context = useContext(AuthContext);
	if (context === undefined) {
		throw new Error("useAuth must be used within a AuthProvider");
	}
	return context;
}
