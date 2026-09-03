<!-- markdownlint-disable MD024 -->
# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Add Outtake chart with optional S3 and Postgres backends by @nicholas-fedor in [#68](https://github.com/PapagoLabs/outtake/pull/68)
- Add S3 and Postgres backends behind config by @nicholas-fedor in [#69](https://github.com/PapagoLabs/outtake/pull/69)
- Add README badge bar and centered header by @nicholas-fedor in [#65](https://github.com/PapagoLabs/outtake/pull/65)
- Add editor gopher brand icon set by @nicholas-fedor in [#62](https://github.com/PapagoLabs/outtake/pull/62)
- Add user-managed clip encode profiles by @nicholas-fedor in [#45](https://github.com/PapagoLabs/outtake/pull/45)
- Add GitHub Actions for templ, vet, lint, and test by @nicholas-fedor in [#3](https://github.com/PapagoLabs/outtake/pull/3)
- Add PapagoLabs style floor to AGENTS.md by @nicholas-fedor
- Add Plex clip manager by @nicholas-fedor

### Changed

- Extract page widgets, view models, and asset js by @nicholas-fedor in [#87](https://github.com/PapagoLabs/outtake/pull/87)
- Nest plex.tv XML decode under decode/plextv by @nicholas-fedor in [#85](https://github.com/PapagoLabs/outtake/pull/85)
- Nest PMS JSON decode under decode/pms by @nicholas-fedor in [#81](https://github.com/PapagoLabs/outtake/pull/81)
- Point README at examples/docker and k8s trees by @nicholas-fedor in [#82](https://github.com/PapagoLabs/outtake/pull/82)
- Nest docker and kubernetes example trees by @nicholas-fedor in [#80](https://github.com/PapagoLabs/outtake/pull/80)
- Nest ffmpeg helpers by concern by @nicholas-fedor in [#78](https://github.com/PapagoLabs/outtake/pull/78)
- Install Helm chart from GitHub by @nicholas-fedor in [#77](https://github.com/PapagoLabs/outtake/pull/77)
- Nest session monitor and accept Fetcher by @nicholas-fedor in [#70](https://github.com/PapagoLabs/outtake/pull/70)
- Document local compose and Helm deploy by @nicholas-fedor in [#74](https://github.com/PapagoLabs/outtake/pull/74)
- Rewrite README as a user-facing guide by @nicholas-fedor in [#63](https://github.com/PapagoLabs/outtake/pull/63)
- Bind reusable build to github environment by @nicholas-fedor in [#58](https://github.com/PapagoLabs/outtake/pull/58)
- Wire reusable goreleaser pipelines by @nicholas-fedor in [#52](https://github.com/PapagoLabs/outtake/pull/52)
- Preview clips and highlight the library browser by @nicholas-fedor in [#46](https://github.com/PapagoLabs/outtake/pull/46)
- Persist clip audio, crop, and name by @nicholas-fedor in [#44](https://github.com/PapagoLabs/outtake/pull/44)
- Map audio and encode browser-safe clips by @nicholas-fedor in [#43](https://github.com/PapagoLabs/outtake/pull/43)
- Detect letterbox crop rectangles by @nicholas-fedor in [#42](https://github.com/PapagoLabs/outtake/pull/42)
- Parse and format FFmpeg timecodes by @nicholas-fedor in [#41](https://github.com/PapagoLabs/outtake/pull/41)
- Pin golangci-lint-action to v9.3.0 by @nicholas-fedor in [#33](https://github.com/PapagoLabs/outtake/pull/33)
- Merge pull request #8 from PapagoLabs/renovate/github.com-google-pprof-digest by @nicholas-fedor in [#8](https://github.com/PapagoLabs/outtake/pull/8)
- Split GitHub Actions into hush-shaped workflows by @nicholas-fedor in [#4](https://github.com/PapagoLabs/outtake/pull/4)
- Adopt WaitGroup.Go and synctest in queue by @nicholas-fedor in [#1](https://github.com/PapagoLabs/outtake/pull/1)
- Unpin code-guide HEAD from AGENTS.md by @nicholas-fedor
- Point AGENTS.md at PapagoLabs/code-guide by @nicholas-fedor

### Chores

- Update orhun/git-cliff-action action to v4.9.0 by @renovate[bot] in [#60](https://github.com/PapagoLabs/outtake/pull/60)
- Update docker/setup-qemu-action action to v4.3.0 by @renovate[bot] in [#59](https://github.com/PapagoLabs/outtake/pull/59)
- Update zizmorcore/zizmor-action action to v0.6.3 by @renovate[bot] in [#56](https://github.com/PapagoLabs/outtake/pull/56)
- Update module modernc.org/sqlite to v1.58.0 by @renovate[bot] in [#38](https://github.com/PapagoLabs/outtake/pull/38)
- Update step-security/harden-runner action to v2.21.1 by @renovate[bot] in [#55](https://github.com/PapagoLabs/outtake/pull/55)
- Update module golang.org/x/crypto to v0.56.0 by @renovate[bot] in [#53](https://github.com/PapagoLabs/outtake/pull/53)
- Update module github.com/klauspost/compress to v1.20.0 by @renovate[bot] in [#49](https://github.com/PapagoLabs/outtake/pull/49)
- Update golang:1.27.1-alpine docker digest to cf6fca6 by @renovate[bot] in [#51](https://github.com/PapagoLabs/outtake/pull/51)
- Migrate gci extras and drop exhaustruct by @nicholas-fedor in [#50](https://github.com/PapagoLabs/outtake/pull/50)
- Drive compose from env files by @nicholas-fedor in [#39](https://github.com/PapagoLabs/outtake/pull/39)
- Update module github.com/gofiber/utils/v2 to v2.4.3 by @renovate[bot] in [#48](https://github.com/PapagoLabs/outtake/pull/48)
- Update golang:1.27-alpine docker digest to cf6fca6 by @renovate[bot] in [#47](https://github.com/PapagoLabs/outtake/pull/47)
- Update module modernc.org/libc to v1.75.7 by @renovate[bot] in [#37](https://github.com/PapagoLabs/outtake/pull/37)
- Update golang:1.27-alpine docker digest to 26402d8 by @renovate[bot] in [#36](https://github.com/PapagoLabs/outtake/pull/36)
- Update github.com/google/pprof digest to ca85771 by @renovate[bot] in [#35](https://github.com/PapagoLabs/outtake/pull/35)
- Bump Go to 1.27.1 by @nicholas-fedor in [#34](https://github.com/PapagoLabs/outtake/pull/34)
- Update go module directive to v1.27.0 by @renovate[bot] in [#21](https://github.com/PapagoLabs/outtake/pull/21)
- Stop libc v2 and Go toolchain automerge by @nicholas-fedor in [#32](https://github.com/PapagoLabs/outtake/pull/32)
- Lock file maintenance by @renovate[bot] in [#30](https://github.com/PapagoLabs/outtake/pull/30)
- Update actions/setup-go action to v7 by @renovate[bot] in [#26](https://github.com/PapagoLabs/outtake/pull/26)
- Update actions/checkout action to v7 by @renovate[bot] in [#25](https://github.com/PapagoLabs/outtake/pull/25)
- Update module github.com/gofiber/schema to v1.8.5 by @renovate[bot] in [#31](https://github.com/PapagoLabs/outtake/pull/31)
- Update zizmorcore/zizmor-action action to v0.6.3 by @renovate[bot] in [#29](https://github.com/PapagoLabs/outtake/pull/29)
- Update github.com/google/pprof digest to 4932ad3 by @renovate[bot] in [#28](https://github.com/PapagoLabs/outtake/pull/28)
- Update module modernc.org/sqlite to v1.57.0 by @renovate[bot] in [#23](https://github.com/PapagoLabs/outtake/pull/23)
- Update module github.com/onsi/gomega to v1.43.0 by @renovate[bot] in [#24](https://github.com/PapagoLabs/outtake/pull/24)
- Update module github.com/nwaples/rardecode/v2 to v2.4.1 by @renovate[bot] in [#22](https://github.com/PapagoLabs/outtake/pull/22)
- Update zizmorcore/zizmor-action action to v0.6.2 by @renovate[bot] in [#20](https://github.com/PapagoLabs/outtake/pull/20)
- Update module modernc.org/libc to v1.75.6 by @renovate[bot] in [#15](https://github.com/PapagoLabs/outtake/pull/15)
- Update module github.com/gofiber/utils/v2 to v2.4.2 by @renovate[bot] in [#11](https://github.com/PapagoLabs/outtake/pull/11)
- Update module github.com/templui/templui to v1.13.2 by @renovate[bot] in [#18](https://github.com/PapagoLabs/outtake/pull/18)
- Update module github.com/andybalholm/brotli to v1.2.3 by @renovate[bot] in [#14](https://github.com/PapagoLabs/outtake/pull/14)
- Update golang.org/x/exp digest to e88cd73 by @renovate[bot] in [#9](https://github.com/PapagoLabs/outtake/pull/9)
- Update github.com/google/pprof digest to 4d45320 by @renovate[bot]
- Bump Go to 1.26.7 by @nicholas-fedor in [#7](https://github.com/PapagoLabs/outtake/pull/7)

### Fixed

- Fix license badge by @nicholas-fedor in [#89](https://github.com/PapagoLabs/outtake/pull/89)
- Clear storage and database lint-go hits by @nicholas-fedor in [#75](https://github.com/PapagoLabs/outtake/pull/75)
- Clear gci noctx wsl and whitespace in handlers by @nicholas-fedor in [#17](https://github.com/PapagoLabs/outtake/pull/17)
- Clear assigned App lint hits by @nicholas-fedor in [#16](https://github.com/PapagoLabs/outtake/pull/16)
- Drop illegal exhaustruct_v5 exclude by @nicholas-fedor in [#13](https://github.com/PapagoLabs/outtake/pull/13)
- Use plex.EmptyServer in unbound handlers by @nicholas-fedor in [#6](https://github.com/PapagoLabs/outtake/pull/6)
- Fill empty structs for exhaustruct_v5 by @nicholas-fedor in [#5](https://github.com/PapagoLabs/outtake/pull/5)
- Redirect HTML form errors to the originating page by @nicholas-fedor in [#2](https://github.com/PapagoLabs/outtake/pull/2)

### New Contributors

- @nicholas-fedor made their first contribution in [#89](https://github.com/PapagoLabs/outtake/pull/89)
- @github-actions[bot] made their first contribution in [#88](https://github.com/PapagoLabs/outtake/pull/88)
- @renovate[bot] made their first contribution in [#60](https://github.com/PapagoLabs/outtake/pull/60)

## Compare Releases


<!-- generated by git-cliff -->
