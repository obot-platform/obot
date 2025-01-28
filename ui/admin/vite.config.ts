import { reactRouter } from "@react-router/dev/vite";
import { safeRoutes } from "safe-routes/vite";
import { defineConfig } from "vite";
import tsconfigPaths from "vite-tsconfig-paths";

export default defineConfig({
	plugins: [
		!process.env.VITEST && reactRouter(),
		tsconfigPaths(),
		safeRoutes(),
	],
	base: "/admin/",
	server: {
		watch: {
			// Exclude test files from HMR
			ignored: !process.env.VITEST
				? ["**/__tests__/**", "**/test/**", "**/__mocks__/**"]
				: [],
		},
	},
	test: {
		globals: true,
		environment: "jsdom",
		setupFiles: "./test/setup.ts",
		css: true,
		watch: true,
	},
});
