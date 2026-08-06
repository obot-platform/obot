import '../app.css';
import { worker } from './mocks/node';
import 'devicon/devicon.min.css';
import { beforeAll, beforeEach, afterEach, afterAll } from 'vitest';

beforeAll(async () => {
	await worker.start();
});

beforeEach(() => {
	localStorage.clear();
});

afterEach(async () => {
	await worker.resetHandlers();
});

afterAll(async () => {
	await worker.stop();
});
