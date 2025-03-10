import { Task } from "~/lib/model/tasks";

export const mockedTask: Task = {
	id: "w1dshmx",
	created: "2025-01-30T17:03:06-05:00",
	links: {
		invoke: "http://localhost:8080/api/invoke/w1dshmx",
	},
	type: "workflow",
	name: "Giving Turmeric",
	description: "",
	steps: [],
	schedule: {
		interval: "daily",
		hour: 0,
		minute: 0,
		day: 1,
		weekday: 0,
	},
	webhook: null,
	email: null,
	onDemand: null,
	alias: "giving-turmeric",
	projectID: "123",
};
