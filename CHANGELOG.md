# Changelog

## [2.0.0](https://github.com/aaronflorey/bin/compare/v2.4.0...v2.0.0) (2026-05-20)


### Features

* Add `go install` ([#232](https://github.com/aaronflorey/bin/issues/232)) ([ac5b0f0](https://github.com/aaronflorey/bin/commit/ac5b0f0d074e9c79ec5017e155df73b9f8208b38))
* add config-set command ([5289cbc](https://github.com/aaronflorey/bin/commit/5289cbcaf4c53b041c480f05cee20cd343c1b67b))
* add generic url install ([17db23d](https://github.com/aaronflorey/bin/commit/17db23dbf6846528bddac93aca591ae31dc6de82))
* add GitHub Action for installing bin and managing binaries ([#4](https://github.com/aaronflorey/bin/issues/4)) ([1cc5c8f](https://github.com/aaronflorey/bin/commit/1cc5c8fddcb0c05d22819a092ff14034f8b68f19))
* add github url normalisation ([ec18313](https://github.com/aaronflorey/bin/commit/ec18313c193b7a92630ccb2feca01ed267badc56))
* add import/export commands for portability ([a687a55](https://github.com/aaronflorey/bin/commit/a687a5516487755ad858e03d0f3bd7a3c794eaa2))
* add install script ([a758a60](https://github.com/aaronflorey/bin/commit/a758a600e8b1e6362c791ffca3edcb8b7f04fa28))
* add min age for updates ([cdd23ad](https://github.com/aaronflorey/bin/commit/cdd23ad48aea8d53ff31d125ad605172dd8b66de))
* add outdated command ([f73e108](https://github.com/aaronflorey/bin/commit/f73e108221f7a95f162c21595f8cce99a157f516))
* add support for multiple inputs when installing ([a8a5250](https://github.com/aaronflorey/bin/commit/a8a525008f7fdb868dd81134d5d76f3b5bd1d63f))
* add version command ([56b0b06](https://github.com/aaronflorey/bin/commit/56b0b06959d119fa3a14399d6b98d2afc734687c))
* Bump Go version to 1.19 ([#150](https://github.com/aaronflorey/bin/issues/150)) ([5e9d193](https://github.com/aaronflorey/bin/commit/5e9d193705d9fdf5b16eeb77d0a4a6287ecef903))
* Bump Go version to 1.20 ([#169](https://github.com/aaronflorey/bin/issues/169)) ([31d8b24](https://github.com/aaronflorey/bin/commit/31d8b2474d21da1fad5db9f0bf0665b087ff70a2))
* **cli:** add processing spinner hooks ([f03f2d2](https://github.com/aaronflorey/bin/commit/f03f2d28fb55a8ea26765b3cafa0cfe0e57d4ac7))
* **cmd:** suggest managed names for partial targets ([f0c90a8](https://github.com/aaronflorey/bin/commit/f0c90a8113156493edb9834d75680f24ee6df7f6))
* **cmd:** suggest managed names for partial targets ([7b1dfa0](https://github.com/aaronflorey/bin/commit/7b1dfa0d145d0476a4648023e7aad90edf69886e))
* **config:** add default_chmod option to control file permissions after install ([f1796c2](https://github.com/aaronflorey/bin/commit/f1796c2b1aba174324843c5df6d6658780744661))
* **config:** Add gh token auth toggle and key hints ([a6077d7](https://github.com/aaronflorey/bin/commit/a6077d75cd334d049e1f71c4070f90d97f8e97dc))
* Detect Go version in the pipeline ([#175](https://github.com/aaronflorey/bin/issues/175)) ([bf56290](https://github.com/aaronflorey/bin/commit/bf562906a1888c8d75f318bda432a58039c77ea3))
* **docker:** query Docker Hub for latest tags ([18b5af5](https://github.com/aaronflorey/bin/commit/18b5af5ee1eced64b5fade8af6c2986d9230dd22))
* **docker:** support configurable wrapper run template ([d3d2129](https://github.com/aaronflorey/bin/commit/d3d21291765e3e52b349a92917488bfc4d09b96b))
* filter releases by arch and remove common unwanted assets ([5c90c50](https://github.com/aaronflorey/bin/commit/5c90c50c8ff04c2859445f75bf70ce475ad4298e))
* filter releases by arch and remove common unwanted assets ([76b6e02](https://github.com/aaronflorey/bin/commit/76b6e02c6bb1277e5acef7164fc0f9a27efcbd5a))
* fix releases with multiple tools ([02d0ebf](https://github.com/aaronflorey/bin/commit/02d0ebfeedf8ad4f4efb4a000456e0e79011afa3))
* **FN-002:** document multi-tool selection persistence ([5acd680](https://github.com/aaronflorey/bin/commit/5acd68045975e75eb4cde897203fcfde7be1f125))
* **goinstall:** support sub-path in goinstall:// URLs ([45a0ec0](https://github.com/aaronflorey/bin/commit/45a0ec062137ffb5e2c9db3915ef2e6886e2db36))
* **hooks:** add pre/post lifecycle hooks for install, update, and remove ([b0358f1](https://github.com/aaronflorey/bin/commit/b0358f12d54ca56acbb517331711158a4dc875ac))
* **import:** Run ensure after import by default ([c17a028](https://github.com/aaronflorey/bin/commit/c17a028c297a66dfbbbfe7b624986ef9cdf882d8))
* improve tie breaking of multiple options, add non-interactive flag ([af2cdf7](https://github.com/aaronflorey/bin/commit/af2cdf7df5a9619bc4859e60efa0a27c81b8e403))
* improves `bin ls` output ([#196](https://github.com/aaronflorey/bin/issues/196)) ([fabd4f2](https://github.com/aaronflorey/bin/commit/fabd4f22aadb7e4c95c7d701bbc7423add772aaf))
* **install:** add --select flag for non-interactive asset selection and BIN_EXE_DIR env var ([e37de2c](https://github.com/aaronflorey/bin/commit/e37de2cc834dbf5958f41503488bbf83d7a68c3d))
* **installer:** enforce overwrite checks in path resolution ([1376ddc](https://github.com/aaronflorey/bin/commit/1376ddcc44b8ca90c6478cc130514b166d7c9ba3))
* **installer:** warn on duplicate managed binary hashes ([48f0876](https://github.com/aaronflorey/bin/commit/48f0876a71f60eeedf63512b0402bf278c63c9f6))
* **install:** prefer release metadata for asset resolution ([eb501e8](https://github.com/aaronflorey/bin/commit/eb501e8f90af2026047cb208de9a9278d174f56b))
* **install:** support tracked dmg app installs ([7aaa6f6](https://github.com/aaronflorey/bin/commit/7aaa6f6fba0fe935495686997b1d61bdc3dffff0))
* **install:** update existing binaries instead of duplicating ([cfe511a](https://github.com/aaronflorey/bin/commit/cfe511a33fef8570681ea11e73b4458316e58138))
* **list:** add json output format ([f731032](https://github.com/aaronflorey/bin/commit/f731032e00045e4a1e4e91d28d4f3025a0fe5464))
* **providers:** verify downloads against sha256 checksums ([d59562f](https://github.com/aaronflorey/bin/commit/d59562f5b0b7d5e30d8a6a108db1cb0cff5062e3))
* **remove:** add interactive multi-select removal flow ([1b57777](https://github.com/aaronflorey/bin/commit/1b5777721e32877fdd1ed12f6f6f9e59f49def62))
* **remove:** add interactive multi-select removal flow ([a8975f9](https://github.com/aaronflorey/bin/commit/a8975f960075427f86bcc9d8ee7928d0d7d44f80))
* **remove:** add provider-specific cleanup hooks ([ecff3ad](https://github.com/aaronflorey/bin/commit/ecff3ad37d871b1d0e682f49a22006bd75a6f2ee))
* resolve musl vs gnu for linux ([a619c49](https://github.com/aaronflorey/bin/commit/a619c49e9940b95ae37bd155bf7292bee92ca8f3))
* **run:** Add cached run command ([0c535c0](https://github.com/aaronflorey/bin/commit/0c535c0893c2e7bd95f08ace7cc93b55a2416f6f))
* **run:** Auto-pass args and prune stale cached versions ([cdb4000](https://github.com/aaronflorey/bin/commit/cdb40001345afbc211df8833038356bf5bc6419d))
* **system-package:** Add opt-in system package install mode ([390fbeb](https://github.com/aaronflorey/bin/commit/390fbebc9dd68f59723211be59397d61ff2253b9))
* **tui:** add interactive changelog browser ([1c56be8](https://github.com/aaronflorey/bin/commit/1c56be8c6c7b985d2ce89dc3aa656f98f8697fed))
* **update:** add configurable parallel update discovery ([9601bbd](https://github.com/aaronflorey/bin/commit/9601bbdc3068c4c5694066ec51eb5a627a54c641))
* **update:** Add interactive multi-select for no-arg updates ([e6cc7ec](https://github.com/aaronflorey/bin/commit/e6cc7ecf4623dec84c2c3346f8d15e8a93944444))
* **update:** interactive multi-select for no-arg updates ([965c391](https://github.com/aaronflorey/bin/commit/965c3915ae870f1dd117c44bee2046e392952a7f))
* **update:** support updating binaries by URL target ([cbbef61](https://github.com/aaronflorey/bin/commit/cbbef616a1defd801f46d9356fc96949d7cacfa9))


### Bug Fixes

* $HOME expansion ([2ceb75e](https://github.com/aaronflorey/bin/commit/2ceb75e247b5191d227cc7bd108fb8cccff32b3c))
* **assets,config:** restore tar archive detection ([512e1e4](https://github.com/aaronflorey/bin/commit/512e1e4b6ec8525715101433dcd970ef26d91853))
* **assets:** detect macOS .app bundles in zip/tar archives ([9c85443](https://github.com/aaronflorey/bin/commit/9c85443996dbdde021731f0bb5cc090055d9714c))
* **assets:** filter out .blockmap metadata files from release assets ([ee999dd](https://github.com/aaronflorey/bin/commit/ee999dd50d6543f00a4433744483d3a8fad4a48a))
* **assets:** Ignore metadata package names ([63e8fa6](https://github.com/aaronflorey/bin/commit/63e8fa6eecbb9283d0cdcf37bbb58e44e357b23e))
* **assets:** prefer native Linux binaries over AppImages ([47e8dc9](https://github.com/aaronflorey/bin/commit/47e8dc916eabbadd759c3d8df1c91f4510eef5d5))
* **assets:** Skip package manager release artifacts ([790859b](https://github.com/aaronflorey/bin/commit/790859b467298682c1409ec9e084e0f001d8d9e1))
* checksum issues ([20b3e48](https://github.com/aaronflorey/bin/commit/20b3e487a2049b562ff2962c6c19d5bb87ed6fb7))
* chown config dir and file to sudo user when installing globally ([9232baa](https://github.com/aaronflorey/bin/commit/9232baa0f1350fefe8cd8c42b0ef0ff8a5fc1e1a))
* chown config dir/file to real user when installing via sudo ([c106ae9](https://github.com/aaronflorey/bin/commit/c106ae9cb96913637475a65900093ccf392d52c6))
* **ci:** quote installer smoke job name ([6ca4038](https://github.com/aaronflorey/bin/commit/6ca4038c7c46559859a879ae420d13a575b45e89))
* **ci:** resolve lint findings ([ac99115](https://github.com/aaronflorey/bin/commit/ac99115a7dbb27316f2c80572dcf31803c891ccd))
* **cmd,config:** harden install and config flows ([6bba9a0](https://github.com/aaronflorey/bin/commit/6bba9a0e0ba6c3817aaf857619d0dc8adf3440c6))
* **cmd:** remove tui changelog browser ([2fbdbee](https://github.com/aaronflorey/bin/commit/2fbdbee59090be6dc3e112b70b1ddf9a82bd254a))
* **config,providers,assets:** harden installs and retire todo ([be78091](https://github.com/aaronflorey/bin/commit/be78091fc288a2c58180b115258247341ebd91d6))
* **config,update:** harden config persistence and prompts ([0228ef5](https://github.com/aaronflorey/bin/commit/0228ef506546b0bda6ae1d59b8b8387dc45049dc))
* **config:** use cross-platform directory write probe ([835de0a](https://github.com/aaronflorey/bin/commit/835de0accbc5604ad3d3ade2edc36b80a2c39c31))
* **deps:** upgrade go-github to v73 and goldmark to v1.8.1 ([3821716](https://github.com/aaronflorey/bin/commit/382171610ca8402306bca5e90a654b021b1610cb))
* **docker:** init client from env variables available to configure docker ([#235](https://github.com/aaronflorey/bin/issues/235)) ([026f8ef](https://github.com/aaronflorey/bin/commit/026f8eff127e160aa4dd45696065f707bd2b3a9d))
* don't download .sbom.json files ([8a04b17](https://github.com/aaronflorey/bin/commit/8a04b17c78bc162026611724d1a5946e1e7aff88))
* ensure config path is present ([cb36e87](https://github.com/aaronflorey/bin/commit/cb36e87c762e0b62e0cfbd743db4c240e568d419))
* **ensure,update:** persist provider/path ([#254](https://github.com/aaronflorey/bin/issues/254)) ([ec88dc1](https://github.com/aaronflorey/bin/commit/ec88dc15bdc86b6331888515dda958f34da9defe))
* **ensure:** Retry without package path on archive mismatch ([67a3eda](https://github.com/aaronflorey/bin/commit/67a3eda8013ec974e583a4aaf6ba852b9e7b37a5))
* **gitlab:** return clear missing release errors ([2b93200](https://github.com/aaronflorey/bin/commit/2b9320092c24ee1dee0ccc4c4f8e82fbeb41b01c))
* improve spinner code ([ea9baf6](https://github.com/aaronflorey/bin/commit/ea9baf6bf9189aa151b2f6354681f3b0fadb7523))
* **install,ci:** harden asset resolution and test installer script ([42a0ecf](https://github.com/aaronflorey/bin/commit/42a0ecf217517011bf821c25410163a66157ecbf))
* **install:** capture verbose logs and improve archive selection ([821309f](https://github.com/aaronflorey/bin/commit/821309fe5cf267666dabed69061c108ef848b747))
* **installer:** improve home path matching ([08de525](https://github.com/aaronflorey/bin/commit/08de525ec329795122368afe31f9d70cf7620c66))
* **install:** honor interactive overwrite confirmation ([951bc41](https://github.com/aaronflorey/bin/commit/951bc410717eee71101b17e2cb6b781de92c5b9a))
* **install:** preseed config for non-interactive bootstrap ([c13a33a](https://github.com/aaronflorey/bin/commit/c13a33a71008705433aa57c2ee9c9217bef8ebc6))
* lint failure in update ([8a684d2](https://github.com/aaronflorey/bin/commit/8a684d2f6c8720ddd670f525ead5557f12384afe))
* **list:** correct column alignment when displaying pinned versions ([8e4a442](https://github.com/aaronflorey/bin/commit/8e4a442a420a3bcfda07d248063d319ef6edff44))
* outdated output was broken ([97f45d2](https://github.com/aaronflorey/bin/commit/97f45d2a21d824e79e1fa3158f547f20cb3d5918))
* panic in tie breaker code ([7212eee](https://github.com/aaronflorey/bin/commit/7212eeeda023dd584b72b2531dbccf3c7f0c6381))
* **providers,assets:** handle final binary checksums ([59911b0](https://github.com/aaronflorey/bin/commit/59911b00a0e24e4ce2a16aa4820f66fc1cd4aa87))
* **providers:** Satisfy staticcheck in GitHub auth guard ([0557efc](https://github.com/aaronflorey/bin/commit/0557efc6bd9bc5218d321cdfbcf1df7ba53a0444))
* some tech debt ([a219d3e](https://github.com/aaronflorey/bin/commit/a219d3ee534d5214a2b86f913e2074f91ed58db8))
* tie breaker breaking on negative scores ([b04f2bf](https://github.com/aaronflorey/bin/commit/b04f2bfa8928a95feb3ad7514357e5462d5a7801))
* **update:** return non-zero exit when partial updates fail ([e11f81a](https://github.com/aaronflorey/bin/commit/e11f81a2e26ab82387d601e84236e4b9cf2e3ba5))
* **update:** skip interactive selector with --yes and --dry-run flags ([16b2563](https://github.com/aaronflorey/bin/commit/16b256397ae80652fe61995435057f5639cadc9d))
* various fixes for edge case releases ([e1530db](https://github.com/aaronflorey/bin/commit/e1530db3fefaac5c22da4077de53a9439aa8d797))


### Performance Improvements

* **assets:** stream archive candidates to temp files ([db9c8cd](https://github.com/aaronflorey/bin/commit/db9c8cd6470999ab722c6ca0dfb0a2b643cf8a53))


### Miscellaneous Chores

* bump ([a7312b1](https://github.com/aaronflorey/bin/commit/a7312b1d35ecfab671fd1130a0a0ab44acbf89cb))

## [2.4.0](https://github.com/aaronflorey/bin/compare/v2.3.0...v2.4.0) (2026-05-01)


### Features

* **cmd:** suggest managed names for partial targets ([ba4fa82](https://github.com/aaronflorey/bin/commit/ba4fa82e72fde7e176e9ae34ea112bc92b6030e2))
* **cmd:** suggest managed names for partial targets ([716e845](https://github.com/aaronflorey/bin/commit/716e84534f0a8d132de30e5346525ae7b2818209))
* **install:** support tracked dmg app installs ([d0653d2](https://github.com/aaronflorey/bin/commit/d0653d2bad6b5a878c0f6878db345ef394e6ad14))
* **list:** add json output format ([5a4a0a0](https://github.com/aaronflorey/bin/commit/5a4a0a0bd84e466eb48cd03e835a25f27132de8b))
* **remove:** add interactive multi-select removal flow ([deb3fb4](https://github.com/aaronflorey/bin/commit/deb3fb4ba855ac2af2a66c6f461b0296b87a242c))
* **remove:** add interactive multi-select removal flow ([5f3d47d](https://github.com/aaronflorey/bin/commit/5f3d47db8c86d7a5381f35173dc921fba6d27488))
* **tui:** add interactive changelog browser ([7729ae4](https://github.com/aaronflorey/bin/commit/7729ae476524aca57331d300f8d026d21a5d9362))
* **update:** Add interactive multi-select for no-arg updates ([2effacb](https://github.com/aaronflorey/bin/commit/2effacb9fff8c8b22025d1cfb050ce3025abe965))
* **update:** interactive multi-select for no-arg updates ([e4236f4](https://github.com/aaronflorey/bin/commit/e4236f48dac22bb3f5ec28a57b8bbc51366e9d73))


### Bug Fixes

* **assets,config:** restore tar archive detection ([0839c44](https://github.com/aaronflorey/bin/commit/0839c44fac1b053aecb325cb04811a99ca1a545c))
* **assets:** prefer native Linux binaries over AppImages ([97f00f9](https://github.com/aaronflorey/bin/commit/97f00f9c34d900d88eec657d0d745d22dab56a09))
* chown config dir and file to sudo user when installing globally ([636b0b9](https://github.com/aaronflorey/bin/commit/636b0b9ad0c9230c28bf2343b0730860053faeae))
* chown config dir/file to real user when installing via sudo ([d8a15ff](https://github.com/aaronflorey/bin/commit/d8a15ff6e6481a7be50bd166c912854e737d9a7d))
* **ci:** resolve lint findings ([3c3a52d](https://github.com/aaronflorey/bin/commit/3c3a52d5b7810162a63f97a865fcf98ff158861e))
* **config,providers,assets:** harden installs and retire todo ([4e13744](https://github.com/aaronflorey/bin/commit/4e13744910866d8e5e01447f2811cb7dc521b513))
* **config,update:** harden config persistence and prompts ([6304958](https://github.com/aaronflorey/bin/commit/6304958255ba2b04742e7aad6f2410d9dbbd08af))
* **install:** capture verbose logs and improve archive selection ([c7f7c2a](https://github.com/aaronflorey/bin/commit/c7f7c2a81ac2325a2b7322359f62ff2334b96498))
* **update:** skip interactive selector with --yes and --dry-run flags ([8d41741](https://github.com/aaronflorey/bin/commit/8d417419e29dff7b3c7e552b60ead6b2b3e61da5))

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
