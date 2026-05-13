---
name: improve-codebase-architecture
description: Find deepening opportunities in a codebase, informed by the domain language in CONTEXT.md and the decisions in docs/adr/. Favour procedural, data-oriented, explicit refactors that reduce indirection and improve locality/testability.
---

# Improve Codebase Architecture

Surface architectural friction and propose **deepening opportunities** — refactors that turn shallow modules into deep ones. Prefer procedural, data-oriented, explicit designs.

## Glossary

Use these terms exactly in every suggestion. Consistent language is the point — don't drift into "component," "service," "API," or "boundary." Full definitions in [LANGUAGE.md](LANGUAGE.md).

- **Module** — anything with an interface and an implementation (function, class, package, slice).
- **Interface** — everything a caller must know to use the module: types, invariants, error modes, ordering, config. Not just the type signature.
- **Implementation** — the code inside.
- **Depth** — leverage at the interface: a lot of behaviour behind a small interface. **Deep** = high leverage. **Shallow** = interface nearly as complex as the implementation.
- **Seam** — where an interface lives; a place behaviour can be altered without editing in place.
- **Adapter** — a concrete thing satisfying an interface at a seam.
- **Leverage** — what callers get from depth.
- **Locality** — what maintainers get from depth: change, bugs, knowledge concentrated in one place.

Key principles (see [LANGUAGE.md](LANGUAGE.md) for full list):

- **Deletion test**: if deleting a module just removes ceremony, it was pass-through.
- **The interface is the test surface.**
- **One adapter = hypothetical seam. Two adapters = real seam.**

Procedural bias:

- Prefer plain data + functions over class/DI hierarchies.
- Flatten wrappers, facades, and single-implementation interfaces.
- Make control flow and dependencies explicit in signatures.
- Keep abstractions only when they demonstrably increase leverage/locality.

This skill is informed by the project's domain model. `CONTEXT.md` gives seam names; ADRs record decisions not to casually re-litigate.

## Process

### 1) Explore

Read `CONTEXT.md` and relevant `docs/adr/*` first.

Then explore code and note friction:

- Understanding one concept requires hopping across many thin modules
- Shallow modules where interface complexity mirrors implementation
- Pure-function extraction done only for mock-heavy tests, hurting locality
- Tightly-coupled modules leaking across seams
- Untestable or hard-to-test behavior through current interfaces
- OOP/DI ceremony hiding simple data transformations

Apply deletion test to suspected shallow modules.

### 2) Present candidates

Present numbered deepening opportunities. For each:

- **Files**
- **Problem**
- **Solution**
- **Benefits** (locality, leverage, and test improvements)
- **Procedural shift** (what indirection is removed; what becomes explicit)

Use `CONTEXT.md` domain terms and [LANGUAGE.md](LANGUAGE.md) architecture terms.

If a candidate contradicts an ADR, only surface when friction is genuinely high; label clearly.

Do not propose final interfaces yet. Ask: **"Which one should we drill into?"**

### 3) Grilling loop

Once user picks a candidate, walk the design tree: constraints, dependencies, module shape, seam placement, tests.

Side effects during this loop:

- If a new domain term is required, add/update `CONTEXT.md` inline.
- If user rejects candidate with a durable reason, offer ADR capture.
- If user wants interface options, use [INTERFACE-DESIGN.md](INTERFACE-DESIGN.md).

## Dependency references

Use only local references for this skill:

- [LANGUAGE.md](LANGUAGE.md)
- [DEEPENING.md](DEEPENING.md)
- [INTERFACE-DESIGN.md](INTERFACE-DESIGN.md)

Do not rely on missing cross-skill files.