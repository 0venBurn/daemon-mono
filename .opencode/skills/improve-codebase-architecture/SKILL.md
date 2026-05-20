---
name: improve-codebase-architecture
description: Find deepening opportunities in a codebase, informed by the domain language in `domain.md` (look in `@docs` at root or module level) and decisions in linear. If `domain.md` doesn't exist, ignore and proceed without it. Favour procedural, data-oriented, explicit refactors that reduce indirection and improve locality/testability.
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

- Prefer plain data + functions over class/DI hierarchies (where language allows).
- In OOP contexts: prefer class-as-namespace (static methods + records) over instance-heavy designs.
- Flatten wrappers, facades, and single-implementation interfaces/traits.
- Make control flow and dependencies explicit in constructors/method signatures.
- Keep abstractions only when they demonstrably increase leverage/locality; collapse shallow class hierarchies.

This skill is informed by the project's domain model. Look for `domain.md` in `@docs` at root or module level for seam names. If `domain.md` doesn't exist, ignore and proceed without it. ADRs record decisions not to casually re-litigate. These can be found in issue tracker on linear.

## OOP/Class-Oriented Mode

When working in OOP codebases (e.g., Java, C#) where procedural refactoring is impractical:

- Treat **classes as API boundaries** (modules) rather than "objects" with identity/behavior coupling.
- Prefer **class-as-namespace** patterns: static methods + data classes (records) over instance-heavy designs.
- Depth still measured at the class interface: few public methods, rich internal behavior.
- Seams live at class boundaries; adapters are concrete implementations of interfaces/traits.
- Apply deletion test to classes: if removing it scatters complexity, it was deep; if it just removes ceremony, it was shallow.

Key OOP moves:

- Collapse shallow hierarchies (e.g., Strategy pattern with one strategy → inline the behavior).
- Replace template methods with composition.
- Make dependencies explicit in constructors (no service locators).
- Use package-private/internal visibility to hide implementation details while keeping procedural locality.

Do not center design around "objects" with mutable state and identity. Center around **class-shaped modules** with small interfaces and deep implementations.

## Process

### 1) Explore

Look for `domain.md` in `@docs` at root or module level. If found, read it for domain terms and seam names. If not found, ignore and proceed without it. Check linear for relevant ADRs.

Then explore code and note friction:

- Understanding one concept requires hopping across many thin modules
- Shallow modules where interface complexity mirrors implementation
- Pure-function extraction done only for mock-heavy tests, hurting locality
- Class hierarchies where each subclass adds little leverage (e.g., one-method interfaces with single implementors)
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

Use domain terms from `domain.md` (if it exists in `@docs` at root or module level) and [LANGUAGE.md](LANGUAGE.md) architecture terms.

If a candidate contradicts an ADR, only surface when friction is genuinely high; label clearly.

Do not propose final interfaces yet. Ask: **"Which one should we drill into?"**

### 3) Grilling loop

Once user picks a candidate, walk the design tree: constraints, dependencies, module shape, seam placement, tests.

Side effects during this loop:

- If a new domain term is required and `domain.md` exists in `@docs`, add/update it inline. If `domain.md` doesn't exist, ignore and proceed without it.
- If user rejects candidate with a durable reason, offer ADR capture.
- If user wants interface options, use [INTERFACE-DESIGN.md](INTERFACE-DESIGN.md).

## Dependency references

Use only local references for this skill:

- [LANGUAGE.md](LANGUAGE.md)
- [DEEPENING.md](DEEPENING.md)
- [INTERFACE-DESIGN.md](INTERFACE-DESIGN.md)

Do not rely on missing cross-skill files.

