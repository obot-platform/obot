import { test, expect, type Locator } from '@playwright/test';
import versions from '../versions.json';

test('current documentation navigation and direct loads', async ({ page }) => {
  test.setTimeout(240_000);
  await page.goto('/next/');
  await expect(page.locator('article').first()).toBeVisible();
  const discovered = new Set<string>();
  async function visit(item: Locator) {
    const toggle = item.locator(':scope > .menu__list-item-collapsible > button, :scope > .menu__list-item-collapsible > a[role=button]');
    if (await toggle.count() && (await item.getAttribute('class'))?.includes('menu__list-item--collapsed')) await toggle.click();
    for (const href of await item.locator('a[href]').evaluateAll(elements => elements.map(el => el.getAttribute('href')!))) {
      if (href !== '#') discovered.add(href);
    }
    const children = item.locator(':scope > ul > li');
    for (let i = 0; i < await children.count(); i++) await visit(children.nth(i));
  }
  const categories = page.locator('.theme-doc-sidebar-menu > li');
  await expect(page.locator('.theme-doc-sidebar-menu > li > .menu__list-item-collapsible > a')).toHaveText([
    'Start Here', 'Architecture', 'MCP Gateway', 'LLM Gateway', 'Agents',
    'Registries & Skills', 'Device Management (Sentry)', 'Security & Governance', 'Deploy & Operate', 'Reference',
  ]);
  for (let i = 0; i < await categories.count(); i++) await visit(categories.nth(i));
  // Cloud deployments is the only approved subgroup. Other references stay
  // in page content rather than additional catch-all navigation groups.
  await expect(page.locator('.theme-doc-sidebar-menu .menu__list-item-collapsible')).toHaveCount(11);
  const cloud = page.locator('.theme-doc-sidebar-menu > li > ul > li').filter({
    has: page.locator(':scope > .menu__list-item-collapsible > a', { hasText: 'Cloud deployments' }),
  });
  await expect(cloud).toHaveCount(1);
  await expect(cloud.locator(':scope > ul > li > a')).toHaveText(['Amazon EKS', 'Azure AKS', 'Google GKE']);
  const links = [...discovered];
  expect(links.length).toBeGreaterThan(0);
  for (const href of links) {
    expect(href).toMatch(/^\/next\//);
    const response = await page.goto(href);
    expect(response?.status(), href).toBe(200);
    await expect(page.locator('article').first(), href).toBeVisible();
  }
  await page.goto('/next/');
  const link = page.locator('.theme-doc-sidebar-menu a[href]').filter({ hasText: 'Server configuration' }).first();
  if (await link.count()) {
    // Exercise React navigation as well as direct HTTP requests.
    await link.evaluate((el: HTMLElement) => el.click());
    await expect(page).toHaveURL(/server-configuration/);
    await expect(page.locator('article').first()).toBeVisible();
    await page.reload();
    await expect(page.locator('article').first()).toBeVisible();
  }
});

test('bookmarked guide sections lead directly to their relocated procedures', async ({ page }) => {
  for (const [source, anchor, target] of [
    ['functionality/virtual-mcps', 'tools-and-profiles', 'mcp-gateway/access'],
    ['functionality/mcp-servers', 'npx-nodetypescript-based-mcp-servers', 'concepts/mcp-hosting'],
    ['functionality/skills', 'adding-a-source', 'registries/publish-skills'],
    ['installation/enabling-authentication', 'step-1-set-environment-variables', 'configuration/auth-providers'],
    ['functionality/audit-logs-and-usage', 'retention', 'security/audit-data'],
    ['installation/cli-setup', 'basic-usage', 'reference/cli-api'],
  ]) {
    await page.goto(`/next/${source}/#${anchor}`);
    const heading = page.locator(`[id="${anchor}"]`);
    await expect(heading).toBeVisible();
    await expect(page.locator('.theme-doc-sidebar-menu')).toBeVisible();
    await heading.locator('xpath=following-sibling::p[1]/a').click();
    expect(new URL(page.url()).pathname).toBe(`/next/${target}/`);
    const fragment = decodeURIComponent(new URL(page.url()).hash.slice(1));
    expect(fragment).not.toBe('');
    await expect(page.locator(`[id="${fragment}"]`)).toBeVisible();
  }
});

test('released versions remain available', async ({ page }) => {
  const routes = ['/', ...versions.slice(1).map(version => `/${version}/`)];
  for (const route of routes) {
    const response = await page.goto(route);
    expect(response?.status()).toBe(200);
    await expect(page.locator('article').first()).toBeVisible();
    expect(new URL(page.url()).pathname).toBe(route);
  }
  await page.goto('/next/');
  await page.locator('.navbar .dropdown').first().hover();
  await page.locator('.navbar .dropdown__menu a').filter({ hasText: versions[0] }).click();
  await expect(page).toHaveURL(/\/$/);
  expect(new URL(page.url()).pathname).toBe('/');
});

test('category titles navigate and separate controls expand without navigating', async ({ page }) => {
  const destinations = [
    ['Start Here', '/next/start-here/choose/'],
    ['Architecture', '/next/concepts/architecture/'],
    ['MCP Gateway', '/next/concepts/mcp-gateway/'],
    ['LLM Gateway', '/next/llm-gateway/how-it-works/'],
    ['Agents', '/next/agents/availability/'],
    ['Registries & Skills', '/next/registries/overview/'],
    ['Device Management (Sentry)', '/next/device-management/how-sentry-works/'],
    ['Security & Governance', '/next/security/model/'],
    ['Deploy & Operate', '/next/installation/overview/'],
    ['Reference', '/next/reference/'],
  ];
  for (const [label, route] of destinations) {
    await page.goto('/next/start-here/choose/');
    const category = page.locator('.theme-doc-sidebar-menu > li').filter({
      has: page.getByRole('link', { name: label, exact: true }),
    });
    await category.locator(':scope > .menu__list-item-collapsible > button').click();
    expect(new URL(page.url()).pathname).toBe('/next/start-here/choose/');
    const title = category.locator(':scope > .menu__list-item-collapsible > a');
    await expect(title).toHaveAttribute('href', route);
    await title.click();
    expect(new URL(page.url()).pathname).toBe(route);
    await expect(page.locator('article').first()).toBeVisible();
  }
});
