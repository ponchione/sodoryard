# Context assembly in plain English

This document explains how context assembly works in this project in everyday language.

It is written to help someone understand the real system that ships today, not just the ideal design in the specs.

## The short version

Every time the user sends a message, the app tries to build a fresh "working packet" of context before the model answers.

That packet is assembled from a few different places:

- recent conversation history
- semantically relevant code from the code index (RAG)
- files the user named directly
- structurally related code from the code graph
- project-brain documents from the configured brain backend
- project conventions
- small bits of git history

Then the system:

1. decides what kind of context is probably needed
2. fetches candidate material from those sources
3. trims it to fit the token budget
4. serializes it into a stable markdown block
5. sends that block along with the prompt to the model
6. stores a report explaining what it did

So the model does not just get a static system prompt and chat history. It gets a custom, per-turn context package.

## The main idea

The core philosophy is:

- do not preload the whole repo into the prompt
- do not rely only on the model to ask for tools later
- instead, proactively assemble the most useful context for this specific turn

In practice, that means the system tries to answer questions like:

- Is the user asking about a specific file?
- Are they asking about a function or symbol?
- Are they creating something new, which means conventions matter?
- Are they asking about project knowledge or past decisions, which means the brain matters?
- Is this a follow-up turn, where recent tool activity should influence what we include?

## The high-level flow

For each turn, the live flow looks like this:

1. The agent loop starts a new turn.
2. It reconstructs the recent conversation history.
3. It asks the context assembler to build context for this turn.
4. The assembler runs analysis, retrieval, budgeting, and serialization.
5. The result is a frozen context package for that turn.
6. The model uses that package for the turn.
7. After the turn, the system checks whether the model still had to search or read more files, and records that as quality feedback.

A useful way to explain it is:

- before the model talks, the system does a small research pass
- after the model talks, the system grades how good that research pass was

## The main pieces

### 1. Turn analyzer

The turn analyzer is the first stage.

Its job is not to answer the user. Its job is to guess what kinds of context the model will need.

It uses rules and heuristics, not another LLM call.

It looks for things like:

- file paths
- symbol names
- verbs like "fix", "change", "refactor", "add", "implement"
- git-related words like "commit", "diff", or "branch"
- follow-up language like "continue" or "keep going"
- brain-oriented language like "why did we", "what's our convention", or "have we seen this before"

The output is a "needs" object. That object says things like:

- search semantically for code about X
- read these explicit files
- look up graph neighbors for these symbols
- include conventions
- include git context
- prefer brain context for this turn

So this stage is the planner.

It does not fetch anything yet.

### 2. Momentum

Momentum is the system's short-term memory of what the conversation was recently working on.

This matters because many follow-up turns are vague.

For example:

- first turn: "look at the auth middleware"
- next turn: "ok now fix the tests"

That second turn does not repeat the path. Momentum helps the system keep the context anchored.

The live implementation mainly gets momentum from recent tool activity, not just from words in the chat.

It looks at recent tool calls such as:

- file reads
- file edits
- semantic searches
- text searches

From those, it extracts recently touched file paths and tries to infer a common module or directory.

So momentum is basically:

- what files have we actually been touching lately?
- what folder or subsystem do those files seem to belong to?

That momentum can then influence retrieval for weak follow-up turns.

### 3. Query extraction

Once the analyzer knows the likely needs, the system builds search queries.

These queries are used mainly for semantic code search and brain search.

The extractor usually builds up to a few queries from:

- a cleaned-up version of the user message
- technical keywords from the message
- a momentum-enhanced version if the conversation is already focused on one module

Important detail:

If the user explicitly named a file or symbol, that does not get left to fuzzy semantic search alone.

Instead:

- explicit files are read directly
- explicit symbols can trigger graph lookups directly

That makes the system more deterministic when the user is specific.

## Where the context comes from

After analysis and query extraction, retrieval starts.

The system can gather context from several parallel lanes.

### 4. Code RAG

