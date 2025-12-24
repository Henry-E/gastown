# Gas Town Vision: Core Innovations

> *"Work is fractal. Money is crystallized labor. Blockchain was the mechanism
> searching for its purpose."*

Gas Town is the IDE of 2026 - not an Integrated Development Environment, but an
**Integrated Delegation Engine**. It turns Claude Code (the Steam Engine) into a
Steam Train, with Beads as the globally distributed railway network.

---

## Core Innovation 1: The Steam Engine Metaphor

```
Claude       = Fire (the energy source)
Claude Code  = Steam Engine (harnesses the fire)
Gas Town     = Steam Train (coordinates engines on tracks)
Beads        = Railroad Tracks (the persistent ledger of work)
```

The engine does work and generates steam. Gas Town coordinates many engines on
a shared network, routing work to the right engines, tracking outcomes, and
ensuring nothing is lost.

| Component | Phase | Role |
|-----------|-------|------|
| **Proto** | Solid (crystal) | Frozen workflow templates |
| **Mol** | Liquid | Flowing durable work instances |
| **Wisp** | Vapor (gas) | Transient ephemeral traces |
| **Digest** | Distillate | Compressed permanent records |

---

## Core Innovation 2: Gas Town is a Village

**The anti-pattern we reject:**
```
Centralized Monitor → watches all workers → single point of failure
                   → fragile protocols → cascading failures
```

**The pattern we embrace:**
```
Every worker → understands the whole → can help any neighbor
           → peek is encouraged → distributed awareness
           → ant colony without murder → self-healing system
```

### The Antifragility Principle

Gas Town is **anti-fragile by design**. Not merely resilient (bounces back from
stress), but anti-fragile (gets stronger from stress).

Key properties:

- **Distributed awareness**: Every agent understands the system deeply
- **Mutual monitoring**: Any agent can peek at any other agent's health
- **Collective intervention**: If you see something stuck, you can help
- **No single point of failure**: The village survives individual failures
- **Organic healing**: Problems get fixed by whoever notices them first

This is an ant colony, except the ants don't kill defective members - they help
them recover. Workers who crash are respawned. Workers who get stuck are nudged.
Workers who need help receive it.

### Practical Implications

1. **Every patrol includes neighbor-checking**
   - Polecats peek at other polecats
   - Witness peeks at Refinery
   - Refinery peeks at Witness
   - Everyone can peek at the Deacon

2. **`gt peek` is universal vocabulary**
   - Any agent can check any other agent's health
   - Health states are shared vocabulary: idle, working, stuck, done

3. **Exit state enums are teaching tools**
   - COMPLETED, BLOCKED, REFACTOR, ESCALATE
   - Every agent learns these
   - When peeking neighbors, agents recognize states and can help

4. **Mail is the nervous system**
   - Asynchronous, persistent, auditable
   - Survives crashes and restarts
   - The village communicates through mail

---

## Core Innovation 3: Rig, Cook, Run

Work in Gas Town flows through three phases:

```
RIG ────→ COOK ────→ RUN
```

| Phase | What Happens | Key Operator |
|-------|--------------|--------------|
| **Rig** | Compose formulas (source level) | extends, compose |
| **Cook** | Instantiate work (pour/wisp) | cook, pour, wisp |
| **Run** | Execute steps | Agent execution |

**Rig** is source-level composition (formula YAML).
**Bond** is artifact-level composition (protos, mols, wisps).

See [rig-cook-run.md](rig-cook-run.md) for the full specification.

### The Three Phases of Matter

Work artifacts exist in three phases:

| Phase | Name | State | Behavior |
|-------|------|-------|----------|
| **Solid** | Proto | Frozen template | Crystallized, immutable, reusable |
| **Liquid** | Mol | Flowing instance | Dynamic, adapting, persistent |
| **Vapor** | Wisp | Ephemeral trace | Transient, dissipates, operational |

### Phase Transition Operators

```
            ┌─────────────┐
            │    PROTO    │
            │   (solid)   │
            └──────┬──────┘
                   │
         ┌─────────┼─────────┐
         │         │         │
       pour      wisp     distill
         │         │         ↑
         ▼         ▼         │
    ┌─────────┐ ┌─────────┐  │
    │   MOL   │ │  WISP   │  │
    │(liquid) │ │ (vapor) │  │
    └────┬────┘ └────┬────┘  │
         │           │       │
      squash      squash     │
         │           │       │
         ▼           ▼       │
    ┌─────────┐ ┌─────────┐  │
    │ DIGEST  │ │evaporates│ │
    │(crystal)│ │ or burn  │ │
    └─────────┘ └──────────┘ │
         │                   │
         └───────────────────┘
            (experience crystallizes)
```

