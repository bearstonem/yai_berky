# Gap Analysis: yai_berky vs claw-code-main

## 1. Tools (yai: 7 → claw: 25+)

yai has 7 tools: `run_command`, `read_file`, `write_file`, `edit_file`, `list_directory`, `search_files`, `find_files`.

| Missing Tool | Priority | Description |
|---|---|---|
| `glob_search` | High | File pattern matching (vs shell `find`) |
| `grep_search` | High | Regex content search across files |
| `WebFetch` | Medium | Fetch URLs, read web content |
| `WebSearch` | Medium | Web search with cited results |
| `Agent` (sub-agent) | Medium | Spawn specialized child agents |
| `TodoWrite` | Medium | Structured task tracking |
| `Skill` | Low | Load local skill definitions |
| `NotebookEdit` | Low | Jupyter notebook manipulation |
| `StructuredOutput` | Low | Return typed JSON output |
| `Config` | Low | Get/set settings via tool |
| `Sleep` | Low | Non-blocking delay |

## 2. Slash Commands (yai: 0 → claw: 25+)

yai has no slash command system. claw has a full command registry with categories.

**Critical missing:**
- `/compact` — summarize old turns to save context
- `/cost` — token usage + cost tracking
- `/diff`, `/commit`, `/branch`, `/pr`, `/issue` — git workflows
- `/resume`, `/session`, `/export` — session management
- `/model`, `/permissions` — runtime switching
- `/help`, `/status`, `/clear` — basics
- `/init` — create starter instruction file
- `/memory`, `/config` — inspect state
- `/plugin`, `/agents`, `/skills` — extensibility

## 3. Permission System

| Feature | yai | claw |
|---|---|---|
| Granularity | Binary sudo toggle | 3-tier (ReadOnly / WorkspaceWrite / DangerFullAccess) |
| Per-tool enforcement | No | Yes — each tool declares required permission |
| Runtime escalation | No | Yes — dynamic prompting when escalation needed |
| CLI flags | No | `--permission-mode`, `--allowedTools`, `--dangerously-skip-permissions` |

## 4. Session Persistence

- **yai**: In-memory only, lost on exit
- **claw**: JSON-based save/load/resume/export to `~/.claw/sessions/`, plus **compaction** (summarize old turns to save context window)

## 5. MCP (Model Context Protocol)

- **yai**: Not implemented
- **claw**: Full implementation with 5 transport types (stdio, SSE, WebSocket, SDK, managed proxy), tool/resource discovery, JSON-RPC protocol (~2000 lines)

## 6. Plugin System

- **yai**: None
- **claw**: Manifest-based plugins (`.claw-plugin/plugin.json`) with tool definitions, command definitions, hook execution, lifecycle hooks, permission model, discovery/enable/disable (~2000 lines)

## 7. Hook Pipeline

- **yai**: None
- **claw**: PreToolUse / PostToolUse hooks — shell commands that can allow/deny/log tool execution, configured in settings.json

## 8. Configuration Layering

- **yai**: Single file (`~/.config/yai.json`)
- **claw**: 3-layer merge: user (`~/.claw/settings.json`) → project (`.claw/settings.json`) → local (`.claw/settings.local.json`)

## 9. Instruction File Discovery (CLAUDE.md / CLAW.md equivalent)

- **yai**: None
- **claw**: Auto-discovers `CLAW.md` files walking up the directory tree, merges them, injects into system prompt (4KB/file cap, 12KB total)

## 10. Cost & Usage Tracking

- **yai**: None
- **claw**: Per-message token counting, cumulative session costs, model-specific pricing, `/cost` command

## 11. OAuth / Auth

- **yai**: API key only
- **claw**: Full OAuth with PKCE flow, browser-based login, auto-refresh, stored at `~/.claw/auth/oauth.json`

## 12. Git Integration

- **yai**: Detects workspace root only
- **claw**: `/diff`, `/commit`, `/branch`, `/worktree`, `/pr`, `/issue` — full git workflow from inside the agent

## 13. System Prompt Enrichment

- **yai**: Basic static prompt per mode with system context (OS, shell, home dir)
- **claw**: Dynamic prompt with boundary marker, includes: environment context, git status/diff snapshot, instruction files, LSP diagnostics, config guidance