This is the semantic code retrieval path.

Plain English version:

- the project is indexed ahead of time
- code is split into meaningful chunks, usually around functions, methods, types, classes, or sections
- each chunk gets a short semantic description
- the system embeds those descriptions into vectors
- when the user asks a question, the query is embedded too
- vector similarity search finds code chunks that are semantically related

This is what people usually mean by RAG in this project.

It is good for questions like:

- "where is auth handled?"
- "how does routing work?"
- "show me the error handling pattern"

The important thing is that the system is not searching raw text only. It is searching a semantic index of code chunks.

### 5. Explicit file reads

If the user names a file, the system reads that file directly.

This is the most straightforward lane.

If the user says:

- "fix internal/auth/middleware.go"

then the assembler does not rely on semantic search to maybe find that file.

It just reads the file.

This direct path is high priority because it is usually the clearest signal of what matters.

### 6. Structural graph / blast radius

This is the "graph thing" on the code side.

It is not the same as RAG.

RAG answers:

- what code is about this concept?

The structural graph answers:

- what code is connected to this function, type, or symbol?

This graph is built from parsing and static analysis.

For example, if a function is identified, the graph can help find:

- callers
- callees
- references
- nearby structural dependencies

This is useful when the user wants to change something and the system needs to understand what else might be affected.

So a simple way to explain it is:

- RAG finds relevant code by meaning
- the graph finds related code by structure

Both are useful, but for different reasons.

### 7. Project brain

The project brain is the long-term project knowledge layer.

This is not the codebase itself. It is the notes layer.

The source of truth is Shunter project memory.

Those notes can contain things like:

- architectural rationale
- conventions
- debugging notes
- past discoveries
- project-specific decisions
- session summaries

There are two broad ways the brain matters here:

1. Reactive use: the model can call brain tools like `brain_search` or `brain_read`.
2. Proactive use: context assembly itself can pull brain notes into the prompt before the model asks.

This is important because many questions are not really code questions. They are project-knowledge questions, like:

- why was this designed this way?
- what is our convention here?
- have we hit this bug before?

In those cases, brain context may matter more than code context.

### 8. Conventions

For some turns, the system includes project conventions.

This is mostly useful when the user is creating or changing code and the model should follow existing patterns.

Conventions are meant to answer questions like:

- how are tests usually written here?
- what error handling style is normal?
- how are handlers and services usually separated?

This is less about raw facts and more about helping the model produce code that looks like the repo.

### 9. Git context

If the turn looks git-related, the system can include small bits of recent git history.

This is intentionally lightweight.

It is there to help with questions like:

- what changed recently?
- what branch work was done?
- what commit is this related to?

It does not try to shove full diffs into the assembled context by default.

## How the code RAG pipeline works

To explain the RAG side clearly to someone else, this is the simplest accurate version.

### Indexing

Before good semantic retrieval can happen, the project has to be indexed.

That indexing pipeline does roughly this:

1. walk the repository files
2. parse supported files
3. break them into meaningful chunks
4. describe each chunk in semantic language
5. embed those descriptions into vectors
6. store the vectors in LanceDB
7. store structural relationships for graph use

So when the user asks a question later, the runtime is not reading the whole repo from scratch. It is querying a prepared index.

### Chunking

Chunks are not arbitrary 500-line windows when the parser can do better.

The system tries to chunk by meaningful units, such as:

- functions
- methods
- types
- classes
- markdown sections

That makes retrieval much better, because each chunk represents an actual concept.

### Descriptions and embeddings

Each code chunk is described in plain language before embedding.

That means the vector search is based more on what the code does than on raw code tokens alone.

That is why a user can ask in normal English and still get useful code hits.

### Search and ranking

When a search runs:

- the query is embedded
- top matches are pulled from the vector store
- duplicate hits are merged
- matches that appear across multiple query variants can be ranked higher
- one-hop graph expansion can add connected symbols around the top hits

So code retrieval is not just one naive nearest-neighbor lookup. It is a small retrieval pipeline.