| Operator | From | To | Effect |
|----------|------|------|--------|
| `pour` | Proto | Mol | Instantiate as persistent liquid |
| `wisp` | Proto | Wisp | Instantiate as ephemeral vapor |
| `bond` | Any + Any | Compound | Polymorphic combination |
| `squash` | Mol/Wisp | Digest | Condense to permanent record |
| `burn` | Wisp | Nothing | Discard without record |
| `distill` | Mol | Proto | Extract reusable template |

### The Polymorphic Bond Operator

**Bond** is the artifact-level combiner (distinct from **rig**, which composes
source formulas). Bond adapts to its operands:

| bond | Proto | Mol | Wisp |
|------|-------|-----|------|
| **Proto** | Compound Proto | Spawn + attach | Spawn wisp + attach |
| **Mol** | Spawn + attach | Link | Link |
| **Wisp** | Spawn + attach | Link | Link |

This enables patterns like:
- Patrol wisp discovers issue → bonds new work mol
- Feature work needs diagnostic → bonds vapor wisp
- Witness tracks polecats → bonds lease per polecat

---

## Core Innovation 4: Beads as Universal Data Plane

Beads is Git + Issues + Molecules in one human-readable format.

**Key properties:**
- **Git-backed**: Cryptographic hashes, Merkle trees, distributed
- **Human-readable**: Markdown, auditable, trustworthy
- **Fractal**: Work at any scale (task → epic → project → organization)
- **Federated**: Multi-repo, multi-org, platform-agnostic

**The insight:**
> "Git IS already a blockchain (Merkle tree, cryptographic hashes, distributed
> consensus). Beads is what blockchain was meant to enable - not coin
> speculation, but a universal ledger of work and capability."

### The GUPP Principle

**Git as Universal Persistence Protocol**

Everything persists through git:
- Issues are JSONL in `.beads/`
- Molecules are structured issues
- Mail is issues with labels
- Work history is commit history
- Entity chains are git histories

This means:
- Offline-first by default
- Distributed without infrastructure
- Auditable forever
- No vendor lock-in

### Control Plane = Data Plane

Gas Town uses Beads as both control plane and data plane:

| Data Type | Beads Representation |
|-----------|---------------------|
| Work items | Issues (tasks, bugs, features) |
| Workflows | Molecules (type=molecule) |
| Messages | Mail beads (type=message) |
| Merge requests | Queue entries (type=merge-request) |
| Agent state | Status on assigned issues |

The control state IS data in Beads. Agents read Beads to know what to do next.
There is no separate orchestrator - Beads IS the orchestrator.

---

## Core Innovation 5: The Patrol System

Gas Town runs on continuous monitoring loops called **patrols**.

### Patrol Agents

| Agent | Role | Patrol Focus |
|-------|------|--------------|
| **Deacon** | Town-level daemon | Health of all agents, plugin execution |
| **Witness** | Per-rig polecat monitor | Polecat lifecycle, nudging, cleanup |
| **Refinery** | Per-rig merge processor | Merge queue, validation, integration |

### Patrol Wisps

Patrol agents run ephemeral wisps for their cycles:
- Wisp starts at cycle begin
- Steps complete as work progresses
- Wisp squashes to digest at cycle end
- New wisp spawns for next cycle

This prevents accumulation: patrol work is vapor that condenses to minimal
digests, not liquid that pools forever.

### The Witness Polecat-Tracking Wisp

The Witness maintains a rolling wisp with a **lease** per active polecat:

```
wisp-witness-patrol
├── lease: furiosa (boot → working → done)
├── lease: nux (working)
└── lease: slit (done, closed)
```

Each lease is a bonded vapor molecule tracking one polecat's lifecycle.
When a polecat exits, its lease closes. When all leases close, the wisp
squashes to a summary digest.

---

## Core Innovation 6: Propulsion Over Protocol

**The Propulsion Principle**

Agents don't wait for explicit commands. They propel themselves through work:

1. **Check hook/pin** - What's attached to me?
2. **Find next step** - What's ready in my molecule?
3. **Execute** - Do the work
4. **Advance** - Close step, find next
5. **Exit properly** - One of four exit types

This is **pull-based work**, not push-based commands. The molecule IS the
instruction set. The agent IS the executor.

