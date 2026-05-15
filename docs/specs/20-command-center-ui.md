# 20 - Command Center UI

**Status:** Superseded
**Last Updated:** 2026-05-01
**Owner:** Mitchell

---

## Superseded Direction

The browser-first command center direction has been replaced by a desktop-first split:

- [[24-electron-desktop-app]] is the active daily-driver operator console spec.
- [[21-web-inspector]] is the active browser inspector spec.

The old command-center plan tried to make the React app own project readiness, launch drafting, agent selection, chain control, document intake, metrics, project browsing, and rich context inspection. That would have made the browser the primary product surface.

The new direction is narrower and better aligned with how Yard is used:

- desktop-first for normal operation through Yard Desktop
- CLI-first for scripts and one-shot commands
- browser-only where rich inspection genuinely pays for itself

Do not implement this file as a browser route/API build target. Treat it as a compatibility pointer for old links and discussions.
