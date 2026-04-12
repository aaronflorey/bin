# Changelog

## [2.3.0](https://github.com/aaronflorey/bin/compare/v2.2.0...v2.3.0) (2026-04-12)


### Features

* **config:** Add gh token auth toggle and key hints ([e07d6bd](https://github.com/aaronflorey/bin/commit/e07d6bd090bdc16816bbc0feb43ad87e36292270))
* **run:** Auto-pass args and prune stale cached versions ([8e63212](https://github.com/aaronflorey/bin/commit/8e63212054ece00973136cce5bd909dad895d8dc))


### Bug Fixes

* **providers:** Satisfy staticcheck in GitHub auth guard ([75f3c55](https://github.com/aaronflorey/bin/commit/75f3c55d3227112809f256e7f49f8492f1cb6209))

## [2.2.0](https://github.com/aaronflorey/bin/compare/v2.1.2...v2.2.0) (2026-04-12)


### Features

* fix releases with multiple tools ([2a9ced6](https://github.com/aaronflorey/bin/commit/2a9ced652b9a2c6c58dff0c27d080868917b1422))
* **import:** Run ensure after import by default ([20ce645](https://github.com/aaronflorey/bin/commit/20ce645ee8630ae6a65fa4790f9c5daf0d4be39d))
* **run:** Add cached run command ([321fd3c](https://github.com/aaronflorey/bin/commit/321fd3cf317430092deeafcc1d44b296c0e7278f))
* **system-package:** Add opt-in system package install mode ([aace7b0](https://github.com/aaronflorey/bin/commit/aace7b04307de00c2633abb57229d16d6a3b9f52))


### Bug Fixes

* **assets:** Ignore metadata package names ([25d2b4c](https://github.com/aaronflorey/bin/commit/25d2b4c280985d7d278a08844d90f54a0ea618d8))
* **assets:** Skip package manager release artifacts ([b444d30](https://github.com/aaronflorey/bin/commit/b444d30bc23a23478d79bc671ee4b7842c33c003))
* **ensure:** Retry without package path on archive mismatch ([d6c3f83](https://github.com/aaronflorey/bin/commit/d6c3f83b4fce870b42576bc1e7a9b2850ba1b22d))

## [2.1.2](https://github.com/aaronflorey/bin/compare/v2.1.1...v2.1.2) (2026-04-08)


### Bug Fixes

* don't download .sbom.json files ([a1bf130](https://github.com/aaronflorey/bin/commit/a1bf130f7675a2dd4d0d05827ca1a3623c9b68fb))

## [2.1.1](https://github.com/aaronflorey/bin/compare/v2.1.0...v2.1.1) (2026-04-07)


### Bug Fixes

* panic in tie breaker code ([81c5ced](https://github.com/aaronflorey/bin/commit/81c5ced023599b81014ef20357b85f1b9f4a5a95))

## [2.1.0](https://github.com/aaronflorey/bin/compare/v2.0.0...v2.1.0) (2026-04-07)


### Features

* add generic url install ([49afd03](https://github.com/aaronflorey/bin/commit/49afd03c3efed20df456bcb33040d413edbee742))
* add support for multiple inputs when installing ([2ac472b](https://github.com/aaronflorey/bin/commit/2ac472beebef74db544e1488529b47a95fd6c8c3))


### Bug Fixes

* checksum issues ([94f2baf](https://github.com/aaronflorey/bin/commit/94f2baffd6b9640f0d46f11c4f5d0cbc51bb6cca))
* improve spinner code ([b679337](https://github.com/aaronflorey/bin/commit/b679337c28e44feb54475b323b38f425b05192de))
* **install:** honor interactive overwrite confirmation ([f099d42](https://github.com/aaronflorey/bin/commit/f099d422934241e4fc7b94bd3ee185acdda969b0))
* outdated output was broken ([1dfe42c](https://github.com/aaronflorey/bin/commit/1dfe42ce58853d39c6a810ddbf026dd6d885ab71))
* tie breaker breaking on negative scores ([c8c7638](https://github.com/aaronflorey/bin/commit/c8c7638b1f9a51e5b003bf465984da479fd894c5))

## [2.0.0](https://github.com/aaronflorey/bin/compare/v1.1.0...v2.0.0) (2026-04-01)


### Features

* add config-set command ([22d76e4](https://github.com/aaronflorey/bin/commit/22d76e49700a1ee3633bffb8f43f6e2f8e43d105))
* add GitHub Action for installing bin and managing binaries ([#4](https://github.com/aaronflorey/bin/issues/4)) ([ff211e3](https://github.com/aaronflorey/bin/commit/ff211e3d4bda2e35c6b1c018b0544adea5f311a7))
* add install script ([e8db3c6](https://github.com/aaronflorey/bin/commit/e8db3c6f2cfe33d5705aded0bfc1dda2afeff654))
* add min age for updates ([b9302d4](https://github.com/aaronflorey/bin/commit/b9302d45f62509c695e26f1e6fd9b9d6f11de608))
* add outdated command ([0881a36](https://github.com/aaronflorey/bin/commit/0881a36d985a7515f26049eecedee14796674f58))
* add version command ([b8543a2](https://github.com/aaronflorey/bin/commit/b8543a2febb92d621ee12b2bb1147f0170acdf3c))
* **cli:** add processing spinner hooks ([8dab9ee](https://github.com/aaronflorey/bin/commit/8dab9ee5bed6ae0af49a598ab1841c5a7de1eb60))
* **config:** add default_chmod option to control file permissions after install ([feef4c8](https://github.com/aaronflorey/bin/commit/feef4c812dff8a3293b4dc277d122d69098cf32e))
* **docker:** query Docker Hub for latest tags ([0843f5d](https://github.com/aaronflorey/bin/commit/0843f5d2f242c0fc4e7f01c13968745c804b392d))
* **docker:** support configurable wrapper run template ([80048e4](https://github.com/aaronflorey/bin/commit/80048e4af61f397c060a671660c8fddcb9ddcdce))
* **goinstall:** support sub-path in goinstall:// URLs ([640f47e](https://github.com/aaronflorey/bin/commit/640f47eba20c2017065fa67321a95ceb2e0f51fd))
* **hooks:** add pre/post lifecycle hooks for install, update, and remove ([16e7acf](https://github.com/aaronflorey/bin/commit/16e7acfb33744068017e93f2a776dc23b1737518))
* improve tie breaking of multiple options, add non-interactive flag ([9285419](https://github.com/aaronflorey/bin/commit/92854193640b28cd0fa6c110a77709a1805fa3b9))
* **install:** add --select flag for non-interactive asset selection and BIN_EXE_DIR env var ([1680836](https://github.com/aaronflorey/bin/commit/168083649755d5d48157638502e087705c084cdd))
* **installer:** enforce overwrite checks in path resolution ([1fbd659](https://github.com/aaronflorey/bin/commit/1fbd659b7b01a98772f29b9890f5839e0e367e81))
* **installer:** warn on duplicate managed binary hashes ([d713691](https://github.com/aaronflorey/bin/commit/d713691cfec877440a90ae9892810ae53a18ba1c))
* **install:** prefer release metadata for asset resolution ([2b79f29](https://github.com/aaronflorey/bin/commit/2b79f29bb6305586c79bab1c245dd9739d0fdba6))
* **install:** update existing binaries instead of duplicating ([9c249fd](https://github.com/aaronflorey/bin/commit/9c249fdf6a87bbc764ec79149901c994265fba47))
* **providers:** verify downloads against sha256 checksums ([79edcf5](https://github.com/aaronflorey/bin/commit/79edcf535ff964cc3d8a86ed64e087b53c0a8074))
* **remove:** add provider-specific cleanup hooks ([aca6c4a](https://github.com/aaronflorey/bin/commit/aca6c4a3e445033193df37ed87ff81a12fc318d3))
* resolve musl vs gnu for linux ([533e6d2](https://github.com/aaronflorey/bin/commit/533e6d2abd784431141de661bb8acff131edca3b))
* **update:** add configurable parallel update discovery ([fd2d3dc](https://github.com/aaronflorey/bin/commit/fd2d3dc47c3888345bba5811061f9110f9fbbfe6))
* **update:** support updating binaries by URL target ([67c66e2](https://github.com/aaronflorey/bin/commit/67c66e2b72a4b4dfb3bbd405fcf750790674185e))


### Bug Fixes

* $HOME expansion ([e357377](https://github.com/aaronflorey/bin/commit/e357377493c63ef38315afb95174bec3f1e8e4e0))
* **ci:** quote installer smoke job name ([3ea87a6](https://github.com/aaronflorey/bin/commit/3ea87a6d003fffc125863f50c136289afc2191d2))
* **config:** use cross-platform directory write probe ([18da200](https://github.com/aaronflorey/bin/commit/18da2004830789c27bd4f46a66c0680b05ed1c7e))
* **deps:** upgrade go-github to v73 and goldmark to v1.8.1 ([be1143d](https://github.com/aaronflorey/bin/commit/be1143d634885edf68c4d6c19f4864b334e48855))
* ensure config path is present ([91875ae](https://github.com/aaronflorey/bin/commit/91875aeb9f7c5730a6265d81080309d83dbe21cd))
* **gitlab:** return clear missing release errors ([32fa31d](https://github.com/aaronflorey/bin/commit/32fa31ddde287757fe2d94f948e85162aba0610f))
* **install,ci:** harden asset resolution and test installer script ([7290d60](https://github.com/aaronflorey/bin/commit/7290d603160001880b6d24e88a99ac882da87a5f))
* **installer:** improve home path matching ([44c42fc](https://github.com/aaronflorey/bin/commit/44c42fce143c19b79b96c4e8394d06f7dbf37263))
* **install:** preseed config for non-interactive bootstrap ([e23f0f5](https://github.com/aaronflorey/bin/commit/e23f0f5207729d5275d244ccf87c83579880d80b))
* lint failure in update ([dd0d1c6](https://github.com/aaronflorey/bin/commit/dd0d1c6d516aeb3886dec9a60ee88227eef7cb8f))
* **list:** correct column alignment when displaying pinned versions ([9a25872](https://github.com/aaronflorey/bin/commit/9a25872f5f22826413cdb664c86b59d8a61c5385))
* some tech debt ([9506f5a](https://github.com/aaronflorey/bin/commit/9506f5a1797d483e1d920e1194a1979eb7c30784))
* **update:** return non-zero exit when partial updates fail ([5879a82](https://github.com/aaronflorey/bin/commit/5879a82b8063f17910b5c90a8a847083ca1efa41))
* various fixes for edge case releases ([71b77cc](https://github.com/aaronflorey/bin/commit/71b77cc63f1995a821e1eb8d91787b81b150ea99))


### Performance Improvements

* **assets:** stream archive candidates to temp files ([c11eaca](https://github.com/aaronflorey/bin/commit/c11eaca232ce66deee00db1b045832007976d996))


### Miscellaneous Chores

* bump ([7b7e741](https://github.com/aaronflorey/bin/commit/7b7e741237c401eb2980dc6cf85420de1baa9f1a))

## [1.1.0](https://github.com/aaronflorey/bin/compare/v1.0.0...v1.1.0) (2026-03-24)


### Features

* Add `go install` ([#232](https://github.com/aaronflorey/bin/issues/232)) ([6a0f17e](https://github.com/aaronflorey/bin/commit/6a0f17e2c8b8ec71f9ed99570d6c94eb8910ed28))
* add github url normalisation ([d1be891](https://github.com/aaronflorey/bin/commit/d1be891d925af953d17d41db8845f93aebbc38d7))
* add import/export commands for portability ([07e3e09](https://github.com/aaronflorey/bin/commit/07e3e0960ba9ba9426dcc471e2f4116c92b55386))
* Bump Go version to 1.19 ([#150](https://github.com/aaronflorey/bin/issues/150)) ([963a6f4](https://github.com/aaronflorey/bin/commit/963a6f4010306b4efb3c32b5456fe7e66b4dba44))
* Bump Go version to 1.20 ([#169](https://github.com/aaronflorey/bin/issues/169)) ([eb65e2d](https://github.com/aaronflorey/bin/commit/eb65e2d4dbc28e71954d70cc063c514fb85eb127))
* Detect Go version in the pipeline ([#175](https://github.com/aaronflorey/bin/issues/175)) ([f715904](https://github.com/aaronflorey/bin/commit/f715904e0c3f503f9daaeca437ce7d17f6c926d4))
* filter releases by arch and remove common unwanted assets ([369652d](https://github.com/aaronflorey/bin/commit/369652d5208043885b23e9578d74fb1d704a273f))
* filter releases by arch and remove common unwanted assets ([44f5156](https://github.com/aaronflorey/bin/commit/44f5156bf050a0a54d85dc660f5fa23cfaa3c5e3))
* improves `bin ls` output ([#196](https://github.com/aaronflorey/bin/issues/196)) ([24eae61](https://github.com/aaronflorey/bin/commit/24eae6131ef72d9f34fecb28d1c2147ce6bbb780))


### Bug Fixes

* **docker:** init client from env variables available to configure docker ([#235](https://github.com/aaronflorey/bin/issues/235)) ([21392fe](https://github.com/aaronflorey/bin/commit/21392fef66be73e7381ab4488c1834174ad499c6))
* **ensure,update:** persist provider/path ([#254](https://github.com/aaronflorey/bin/issues/254)) ([7e93aa5](https://github.com/aaronflorey/bin/commit/7e93aa50b7238155f3c9bcac786fdee1be292a49))