## 14. Output Formats

- **yai**: Terminal only (TUI via bubbletea)
- **claw**: Text (default), JSON (programmatic), Prompt (integration mode)

## 15. LSP Integration

- **yai**: None
- **claw**: Workspace diagnostics via Language Server Protocol, enriches system prompt with project-level diagnostics

## 16. Sandbox / Filesystem Isolation

- **yai**: None
- **claw**: Configurable sandbox modes for filesystem isolation

## 17. Streaming in Agent Mode

- **yai**: Uses non-streaming `CompleteWithTools()` per iteration
- **claw**: Streams all responses including during tool-use loops

## 18. Agent Iteration Limits

- **yai**: Hard cap at 50 iterations
- **claw**: Default `usize::MAX` (effectively unlimited)

---

## Recommended Prioritization

### Phase 1 — Foundation
1. Session persistence (save/resume/export)
2. Instruction file discovery (CLAW.md / YAI.md equivalent)
3. Configuration layering (user → project → local)
4. `glob_search` + `grep_search` tools

### Phase 2 — Developer Experience
5. Slash command framework + basic commands (`/help`, `/clear`, `/compact`, `/cost`)
6. Git commands (`/diff`, `/commit`, `/pr`)
7. 3-tier permission system with per-tool enforcement
8. Cost/token tracking

### Phase 3 — Extensibility

#### 9. Hook Pipeline (PreToolUse / PostToolUse)
- [x] 9a. Define `HookEvent` enum (PreToolUse, PostToolUse) and `HookConfig` struct
- [x] 9b. Add `hooks` section to config schema and viper parsing
- [x] 9c. Implement `HookRunner` — execute shell commands with tool context as env vars
- [x] 9d. Hook result actions: Allow (continue), Deny (block tool + return message), Log (continue + capture output)
- [x] 9e. Integrate hooks into `ToolExecutor.Execute()` — run PreToolUse before, PostToolUse after
- [x] 9f. Tests for hook pipeline

#### 10. Plugin System
- [ ] 10a. Define plugin manifest schema (`.yai/plugins/<name>/plugin.json`)
- [ ] 10b. Plugin discovery — scan `.yai/plugins/` for valid manifests
- [ ] 10c. Plugin tool registration — merge plugin-defined tools into AgentTools
- [ ] 10d. Plugin command registration — merge plugin-defined slash commands into Registry
- [ ] 10e. Plugin lifecycle hooks (onLoad, onUnload)
- [ ] 10f. `/plugin list`, `/plugin enable`, `/plugin disable` commands
- [ ] 10g. Tests for plugin system

#### 11. MCP Support
- [ ] 11a. JSON-RPC 2.0 message types (Request, Response, Notification)
- [ ] 11b. Stdio transport — launch subprocess, communicate via stdin/stdout
- [ ] 11c. MCP client — initialize, list tools, call tools, list resources
- [ ] 11d. MCP tool bridge — expose discovered MCP tools as yai Tool structs
- [ ] 11e. MCP config — `mcp_servers` section in settings with command/args/env
- [ ] 11f. Auto-start MCP servers on session init, shutdown on exit
- [ ] 11g. Tests for MCP client and tool bridge

#### 12. WebFetch / WebSearch Tools
- [ ] 12a. `web_fetch` tool — HTTP GET with response body extraction (HTML→text)
- [ ] 12b. `web_search` tool — search via DuckDuckGo HTML or similar free API
- [ ] 12c. Add to AgentTools and ToolExecutor dispatch
- [ ] 12d. Respect permission mode (require FullAccess)
- [ ] 12e. Tests for web tools

#### 13. Sub-agent Spawning
- [ ] 13a. `spawn_agent` tool — create child agent with scoped prompt and tools
- [ ] 13b. Child agent runs in its own goroutine with message isolation
- [ ] 13c. Result collection — parent receives child's final output as tool result
- [ ] 13d. Resource limits — max concurrent agents, iteration cap per child
- [ ] 13e. Tests for sub-agent spawning

### Phase 4 — Polish
14. OAuth support
15. LSP integration
16. Sandbox / filesystem isolation
17. JSON output format
18. Streaming in agent mode
