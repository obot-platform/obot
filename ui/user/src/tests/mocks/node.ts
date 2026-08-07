import { handlers } from './handlers';
import { setupWorker } from 'msw/browser';

export const worker = setupWorker(...handlers);
