import { faker } from '@faker-js/faker';

/**
 * Generate a random project name
 * Format: Automation-Obot-MCP-XXXXXX
 */
export function generateProjectName(): string {
  const randomNumber = faker.number.int({ min: 100000, max: 999999 });
  return `Automation-Obot-MCP-${randomNumber}`;
}

console.log(generateProjectName());