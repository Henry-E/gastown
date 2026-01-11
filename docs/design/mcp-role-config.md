# Role-Based MCP Server Configuration Design

**Issue**: hq-o6fm
**Author**: max (polecat)
**Date**: 2026-01-11

## Problem Statement

Gas Town is experiencing significant process bloat from MCP (Model Context Protocol) servers:
- **1,068 MCP processes** currently running
- Each Claude session spawns 4 MCP servers: context7, playwright, filesystem, sequential-thinking
- Task subagents also spawn full MCP sets
- Processes accumulate over time with no cleanup

This wastes system resources and degrades performance as the town scales.

## Current MCP Servers

| Server | Package | Purpose |
|--------|---------|---------|
| context7 | `@upstash/context7-mcp` | Documentation lookup |
| playwright | `@playwright/mcp` | Browser automation |
| filesystem | `@modelcontextprotocol/server-filesystem` | File operations |
| sequential-thinking | `@modelcontextprotocol/server-sequential-thinking` | Reasoning chains |

## Analysis: How MCP Servers Are Spawned

### Discovery Mechanism

1. Claude Code reads MCP config from multiple sources:
   - Global: `~/.claude.json` → `mcpServers` object
   - Per-plugin: `~/.claude/plugins/cache/<plugin>/.mcp.json`
   - Per-project: `.mcp.json` in project directory

2. Plugin MCP servers are auto-discovered when plugins are enabled in `enabledPlugins`

3. Each Claude session spawns its own set of MCP server processes

### Existing Control Mechanisms

From investigation of Claude Code (v1.x, 2025-2026):

| Mechanism | Location | Status |
|-----------|----------|--------|
| `disabledMcpjsonServers` | Project config in `.claude.json` | Exists but buggy ([Issue #4938](https://github.com/anthropics/claude-code/issues/4938)) |
| `enabledMcpjsonServers` | Project config in `.claude.json` | Exists, limited docs |
| `managed-mcp.json` | System-level `/etc/claude-code/` | Enterprise control, not per-role |
| `PreToolUse` hook | Settings hooks | Can block tools by pattern |
| `enabledPlugins` | Settings | Controls plugin loading |

### Environment Variables

- `MAX_MCP_OUTPUT_TOKENS` - Output token limit (default 25,000)
- `MCP_TIMEOUT` - Startup timeout in ms
- No dedicated `CLAUDE_MCP_DISABLE_*` variables exist

## Proposed Solution: Role-Based MCP Mapping

### Role-to-MCP Matrix

| Role | context7 | playwright | filesystem | seq-thinking | Rationale |
|------|:--------:|:----------:|:----------:|:------------:|-----------|
| Mayor | :white_check_mark: | :x: | :white_check_mark: | :white_check_mark: | Needs docs, planning |
| Witness | :x: | :x: | :white_check_mark: | :x: | Minimal - monitoring only |
| Refinery | :x: | :x: | :white_check_mark: | :x: | Minimal - merge ops only |
| Deacon | :x: | :x: | :white_check_mark: | :x: | Minimal - coordination |
| Polecat (code) | :white_check_mark: | :x: | :white_check_mark: | :white_check_mark: | Full coding support |
| Polecat (test) | :x: | :white_check_mark: | :white_check_mark: | :x: | Browser testing |
| Crew | :white_check_mark: | :x: | :white_check_mark: | :white_check_mark: | Interactive coding |
| Task subagent | :x: | :x: | :white_check_mark: | :x: | Minimal - scoped tasks |

**Expected reduction**: From 4 servers/session to 1-3 servers/session (50-75% reduction)

## Implementation Options

### Option A: PreToolUse Hook Blocking (Recommended for MVP)

**Approach**: Add role-aware PreToolUse hooks that block MCP tools based on role.

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "mcp__context7.*|mcp__playwright.*|mcp__sequential-thinking.*",
        "hooks": [
          {
            "type": "command",
            "command": "gt mcp-gate"
          }
        ]
      }
    ]
  }
}
```

The `gt mcp-gate` command would:
1. Read `GT_ROLE` environment variable
2. Check if the tool is allowed for that role
3. Return exit code 0 (allow) or non-zero (block)

**Pros**:
- Uses existing hook infrastructure
- No changes to Claude Code required
- Easy to implement and test
- Backward compatible

**Cons**:
- MCP servers still spawn (just tools are blocked)
- Adds latency to every MCP tool call
- Doesn't solve the process bloat root cause

### Option B: Role-Specific Settings Templates

**Approach**: Generate role-specific `.claude/settings.json` with different `enabledPlugins`.

```go
// internal/claude/config/settings-witness.json
{
  "enabledPlugins": {
    "context7@claude-plugins-official": false,
    "playwright@claude-plugins-official": false,
    "sequential-thinking@claude-plugins-official": false
  },
  ...
}
```

Modify `EnsureSettingsForRole()` to use role-specific templates instead of just autonomous/interactive.

**Pros**:
- Prevents MCP servers from spawning entirely
- Clean solution at the right layer
- Per-role templates are maintainable

**Cons**:
- Requires understanding plugin naming conventions
- May not work if MCP comes from global config
- More templates to maintain

### Option C: Per-Account Role Directories

**Approach**: Create role-specific account config directories.

```
~/.claude-accounts/syn/
├── roles/
│   ├── mayor/
│   │   └── .claude.json  (with mcpServers filtered)
│   ├── witness/
│   │   └── .claude.json
│   └── polecat/
│       └── .claude.json
```

Modify account resolution to append role path:
```go
claudeConfigDir = filepath.Join(accountDir, "roles", role)
```

**Pros**:
- Complete isolation per role
- Works with all MCP config mechanisms
- Future-proof

**Cons**:
- Complex directory structure
- Credential/cache duplication concerns
- Major refactor of account handling

### Option D: RuntimeConfig MCP Extension

**Approach**: Extend `RuntimeConfig` with MCP settings.

```go
type RuntimeMCPConfig struct {
    // DisabledServers lists MCP server names to block for this role
    DisabledServers []string `json:"disabled_servers,omitempty"`

    // AllowedServers lists only allowed servers (whitelist mode)
    AllowedServers []string `json:"allowed_servers,omitempty"`
}

