import '../app.css';
import { worker } from './mocks/node';
import 'devicon/devicon.min.css';
import { beforeAll, afterEach, afterAll } from 'vitest';

beforeAll(async () => {
	await worker.start();
});

afterEach(async () => {
	await worker.resetHandlers();
});

afterAll(async () => {
	await worker.stop();
});