## How the brain side works

The easiest honest explanation is:

- Shunter project memory is the source of truth
- vector indexes and other retrieval metadata are derived helper layers

### Project memory documents

The real brain content lives in Shunter project memory.

The brain backend reads, writes, patches, lists, and keyword-searches those documents.

So if someone asks "where is the brain stored?", the answer is:

- in Shunter project memory under `.yard/shunter/project-memory`

### Derived metadata and graph

The system can also build a derived index for brain content.

That index includes things like:

- note metadata
- parsed tags
- parsed links between notes
- semantic chunks for vector search

Parsed metadata and link helpers are rebuilt from Shunter documents, and semantic chunks are stored in LanceDB.

This means the brain can support more than raw keyword search.

Depending on what is available and fresh, the runtime can mix:

- keyword matches from the configured brain backend
- semantic matches from the brain vector index
- graph expansion through note links and backlinks

### Important freshness caveat

The configured brain backend is the truth, but the derived brain indexes can go stale.

So if brain documents are edited, the backend is updated immediately, but the semantic and graph helpers may need a reindex to catch up.

That means the current brain system is powerful, but it is not a magical always-perfectly-live graph database.

## What the database is doing

There are really two different kinds of storage involved here.

### A. Project memory and vector stores hold the content/indexes

- code vectors go in LanceDB
- brain vectors can also go in LanceDB
- brain source documents live in the configured brain backend

### B. The memory backend holds operational and observability data

Shunter project memory stores the app's canonical internal state and reporting data.

For context assembly specifically, the configured memory backend stores the context assembly report for each turn.

That report includes things like:

- what the analyzer thought the turn needed
- what signals triggered those decisions
- what RAG hits were found
- what brain hits were found
- what graph hits were found
- what explicit files were read
- what budget was available and used
- what got included vs excluded
- whether the model later had to search again anyway
- what files it still had to read
- a context hit-rate score

So if someone asks "where does the system remember what context it assembled?" the answer is:

- in the configured memory backend, as a per-turn context report; new Shunter-mode projects store that report in Shunter

## Budgeting: how it decides what actually fits

The system almost always finds more candidate context than it can safely include.

So it has a budget manager.

The budget manager estimates how many tokens are available after reserving space for:

- the base system prompt
- tool schemas
- conversation history
- model output headroom

Then it decides what to keep.

The current selection strategy is intentionally simple and predictable.

It prefers, roughly in this order:

1. explicit files
2. brain hits
3. strongest RAG code hits
4. graph hits
5. conventions
6. git context
7. lower-ranked RAG code hits if space remains

That means the system is biased toward:

- what the user asked for directly
- what the project already knows is important
- then the best semantically relevant code

This is not a fancy optimizer. It is a priority-based packing step.

That is actually helpful operationally because it is easier to reason about and debug.

## Serialization: how the final package is formatted

After the system chooses what to keep, it serializes the selected material into a stable markdown block.

The block usually has sections like:

- Project Brain
- Relevant Code
- Structural Context
- Project Conventions
- Recent Changes

This stability matters for two reasons:

1. The model can read a consistent shape.
2. Prompt caching works better when the format is stable.

So the serializer is not just cosmetic. It is part of the runtime design.

## Why the context is "frozen" per turn

Once a turn starts, the assembled context package is treated as fixed for that turn.

That means:

- the system does not keep rewriting the context package every iteration inside the same turn
- if the model needs more information mid-turn, it can still use tools
- but the original assembled package stays stable

This improves predictability and caching.

A simple way to explain it is:

- the system prepares a briefing packet at the start of the turn
- the packet stays the same for that turn
- if more investigation is needed, the model goes and fetches it with tools

## The context report and inspector

A big part of this system is observability.

The project does not just assemble context silently. It records what happened.

For each turn, the app can store a context report that acts like a debug record.

That report is later surfaced in the inspector UI.

The inspector can show things like:

- what signals were extracted from the user's message
- what semantic queries were generated
- what files and symbols were recognized
- what RAG, brain, graph, and explicit-file results were found
- how the token budget was spent
- what was cut out
- whether the agent still had to perform reactive searches
- which later reads were not already covered by proactive context

So if someone asks "how do we know whether context assembly is working well?" the answer is:

- the system stores a detailed report in the configured memory backend
- the web inspector reads that report and visualizes it
- post-turn quality metrics tell us whether the proactive context was actually sufficient

## How the UI gets this data

The web app reads stored context reports through metrics endpoints.

There are endpoints for:

- the main per-turn context report
- the per-turn signal flow

The frontend combines those into the inspector view.

There is also a live event path for `context_debug`, so the newest turn can show live context details before or alongside the durable stored version.

So the UI view is a combination of:

- durable per-turn report data from the configured memory backend
- live per-turn updates from the websocket event stream

## The simplest way to explain the whole system to someone else

If you need a very plain-English explanation, use this:

This project does a custom context-building pass before every model turn. It first analyzes the user's message to guess what kind of information is needed. Then it pulls candidate context from several places: semantic code search, direct file reads, the code relationship graph, project-brain notes, conventions, and recent git history. It trims that material to fit the prompt budget, formats it into a stable markdown block, and gives that block to the model as the turn's working context. After the turn, it records what it assembled and whether the model still had to go searching, so the team can inspect and improve retrieval quality over time.

## The important distinctions

These are the distinctions most worth keeping straight.

### RAG vs graph

RAG:

- semantic similarity
- finds code that is about the topic

Graph:

- structural relationships
- finds code connected to a specific symbol or chunk

### Brain vs code

Brain:

- project knowledge in notes
- rationale, conventions, debugging history, prior decisions

Code:

- actual implementation in the repository
- functions, files, symbols, dependencies

### Shunter/LanceDB

Shunter:

- source of truth for brain notes and project memory

LanceDB:

- vector storage for semantic retrieval

### Proactive vs reactive retrieval

Proactive retrieval:

- context assembly fetches context before the model answers

Reactive retrieval:

- the model later uses tools like file reads or searches during the turn

## Current reality vs idealized docs

One important nuance: some older docs describe parts of this system as more future-facing or more absolute than the current implementation.

The real current behavior is:

- per-turn context assembly is live
- code RAG is live
- direct file inclusion is live
- structural graph retrieval is live
- proactive brain retrieval is live
- brain notes can also be searched and read reactively through tools
- context reports and the inspector are live

There is also an important runtime nuance:

- code semantic search is not literally forced on every single turn
- if the turn clearly looks like a brain-oriented question and there are no explicit code targets, the system can prefer brain retrieval and suppress generic code RAG for that turn

That is a practical routing choice.

## Practical strengths of this design

In simple terms, the design is good because:

- it gives the model repo-specific context before it starts guessing
- it does not depend only on static context files
- it does not depend only on the model deciding to search later
- it combines semantic relevance with structural relationships
- it can include project knowledge, not just code
- it is observable, so the team can see whether it is working

## Practical limitations to understand

Also important to communicate clearly:

- retrieval quality depends on the indexes being built and reasonably fresh
- brain-derived indexes can lag behind backend document edits until reindexing runs
- budgeting means some relevant material may still be cut
- the analyzer is heuristic-based, so it can miss intent or over-trigger sometimes
- the model can still need reactive tool use even after proactive assembly

That is normal. The system is designed to reduce missing context, not to eliminate all tool use.

## Bottom line

The simplest accurate one-paragraph summary is:

This project assembles a fresh context package at the start of every turn. It uses heuristics to figure out what the user is asking about, pulls relevant material from code search, direct file reads, the structural code graph, the project brain, conventions, and git history, trims that material to fit the model's budget, and passes the result into the prompt. It also records exactly what it did in the configured memory backend so the team can inspect and improve context quality over time. In other words, the system tries to do the first round of repository research automatically before the model answers.
