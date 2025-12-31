#!/usr/bin/env npx ts-node

import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import {
  CallToolRequestSchema,
  ListToolsRequestSchema,
  Tool,
} from "@modelcontextprotocol/sdk/types.js";

const RETRIEVAL_API_URL = process.env.RETRIEVAL_API_URL || "http://localhost:8080";

interface RetrievalRequest {
  query: string;
  project_id?: string;
  top_k?: number;
  score_threshold?: number;
}

interface RetrievalResult {
  source: string;
  symbol: string;
  symbol_type: string;
  language: string;
  start_line: number;
  end_line: number;
  content: string;
  score: number;
  project_id: string;
  module: string;
}

interface RetrievalResponse {
  results: RetrievalResult[];
  query_time_ms: number;
}

// Tool definitions
const TOOLS: Tool[] = [
  {
    name: "search_codebase",
    description: `Search the indexed codebase for relevant code snippets using semantic search.
Use this tool when you need to:
- Find implementations of specific functionality
- Locate related code across the project
- Understand how a feature is implemented
- Find usage examples of functions, classes, or patterns

The search uses embeddings to find semantically similar code, not just keyword matches.`,
    inputSchema: {
      type: "object" as const,
      properties: {
        query: {
          type: "string",
          description: "Natural language query describing what you're looking for. Be descriptive for better results.",
        },
        project_id: {
          type: "string",
          description: "Project ID to search in. If not specified, searches across all indexed projects.",
        },
        top_k: {
          type: "number",
          description: "Maximum number of results to return (default: 5, max: 20)",
          default: 5,
        },
      },
      required: ["query"],
    },
  },
  {
    name: "list_projects",
    description: "List all indexed projects available for search.",
    inputSchema: {
      type: "object" as const,
      properties: {},
      required: [],
    },
  },
];

async function searchCodebase(params: RetrievalRequest): Promise<string> {
  const body: RetrievalRequest = {
    query: params.query,
    top_k: Math.min(params.top_k || 5, 20),
    score_threshold: params.score_threshold || 0.3,
  };

  if (params.project_id) {
    body.project_id = params.project_id;
  }

  try {
    const response = await fetch(`${RETRIEVAL_API_URL}/retrieve`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });

    if (!response.ok) {
      const error = await response.json();
      return `Error: ${error.error || response.statusText} (code: ${error.code || "UNKNOWN"})`;
    }

    const data: RetrievalResponse = await response.json();

    if (data.results.length === 0) {
      return `No results found for query: "${params.query}"${params.project_id ? ` in project ${params.project_id}` : ""}`;
    }

    // Format results for context
    let output = `## Search Results for: "${params.query}"\\n`;
    output += `Found: ${data.results.length} results (${data.query_time_ms}ms)\\n\\n`;

    for (const result of data.results) {
      output += `### ${result.source}:${result.start_line}-${result.end_line}\n`;
      output += `**${result.symbol}** (${result.symbol_type}) | Score: ${(result.score * 100).toFixed(1)}%\n`;
      output += "```" + result.language + "\n";
      output += result.content;
      output += "\n```\n\n";
    }

    return output;
  } catch (error) {
    return `Error connecting to retrieval API: ${error instanceof Error ? error.message : "Unknown error"}. Make sure the API is running at ${RETRIEVAL_API_URL}`;
  }
}

async function listProjects(): Promise<string> {
  try {
    const response = await fetch(`${RETRIEVAL_API_URL}/projects`);

    if (!response.ok) {
      return `Error fetching projects: ${response.statusText}`;
    }

    const data = await response.json();

    if (!data.projects || data.projects.length === 0) {
      return "No indexed projects found.";
    }

    let output = "## Indexed Projects\n\n";
    for (const project of data.projects) {
      output += `- **${project.id}**: ${project.chunk_count || 0} chunks indexed\n`;
    }

    return output;
  } catch (error) {
    return `Error connecting to retrieval API: ${error instanceof Error ? error.message : "Unknown error"}`;
  }
}

// Main server setup
const server = new Server(
  {
    name: "project-indexer",
    version: "1.0.0",
  },
  {
    capabilities: {
      tools: {},
    },
  }
);

// Handle tool listing
server.setRequestHandler(ListToolsRequestSchema, async () => {
  return { tools: TOOLS };
});

// Handle tool calls
server.setRequestHandler(CallToolRequestSchema, async (request) => {
  const { name, arguments: args } = request.params;

  try {
    let result: string;

    switch (name) {
      case "search_codebase": {
        const params = args as unknown as RetrievalRequest;
        if (!params.query) {
          return {
            content: [{ type: "text", text: "Error: query parameter is required" }],
            isError: true,
          };
        }
        result = await searchCodebase(params);
        break;
      }
      case "list_projects":
        result = await listProjects();
        break;
      default:
        return {
          content: [{ type: "text", text: `Unknown tool: ${name}` }],
          isError: true,
        };
    }

    return {
      content: [{ type: "text", text: result }],
    };
  } catch (error) {
    return {
      content: [
        {
          type: "text",
          text: `Error: ${error instanceof Error ? error.message : "Unknown error"}`,
        },
      ],
      isError: true,
    };
  }
});

// Start server
async function main() {
  const transport = new StdioServerTransport();
  await server.connect(transport);
  console.error("Project Indexer MCP Server running on stdio");
}

main().catch(console.error);