type RuntimeConfig struct {
    // ... existing fields ...
    MCP *RuntimeMCPConfig `json:"mcp,omitempty"`
}
```

Then inject `disabledMcpjsonServers` into project config on spawn.

**Pros**:
- Integrates with existing RuntimeConfig pattern
- Configurable per-rig in settings/config.json
- Uses Claude Code's native mechanism

**Cons**:
- Depends on buggy `disabledMcpjsonServers` feature
- Requires modifying Claude's project config
- More complex implementation

## Recommended Approach

### Phase 1: PreToolUse Hook Blocking (MVP)

Implement Option A for immediate relief:
1. Add `gt mcp-gate` command
2. Create role-based MCP allowlist config
3. Add PreToolUse hook to settings templates
4. Test with witness/refinery first (minimal MCPs needed)

**Estimated impact**: Blocks unnecessary MCP tool usage, reducing load

### Phase 2: Role-Specific Settings Templates

Implement Option B for proper prevention:
1. Create per-role settings templates
2. Map roles to appropriate `enabledPlugins`
3. Modify `EnsureSettingsForRole()` to select template by role
4. Deprecate autonomous/interactive distinction

**Estimated impact**: 50-75% reduction in MCP processes

### Phase 3: Subagent MCP Inheritance (Future)

Address Task subagent MCP spawning:
1. Investigate Claude Code subagent spawn mechanism
2. Propose upstream changes or workarounds
3. Consider Task tool modifications to pass MCP hints

## Configuration Format

```json
// settings/config.json
{
  "mcp": {
    "role_config": {
      "witness": {
        "allowed": ["filesystem"]
      },
      "refinery": {
        "allowed": ["filesystem"]
      },
      "polecat": {
        "allowed": ["context7", "filesystem", "sequential-thinking"]
      },
      "polecat:test": {
        "allowed": ["playwright", "filesystem"]
      },
      "mayor": {
        "allowed": ["context7", "filesystem", "sequential-thinking"]
      },
      "crew": {
        "allowed": ["context7", "filesystem", "sequential-thinking"]
      }
    },
    "default_allowed": ["filesystem"]
  }
}
```

## Files to Modify

### Phase 1 (Hook Blocking)
- `cmd/gt/mcp_gate.go` - New command for hook
- `internal/claude/config/settings-autonomous.json` - Add PreToolUse hook
- `internal/claude/config/settings-interactive.json` - Add PreToolUse hook

### Phase 2 (Settings Templates)
- `internal/claude/settings.go` - Role-based template selection
- `internal/claude/config/settings-witness.json` - New template
- `internal/claude/config/settings-refinery.json` - New template
- `internal/claude/config/settings-polecat.json` - New template
- `internal/claude/config/settings-mayor.json` - New template

## Testing Plan

1. **Process count baseline**: Count MCP processes before changes
2. **Per-role testing**: Spawn each role, verify correct MCPs
3. **Tool availability**: Verify allowed tools work, blocked tools fail gracefully
4. **Integration test**: Full town startup, measure process reduction
5. **Subagent test**: Verify Task subagents inherit restrictions

## Related Issues

- hq-kkkm: gt cleanup/ps command (process management)
- hq-0esz: polecat pooling design (reduces total sessions)

## References

- [Claude Code MCP Docs](https://code.claude.com/docs/en/mcp)
- [GitHub Issue #4938](https://github.com/anthropics/claude-code/issues/4938) - disabledMcpjsonServers bugs
- [GitHub Issue #5722](https://github.com/anthropics/claude-code/issues/5722) - Enable/Disable toggle request
- [GitHub Issue #6313](https://github.com/anthropics/claude-code/issues/6313) - System-level disable issues
