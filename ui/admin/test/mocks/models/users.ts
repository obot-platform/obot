import { User } from "~/lib/model/users";

export const mockedBootstrappedUser: User = {
	id: "1",
	created: "2025-02-04T16:08:24.074959-05:00",
	username: "bootstrap",
	role: 1,
	timezone: "America/New_York",
	email: "",
	iconURL: "",
	explicitAdmin: false,
};

// Admin User
export const mockedUser: User = {
	id: "1",
	created: "2025-01-28T13:11:39.243624-05:00",
	username: "107221547212253478536",
	role: 1,
	explicitAdmin: true,
	email: "testuser@acorn.io",
	iconURL: "https://mock.lh3.googleusercontent.com/a/user1-",
	timezone: "America/New_York",
};

// Regular User
export const mockedUser2: User = {
	id: "2",
	created: "2025-01-28T13:11:39.243624-05:00",
	username: "103221547202223478436",
	role: 10,
	explicitAdmin: false,
	email: "testuser2@acorn.io",
	iconURL: "https://mock.lh3.googleusercontent.com/a/user2-",
	timezone: "America/Los_Angeles",
};

export const mockedUsers: User[] = [mockedUser, mockedUser2];
