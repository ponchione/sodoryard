# Task 02: Tailwind CSS and shadcn/ui Setup

**Epic:** 06 — React Scaffolding
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Install and configure Tailwind CSS and shadcn/ui in the React project. Tailwind provides utility-first CSS classes. shadcn/ui provides a set of accessible, customizable React components built on Radix UI primitives. Configure a dark theme as the default (consistent with coding tool aesthetics) using CSS variables so shadcn/ui components theme correctly.

## Acceptance Criteria

- [ ] Tailwind CSS installed and configured — `tailwindcss` and either `@tailwindcss/vite` plugin or PostCSS plugin set up in `vite.config.ts`
- [ ] Tailwind utility classes render correctly in components (verified by adding a test class like `bg-blue-500` and seeing the color)
- [ ] shadcn/ui initialized via `npx shadcn-ui@latest init` — `components.json` created with project settings
- [ ] Core shadcn/ui components installed: Button, Card, Input, ScrollArea, Separator (at minimum)
- [ ] Components installed to `web/src/components/ui/` — these are owned source files, not node_modules
- [ ] `cn()` utility function available (from shadcn/ui's `lib/utils.ts`) for conditional class merging
- [ ] **Dark theme:** Dark theme is the default. CSS variables for colors defined in `globals.css` or equivalent. The `dark` class is applied to the root `<html>` element or Tailwind's `darkMode` is configured
- [ ] shadcn/ui components render correctly with the dark theme — text is light on dark backgrounds, borders and accents are visible
- [ ] Color palette is appropriate for a coding tool (subdued backgrounds, clear text contrast, accent colors for interactive elements)
- [ ] Light theme support is optional for v0.1 — a toggle is not required, but the CSS variable approach should not preclude adding one later
- [ ] No CSS conflicts between Tailwind's reset and shadcn/ui component styles
