import fs from 'fs';
import path from 'path';
import axios from 'axios';
import { LoadContext, Plugin } from '@docusaurus/types';

// Array of permalinks to raw files in different repositories
const FILE_URLS = [
  'https://raw.githubusercontent.com/otto8-ai/python-hash-tool/main/README.md',
  'https://raw.githubusercontent.com/otto8-ai/go-hash-tool/main/README.md',
  'https://raw.githubusercontent.com/otto8-ai/node-hash-tool/main/README.md',
];

// Mapping of file extensions to code block languages for syntax highlighting
const EXTENSION_LANGUAGE_MAP: Record<string, string> = {
  '.py': 'python',
  '.go': 'go',
  '.mod': 'go',
  '.ts': 'typescript',
  '.js': 'javascript',
  '.json': 'json',
  '.yaml': 'yaml',
  '.yml': 'yaml',
  '.gpt': 'yaml',
  '.md': 'markdown',
  '.txt': 'text',
};

async function fetchFiles(outputDir: string) {
  if (!fs.existsSync(outputDir)) {
    fs.mkdirSync(outputDir, { recursive: true });
  }

  await Promise.all(
    FILE_URLS.map(async (url) => {
      try {
        const { data } = await axios.get(url);

        // Extract the repository name and file path from the URL
        const match = url.match(/githubusercontent\.com\/([^/]+)\/([^/]+)\/[^/]+\/(.+)$/);
        if (!match) throw new Error(`Invalid URL format: ${url}`);

        const repoName = match[2].toLowerCase();
        const filePath = match[3].toLowerCase().replace(/\//g, '-');

        // Get the file extension and corresponding language for syntax highlighting
        const ext = path.extname(filePath);
        const language = EXTENSION_LANGUAGE_MAP[ext] || '';  // Default to plain text if extension is unknown

        // Wrap content in a Markdown code block for supported file types
        let wrappedContent = data;
        if (language != 'markdown') {
          wrappedContent = language ? `\`\`\`${language}\n${data}\n\`\`\`` : data;
        }

        const outputFilePath = path.join(outputDir, `${repoName}-${filePath}.mdx`);
        fs.writeFileSync(outputFilePath, wrappedContent);
        console.log(`Fetched and saved ${outputFilePath}`);
      } catch (error) {
        console.error(`Failed to fetch file from ${url}:`, error);
      }
    })
  );
}

export default function pluginFetchSnippets(context: LoadContext): Plugin {
  return {
    name: 'fetch-snippets',
    async loadContent() {
      const outputDir = path.join(__dirname, '../../../snippets');
      await fetchFiles(outputDir);
    }
  };
}
