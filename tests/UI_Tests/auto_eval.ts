import fs from "fs";
import path from "path";
import { glob } from "glob";
import OpenAI from "openai";

// --- Evaluation Configuration ---
const EVAL_CONFIG = {
  grading_model: "gpt-4o-mini",
  prompt: `
You are an expert evaluator for AI chat systems.
Given the user's prompt and the assistant's response,
evaluate whether the response correctly, clearly, and completely addresses the user's prompt.

- If the assistant's response fulfills the user request or explains failure clearly, respond with a JSON object:
  {
    "result": "SUCCESS",
    "task_done": true
  }

- If the assistant's response indicates the task was NOT done, but explains why, respond with:
  {
    "result": "Response given, but Task Failed",
    "task_done": false,
    "reason": "explanation why task was not done"
  }

- If the response is incorrect, incomplete, irrelevant, or contradictory, respond with:
  {
    "result": "FAILURE"
  }

Output strictly a single JSON object as shown.
`,
  input_key: "prompt",
  output_key: "response",
};

// --- Types ---
interface GradeInfo {
  result: string;
  task_done?: boolean;
  reason?: string;
}

interface ToolData {
  responses?: string[];
  task_done?: boolean | null;
  failure_reason?: string[];
  status?: string;
  [key: string]: any;
}

interface PromptData {
  promptText: string;
  tools: Record<string, ToolData>;
}

type ReportData = Record<string, PromptData>;

// --- Helper Functions ---
function createGradingPrompt(
  template: string,
  userPrompt: string,
  assistantResponse: string
): string {
  return template
    .replace("the user's prompt", `"${userPrompt}"`)
    .replace("the assistant's response", `"${assistantResponse}"`);
}

async function gradeResponse(
  client: OpenAI,
  model: string,
  prompt: string
): Promise<GradeInfo> {
  try {
    const response = await client.chat.completions.create({
      model,
      messages: [{ role: "system", content: prompt }],
    });

    const content = response.choices[0]?.message?.content?.trim() || "";

    try {
      return JSON.parse(content);
    } catch {
      return { result: "FAILURE", reason: `Invalid JSON response: ${content}` };
    }
  } catch (err: any) {
    return { result: "FAILURE", reason: `API error: ${err.message}` };
  }
}

async function enhanceReportWithEval(
  originalData: ReportData,
  client: OpenAI,
  model: string,
  promptTemplate: string,
  inputKey: string,
  outputKey: string
): Promise<ReportData> {
  for (const [promptId, promptData] of Object.entries(originalData)) {
    const promptText = promptData.promptText;

    for (const [toolName, toolData] of Object.entries(promptData.tools)) {
      const assistantResponse = (toolData.responses || []).join(" ");
      const gradingPrompt = createGradingPrompt(
        promptTemplate,
        promptText,
        assistantResponse
      );

      const gradeInfo = await gradeResponse(client, model, gradingPrompt);

      toolData.task_done = gradeInfo.task_done ?? null;

      // Merge reasons
      const reasons: string[] = [];
      if (gradeInfo.reason) reasons.push(gradeInfo.reason);
      toolData.failure_reason = reasons;

      // Set status based on grading
      if (gradeInfo.result === "FAILURE") toolData.status = "Failure";
      else if (gradeInfo.task_done === true) toolData.status = "Success";
      else if (gradeInfo.task_done === false) toolData.status = "Failure";
    }
  }
  return originalData;
}

// --- Main Execution ---
async function main(): Promise<void> {
  const reportsFolder = path.join(__dirname, "MCP Server Reports");
  const jsonFiles = fs
  .readdirSync(reportsFolder)
  .filter((file) => file.endsWith(".json"))
  .map((file) => path.join(reportsFolder, file));
  console.log(jsonFiles)

  console.log(`Found ${jsonFiles.length} report files to process...`);

  const apiKey = process.env.OPENAI_API_KEY;
  if (!apiKey) throw new Error("OPENAI_API_KEY is not set.");

  const client = new OpenAI({ apiKey });

  const { grading_model, prompt, input_key, output_key } = EVAL_CONFIG;

  for (const jsonPath of jsonFiles) {
    console.log(`\nEnhancing ${path.basename(jsonPath)} ...`);

    const fileData = fs.readFileSync(jsonPath, "utf-8");
    const originalData: ReportData = JSON.parse(fileData);

    const enhancedReport = await enhanceReportWithEval(
      originalData,
      client,
      grading_model,
      prompt,
      input_key,
      output_key
    );

    fs.writeFileSync(jsonPath, JSON.stringify(enhancedReport, null, 2), "utf-8");

    console.log(
      `Overwritten original report file with enhanced data: ${path.basename(jsonPath)}`
    );
  }

  console.log("\nAll reports processed and enhanced successfully.");
}

if (require.main === module) {
  main().catch((err) => {
    console.error("❌ Error:", err);
    process.exit(1);
  });
}
