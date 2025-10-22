import Selectors from './selectors';
import fs from 'fs';
import path from 'path';
import { clickToElement, element } from './func';
import { ELEMENT_TIMEOUT } from './timeouts';


export async function sendPromptAndWaitForReply(promptText: string) {
  const prevReplies = await $$(Selectors.MCP.lastBotReply);
  let lastTextBefore = "";
  if (prevReplies.length > 0) {
    lastTextBefore = await prevReplies[prevReplies.length - 1].getText();
  }

  // Focus input and send message
  const input = await element(Selectors.MCP.obotInput);
  await input.waitForDisplayed({ timeout: ELEMENT_TIMEOUT });
  await clickToElement(Selectors.MCP.obotInput);
  await $(Selectors.MCP.obotInput).setValue(promptText);
  await clickToElement(Selectors.MCP.submitPrompt);

  // Wait for a new reply to appear
  await browser.waitUntil(async () => {
    const botReplyElements = await $$(Selectors.MCP.lastBotReply);
    if (botReplyElements.length === 0) return false;
    const lastText = await botReplyElements[botReplyElements.length - 1].getText();
    if (prevReplies.length === 0) return lastText.length > 0;
    return lastText && lastText !== lastTextBefore;
  }, {
    timeout: 30000,
    interval: 1000,
    timeoutMsg: `No reply detected for prompt: "${promptText}"`,
  });

  // Wait for stabilization
  let lastText = "";
  let stableCount = 0;
  await browser.waitUntil(async () => {
    const botReplyElements = await $$(Selectors.MCP.lastBotReply);
    if (botReplyElements.length === 0) return false;
    const newestText = await botReplyElements[botReplyElements.length - 1].getText();
    if (newestText === lastText) stableCount++;
    else {
      stableCount = 0;
      lastText = newestText;
    }
    return stableCount >= 3 && newestText.length > 0;
  }, {
    timeout: 60000,
    interval: 1000,
    timeoutMsg: `Bot response not stabilized for: "${promptText}"`,
  });

  // Return the stabilized latest reply text
  const botReplyElements = await $$(Selectors.MCP.lastBotReply);
  return botReplyElements.length > 0
    ? await botReplyElements[botReplyElements.length - 1].getText()
    : "";
}

export async function sendPromptValidateAndCollect(promptText: string, toolList: string[], index: number) {
  // Count current replies before sending
  const beforeCount = (await $$('//div[@class="message-content"]')).length;
  
  // Count tools visible before sending the prompt
  const toolsBeforeElements = await $$('//div[@class="flex flex-col"]/div[@class="mb-1 flex items-center space-x-2"]/span[1]');
  const toolsBeforeCount = toolsBeforeElements.length;

  // Send and wait for reply
  const reply = await sendPromptAndWaitForReply(promptText);
  await browser.pause(2000);

  // Wait until a new message-content div appears
  await browser.waitUntil(async () => {
    const afterCount = (await $$('//div[@class="message-content"]')).length;
    return afterCount > beforeCount;
  }, {
    timeout: 60000,
    interval: 1000,
    timeoutMsg: `No new message-content appeared for prompt: "${promptText}"`
  });

  // Count tools visible after the prompt response
  const toolsAfterElements = await $$('//div[@class="flex flex-col"]/div[@class="mb-1 flex items-center space-x-2"]/span[1]');
  const toolsAfterCount = toolsAfterElements.length;

  // Calculate new tools added (slice by index difference)
  const newToolsElements = toolsAfterElements.slice(toolsBeforeCount, toolsAfterCount);
  const toolsTexts: string[] = [];
  for (const el of newToolsElements) {
    const rawText = await el.getText();
    const match = rawText.match(/->\s*(.*)$/);
    toolsTexts.push(match ? match[1].trim() : rawText.trim());
  }

  // Get latest message element (reply)
  const promptReplies = await $$('//div[@class="message-content"]');
  const currReply = promptReplies[promptReplies.length - 1];
  if (!currReply) throw new Error(`No reply container found even after waiting for prompt: "${promptText}"`);

  // Validation regex
  const successRegex = /(success|completed|connected|created|retrieved|posted|updated|closed|deleted|functioning|valid|available|ready to use)/i;
  const failureRegex = /(not valid|failed|error|cannot access|do not have|insufficient|not available|required|troubleshooting)/i;

  const hasSuccess = successRegex.test(reply);
  const hasFailure = failureRegex.test(reply);

  let errorMessage = '';
  if (!hasSuccess && !hasFailure) {
    errorMessage = `No success or actionable failure detected in prompt #${index + 1} response.`;
  }

  console.log(`Prompt #${index + 1}: Tools used: ${toolsTexts.length ? toolsTexts.join(', ') : 'None'} | Status: ${hasSuccess ? 'Success' : (hasFailure ? 'Failure' : 'Unknown')}`);

  // Return data for reporting
  return {
    prompt: promptText,
    reply,
    replyElement: currReply,
    tools: toolsTexts,
    status: hasSuccess ? 'Success' : (hasFailure ? 'Failure' : 'Unknown'),
    error: errorMessage || null,
  };
}

function maxStatus(s1: string, s2: string): string {
  const priority = { 'Failure': 3, 'Success': 2, 'Unknown': 1 };
  return (priority[s1] || 0) > (priority[s2] || 0) ? s1 : s2;
}

export function aggregateToolResponses(promptResults: any[]) {
  const report: Record<string, {
    promptText: string,
    tools: Record<string, { responses: string[]; status: string; errors: string[] }>
  }> = {};

  for (let i = 0; i < promptResults.length; i++) {
    const result = promptResults[i];
    const { prompt, tools, reply, status, error } = result;
    if (!reply && !error) continue;

    const promptKey = `Prompt #${i + 1}`;

    if (!report[promptKey]) {
      report[promptKey] = {
        promptText: prompt,
        tools: {}
      };
    }

    const toolsToUse = tools.length > 0 ? tools : ['NO_TOOL'];

    for (const tool of toolsToUse) {
      if (!report[promptKey].tools[tool]) {
        report[promptKey].tools[tool] = { responses: [], status: 'Unknown', errors: [] };
      }

      if (reply) report[promptKey].tools[tool].responses.push(reply);
      if (error) report[promptKey].tools[tool].errors.push(error);

      report[promptKey].tools[tool].status =
        maxStatus(status, report[promptKey].tools[tool].status);
    }
  }

  return report;
}

export function saveMCPReport(mcpName: string, reportJson: any) {
  const folderName = `MCP Server Reports`;
  const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
  const fileName = `${mcpName}_MCP_Report_${timestamp}.json`;
  const dirPath = path.join(process.cwd(), folderName);
  const filePath = path.join(dirPath, fileName);

  if (!fs.existsSync(dirPath)) {
    fs.mkdirSync(dirPath, { recursive: true });
  }

  fs.writeFileSync(filePath, JSON.stringify(reportJson, null, 2), 'utf-8');
  console.log(`MCP report saved: ${filePath}`);
}