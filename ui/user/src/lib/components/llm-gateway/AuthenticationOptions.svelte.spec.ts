import { renderOpenDialog } from '../../../tests/helpers/openDialog';
import AddProviderDialog from './AddProviderDialog.svelte';
import ProviderKeyDialog from './ProviderKeyDialog.svelte';
import { describe, expect, it } from 'vitest';

describe('authentication options', () => {
	for (const provider of ['bedrock', 'azure']) {
		const aws = provider === 'bedrock';
		for (const addingProvider of [true, false]) {
			it(`${provider}: offers only API key and explicit credentials when adding ${addingProvider ? 'a provider' : 'a key'}`, async () => {
				const dialog = addingProvider
					? await renderOpenDialog(AddProviderDialog, { configured: [], existingKeyNames: [] })
					: await renderOpenDialog(ProviderKeyDialog, { provider });
				if (addingProvider)
					await dialog
						.getByRole('button', { name: aws ? /^AWS Bedrock/ : /^Azure OpenAI/ })
						.click();
				await expect
					.element(dialog.getByRole('radio', { name: 'Server identity', exact: true }))
					.not.toBeInTheDocument();
				await expect
					.element(dialog.getByRole('radio', { name: 'API Key', exact: true }))
					.toBeChecked();
				await dialog
					.getByRole('radio', { name: aws ? 'AWS credentials' : 'Microsoft Entra ID', exact: true })
					.click();
				await expect
					.element(dialog.getByLabelText(aws ? /^Access Key ID/ : /^Entra Client ID/))
					.toBeInTheDocument();
			});
		}
	}
});
