# rbitr UI

The rbitr UI is a web-based interface for managing user controls.

## Development

```bash
npm install
npm run dev
```

The UI uses Vite + React + TypeScript + shadcn/ui (Radix primitives + Tailwind CSS v4). Set
`VITE_API_BASE_URL` to point at the gateway API (e.g., `http://localhost:8080`).

## Design system

Design tokens live in `src/styles/globals.css` as OKLCH CSS variables with light and dark values.
The brand purple is single-sourced as `--brand`; neutrals are tinted toward the brand hue. All
token pairs are calibrated to WCAG 2.1 AA (4.5:1 minimum).

### Color rules

- Never use raw Tailwind palette classes (`text-red-500`, `bg-emerald-100`, ...). Use tokens.
- Status semantics use the semantic tokens: `success`, `warning`, `destructive`, each with a
  `-subtle` background variant (e.g. `text-success`, `bg-warning-subtle`).
- Theme switching toggles the `dark` class on `<html>` (see `src/components/theme-provider.tsx`).
  Every color decision must hold up in both themes.

### Status display

Use the shared components instead of hand-rolled badge styling:

- `<StatusBadge status={value} />` (`src/components/status-badge.tsx`) — renders a status string
  with a tone inferred from its value (`approved`/`executed`/`resolved` → success,
  `pending`/`executing` → warning, `failed`/`denied`/`expired` → danger, anything else → neutral).
  Pass `tone` to override the inferred mapping.
- `<Badge variant="success" | "warning" | "danger">` — for status chips whose label is not the
  status value itself (e.g. "Passed" / "Failed"). `destructive` remains the solid high-emphasis
  variant; the status variants use subtle backgrounds.