### Hooks and Pins

Agents have **hooks** where work hangs. Work gets **pinned** to hooks.

```
Agent (with hook)
    └── pinned mol (or wisp)
        ├── step 1 (done)
        ├── step 2 (in_progress)
        └── step 3 (pending)
```

This enables:
- **Crash recovery**: Agent restarts, reads pinned mol, continues
- **Context survival**: Mol state persists across sessions
- **Handoff**: New session reads predecessor's pinned work
- **Observability**: `bd hook` shows what an agent is working on

### The Four Exits

Every polecat converges on one of four exits:

| Exit | Meaning | Action |
|------|---------|--------|
| **COMPLETED** | Work finished | Submit to merge queue |
| **BLOCKED** | External dependency | File blocker, defer, notify |
| **REFACTOR** | Work too large | Break down, defer rest |
| **ESCALATE** | Need human judgment | Document, mail human, defer |

All exits pass through the exit-decision step. All exits end in request-shutdown.
The polecat never exits directly - it waits to be killed by the Witness.

---

## Core Innovation 7: Nondeterministic Idempotence

The key property enabling autonomous operation:

- **Deterministic structure**: Molecule defines exactly what steps exist
- **Nondeterministic execution**: Any worker can execute any ready step
- **Idempotent progress**: Completed steps stay completed

```
Worker A picks up "design" step
Worker A completes "design"
Worker A crashes mid-"implement"
Worker B restarts, queries ready work
Worker B sees "implement" is ready (design done, implement pending)
Worker B continues from exactly where A left off
```

No work is lost. No state is in memory. Any worker can continue any molecule.

---

## The Agent Hierarchy

### Overseer (Human)
- Sets strategy and priorities
- Reviews and approves output
- Handles escalations
- Operates the system

### Mayor (AI - Town-wide)
- Dispatches work across rigs
- Coordinates cross-project dependencies
- Handles strategic decisions

### Deacon (AI - Town-level daemon)
- Ensures patrol agents are running
- Executes maintenance plugins
- Handles lifecycle requests

### Witness (AI - Per-rig)
- Manages polecat lifecycle
- Detects stuck workers
- Handles session cycling

### Refinery (AI - Per-rig)
- Processes merge queue
- Reviews and integrates code
- Maintains branch hygiene

### Polecat (AI - Ephemeral workers)
- Executes work molecules
- Files discovered issues
- Ephemeral - spawn, work, disappear

---

## The Steam Train in Action

Putting it all together:

```
1. Human files issue in Beads
2. Mayor dispatches: gt spawn --issue <id>
3. Polecat created with:
   - Fresh worktree
   - mol-polecat-work pinned to hook
   - Work assignment in mail
4. Deacon/Witness notified: POLECAT_STARTED
5. Witness bonds lease to patrol wisp
6. Polecat:
   - Reads polecat.md, orients
   - Reads mail, gets assignment
   - Executes mol-polecat-work steps
   - Makes commits, runs tests
   - Submits to merge queue
   - Exits via request-shutdown
7. Witness:
   - Receives SHUTDOWN mail
   - Closes polecat's lease
   - Kills session, cleans up
8. Refinery:
   - Processes merge queue
   - Rebases, tests, merges
   - Pushes to main
9. Digest created: work outcome crystallized
10. Loop: new work, new polecats, new cycles
```

The flywheel spins. The village watches itself. The train keeps running.

---

## Why "IDE of 2026"?

The IDE evolved from text editor → syntax highlighting → autocomplete → AI suggestions.

The next evolution isn't better suggestions - it's **AI as worker, not assistant**.

| 2024 IDE | 2026 IDE (Gas Town) |
|----------|---------------------|
| AI suggests code | AI writes code |
| Human reviews suggestions | Human reviews pull requests |
| AI helps with tasks | AI completes tasks |
| Single agent | Coordinated village |
| Context in memory | Context in Beads |
| Manual quality checks | Molecule-enforced gates |

Gas Town is what happens when you treat AI agents as employees, not tools.

---

## The Vision

Gas Town is the **Integrated Delegation Engine**.

For developers today. For all knowledge workers tomorrow.

The world has never had a system where:
- Work is fractal and composable
- Execution is distributed and self-healing
- History is permanent and auditable
- Agents are autonomous yet accountable
- The village watches itself

Beads is the ledger.
Gas Town is the execution engine.
The village watches itself.
The train keeps running.

---

*"If you're not a little bit scared, you're not paying attention."*
