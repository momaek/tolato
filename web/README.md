# Tolato Web

Vue 3 + Vite + TypeScript frontend. Talks to the Go server via REST + WebSocket.

See the [root README](../README.md) for the full stack.

## Stack

- Vue 3 (`<script setup>`) + TypeScript
- Vite 8
- Pinia (stores: `app`, `chat`, `nodes`, `monitor`, `settings`)
- Vue Router + auth guard
- shadcn-vue (Radix + Tailwind v4) — components in [src/components/ui](src/components/ui)
- markstream-vue + Shiki — streaming Markdown in chat
- Chart.js — probe metric charts
- vue-sonner — toasts
- vue-i18n

## Scripts

```sh
pnpm install
pnpm dev      # vite dev server on :5173
pnpm build    # vue-tsc + vite build
pnpm preview  # preview production build
```

Dev server expects the Go server on `:8080`. See [vite.config.ts](vite.config.ts) for proxy config.

## Layout

```
src/
├── components/
│   ├── chat/      # chat UI (message types, tool cards, confirm card)
│   ├── layout/    # app shell + sidebar + conversation list
│   ├── monitor/   # topology canvas, node cards, metric charts
│   └── ui/        # shadcn-vue primitives
├── composables/   # useAutoScroll, useTheme, etc.
├── services/      # api.ts (REST), ws.ts (chat WS client)
├── stores/        # Pinia
├── views/         # route-level pages
├── router/
├── i18n/
└── types/         # API + WS type definitions
```
