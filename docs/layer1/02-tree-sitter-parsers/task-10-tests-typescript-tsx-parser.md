# Task 10: Unit Tests — TypeScript/TSX Tree-sitter Parser

**Epic:** 02 — Tree-sitter Parsers
**Status:** ⬚ Not started
**Dependencies:** Task 04

---

## Description

Write comprehensive unit tests for the TypeScript and TSX tree-sitter parsers. Tests use inline TypeScript/TSX source strings and assert exact `RawChunk` fields. Both the TypeScript and TSX parsers share extraction logic, so most tests are parameterized to run against both grammars.

## Acceptance Criteria

- [ ] Test file at `internal/codeintel/treesitter/typescript_parser_test.go`
- [ ] **Function declaration test:** parsing `function greet(name: string): string { return name; }` produces:
  - `Name` = `"greet"`
  - `Signature` = `"function greet(name: string): string"`
  - `ChunkType` = `Function`
  - `Body` contains the full function text
- [ ] **Async function test:** parsing `async function fetchData(url: string): Promise<Response> { return fetch(url); }` produces:
  - `Name` = `"fetchData"`
  - `Signature` contains `async function fetchData(url: string): Promise<Response>`
  - `ChunkType` = `Function`
- [ ] **Class declaration test:** parsing `class UserService extends BaseService implements IService { }` produces:
  - `Name` = `"UserService"`
  - `Signature` = `"class UserService extends BaseService implements IService"`
  - `ChunkType` = `Class`
- [ ] **Interface declaration test:** parsing `interface Config { host: string; port: number; }` produces:
  - `Name` = `"Config"`
  - `ChunkType` = `Interface`
- [ ] **Type alias test:** parsing `type Result = Success | Failure;` produces:
  - `Name` = `"Result"`
  - `ChunkType` = `Type`
  - `Signature` = `"type Result = Success | Failure;"`
- [ ] **Enum declaration test:** parsing `enum Direction { Up, Down, Left, Right }` produces:
  - `Name` = `"Direction"`
  - `ChunkType` = `Type`
- [ ] **Exported arrow function test:** parsing `export const handler = (req: Request): Response => { return new Response(); }` produces:
  - `Name` = `"handler"`
  - `ChunkType` = `Function`
  - `Signature` contains text up through the `=>`
- [ ] **Non-exported arrow function ignored test:** parsing `const helper = (x: number) => x * 2;` returns an empty `[]RawChunk` (non-exported top-level arrow functions are not extracted)
- [ ] **Multiple declarations test:** a source with 1 function, 1 class, 1 interface, and 1 exported arrow function returns exactly 4 chunks in source order
- [ ] **TSX-specific test:** parsing TSX source containing a function component `function App(): JSX.Element { return <div />; }` with the TSX parser produces a valid `RawChunk` with `Name` = `"App"` and `ChunkType` = `Function`
- [ ] **No declarations test:** parsing `import { foo } from 'bar';\nconsole.log(foo);` returns an empty `[]RawChunk`
