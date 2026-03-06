# Gas Town Operational Health Checklist

Run through this when reviewing whether Gas Town is functioning as intended.

## 1. Build Pipeline

- [ ] Source repo (`~/gt/gastown/mayor/rig/`) is on main, clean, no unpushed commits
- [ ] `gt version` shows no stale warning
- [ ] Binary not built dirty (no `-dirty` suffix)
- [ ] Daemon running with recent heartbeat (`gt daemon status`)
- [ ] Deacon dog clones (`~/gt/deacon/dogs/*/gastown/`) match origin/main

## 2. Dolt Data Plane

- [ ] `gt dolt status` shows server running, low latency
- [ ] Circuit breaker closed (`/tmp/beads-dolt-circuit-3307.json`)
- [ ] No orphan/smoke/test databases (`SHOW DATABASES` shouldn't have testdb_*, smoke_*, beads_t*)
- [ ] Connection count reasonable (< 50% of max)
- [ ] Disk usage stable (not growing unbounded)

## 3. Agent Lifecycle

- [ ] Mayor is responsive (check `tmux capture-pane -t hq-mayor`)
- [ ] Deacon is running, not in crash loop (`gt daemon status`, check for backoff)
- [ ] No agents running for deleted/nonexistent rigs
- [ ] No stale sessions (agents idle > 24h with no work)
- [ ] Mail backlogs reasonable (no agent with > 10 unread unless actively processing)
- [ ] No zombie tmux sessions (`tmux list-sessions` — check for test leftovers)

## 4. Active Rigs

- [ ] `gt status` — identify which rigs have running agents (●) vs dormant (○)
- [ ] Active rigs have witness + refinery running
- [ ] No MQ items sitting on dormant refineries (work stuck with nobody to process)
- [ ] Crew agents with unread mail are actually processing (not stuck)
- [ ] Polecats completing work (check `gt feed` for recent `done:` events)

## 5. System Resources

- [ ] RAM: check `free -h` — available > 5 GB
- [ ] Swap: < 3 GB in use
- [ ] No runaway processes (`ps aux --sort=-%mem | head -20`)
- [ ] CPU load reasonable for core count (`uptime`)
- [ ] Disk not filling (`df -h /home`)

## 6. Logs

- [ ] `~/gt/daemon/dolt.log` — current log < 100 MB, rotation working
- [ ] `~/gt/daemon/daemon.log` — no repeated error patterns
- [ ] No old log archives or debug captures accumulating
- [ ] Check for high-frequency spam ("nothing to commit", "no wisp config")

## 7. Code Health

- [ ] Recent commits: `git log --oneline -20` — look for revert/reapply churn
- [ ] Tests pass: `go test ./...` in the rig
- [ ] No dead code left behind from reverted changes
- [ ] GitHub issues filed for known tech debt / architectural concerns
- [ ] No agents committing to the same subsystem without coordination

## Quick Commands

```bash
# Full status overview
gt status

# Build health
gt version
cd ~/gt/gastown/mayor/rig && git status && git log origin/main..HEAD --oneline

# Dolt health
gt dolt status

# Agent processes and memory
ps aux --sort=-%mem | head -20

# Event feed (what's actually happening)
gt feed | head -30

# Deacon state
gt daemon status

# Dog clone freshness
for d in ~/gt/deacon/dogs/*/gastown; do echo "=== $d ===" && git -C "$d" log --oneline -1; done

# Find stuck work
gt status 2>&1 | grep -E 'MQ:|📬'
```
