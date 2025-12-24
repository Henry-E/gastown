# Rig, Cook, Run: The Gas Town Work Lifecycle

> **Status**: Canonical specification
> **Discovery**: 2025-12-23

## The Three Phases

Gas Town work flows through three phases:

```
RIG ────→ COOK ────→ RUN
```

| Phase | What Happens | Input | Output |
|-------|--------------|-------|--------|
| **Rig** | Compose formulas | Formula YAML | Compound Formula |
| **Cook** | Instantiate work | Formula/Proto | Proto/Mol/Wisp |
| **Run** | Execute steps | Mol/Wisp | Work done |

## Phase 1: Rig (Authoring)

**Rigging** is source-level composition. You write formula YAML files that
reference and compose other formulas.

```yaml
# shiny-enterprise.formula.yaml
formula: shiny-enterprise
extends: shiny                    # ← rigging: inherit from shiny
compose:
  - expand:
      target: implement
      with: rule-of-five          # ← rigging: expand with macro
  - aspect:
      pointcut: "implement.*"
      with: security-audit        # ← rigging: weave in aspect
```

**Rig operators** (in formula YAML):
- `extends` - inherit from another formula
- `compose` - apply transformations (expand, aspect, gate, loop, branch)

**Rigging is static** - it happens before cooking, at authoring time.

## Phase 2: Cook (Instantiation)

**Cooking** transforms formulas into executable work.

```bash
bd cook shiny-enterprise    # Formula → Proto (flat, expanded)
bd pour shiny-enterprise    # Proto → Mol (liquid, persistent)
bd wisp shiny-enterprise    # Proto → Wisp (vapor, ephemeral)
```

**Cook produces three artifact types:**

| Command | Output | Phase of Matter | Storage |
|---------|--------|-----------------|---------|
| `bd cook` | Proto | Solid (frozen) | `.beads/` |
| `bd pour` | Mol | Liquid (flowing) | `.beads/` |
| `bd wisp` | Wisp | Vapor (ephemeral) | `.beads-wisp/` |

**Cooking is deterministic** - same formula always produces same structure.

## Phase 3: Run (Execution)

**Running** executes the steps in a mol or wisp.

```bash
gt sling shiny-enterprise polecat/alpha   # Cook + assign + run
```

Agents execute mols/wisps by:
1. Reading the pinned work from their hook
2. Completing steps in dependency order
3. Closing steps as they finish
4. Exiting when all steps complete

**Running is where cognition happens** - agents provide judgment, code, decisions.

## The Two Composition Operators

### Rig (Source-Level)

Operates on **formulas** (YAML source files).

```
Formula + Formula ──rig──→ Compound Formula
```

Use rigging when:
- You're authoring new workflows
- You want version-controlled composition
- You need the full algebra (extends, aspects, macros)

### Bond (Artifact-Level)

Operates on **artifacts** (protos, mols, wisps).

```
Artifact + Artifact ──bond──→ Combined Artifact
```

**The Bond Table** (symmetric, complete):

| bond | Proto | Mol | Wisp |
|------|-------|-----|------|
| **Proto** | Compound Proto | Spawn Mol, attach | Spawn Wisp, attach |
| **Mol** | Spawn Mol, attach | Link via edges | Link via edges |
| **Wisp** | Spawn Wisp, attach | Link via edges | Link via edges |

Use bonding when:
- Runtime dynamic composition (patrol discovers issue → bond bug mol)
- Ad-hoc combination without writing YAML
- Combining Mol Mall artifacts without source
- Programmatic proto generation

## The Complete Picture

```
                    SOURCE LEVEL
                    ════════════

Formula ─────rig─────→ Compound Formula
    │                        │
    │                        │
    └──────────┬─────────────┘
               │
              cook
               │
               ▼
                    ARTIFACT LEVEL
                    ══════════════

Proto ──────bond─────→ Compound Proto
  │ \                      │ \
  │  \                     │  \
pour  wisp               pour  wisp
  │     \                  │     \
  ▼      ▼                 ▼      ▼
 Mol    Wisp ───bond───→ Linked Work
  │       │
  └───┬───┘
      │
     run
      │
      ▼
                    EXECUTION
                    ═════════

              Steps complete
              Work gets done
              Digests created
```

## Vocabulary Summary

| Term | Meaning | Example |
|------|---------|---------|
| **Formula** | Source YAML with composition rules | `shiny.formula.yaml` |
| **Rig** | Compose formulas (source level) | `extends: shiny` |
| **Cook** | Transform formula → proto | `bd cook shiny` |
| **Proto** | Frozen workflow template | Cooked, immutable structure |
| **Pour** | Instantiate as mol (liquid) | `bd pour shiny` |
| **Wisp** | Instantiate as wisp (vapor) | `bd wisp shiny` |
| **Mol** | Flowing persistent work | Tracked in `.beads/` |
| **Wisp** | Ephemeral transient work | Tracked in `.beads-wisp/` |
| **Bond** | Combine artifacts (any level) | `bd bond proto-a proto-b` |
| **Sling** | Cook + assign to agent hook | `gt sling shiny polecat/alpha` |
| **Run** | Execute mol/wisp steps | Agent follows molecule |

## The Metaphor Sources

| Concept | Breaking Bad | Mad Max |
|---------|--------------|---------|
| Formula | Secret recipe | Fuel formula |
| Rig | Lab setup | War Rig assembly |
| Cook | "Let's cook" | Refining guzzoline |
| Proto | 99.1% pure crystal | Refined fuel |
| Pour/Wisp | Distribution | Pumping/vapor |
| Mol/Wisp | Product in use | Fuel burning |
| Bond | Cutting/mixing | Fuel blending |
| Sling | Dealing | Fuel dispatch |
| Run | The high | The chase |

## Why This Matters

1. **Clean separation**: Source composition (rig) vs artifact composition (bond)
2. **Symmetric operations**: Bond table is complete, no holes
3. **Clear phases**: Rig → Cook → Run, each with distinct purpose
4. **Metaphor coherence**: Breaking Bad + Mad Max vocabulary throughout
5. **Implementation clarity**: Each phase maps to specific commands

---

*This is a discovery, not an invention. The structure was always there.*
