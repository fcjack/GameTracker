# Screenshots

Images in this folder are used by the [README](../../README.md). Regenerate them after major UI changes.

## Capture

With the app running locally:

```bash
# macOS / Docker Desktop (app on host port 8080)
make screenshots \
  APP_URL=http://host.docker.internal:8080 \
  SCREENSHOT_USER=your-username \
  SCREENSHOT_PASSWORD=your-password \
  SCREENSHOT_GAME_ID=820

# Linux (app on localhost:8080; Makefile uses --network host)
make screenshots \
  APP_URL=http://localhost:8080 \
  SCREENSHOT_USER=your-username \
  SCREENSHOT_PASSWORD=your-password \
  SCREENSHOT_GAME_ID=820
```

`SCREENSHOT_GAME_ID` is optional; when set, also writes `game-detail.png`.

After capturing authenticated pages, add the images to the README screenshot table if desired.

## Files

| File | Page |
|------|------|
| `login.png` | Sign-in |
| `dashboard.png` | Dashboard (stats + platform playtime) |
| `library.png` | Library grid |
| `game-detail.png` | Game detail (optional) |
| `profile.png` | Profile (linked accounts) |
