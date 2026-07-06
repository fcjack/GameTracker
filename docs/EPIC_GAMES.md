# Epic Games — not supported

GameTracker does **not** support linking Epic Games accounts or importing a user's Epic Games Store library.

## Why

Epic exposes a library API used by desktop clients (Legendary, Playnite, the Epic Games Launcher):

```
GET https://library-service.live.use1a.on.epicgames.com/library/api/public/items?includeMetadata=true&platform=Windows
```

That endpoint requires an **`eg1~` session token** issued by the Epic Games Launcher OAuth flow on `account-public-service-prod03.ol.epicgames.com`.

OAuth apps created in the [Epic Developer Portal](https://dev.epicgames.com/) (`api.epicgames.dev`, `basic_profile` scope) return **`epic_id` JWT access tokens**. Those tokens authenticate successfully for profile endpoints but **cannot access the library service** — requests are rejected with permission errors.

We explored several workarounds during development (token exchange, `external_auth`, launcher-style OAuth). None are viable for a self-hosted web application using a custom Developer Portal OAuth client:

- **`external_auth`** — rejected by Epic's account service
- **Exchange token code** — `403` missing `account:oauth:exchangeTokenCode CREATE`
- **Strict `eg1~` validation** — produced false "re-link your account" errors for otherwise valid-looking tokens

There is no supported path for this project to import EGS libraries the way Steam and Xbox integrations work.

## What remains in the codebase

- **Database columns** `epic_catalog_item_id` and `epic_namespace` on `games` (migration `017_epic_games.sql`) — kept for existing data; not used for import
- **IGDB game detail** may show an Epic Games Store link when metadata includes it — display only, not account linking

## Related issues

Epic integration issues ([#35](https://github.com/fcjack/GameTracker/issues/35), [#44](https://github.com/fcjack/GameTracker/issues/44), [#45](https://github.com/fcjack/GameTracker/issues/45)) were closed when the implementation was removed.
