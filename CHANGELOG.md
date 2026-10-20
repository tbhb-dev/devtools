# Changelog

All notable changes to this project will be documented in this file. See [conventional commits](https://www.conventionalcommits.org/) for commit guidelines.

- - -
<!-- release -->

## [v0.13.0](https://github.com/tbhb-dev/devtools/compare/e6b8b816ad43d3ce6539f7febe47ba1e12ced6a4..v0.13.0) - 2026-10-20

### Features

- (**checks**) assess saved pull request check summaries (#242) - ([a433e7d](https://github.com/tbhb-dev/devtools/commit/a433e7d20524088f93a9c9a608ebafe2cb990146)) - tbhb.dev Agent
- (**commit**) lint outgoing messages before push (#278) - ([388b543](https://github.com/tbhb-dev/devtools/commit/388b543b7d24187a3a97c1f5e3afeb8c8607df1e)) - tbhb.dev Agent
- (**lint**) replace shell commit-message linters with Go (#257) - ([3628de6](https://github.com/tbhb-dev/devtools/commit/3628de6f1788d3262d947f0404f1101410bd9f01)) - tbhb.dev Agent
- (**rust**) require justified and expiring lint exceptions (#256) - ([046ae37](https://github.com/tbhb-dev/devtools/commit/046ae3782ba828d5f7d91a02c6efce1abfc508aa)) - tbhb.dev Agent
- (**skills**) add unattended pull request merging (#279) - ([ea48037](https://github.com/tbhb-dev/devtools/commit/ea480377b1e282d747fdf4c8590edd1444436352)) - tbhb.dev Agent
- (**skills**) add unattended pull request publication (#269) - ([0a501c0](https://github.com/tbhb-dev/devtools/commit/0a501c0edec6fe99a10399646f2b445193be7c76)) - tbhb.dev Agent
- (**skills**) add unattended agent commits (#268) - ([5e8a49f](https://github.com/tbhb-dev/devtools/commit/5e8a49f3d60b7e75af36cf178f5390a112e8976a)) - tbhb.dev Agent
- (**tasks**) add opt-in Rust gates and starter policy (#247) - ([ebc577a](https://github.com/tbhb-dev/devtools/commit/ebc577ae7216e7bc1e75c4f325802991c460c8c0)) - tbhb.dev Agent
- (**tools**) establish the shell migration baseline (#243) - ([8ce2704](https://github.com/tbhb-dev/devtools/commit/8ce2704e44088340895e60676aa0ae0582577235)) - tbhb.dev Agent

### Bug Fixes

- (**commit**) provision commit-lint for consumers (#302) - ([cc11120](https://github.com/tbhb-dev/devtools/commit/cc11120a822c57b15940343896201aa43a108495)) - tbhb.dev Agent
- (**commit**) fall back to Git sign-off identity (#299) - ([bd6160f](https://github.com/tbhb-dev/devtools/commit/bd6160fddc6dc3257e165ece467faf2177f36142)) - tbhb.dev Agent
- (**commit**) refuse configured agent sign-offs (#270) - ([f27dbed](https://github.com/tbhb-dev/devtools/commit/f27dbed82b0f1adbf973b49fc4e343cd327b4f7d)) - tbhb.dev Agent
- (**deps**) update module github.com/pelletier/go-toml/v2 to v2.4.3 (#271) - ([21c2af4](https://github.com/tbhb-dev/devtools/commit/21c2af4e72498f289cbf6b23eb16685068683e3c)) - tbhb-dev-renovate[bot]
- (**docs**) split the script inventory into smaller sections (#248) - ([1fc1359](https://github.com/tbhb-dev/devtools/commit/1fc135971e4c5b34f916d6cd3cecdb293dcec8f1)) - tbhb.dev Agent
- (**lint**) check first-party agent skills and rules (#239) - ([f07e183](https://github.com/tbhb-dev/devtools/commit/f07e183a04dee24b9d7bc18f0135791f8960ef47)) - tbhb.dev Agent
- (**merge-pr**) confirm merge completion before cleanup (#258) - ([91b0570](https://github.com/tbhb-dev/devtools/commit/91b05700166c11df8cb9dbe3e7a8a8ec8ba9ac6d)) - tbhb.dev Agent
- (**release**) check authentication for the active account (#255) - ([e923ed9](https://github.com/tbhb-dev/devtools/commit/e923ed90ab65ffe8c0c29f543970b8f201cd65ef)) - tbhb.dev Agent
- (**renovate**) keep Rust MSRV updates with the toolchain pin (#276) - ([8a9ca34](https://github.com/tbhb-dev/devtools/commit/8a9ca348bf59742e00fd59bf0df570ba5b5ae59a)) - tbhb.dev Agent
- (**review**) bound reviewer execution and clean up its process group (#267) - ([5482f60](https://github.com/tbhb-dev/devtools/commit/5482f6040309d950b9903f6571840fd347d96054)) - tbhb.dev Agent
- (**skills**) allow pull request drafting in the calling session (#252) - ([efbad91](https://github.com/tbhb-dev/devtools/commit/efbad91e056ab77b24294e3ace8608a0172b1b91)) - tbhb.dev Agent
- (**tests**) pin the scanner stub in the mise fixture (#250) - ([152f925](https://github.com/tbhb-dev/devtools/commit/152f92555ab91568640280cc742c88c2a15a2e22)) - tbhb.dev Agent
- apply commit rules to message previews (#238) - ([e6b8b81](https://github.com/tbhb-dev/devtools/commit/e6b8b816ad43d3ce6539f7febe47ba1e12ced6a4)) - tbhb.dev Agent

- - -

<!-- release -->

## [v0.12.0](https://github.com/tbhb-dev/devtools/compare/d318a049d711d71d610276b1864fb039ce99923c..v0.12.0) - 2026-10-17

### Features

- (**ci**) publish consumer lint and security workflows (#192) - ([f9a46cd](https://github.com/tbhb-dev/devtools/commit/f9a46cd4ba3079c1d4171887cc110d561caa7d87)) - tbhb.dev Agent
- (**leaks**) skip configured paths in working-tree scans (#222) - ([3bcdd80](https://github.com/tbhb-dev/devtools/commit/3bcdd80f85b2bd6d32d61db9b8ce06e69605fa5a)) - tbhb.dev Agent
- (**signing**) provide an isolated SSH signing agent (#221) - ([bf8b432](https://github.com/tbhb-dev/devtools/commit/bf8b4321ea31bf40d0c9bfca7f10510a904631d2)) - tbhb.dev Agent
- (**skills**) support trusted repository review policy (#201) - ([a04a6b1](https://github.com/tbhb-dev/devtools/commit/a04a6b1df6a776678a5f31a6deca70856f135352)) - tbhb.dev Agent
- (**skills**) unite the pr family under shared sources (#183) - ([d318a04](https://github.com/tbhb-dev/devtools/commit/d318a049d711d71d610276b1864fb039ce99923c)) - tbhb.dev Agent
- (**tasks**) allow declared consumer grant scopes (#191) - ([494b2ce](https://github.com/tbhb-dev/devtools/commit/494b2ceda418bcc1d0ac129b13b315d41684cb1a)) - tbhb.dev Agent
- gate dependency commits with gomodscan (#220) - ([93cf18c](https://github.com/tbhb-dev/devtools/commit/93cf18c450e83cfdcf075d3cb78afd3f32ab9d46)) - tbhb.dev Agent

### Bug Fixes

- (**adoption**) provide the consumer runtime payload (#195) - ([67f6107](https://github.com/tbhb-dev/devtools/commit/67f61074b83552155aeb8825e939caf1530fa350)) - tbhb.dev Agent
- (**agents**) order workflow gates and share clean-tree passes (#223) - ([b006694](https://github.com/tbhb-dev/devtools/commit/b00669404cca40d4fff943097b3c5d3657d660ff)) - tbhb.dev Agent
- (**commit**) preview the sign-off added by git commit -s (#196) - ([d563e8d](https://github.com/tbhb-dev/devtools/commit/d563e8d27831ea70bb9d25d54e12fc73aeaade75)) - tbhb.dev Agent
- (**leaks**) keep consumer leak setup opt-in (#215) - ([ed64093](https://github.com/tbhb-dev/devtools/commit/ed64093f227bd5c82012e349f5d2f1b03de82db1)) - tbhb.dev Agent
- (**lint**) skip workflow lint when no workflows exist (#185) - ([e1c52e3](https://github.com/tbhb-dev/devtools/commit/e1c52e3a001e04376ebd94b40e650f225e4f8f3b)) - tbhb.dev Agent
- (**merge-pr**) wait for approvals and review complete pinned diffs (#194) - ([9d9737b](https://github.com/tbhb-dev/devtools/commit/9d9737ba05b178f0baacc0d1f81b8a7262941633)) - tbhb.dev Agent
- (**python**) keep vendored config templates undiscovered (#216) - ([80a19e7](https://github.com/tbhb-dev/devtools/commit/80a19e70425b56494f4b31a9c0b12b360e308678)) - tbhb.dev Agent
- (**shell**) gate masked command failures and unsafe curl output (#209) - ([8498edd](https://github.com/tbhb-dev/devtools/commit/8498edd3be236e69d398f06bfba57705d1e644f5)) - tbhb.dev Agent
- (**shell**) preserve failures in repository tooling (#205) - ([f79d2c0](https://github.com/tbhb-dev/devtools/commit/f79d2c0548d3fdf33e2f40d163c95e8bf15e318f)) - tbhb.dev Agent
- (**skills**) name shared tasks in failure diagnosis (#219) - ([9fba789](https://github.com/tbhb-dev/devtools/commit/9fba7892460f6b9f42742662d73d7f53952aabe8)) - tbhb.dev Agent
- (**skills**) preserve failures in shell status checks (#208) - ([8d972ce](https://github.com/tbhb-dev/devtools/commit/8d972cef163dc509a348e378ab3f4e5f9791b4d7)) - tbhb.dev Agent
- (**skills**) allow delegated writers to run without prompts (#204) - ([65eb052](https://github.com/tbhb-dev/devtools/commit/65eb052ea86c12c52dd1e85cbbd52107f68e77dc)) - tbhb.dev Agent
- (**skills**) distinguish API quota exhaustion from identity failures (#200) - ([a3692e9](https://github.com/tbhb-dev/devtools/commit/a3692e9061cbbd2908461537f30e0671ccad5bd4)) - tbhb.dev Agent
- (**tasks**) handle empty pre-approval scopes on bash 3.2 (#197) - ([c4cd0ba](https://github.com/tbhb-dev/devtools/commit/c4cd0bacf4e899a1f11bef6f155d879d36d42eb2)) - tbhb.dev Agent
- (**tasks**) share exclusions for shell and Markdown gates (#193) - ([494c426](https://github.com/tbhb-dev/devtools/commit/494c4265b5fcb8c97fa79dd4862532e60472154c)) - tbhb.dev Agent
- (**tests**) remove scheduling races from macOS fixtures (#226) - ([9bc950c](https://github.com/tbhb-dev/devtools/commit/9bc950c576494e0fb10a9102c668c999950d4741)) - tbhb.dev Agent
- (**tests**) isolate scratch repositories from inherited Git settings (#184) - ([c325117](https://github.com/tbhb-dev/devtools/commit/c3251172e4bc64ecd7017e9cadfaff5774be5f4e)) - tbhb.dev Agent
- (**vendor**) detect drift hidden by personal ignores (#188) - ([10ba48c](https://github.com/tbhb-dev/devtools/commit/10ba48c9f0bb5dfc17bb0a90fe228483c6cd1bb8)) - tbhb.dev Agent
- preserve lint exclusions and Bash 3.2 status handling (#217) - ([e49a286](https://github.com/tbhb-dev/devtools/commit/e49a286aba5970f4286eb9e84bf9e5fcdcea8869)) - tbhb.dev Agent

### Documentation

- update v0.12 review and workflow guidance (#207) - ([5ca072e](https://github.com/tbhb-dev/devtools/commit/5ca072ec278bb6e56c86f765ac552493e7076a85)) - tbhb.dev Agent
- integrate consumer adoption and agent guidance (#199) - ([5b5cb02](https://github.com/tbhb-dev/devtools/commit/5b5cb028f50017ce33264993f3a54621bc04bd16)) - tbhb.dev Agent

### Build system

- (**config**) provide neutral starter vocabularies (#187) - ([356930f](https://github.com/tbhb-dev/devtools/commit/356930fe19b24882102e89416db39902da843389)) - tbhb.dev Agent
- (**go**) move the toolchain pin to 1.27.2 (#186) - ([7b223a9](https://github.com/tbhb-dev/devtools/commit/7b223a9f285f7d8663e930bdc7ae158b700708bd)) - tbhb.dev Agent
- (**security**) provide a default gitleaks fixture policy (#189) - ([0c88f8c](https://github.com/tbhb-dev/devtools/commit/0c88f8ca1c9f1bf0a82f3586f0b79b2c91cad681)) - tbhb.dev Agent

### Continuous Integration

- run bats regression suites on Linux and macOS (#190) - ([c83f778](https://github.com/tbhb-dev/devtools/commit/c83f778eee2380e226b368729782fd03c328bbd7)) - tbhb.dev Agent

- - -

<!-- release -->

## [v0.11.0](https://github.com/tbhb-dev/devtools/compare/7f66d9a5a04b507f00b9fda7566e6ed472bd7c60..v0.11.0) - 2026-10-16

### Features

- (**security**) share the private identifier leak gate (#172) - ([47d9027](https://github.com/tbhb-dev/devtools/commit/47d90277caaf3b3ba8b19bab2c03787fc7353992)) - tbhb.dev Agent
- (**skills**) unify session analysis and clarify hook retries (#173) - ([c1c5e7f](https://github.com/tbhb-dev/devtools/commit/c1c5e7fa56945e33bf2b50584b6ac192a04dc499)) - tbhb.dev Agent
- (**skills**) unite the prose family under shared sources (#169) - ([154829e](https://github.com/tbhb-dev/devtools/commit/154829e1867973123fb13ea3820a5006234bbbb4)) - tbhb.dev Agent
- (**skills**) bind merge approval to the reviewed pull request (#164) - ([e280d72](https://github.com/tbhb-dev/devtools/commit/e280d7286ada9c6f2e9da9234c2a26a075bb3de2)) - tbhb.dev Agent
- ![BREAKING](https://img.shields.io/badge/BREAKING-red) (**skills**) unify harness skills under shared sources (#155) - ([fd7ec94](https://github.com/tbhb-dev/devtools/commit/fd7ec949991aeda999269f01a60633a2e890f962)) - tbhb.dev Agent
- (**tasks**) add opt-in Go and Python gates (#171) - ([297d031](https://github.com/tbhb-dev/devtools/commit/297d031b1642ebdc7f83b4debcce355b80f6cb95)) - tbhb.dev Agent

### Bug Fixes

- (**pins**) track devtools Renovate presets (#162) - ([8f42015](https://github.com/tbhb-dev/devtools/commit/8f420153cc92c3cee244724333daf8b34631ba9c)) - tbhb.dev Agent
- (**release**) refresh local APM versions during the bump (#168) - ([2374bce](https://github.com/tbhb-dev/devtools/commit/2374bce953703eb7143e5d0401641e3e84cdc96e)) - tbhb.dev Agent
- (**renovate**) pin the runner and validator to the same image (#149) - ([7f66d9a](https://github.com/tbhb-dev/devtools/commit/7f66d9a5a04b507f00b9fda7566e6ed472bd7c60)) - tbhb.dev Agent
- (**security**) redact secret findings and scan history in CI (#163) - ([9b4e82a](https://github.com/tbhb-dev/devtools/commit/9b4e82aeb3bc62cf521f2690f424d6084f49ea6d)) - tbhb.dev Agent
- (**tools**) report unfinished prose replacements as failures (#150) - ([744335b](https://github.com/tbhb-dev/devtools/commit/744335b9913412c193eca67fc9f40afce1b30cf1)) - tbhb.dev Agent
- (**vale**) prevent a rule crash in the prose gate (#151) - ([8daff0b](https://github.com/tbhb-dev/devtools/commit/8daff0bb36155c66b342b036a52bb45b7b676994)) - tbhb.dev Agent

### Refactoring

- (**skills**) unite the rebase family under shared sources (#166) - ([4dd50c7](https://github.com/tbhb-dev/devtools/commit/4dd50c771edfb31e5e637c3a67225771210fa16d)) - tbhb.dev Agent

- - -

<!-- release -->

## [v0.10.0](https://github.com/tbhb-dev/devtools/compare/26e9d79de6ab1b045725386749647528ed745e14..v0.10.0) - 2026-10-11

### Bug Fixes

- (**apm**) regenerate the lockfile for the v0.9.0 package manifests - ([05a6818](https://github.com/tbhb-dev/devtools/commit/05a68189c5178f1bae78583501af581589f6b711)) - [@tbhb](https://github.com/tbhb)
- (**deps**) update go-deps (#72) - ([3f08922](https://github.com/tbhb-dev/devtools/commit/3f089224376150f5c5cbe7b24de3f6c8ca227d6c)) - tbhb-renovate[bot]
- (**release**) let readiness pass over a failed release run on HEAD (#133) - ([7306236](https://github.com/tbhb-dev/devtools/commit/730623682a91dc6053208202301b9cffe0e49897)) - tbhb.dev Agent
- (**release**) request the release token as the tbhb-dev-releases app (#130) - ([e7499a2](https://github.com/tbhb-dev/devtools/commit/e7499a2f15744c7a57244010ac05f68b8998ba65)) - tbhb.dev Agent
- (**release**) render the changelog preview and fail an empty one (#68) - ([72af9d8](https://github.com/tbhb-dev/devtools/commit/72af9d8cf28907d5e18ec2e11fbf9a10a5ca3427)) - [@tbhb](https://github.com/tbhb)
- (**skills**) stop the release version context dying on a bounded diff (#129) - ([3073dd5](https://github.com/tbhb-dev/devtools/commit/3073dd51412a6370b5562456a58a7ccb434f8344)) - tbhb.dev Agent
- (**skills**) avoid a find/exec chain the skill scanner flags critical - ([1499428](https://github.com/tbhb-dev/devtools/commit/14994287efab46e382e4e2377e3f44ccf7826dda)) - [@tbhb](https://github.com/tbhb)

### Build system

- install editorconfig-checker v4 from its release archives (#127) - ([a8d0686](https://github.com/tbhb-dev/devtools/commit/a8d0686efeee597a5f8e6fb6153021df3fd4063d)) - tbhb.dev Agent
- install ryl from release archives and lint through ryl check (#126) - ([cfd622f](https://github.com/tbhb-dev/devtools/commit/cfd622fd403cd134e2bea296c28b54218b5c701b)) - tbhb.dev Agent

### Refactoring

- ![BREAKING](https://img.shields.io/badge/BREAKING-red) move the repository to tbhb-dev/devtools (#102) - ([5194088](https://github.com/tbhb-dev/devtools/commit/519408860cb7c6af8b18aeb26d8e5ee4b79806a4)) - tbhb.dev Agent

- - -

<!-- release -->

## [v0.9.0](https://github.com/tbhb-dev/devtools/compare/49c1017fba1def1ab6099fe53e16678d79bf4020..v0.9.0) - 2026-08-17

### Features

- (**skills**) add Codex-native agent workflows (#66) - ([10dd0d1](https://github.com/tbhb-dev/devtools/commit/10dd0d1c785c06695e336a8b6e0a22c6a56c5e69)) - [@tbhb](https://github.com/tbhb)

### Bug Fixes

- (**apm**) exclude nested worktrees from audit - ([71c9980](https://github.com/tbhb-dev/devtools/commit/71c9980036d533d1a76a97b6cab9ab38f8ff5cda)) - [@tbhb](https://github.com/tbhb)
- (**skills**) stop requiring TaskCreate for the step checklists (#65) - ([49c1017](https://github.com/tbhb-dev/devtools/commit/49c1017fba1def1ab6099fe53e16678d79bf4020)) - [@tbhb](https://github.com/tbhb)

- - -

<!-- release -->

## [v0.8.0](https://github.com/tbhb-dev/devtools/compare/54de242307a5745ee1e6efad2f2157943a40a917..v0.8.0) - 2026-08-15

### Bug Fixes

- (**tooling**) stop lint-types failing on a worktree's empty view (#54) - ([ef15c6c](https://github.com/tbhb-dev/devtools/commit/ef15c6c24a56bc16ba00cbd65c63ac4fe7c819c9)) - [@tbhb](https://github.com/tbhb)

### Build system

- (**apm**) split the package into per-harness sub-packages (#55) - ([7de08d2](https://github.com/tbhb-dev/devtools/commit/7de08d24ce048eb6aac71a12ce800e9f550d0620)) - [@tbhb](https://github.com/tbhb)

- - -

<!-- release -->

## [v0.7.1](https://github.com/tbhb-dev/devtools/compare/3b1d0a77791684a5ab64170acaba5132c63550b7..v0.7.1) - 2026-08-15

### Bug Fixes

- (**renovate**) stop tracking vendored tool pins in consumer repos (#53) - ([ae9f41f](https://github.com/tbhb-dev/devtools/commit/ae9f41ffee0d4477eceba751158af8e6d7e95b97)) - [@tbhb](https://github.com/tbhb)

- - -

<!-- release -->

## [v0.7.0](https://github.com/tbhb-dev/devtools/compare/ecd105a8a91f9c649d9ef8a87adfb9442d0ba637..v0.7.0) - 2026-08-04

### Bug Fixes

- (**skills**) tighten what the review loops spend a round on (#40) - ([ecd105a](https://github.com/tbhb-dev/devtools/commit/ecd105a8a91f9c649d9ef8a87adfb9442d0ba637)) - [@tbhb](https://github.com/tbhb)

### Build system

- retire the just pin and its shared tasks (#41) - ([b6e309e](https://github.com/tbhb-dev/devtools/commit/b6e309e55b5f0298c2eedfce3003245096ff6293)) - [@tbhb](https://github.com/tbhb)

- - -

<!-- release -->

## [v0.6.0](https://github.com/tbhb-dev/devtools/compare/1f1e78e755f924112c90f4c7f31838fdf2749df8..v0.6.0) - 2026-08-04

### Features

- (**skills**) base the release on state the skill reads for itself - ([28cbc46](https://github.com/tbhb-dev/devtools/commit/28cbc463332f61dd24f2b1e7891df9205ff6a169)) - [@tbhb](https://github.com/tbhb)
- (**skills**) drive a release from readiness to a verified tag - ([cdd680b](https://github.com/tbhb-dev/devtools/commit/cdd680b7202c53fd3b8ca200c68537c6221a0511)) - [@tbhb](https://github.com/tbhb)
- (**skills**) hand review-squash-message its inputs up front (#32) - ([19bed45](https://github.com/tbhb-dev/devtools/commit/19bed45bfdf6be19df044a0bb8f9e5c448cd47ab)) - [@tbhb](https://github.com/tbhb)
- (**tools**) report each skill's tool calls and its time on the clock (#30) - ([0c891d4](https://github.com/tbhb-dev/devtools/commit/0c891d414ba61df2dc7858a2767cf891fde068ed)) - [@tbhb](https://github.com/tbhb)

### Bug Fixes

- (**ci**) read the release commit's file list out of a file - ([62a6fe8](https://github.com/tbhb-dev/devtools/commit/62a6fe8730cb7a20d0fd5c6aa0ec022acb0b4222)) - [@tbhb](https://github.com/tbhb)
- (**ci**) build the release commit's file list without argument limits - ([8072914](https://github.com/tbhb-dev/devtools/commit/8072914a0b5e6f534dbfbdc24194fdb8fcce8d15)) - [@tbhb](https://github.com/tbhb)
- (**tools**) hold the dispatched version to digits and dots alone - ([3402d34](https://github.com/tbhb-dev/devtools/commit/3402d347e8b6e14441ebcf7962fb17c2db8c1029)) - [@tbhb](https://github.com/tbhb)
- (**tools**) name the release ref rather than inferring it (#38) - ([d766538](https://github.com/tbhb-dev/devtools/commit/d7665385f0e2dac7dcfeacf676ef805a677c4988)) - [@tbhb](https://github.com/tbhb)
- verify a pinned preset reference at the ref it names (#37) - ([444f921](https://github.com/tbhb-dev/devtools/commit/444f921633380c28a7c2dfabe88fdb35f627f0ff)) - [@tbhb](https://github.com/tbhb)

### Revert

- undo the partial v0.6.0 bump left on main - ([87a4bdc](https://github.com/tbhb-dev/devtools/commit/87a4bdcf100937ebfd509fde61b39ef3371a348e)) - [@tbhb](https://github.com/tbhb)

### Build system

- scope Python coverage from the task instead of a member list (#35) - ([ff44888](https://github.com/tbhb-dev/devtools/commit/ff44888d896d181bfc2af6a2ce5145e42ff6683d)) - [@tbhb](https://github.com/tbhb)
- pre-approve the commit, pr, and merge confirmations per session (#36) - ([6f27794](https://github.com/tbhb-dev/devtools/commit/6f277944a6751a9ab9b5881906afef37d0d2d06a)) - [@tbhb](https://github.com/tbhb)
- install every workspace member and pin what resolves them (#34) - ([1f1e78e](https://github.com/tbhb-dev/devtools/commit/1f1e78e755f924112c90f4c7f31838fdf2749df8)) - [@tbhb](https://github.com/tbhb)

### Continuous Integration

- create the release commit through the API so GitHub signs it (#39) - ([de08aa8](https://github.com/tbhb-dev/devtools/commit/de08aa8615af97b4c9d73e782f3ab56027ace327)) - [@tbhb](https://github.com/tbhb)

- - -

<!-- release -->

## [v0.5.0](https://github.com/tbhb-dev/devtools/compare/2b68005951deb9096af2e59b460b9f366867ae2c..v0.5.0) - 2026-08-03

### Bug Fixes

- use the published vale agent template and read vale's exit status (#26) - ([2b68005](https://github.com/tbhb-dev/devtools/commit/2b68005951deb9096af2e59b460b9f366867ae2c)) - [@tbhb](https://github.com/tbhb)

### Build system

- add the release readiness, dispatch, and verification tasks (#31) - ([044bae4](https://github.com/tbhb-dev/devtools/commit/044bae4070de881ff5fd9c71bd86f5f07042dfa1)) - [@tbhb](https://github.com/tbhb)
- move a consumer between devtools releases with one command (#28) - ([83de50f](https://github.com/tbhb-dev/devtools/commit/83de50fd4f8ffee9f4c2fcca2041febe5b16d6a5)) - [@tbhb](https://github.com/tbhb)
- rewrite every published version literal during a bump (#29) - ([6223b87](https://github.com/tbhb-dev/devtools/commit/6223b87d9fe5754bc9efd4e71296fb277daf4294)) - [@tbhb](https://github.com/tbhb)
- retire the Justfile and run every gate through mise (#27) - ([fa6adbc](https://github.com/tbhb-dev/devtools/commit/fa6adbcf0a64962e7ed8fc08f5a6fd845e5040ed)) - [@tbhb](https://github.com/tbhb)

- - -

<!-- release -->

## [v0.4.0](https://github.com/tbhb-dev/devtools/compare/1e0a27d2bb8020f69e57cf86ab1c7e9353037ad7..v0.4.0) - 2026-08-03

### Bug Fixes

- name the markdown parser when linting a commit message (#24) - ([cab0890](https://github.com/tbhb-dev/devtools/commit/cab0890bfe2a887aabf877a4e44444fe0ddeb13a)) - [@tbhb](https://github.com/tbhb)
- correct the script defects the gates couldn't see (#22) - ([de2c6dd](https://github.com/tbhb-dev/devtools/commit/de2c6ddfb2d2dba527b6c26c59311946d01758a9)) - [@tbhb](https://github.com/tbhb)
- scrub the git environment around the vendored payload sync (#21) - ([634e7f9](https://github.com/tbhb-dev/devtools/commit/634e7f9fac7c21c0238504d0aa7afeecea8f0dde)) - [@tbhb](https://github.com/tbhb)

### Build system

- move the gitleaks pin and task into the shared payload (#23) - ([afd036c](https://github.com/tbhb-dev/devtools/commit/afd036c222be3f8749b8c139afd8d19717439c5c)) - [@tbhb](https://github.com/tbhb)
- lint YAML with ryl in place of yamllint (#20) - ([5515549](https://github.com/tbhb-dev/devtools/commit/55155490865c3ccf2c147f8ed1212eb7719a3858)) - [@tbhb](https://github.com/tbhb)
- pin the apm CLI through mise instead of a curl step (#19) - ([e57f334](https://github.com/tbhb-dev/devtools/commit/e57f334affa3509305c95dca24e91204010335ef)) - [@tbhb](https://github.com/tbhb)

- - -

<!-- release -->

## [v0.3.2](https://github.com/tbhb-dev/devtools/compare/d00af6c7211527f4880e007ff2e96f63987666f6..v0.3.2) - 2026-08-03

### Bug Fixes

- scope the fix-prose guard release to a single edit (#18) - ([9b269d9](https://github.com/tbhb-dev/devtools/commit/9b269d90429a4da30c46e1f8860f79dcf8b47c29)) - [@tbhb](https://github.com/tbhb)
- release the fix-prose lock when its document no longer exists (#16) - ([0811139](https://github.com/tbhb-dev/devtools/commit/0811139c65ecd93d3c45cae0eb4f608382e41d44)) - [@tbhb](https://github.com/tbhb)

### Build system

- package the shared mise tasks and pins as a vendir payload (#17) - ([803bdfe](https://github.com/tbhb-dev/devtools/commit/803bdfea2f0d03a77c41b2a86c97463ff861cbc1)) - [@tbhb](https://github.com/tbhb)
- convert the container-run gates to mise tool pins (#15) - ([5cd9ce1](https://github.com/tbhb-dev/devtools/commit/5cd9ce160e535f817d365b6f30b289f58276066c)) - [@tbhb](https://github.com/tbhb)

### Continuous Integration

- remove the unused setup composites and their Renovate manager (#14) - ([d00af6c](https://github.com/tbhb-dev/devtools/commit/d00af6c7211527f4880e007ff2e96f63987666f6)) - [@tbhb](https://github.com/tbhb)

- - -

<!-- release -->

## [v0.3.1](https://github.com/tbhb-dev/devtools/compare/08897dcadc2db76afb525c7840d2a14564831399..v0.3.1) - 2026-08-02

### Bug Fixes

- sign the release tag in CI and push through the app token (#13) - ([6d7d6e3](https://github.com/tbhb-dev/devtools/commit/6d7d6e3e6917c9a0d016f3f8103518344ef1801e)) - [@tbhb](https://github.com/tbhb)

### Build system

- pin the toolchain with mise from local runs through CI (#12) - ([ec746f3](https://github.com/tbhb-dev/devtools/commit/ec746f30b30a123d63f3a07edbc2a52fcd4d62b5)) - [@tbhb](https://github.com/tbhb)

### Continuous Integration

- permit the mise lock refresh and drop the dead vale rules (#11) - ([08897dc](https://github.com/tbhb-dev/devtools/commit/08897dcadc2db76afb525c7840d2a14564831399)) - [@tbhb](https://github.com/tbhb)

- - -

<!-- release -->

## [v0.3.0](https://github.com/tbhb-dev/devtools/compare/3be10c4ea0eb708758f44f45345b6a90b940ba7d..v0.3.0) - 2026-08-02

### Features

- import the shared workflows, actions, and Renovate presets - ([29ae048](https://github.com/tbhb-dev/devtools/commit/29ae048a69f9079d480fe5a8463592d1137c2723)) - [@tbhb](https://github.com/tbhb)
- import the shared commit-msg hooks from tbhb/pre-commit-hooks - ([c6999ed](https://github.com/tbhb-dev/devtools/commit/c6999edffe6fb59dfb40665174aa9f2605122880)) - [@tbhb](https://github.com/tbhb)
- clear a draft's prose findings in a subagent - ([d7e0da0](https://github.com/tbhb-dev/devtools/commit/d7e0da0e9526728e9438ef00bb3715a5a75b5c61)) - [@tbhb](https://github.com/tbhb)
- apply the corrections vale already wrote - ([2f817fb](https://github.com/tbhb-dev/devtools/commit/2f817fb75fe274349871298455e3efcdc6f97106)) - [@tbhb](https://github.com/tbhb)
- judge a whole draft in one call - ([3478b0e](https://github.com/tbhb-dev/devtools/commit/3478b0e9b219909218b4951beda0ca7aa7a25cb0)) - [@tbhb](https://github.com/tbhb)
- analyze a past session into a retrospective - ([0142bda](https://github.com/tbhb-dev/devtools/commit/0142bdac6461f06829cb41e4e8ad249c97bf456d)) - [@tbhb](https://github.com/tbhb)
- settle a rebase without reading the markers - ([47a3315](https://github.com/tbhb-dev/devtools/commit/47a3315a56c268e97e47279dec82bcfb2ac425c7)) - [@tbhb](https://github.com/tbhb)
- write a squash message from the commits it collapses - ([4fca3d3](https://github.com/tbhb-dev/devtools/commit/4fca3d3ecf438fff4190158da0e84520a6990267)) - [@tbhb](https://github.com/tbhb)
- redeploy APM primitives from a cleared hook state - ([51343b9](https://github.com/tbhb-dev/devtools/commit/51343b9bf585b509a0e88a1aa7740ace322634f2)) - [@tbhb](https://github.com/tbhb)
- ![BREAKING](https://img.shields.io/badge/BREAKING-red) drop the go-lint hook from the package - ([2efb7ce](https://github.com/tbhb-dev/devtools/commit/2efb7ce34f56efd733ee9861433f6f89a0e1bc44)) - [@tbhb](https://github.com/tbhb)
- check a commit against what the index held - ([da96951](https://github.com/tbhb-dev/devtools/commit/da969519a75d8d0585f93f1889639d87ac0a80b5)) - [@tbhb](https://github.com/tbhb)

### Bug Fixes

- hash the vale tree without find -exec - ([427faae](https://github.com/tbhb-dev/devtools/commit/427faaebb432953111486ce93d8d76b73e69c5d8)) - [@tbhb](https://github.com/tbhb)
- give the squash message a vale scope - ([9a50c31](https://github.com/tbhb-dev/devtools/commit/9a50c3114b324b2c284b4e9594e2eae846f01725)) - [@tbhb](https://github.com/tbhb)
- stop the wrap check refusing a GitHub alert - ([e7f7352](https://github.com/tbhb-dev/devtools/commit/e7f735254fc81569c59a28632d427df7c6891c39)) - [@tbhb](https://github.com/tbhb)
- stamp an amend review the caller asked for - ([778790d](https://github.com/tbhb-dev/devtools/commit/778790d94ddfe68f5a47487c8b1324bfa15bf0db)) - [@tbhb](https://github.com/tbhb)
- stop the token count recording what it never measured - ([e5043ab](https://github.com/tbhb-dev/devtools/commit/e5043abf2b419cde5ab71bf521dcb2798a144432)) - [@tbhb](https://github.com/tbhb)
- exempt the generated changelog from the wrap rule - ([69c0821](https://github.com/tbhb-dev/devtools/commit/69c0821d0c14cb9f9a20f3a1d7aa150d11c22097)) - [@tbhb](https://github.com/tbhb)
- let a sibling skill start its own rebase - ([eadde56](https://github.com/tbhb-dev/devtools/commit/eadde562ea4404070c252c21f22d6a8e4e45fc56)) - [@tbhb](https://github.com/tbhb)
- fail a secret scan that walked no commits - ([efa1838](https://github.com/tbhb-dev/devtools/commit/efa1838d9ef29091338bba326ea966c8c661018b)) - [@tbhb](https://github.com/tbhb)
- hold the whitespace gate to files git tracks - ([b5db2f8](https://github.com/tbhb-dev/devtools/commit/b5db2f83624845ea4d78cb06392bfcc33b0d7655)) - [@tbhb](https://github.com/tbhb)
- read the wider script list in the hygiene check - ([63ea838](https://github.com/tbhb-dev/devtools/commit/63ea8382f0cab9955783de6d59d5b81c88ef867d)) - [@tbhb](https://github.com/tbhb)
- leave one way to commit - ([b4d1dd5](https://github.com/tbhb-dev/devtools/commit/b4d1dd5c69c1db504f2eeafe93d716258625878d)) - [@tbhb](https://github.com/tbhb)
- format the scripts git isn't tracking yet - ([09be789](https://github.com/tbhb-dev/devtools/commit/09be789a836c7d58c6cc51d8f1c14126016a7bfb)) - [@tbhb](https://github.com/tbhb)
- restore vocabulary casing checks in Go source - ([78c1a18](https://github.com/tbhb-dev/devtools/commit/78c1a189d9f8fe2378bcc07536a6b78523e278ef)) - [@tbhb](https://github.com/tbhb)
- stand the commit guard down when its work is done - ([15abe71](https://github.com/tbhb-dev/devtools/commit/15abe711eb99e9e619ca61a87620883ed28347cc)) - [@tbhb](https://github.com/tbhb)
- read a cd target the way the shell does - ([2faf047](https://github.com/tbhb-dev/devtools/commit/2faf04724a081b8caaa158043f6b0ac9758c0807)) - [@tbhb](https://github.com/tbhb)
- lint the scripts git isn't tracking yet - ([38896a5](https://github.com/tbhb-dev/devtools/commit/38896a55044eb8c4dc457282596aa1cd5310734e)) - [@tbhb](https://github.com/tbhb)
- stop the vocabulary capitalizing a common noun - ([10666ee](https://github.com/tbhb-dev/devtools/commit/10666eef0b2b1dd61ab39924a4efd4a6f337ef69)) - [@tbhb](https://github.com/tbhb)
- unblock the Windows test arm and the description gate (#5) - ([3be10c4](https://github.com/tbhb-dev/devtools/commit/3be10c4ea0eb708758f44f45345b6a90b940ba7d)) - [@tbhb](https://github.com/tbhb)

### Documentation

- clear the findings the ai-tells bump surfaced - ([c6ea9d6](https://github.com/tbhb-dev/devtools/commit/c6ea9d6eb66d8fcf52a4f3adab708450ff2a08ba)) - [@tbhb](https://github.com/tbhb)
- correct the version the hook bugs were found on - ([723fc4f](https://github.com/tbhb-dev/devtools/commit/723fc4f81d6dd0dc3482d4e2c56ee71f42210c03)) - [@tbhb](https://github.com/tbhb)
- measure a skill before deploying it - ([50e96cf](https://github.com/tbhb-dev/devtools/commit/50e96cf6867376f1b49f3041e1d758c9138bcc62)) - [@tbhb](https://github.com/tbhb)
- put the remaining skill descriptions on one line - ([4ff1596](https://github.com/tbhb-dev/devtools/commit/4ff159639a52656f4f65280251061aa60dd3c8ba)) - [@tbhb](https://github.com/tbhb)

### Build system

- generate a changelog the Markdown gate accepts - ([94cbd75](https://github.com/tbhb-dev/devtools/commit/94cbd751443312a763f13c07d73eb06f16e87fad)) - [@tbhb](https://github.com/tbhb)

### Refactoring

- ![BREAKING](https://img.shields.io/badge/BREAKING-red) rename the APM package to devtools - ([d422df1](https://github.com/tbhb-dev/devtools/commit/d422df1f8576ba63ade2596f64daddc2863353e8)) - [@tbhb](https://github.com/tbhb)
- ![BREAKING](https://img.shields.io/badge/BREAKING-red) rename the Python distribution to devtools - ([9e1e4d8](https://github.com/tbhb-dev/devtools/commit/9e1e4d895084934ec7f134f74d3ec6186f0612ed)) - [@tbhb](https://github.com/tbhb)
- ![BREAKING](https://img.shields.io/badge/BREAKING-red) move the Go module to github.com/tbhb-dev/devtools - ([607c4ad](https://github.com/tbhb-dev/devtools/commit/607c4ade56929d993ddd24efee1cbc040973fb71)) - [@tbhb](https://github.com/tbhb)

- - -

<!-- release -->

## [v0.2.0](https://github.com/tbhb/agent-tools/compare/ef1a589925a9fa434c9867021ddc76da9766b69e..v0.2.0) - 2026-08-02

### Features

- let a fix land as an amend - ([8d4029b](https://github.com/tbhb/agent-tools/commit/8d4029b1c1c8f060605f0a8668e4c2978c84a680)) - [@tbhb](https://github.com/tbhb)
- draft pull request descriptions in a forked agent - ([871086a](https://github.com/tbhb/agent-tools/commit/871086a8ba8c5389510a708ee082fa6f803f07ff)) - [@tbhb](https://github.com/tbhb)
- run pull requests through a reviewed workflow - ([1cfa984](https://github.com/tbhb/agent-tools/commit/1cfa984c6e7a135cded45b90ab2e139d1f6457e6)) - [@tbhb](https://github.com/tbhb)
- measure what a skill costs to carry - ([b8c33db](https://github.com/tbhb/agent-tools/commit/b8c33db393f37222e40acf1aac938e776a0a7d9c)) - [@tbhb](https://github.com/tbhb)
- wire check-markdown into every gate that runs it - ([ffb6307](https://github.com/tbhb/agent-tools/commit/ffb630727374b68fe0d91b703190e070a98e99c5)) - [@tbhb](https://github.com/tbhb)
- add check-markdown for one-line Markdown paragraphs - ([714bdf8](https://github.com/tbhb/agent-tools/commit/714bdf82fafd7b492dbf6e79e6190d20f77b4114)) - [@tbhb](https://github.com/tbhb)
- register the go-lint hook as an apm primitive - ([c60e9da](https://github.com/tbhb/agent-tools/commit/c60e9da318380026668f8addb3f2879eb134a9ea)) - [@tbhb](https://github.com/tbhb)
- gate agent commits behind review and a guard - ([ef1a589](https://github.com/tbhb/agent-tools/commit/ef1a589925a9fa434c9867021ddc76da9766b69e)) - [@tbhb](https://github.com/tbhb)

### Bug Fixes

- stop the skill check passing a file it never read - ([77ebce0](https://github.com/tbhb/agent-tools/commit/77ebce0ca05c90409d68dd69291c1a1d410b5efa)) - [@tbhb](https://github.com/tbhb)
- pin the settings that reshape what an agent reads - ([b020218](https://github.com/tbhb/agent-tools/commit/b020218b81cafb4aaf2465339cf491edb5536cd8)) - [@tbhb](https://github.com/tbhb)
- anchor the commit guard on real invocations - ([e3c14fd](https://github.com/tbhb/agent-tools/commit/e3c14fd1fc889726b409a913c46b519a01248e04)) - [@tbhb](https://github.com/tbhb)
- ![BREAKING](https://img.shields.io/badge/BREAKING-red) rename the Markdown guard off the check prefix - ([b2cf3df](https://github.com/tbhb/agent-tools/commit/b2cf3dffbf5f58caf0fd99e9dfbe0cdf850f3db3)) - [@tbhb](https://github.com/tbhb)
- create the cosmic-ray session directory before init - ([782a761](https://github.com/tbhb/agent-tools/commit/782a761f57da7e0892797b9d0962c740c0b55a38)) - [@tbhb](https://github.com/tbhb)
- point the Python fuzz sweep at the real property tests - ([7441bfa](https://github.com/tbhb/agent-tools/commit/7441bfac68954069bf4b3b2954d085ac6397a4d9)) - [@tbhb](https://github.com/tbhb)

### Documentation

- point the Python comments at packages/ - ([ea2cefd](https://github.com/tbhb/agent-tools/commit/ea2cefdfd66c5d8b7b0318a1f995a1b6a83e5a70)) - [@tbhb](https://github.com/tbhb)
- describe the Python toolchain and the recipe naming - ([40794bf](https://github.com/tbhb/agent-tools/commit/40794bfa9edc5fdca595ad370cbd86812915b417)) - [@tbhb](https://github.com/tbhb)

### Build system

- aim the Python mutation sweep at the workspace packages - ([02dd6b3](https://github.com/tbhb/agent-tools/commit/02dd6b3f25ca1d4ae04c34f180626950b19e182e)) - [@tbhb](https://github.com/tbhb)
- gate Python on the pre-commit stage - ([16277a9](https://github.com/tbhb/agent-tools/commit/16277a90a1f084cce8316f4ddb8b7e51d8826bb4)) - [@tbhb](https://github.com/tbhb)
- add the Python recipe surface - ([e037845](https://github.com/tbhb/agent-tools/commit/e03784550a1e64ec125ea55603d56f94347e8986)) - [@tbhb](https://github.com/tbhb)
- suffix the Go recipes with -go - ([22ac113](https://github.com/tbhb/agent-tools/commit/22ac113947e3708e087430335628f0e7a53ba1ac)) - [@tbhb](https://github.com/tbhb)
- add the Python toolchain on a uv workspace - ([09a2329](https://github.com/tbhb/agent-tools/commit/09a23296d5526ada3a4678d5152dfae05022a26d)) - [@tbhb](https://github.com/tbhb)

- - -

<!-- release -->

## [v0.1.1](https://github.com/tbhb/agent-tools/compare/b0b1c9f71068f0920ab239d42681f2af34f345a9..v0.1.1) - 2026-08-01

### Documentation

- stop hard-wrapping the markdown prose (#4) - ([6f75183](https://github.com/tbhb/agent-tools/commit/6f75183cda5663100cdc9737970b5b8ca8c21a0f)) - [@tbhb](https://github.com/tbhb)

### Continuous Integration

- source the commit-msg gates from the tbhb hook repo (#1) - ([b0b1c9f](https://github.com/tbhb/agent-tools/commit/b0b1c9f71068f0920ab239d42681f2af34f345a9)) - [@tbhb](https://github.com/tbhb)

- - -

<!-- release -->

## [v0.1.0](https://github.com/tbhb/agent-tools/compare/049e81a012a34887cd44a1a256cdfbd5656de6d0..v0.1.0) - 2026-08-01

### Features

- publish the agent primitives as an apm package - ([aab294e](https://github.com/tbhb/agent-tools/commit/aab294e22a54ac108f6325657669e86f84946e0f)) - [@tbhb](https://github.com/tbhb)
- add the agent CLIs as a vendored Go module - ([1136ae6](https://github.com/tbhb/agent-tools/commit/1136ae65ad62993885fc003a7d689497d547c539)) - [@tbhb](https://github.com/tbhb)

### Documentation

- explain the tools and route review - ([cacbd52](https://github.com/tbhb/agent-tools/commit/cacbd52b9f2cf9e09b2a660cb08c067b69287174)) - [@tbhb](https://github.com/tbhb)

### Build system

- put just in front of the whole toolchain - ([0e43e85](https://github.com/tbhb/agent-tools/commit/0e43e854e86dae7af53f147b65c0522e03af09aa)) - [@tbhb](https://github.com/tbhb)

### Continuous Integration

- run the gates and the scanners on GitHub Actions - ([42f6a6a](https://github.com/tbhb/agent-tools/commit/42f6a6a7e6b2b7e944f5d3a7f908a500a827d2ae)) - [@tbhb](https://github.com/tbhb)
