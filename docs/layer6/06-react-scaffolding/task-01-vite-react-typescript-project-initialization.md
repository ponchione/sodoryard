# Task 01: Vite + React + TypeScript Project Initialization

**Epic:** 06 — React Scaffolding
**Status:** ⬚ Not started
**Dependencies:** Epic 01 (HTTP Server Foundation — for embed.FS integration)

---

## Description

Initialize the React frontend project in the `web/` directory at the project root using Vite with React and TypeScript. Set up `package.json` with all required dependencies, configure TypeScript with strict settings, and verify the project builds cleanly. This is the foundation that all frontend epics build on.

## Acceptance Criteria

- [ ] `web/` directory exists at the project root (not inside `internal/`)
- [ ] Vite + React + TypeScript project initialized (via `npm create vite@latest` or manual setup)
- [ ] `package.json` includes production dependencies: `react`, `react-dom`, `react-router-dom`
- [ ] `package.json` includes dev dependencies: `typescript`, `vite`, `@vitejs/plugin-react`, `@types/react`, `@types/react-dom`
- [ ] `tsconfig.json` configured with strict mode enabled (`strict: true`), path aliases if desired (e.g., `@/` maps to `src/`)
- [ ] `vite.config.ts` configured with the React plugin and output to `dist/`
- [ ] `npm install` completes without errors
- [ ] `npm run build` produces `web/dist/` with `index.html` and asset files
- [ ] `npm run dev` starts the Vite dev server on port 5173 (default)
- [ ] The project structure follows conventions: `web/src/main.tsx` (entry point), `web/src/App.tsx` (root component), `web/index.html`
- [ ] `web/dist/` is listed in the project's `.gitignore` (build artifacts should not be committed)
- [ ] `web/node_modules/` is listed in the project's `.gitignore`
- [ ] Frontend compiles with zero TypeScript errors
