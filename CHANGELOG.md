# Changelog

## [0.5.0](https://github.com/fcjack/GameTracker/compare/v0.4.0...v0.5.0) (2026-06-28)


### Features

* **library:** add platform filter to library page ([9f36590](https://github.com/fcjack/GameTracker/commit/9f36590a1de4e8c64f5708686088484b480b40f0))
* **profile:** add Steam and Xbox account unlink ([a6289c9](https://github.com/fcjack/GameTracker/commit/a6289c92dda92f296e670e1f78db934d46f3ff1e))

## [0.4.0](https://github.com/fcjack/GameTracker/compare/v0.3.0...v0.4.0) (2026-06-28)


### Features

* add Xbox account linking via Microsoft OAuth ([9528953](https://github.com/fcjack/GameTracker/commit/9528953849efa884dafacedb79fb7c1f34cde28e))
* **db:** add xbox_title_id column and Xbox game model helpers ([829efd5](https://github.com/fcjack/GameTracker/commit/829efd56cca05afb6bfe0415e5e45dd9efd6853c))
* **igdb:** add Xbox title ID lookup with name fallback ([e2d434f](https://github.com/fcjack/GameTracker/commit/e2d434f7b9c9e7ffaae68ff4f827a111f2ae8c2d))
* **import:** add cancel import for Steam and Xbox library sync ([381756b](https://github.com/fcjack/GameTracker/commit/381756bd4b1e8746eb601b4c9e883450daed528b))
* **import:** add Xbox library import job ([6f1aaec](https://github.com/fcjack/GameTracker/commit/6f1aaecb08c84b64074f6ac9400e058d8ac55bd9))
* wire Xbox library import routes, profile UI, and scheduled sync ([b42ca52](https://github.com/fcjack/GameTracker/commit/b42ca527c77a197de3cc3d0b1e275379c3b24752))
* **xbox:** add OAuth token refresh and EnsureFreshTokens helper ([ceb5159](https://github.com/fcjack/GameTracker/commit/ceb5159d9ccf2271731371ed7f78de5c9ebf1ae4))
* **xbox:** add Title Hub library client with Game Pass activity filter ([2c617c7](https://github.com/fcjack/GameTracker/commit/2c617c79abb6f7c6aebf39e9aa64559d55c15bc7))


### Bug Fixes

* **xbox:** auto-import on link and visible import progress bar ([7c761d6](https://github.com/fcjack/GameTracker/commit/7c761d6790d20a5d13e1086c26a60260dd6ec17d))

## [0.3.0](https://github.com/fcjack/GameTracker/compare/v0.2.1...v0.3.0) (2026-06-28)


### Features

* add scheduled background library sync ([4d11b67](https://github.com/fcjack/GameTracker/commit/4d11b67eafe0496ac131511a87657e6bd000ec68))
* store and display per-game play time on library cards ([56ec6a2](https://github.com/fcjack/GameTracker/commit/56ec6a211c274dda9f00a964741bd7666f3d395d)), closes [#22](https://github.com/fcjack/GameTracker/issues/22)
* toggle game status buttons back to backlog ([e049e71](https://github.com/fcjack/GameTracker/commit/e049e71b1cd0650e95d64521a7182e6ea8ac65f3))


### Bug Fixes

* format game playtime as hours and minutes on cards ([d79b02b](https://github.com/fcjack/GameTracker/commit/d79b02b2f1c1e1993ed60c8080798fe1c6218744))
* include played free games in Steam library import ([e085fc9](https://github.com/fcjack/GameTracker/commit/e085fc9ad89beab2cd597016df4cf3c6634f2f63))
* make profile account rows responsive on small screens ([d297fb1](https://github.com/fcjack/GameTracker/commit/d297fb1e36b4d3fe61ef52848960041792d7e63d))

## [0.2.1](https://github.com/fcjack/GameTracker/compare/v0.2.0...v0.2.1) (2026-06-10)


### Performance Improvements

* paginate library grid and serve built assets locally ([57cdd7b](https://github.com/fcjack/GameTracker/commit/57cdd7bf72ac170be27c9eeb2f7b4f0865412afa))

## [0.2.0](https://github.com/fcjack/GameTracker/compare/v0.1.1...v0.2.0) (2026-06-10)


### Features

* cache game covers with Steam-to-IGDB fallback and placeholder ([3c0caa4](https://github.com/fcjack/GameTracker/commit/3c0caa4476819da506006647632290b8268dc66f))

## [0.1.1](https://github.com/fcjack/GameTracker/compare/v0.1.0...v0.1.1) (2026-06-10)


### Miscellaneous Chores

* document conventional commits and release process ([086510d](https://github.com/fcjack/GameTracker/commit/086510d1eb1633116f6b3fd11a8464e8fb8d543c))

## 0.1.0 (2026-06-10)


### Miscellaneous Chores

* trigger v0.1.0 release ([795ab49](https://github.com/fcjack/GameTracker/commit/795ab49443d71502063570a2ff5bbd130fcfa7ba))
