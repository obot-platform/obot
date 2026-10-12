import { test, expect } from '@playwright/test';

test('documented task entry points match the Docker UI', async ({ page }) => {
  await page.goto('/skills?view=sources');
  await page.getByRole('button', { name: 'Add Source URL', exact: true }).click();
  await expect(page.getByLabel('Source URL', { exact: true })).toBeVisible();
  await expect(page.getByLabel('Reference', { exact: true })).toBeVisible();

  await page.goto('/skills?view=access-policies');
  await page.getByRole('button', { name: 'Add Access Policy', exact: true }).first().click();
  await expect(page.getByRole('button', { name: 'Add Skill', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Save', exact: true })).toBeVisible();

  await page.goto('/mcp-servers?view=access-policies');
  await page.getByRole('button', { name: 'Add Access Policy', exact: true }).first().click();
  await expect(page.getByRole('button', { name: 'Add Server', exact: true })).toBeVisible();

  await page.goto('/mcp-servers?view=tunnels');
  await page.getByRole('navigation').getByRole('button', { name: 'Create MCP Tunnel', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Create Tunnel', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Allowed URL', exact: true })).toBeVisible();

  await page.goto('/identity-access?view=agents');
  await page.getByRole('button', { name: 'Create Agent Identity', exact: true }).click();
  await expect(page.getByLabel('Name', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Save', exact: true })).toBeVisible();

  await page.goto('/identity-access?view=auth-providers');
  await page.getByRole('button', { name: 'Auth Providers', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Configure', exact: true }).first()).toBeVisible();
  await page.goto('/audit-logs');
  await expect(page.getByRole('button', { name: 'MCP', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Model', exact: true })).toBeVisible();
});

test('governed tool discovery, call, and audit in Docker', async ({ page }, testInfo) => {
  await page.goto(`/vmcps/${process.env.OBOT_TEST_VMCP_ID}`);
  await page.getByRole('button', { name: 'Inspector', exact: true }).click();
  const tools = page.getByRole('button', { name: /^echo Return supplied text/ });
  const start = page.getByRole('button', { name: 'Start Session', exact: true });
  await expect(tools.or(start)).toBeVisible();
  if (await start.isVisible()) {
    await start.click();
    await page.getByRole('button', { name: 'Continue', exact: true }).click();
  }
  await expect(tools).toBeVisible();
  await expect(page.getByRole('button', { name: /^private_echo / })).toHaveCount(0);
  await tools.click();
  await page.locator('input[type="text"]:visible').fill('Documentation verification succeeded');
  const calledAt = Date.now();
  await page.getByRole('button', { name: 'Call', exact: true }).click();
  await expect(page.getByText('Succeeded', { exact: true })).toBeVisible();
  await expect(page.getByText('Documentation verification succeeded', { exact: true })).toBeVisible();
  await testInfo.attach('successful-tool-call', { body: await page.screenshot(), contentType: 'image/png' });

  await page.goto('/audit-logs');
  const row = page.getByRole('row').filter({ hasText: 'Docs managed tools' }).filter({ hasText: 'echo' }).first();
  await expect(async () => {
    await page.reload();
    await expect(row).toContainText(/success/i, { timeout: 2_000 });
    await expect(row).toContainText('tools/call');
    const timestamp = (await row.innerText()).match(/\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2} UTC/);
    expect(timestamp).not.toBeNull();
    expect(Date.parse(timestamp![0])).toBeGreaterThanOrEqual(calledAt - 1_000);
  }).toPass({ timeout: 30_000, intervals: [2_000] });
  await testInfo.attach('audit-log', { body: await page.screenshot(), contentType: 'image/png' });
});
