# Changelog

## [2.0.0](https://github.com/jrschumacher/wails-kit/compare/v1.2.0...v2.0.0) (2026-03-16)


### ⚠ BREAKING CHANGES

* **llm:** consumers importing `wails-kit/llm` should migrate to `settings/templates/anyllm` with any-llm-go.

### Features

* add CI pipeline for publishing split Go modules ([#86](https://github.com/jrschumacher/wails-kit/issues/86)) ([5192cc1](https://github.com/jrschumacher/wails-kit/commit/5192cc138b7c032a163565fc892335c38b3afa14))
* cross-platform builds and managed install detection ([#88](https://github.com/jrschumacher/wails-kit/issues/88)) ([e637910](https://github.com/jrschumacher/wails-kit/commit/e637910cad8e535632b8ed29a31532c69615076e))
* **llm:** remove llm package ([#81](https://github.com/jrschumacher/wails-kit/issues/81)) ([0218119](https://github.com/jrschumacher/wails-kit/commit/0218119225e7425a06fdd2d8df6325f3a4ce44f3))
* **settings:** add headless/CLI adapter ([#82](https://github.com/jrschumacher/wails-kit/issues/82)) ([525916d](https://github.com/jrschumacher/wails-kit/commit/525916df8611e152b1a7330f8a95f7a846ad041e))
* **settings:** add WithStoragePath for workspace-local configs ([#84](https://github.com/jrschumacher/wails-kit/issues/84)) ([a0ab769](https://github.com/jrschumacher/wails-kit/commit/a0ab7692aef2353079c8f0a82ffd9689b8a080e9))
* **state:** add generic typed state persistence package ([#85](https://github.com/jrschumacher/wails-kit/issues/85)) ([b1f8250](https://github.com/jrschumacher/wails-kit/commit/b1f825033bdc9e34b5d2f4d7458fb8a230dc9410))


### Bug Fixes

* **taskfiles:** fix sed in-place edit on macOS ([#89](https://github.com/jrschumacher/wails-kit/issues/89)) ([9d17128](https://github.com/jrschumacher/wails-kit/commit/9d17128736a9377c8fe4140bdb35a8a84623a2c4))

## [1.2.0](https://github.com/jrschumacher/wails-kit/compare/v1.1.0...v1.2.0) (2026-03-09)


### Features

* **database:** add schema version guard and pre-migration backup ([#70](https://github.com/jrschumacher/wails-kit/issues/70)) ([cf098cc](https://github.com/jrschumacher/wails-kit/commit/cf098cca77660727e31cb3891f4d26e5361ee525))

## [1.1.0](https://github.com/jrschumacher/wails-kit/compare/v1.0.1...v1.1.0) (2026-03-09)


### Features

* **database:** add WithBaselineVersion for pre-existing schemas ([#67](https://github.com/jrschumacher/wails-kit/issues/67)) ([57f953b](https://github.com/jrschumacher/wails-kit/commit/57f953b1f2406d89ca45dc983ea883c1c50d78d2)), closes [#66](https://github.com/jrschumacher/wails-kit/issues/66)

## [1.0.1](https://github.com/jrschumacher/wails-kit/compare/v1.0.0...v1.0.1) (2026-03-09)


### Bug Fixes

* **release:** fix darwin build, entitlements, cask SHA, and local taskfile docs ([#64](https://github.com/jrschumacher/wails-kit/issues/64)) ([02f97a0](https://github.com/jrschumacher/wails-kit/commit/02f97a08e677b00e7c2997bf124dcf91e0201101))

## 1.0.0 (2026-03-09)


### Features

* **appdirs:** add OS-standard app directory paths ([#11](https://github.com/jrschumacher/wails-kit/issues/11)) ([e23880e](https://github.com/jrschumacher/wails-kit/commit/e23880ee5afcd060fe4fc4671338f682a2373186))
* **database:** add SQLite database package with goose migrations ([#22](https://github.com/jrschumacher/wails-kit/issues/22)) ([226de00](https://github.com/jrschumacher/wails-kit/commit/226de002f8ba33d73cd7074957a93ee1f384111d))
* **diagnostics:** add support bundle creation ([#23](https://github.com/jrschumacher/wails-kit/issues/23)) ([0f0f70c](https://github.com/jrschumacher/wails-kit/commit/0f0f70c29705f821051478a72b26899f2e492d0d))
* **diagnostics:** phase 2 implementation ([#48](https://github.com/jrschumacher/wails-kit/issues/48)) ([38f4eaf](https://github.com/jrschumacher/wails-kit/commit/38f4eaf762d483989708cfcd167601d3e7a55c0b))
* **events:** add event history, replay, and middleware pipeline ([#41](https://github.com/jrschumacher/wails-kit/issues/41)) ([57836af](https://github.com/jrschumacher/wails-kit/commit/57836af4ee68d5271439e89640abb3826ba11d4b))
* **events:** add multi-window IPC and typed pub/sub ([#17](https://github.com/jrschumacher/wails-kit/issues/17)) ([b13295b](https://github.com/jrschumacher/wails-kit/commit/b13295bed40e9430b6019394d0c0d83135f37ace))
* **events:** add scoped emitters, async option, and unsubscribe ([#46](https://github.com/jrschumacher/wails-kit/issues/46)) ([a2fae13](https://github.com/jrschumacher/wails-kit/commit/a2fae1381bd4d49f6cc9069f2bd925f7ae6c8540))
* **frontend:** add TypeScript typecheck and lint CI jobs ([#28](https://github.com/jrschumacher/wails-kit/issues/28)) ([ebc849f](https://github.com/jrschumacher/wails-kit/commit/ebc849feb2765a1648aa7c93cba71e1463c6c5ee))
* **frontend:** add TypeScript types for settings, events, errors ([#16](https://github.com/jrschumacher/wails-kit/issues/16)) ([bcbfe9b](https://github.com/jrschumacher/wails-kit/commit/bcbfe9b19f8a1305d5b8ef4551ebe148fdcd6dfb))
* **lifecycle:** add service lifecycle manager with dependency ordering ([#21](https://github.com/jrschumacher/wails-kit/issues/21)) ([c56b2ec](https://github.com/jrschumacher/wails-kit/commit/c56b2ec000e374bf92f776202c20c27fa63432f9))
* **lifecycle:** add startup/shutdown timeouts and health checks ([#38](https://github.com/jrschumacher/wails-kit/issues/38)) ([beaabe8](https://github.com/jrschumacher/wails-kit/commit/beaabe8204b3ebecc4504414666174f7245e6828))
* **llm:** add per-model token budgets and configurable context builder ([#40](https://github.com/jrschumacher/wails-kit/issues/40)) ([213ca15](https://github.com/jrschumacher/wails-kit/commit/213ca15373d9199f2fa22712f4c58e1b9220a495)), closes [#30](https://github.com/jrschumacher/wails-kit/issues/30)
* **settings:** add error code to ValidationError for field-level errors ([#42](https://github.com/jrschumacher/wails-kit/issues/42)) ([de1d0a3](https://github.com/jrschumacher/wails-kit/commit/de1d0a340099c8894b9ae57514b506df4bd2e28b)), closes [#34](https://github.com/jrschumacher/wails-kit/issues/34)
* **settings:** add headless TypeScript settings logic library ([#45](https://github.com/jrschumacher/wails-kit/issues/45)) ([ac71cf7](https://github.com/jrschumacher/wails-kit/commit/ac71cf7bc2c89bbe4c405b0e7e57a9deb43e5532))
* **shortcuts:** add native menu shortcuts package ([#44](https://github.com/jrschumacher/wails-kit/issues/44)) ([87caa18](https://github.com/jrschumacher/wails-kit/commit/87caa18371a3470eef4a2f8ff6b42c773bc0d05d))
* **updates:** add auto-update service and shared release taskfiles ([#2](https://github.com/jrschumacher/wails-kit/issues/2)) ([ee9acbe](https://github.com/jrschumacher/wails-kit/commit/ee9acbeef2532d3e32ba531acaae522c7383abb5))
* **updates:** add Ed25519 binary signature verification ([#43](https://github.com/jrschumacher/wails-kit/issues/43)) ([11616da](https://github.com/jrschumacher/wails-kit/commit/11616daa2680ece9b5c408b5d860450ebbcb34b6))


### Bug Fixes

* **ci:** add checkout step to release workflow ([#49](https://github.com/jrschumacher/wails-kit/issues/49)) ([0608bc3](https://github.com/jrschumacher/wails-kit/commit/0608bc38e937037ac36a8532a478d93592afb398))
* **ci:** fall back to github.token for release ([#54](https://github.com/jrschumacher/wails-kit/issues/54)) ([5f49e81](https://github.com/jrschumacher/wails-kit/commit/5f49e81cbcb8efdec7e8a81f20c081cade45d0f9))
* **ci:** skip conventional commits check on release-please PRs and start at v0.1.0 ([#56](https://github.com/jrschumacher/wails-kit/issues/56)) ([55e7893](https://github.com/jrschumacher/wails-kit/commit/55e78939f25d7006fdd662452c7800e0504cd532))
* cover frontend checks and sync update error types ([#50](https://github.com/jrschumacher/wails-kit/issues/50)) ([1842d80](https://github.com/jrschumacher/wails-kit/commit/1842d8057d7a2d17bc3c100dd3f77782463c9451))
* **errors:** add JSON tags to UserError and use ErrorCode type ([#27](https://github.com/jrschumacher/wails-kit/issues/27)) ([001dbaa](https://github.com/jrschumacher/wails-kit/commit/001dbaa718d9343566f367dbf8ab0e66bb5b32ff))
* **llm:** log warning on malformed tool input JSON in OpenAI provider ([#39](https://github.com/jrschumacher/wails-kit/issues/39)) ([b77bcae](https://github.com/jrschumacher/wails-kit/commit/b77bcae3868bd13a8e034a14fd293b6b10237250)), closes [#35](https://github.com/jrschumacher/wails-kit/issues/35)
* resolve critical bugs across all packages ([#10](https://github.com/jrschumacher/wails-kit/issues/10)) ([4f2755f](https://github.com/jrschumacher/wails-kit/commit/4f2755fec8e14ae31801663547986baf838b5ec8))
