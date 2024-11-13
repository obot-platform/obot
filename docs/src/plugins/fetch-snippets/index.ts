import fs from 'fs';
import path from 'path';
import axios from 'axios';
import crypto from 'crypto';
import { LoadContext, Plugin } from '@docusaurus/types';

const FILE_URLS = [
  'https://raw.githubusercontent.com/otto8-ai/python-hash-tool/main/README.md',
  'https://raw.githubusercontent.com/otto8-ai/go-hash-tool/main/README.md',
  'https://raw.githubusercontent.com/otto8-ai/node-hash-tool/main/README.md',
];

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

        let wrappedContent = data;
        if (language !== 'markdown') {
          // Wrap content in a Markdown code block for supported file types
          wrappedContent = language ? `\`\`\`${language}\n${data}\n\`\`\`` : data;
        } else {
          // Add unique and explicit IDs to all headers in Markdown content
          wrappedContent = addUniqueHeaderIds(url, data);
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

// Function to add unique IDs to Markdown headers
function addUniqueHeaderIds(url, markdown: string): string {
  const urlHash = crypto.createHash('sha256').update(url).digest('hex').substring(0, 8);

  let headerCounts: Record<string, number> = {};
  return markdown.replace(/^(#{1,6})\s+(.+)$/gm, (match, hashes, title) => {
    const slugBase = title.toLowerCase().replace(/[^\w]+/g, '-').replace(/^-|-$/g, '');
    const count = headerCounts[slugBase] || 0;
    const uniqueSlug = count ? `${slugBase}-${urlHash}-${count}` : `${slugBase}-${urlHash}`;
    headerCounts[slugBase] = count + 1;

    return `${hashes} ${title} {#${uniqueSlug}}`;
  });
}

function getShortHash(input: string): string {
  return crypto.createHash('sha256').update(input).digest('hex').substring(0, 8);
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
