<!-- markdownlint-disable MD024 -->
# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- Render an sdr version of hdr clips with the clip by @nicholas-fedor in [#390](https://github.com/PapagoLabs/outtake/pull/390)
- Make keep hdr a clip profile setting by @nicholas-fedor in [#386](https://github.com/PapagoLabs/outtake/pull/386)
- Export clips that keep hdr as hevc main 10 by @nicholas-fedor in [#378](https://github.com/PapagoLabs/outtake/pull/378)
- Ship static ffmpeg 9.0.2 from a digest-pinned image by @nicholas-fedor in [#376](https://github.com/PapagoLabs/outtake/pull/376)
- Parse progress incrementally and stop downloading to check files by @nicholas-fedor in [#351](https://github.com/PapagoLabs/outtake/pull/351)
- Persist sessions and csrf tokens in the database by @nicholas-fedor in [#326](https://github.com/PapagoLabs/outtake/pull/326)

### Chores

- Update nicholas-fedor/govulncheck-action action to v1.1.0 by @renovate[bot] in [#388](https://github.com/PapagoLabs/outtake/pull/388)
- Group mock imports and regenerate every mock by @nicholas-fedor in [#384](https://github.com/PapagoLabs/outtake/pull/384)
- Update module golang.org/x/net to v0.60.0 by @renovate[bot] in [#380](https://github.com/PapagoLabs/outtake/pull/380)
- Update golang docker tag to v1.27.2 by @renovate[bot] in [#373](https://github.com/PapagoLabs/outtake/pull/373)
- Update aws-sdk-go-v2 monorepo by @renovate[bot] in [#372](https://github.com/PapagoLabs/outtake/pull/372)
- Update go module directive to v1.27.2 by @renovate[bot] in [#366](https://github.com/PapagoLabs/outtake/pull/366)
- Update github/codeql-action action to v4.38.3 by @renovate[bot] in [#362](https://github.com/PapagoLabs/outtake/pull/362)
- Update step-security/harden-runner action to v2.22.1 by @renovate[bot] in [#353](https://github.com/PapagoLabs/outtake/pull/353)
- Update github.com/google/pprof digest to 7bae8d8 by @renovate[bot] in [#352](https://github.com/PapagoLabs/outtake/pull/352)
- Update golang.org/x/exp digest to f45ad48 by @renovate[bot] in [#348](https://github.com/PapagoLabs/outtake/pull/348)
- Update module github.com/aws/smithy-go to v1.28.4 by @renovate[bot] in [#341](https://github.com/PapagoLabs/outtake/pull/341)
- Update aws-sdk-go-v2 monorepo by @renovate[bot] in [#337](https://github.com/PapagoLabs/outtake/pull/337)
- Update module github.com/fxamacker/cbor/v2 to v2.9.6 by @renovate[bot] in [#330](https://github.com/PapagoLabs/outtake/pull/330)
- Update module github.com/aws/smithy-go to v1.28.3 by @renovate[bot] in [#329](https://github.com/PapagoLabs/outtake/pull/329)
- Update github.com/google/pprof digest to d99a617 by @renovate[bot] in [#324](https://github.com/PapagoLabs/outtake/pull/324)
- Update module github.com/mattn/go-colorable to v0.1.16 by @renovate[bot] in [#318](https://github.com/PapagoLabs/outtake/pull/318)
- Update step-security/harden-runner action to v2.22.0 by @renovate[bot] in [#317](https://github.com/PapagoLabs/outtake/pull/317)
- Update golang.org/x/exp digest to 7677206 by @renovate[bot] in [#314](https://github.com/PapagoLabs/outtake/pull/314)
- Update github.com/google/pprof digest to 639476b by @renovate[bot] in [#313](https://github.com/PapagoLabs/outtake/pull/313)

### Fixed

- Stop duplicate exports and read the plex position when marking by @nicholas-fedor in [#382](https://github.com/PapagoLabs/outtake/pull/382)
- Theme the option lists that selects open by @nicholas-fedor in [#370](https://github.com/PapagoLabs/outtake/pull/370)
- Tone-map with mobius to BT.709 and show HDR previews in SDR on SDR screens by @nicholas-fedor in [#368](https://github.com/PapagoLabs/outtake/pull/368)
- Refresh the bound plex server's token on sign-in by @nicholas-fedor in [#364](https://github.com/PapagoLabs/outtake/pull/364)
- Make keep HDR the only HDR switch and tone-map every still by @nicholas-fedor in [#360](https://github.com/PapagoLabs/outtake/pull/360)
- Play regenerated clips at once and cache pages and plex calls by @nicholas-fedor in [#358](https://github.com/PapagoLabs/outtake/pull/358)
- Fill every clip card from its source so HDR clips offer the HDR choice by @nicholas-fedor in [#356](https://github.com/PapagoLabs/outtake/pull/356)
- Validate selections everywhere and tone map HDR stills and GIFs by @nicholas-fedor in [#347](https://github.com/PapagoLabs/outtake/pull/347)
- Stage renders and publish them only on success by @nicholas-fedor in [#345](https://github.com/PapagoLabs/outtake/pull/345)
- Keep clip edits consistent across the queue and database by @nicholas-fedor in [#343](https://github.com/PapagoLabs/outtake/pull/343)
- Own job copies and never block on submit by @nicholas-fedor in [#339](https://github.com/PapagoLabs/outtake/pull/339)
- Keep tokens server-side and bind a reachable connection by @nicholas-fedor in [#335](https://github.com/PapagoLabs/outtake/pull/335)
- Confine the thumbnail proxy and share plex connections by @nicholas-fedor in [#333](https://github.com/PapagoLabs/outtake/pull/333)
- Copy request strings and bound server timeouts by @nicholas-fedor in [#322](https://github.com/PapagoLabs/outtake/pull/322)
- Restrict outtake to a single plex owner by @nicholas-fedor in [#320](https://github.com/PapagoLabs/outtake/pull/320)

### Tests

- Fake ffmpeg with the test binary instead of shell scripts by @nicholas-fedor in [#327](https://github.com/PapagoLabs/outtake/pull/327)

## [0.3.1] - 2026-10-05

### Added

- Add an in-memory preview render registry by @nicholas-fedor in [#238](https://github.com/PapagoLabs/outtake/pull/238)

### Changed

- Group packages by seam by @nicholas-fedor in [#303](https://github.com/PapagoLabs/outtake/pull/303)
- Show durations in the short form across the media UI by @nicholas-fedor in [#261](https://github.com/PapagoLabs/outtake/pull/261)
- Tone map HDR sources by default and tag the output for its transfer by @nicholas-fedor in [#257](https://github.com/PapagoLabs/outtake/pull/257)
- Show preview progress and allow cancelling a render by @nicholas-fedor in [#244](https://github.com/PapagoLabs/outtake/pull/244)
- Render previews in the background by @nicholas-fedor in [#242](https://github.com/PapagoLabs/outtake/pull/242)
- Key previews by their content and stage them before publishing by @nicholas-fedor in [#240](https://github.com/PapagoLabs/outtake/pull/240)
- Bound concurrent preview encodes by @nicholas-fedor in [#232](https://github.com/PapagoLabs/outtake/pull/232)
- Cache cropdetect and luma analysis by file identity by @nicholas-fedor in [#230](https://github.com/PapagoLabs/outtake/pull/230)
- Cache probe results by file identity by @nicholas-fedor in [#226](https://github.com/PapagoLabs/outtake/pull/226)
- Generate templ before the vulnerability scan by @nicholas-fedor in [#218](https://github.com/PapagoLabs/outtake/pull/218)
- Use the commit date for archive mtime by @nicholas-fedor in [#198](https://github.com/PapagoLabs/outtake/pull/198)
- Use the commit date for archive mtime by @nicholas-fedor in [#195](https://github.com/PapagoLabs/outtake/pull/195)

### Chores

- Update templ installation to v0.3.1070 by @nicholas-fedor in [#311](https://github.com/PapagoLabs/outtake/pull/311)
- Update module github.com/jackc/puddle/v2 to v2.2.3 by @renovate[bot] in [#309](https://github.com/PapagoLabs/outtake/pull/309)
- Track generated templ files by @nicholas-fedor in [#307](https://github.com/PapagoLabs/outtake/pull/307)
- Update module github.com/minio/minlz to v1.2.2 by @renovate[bot] in [#304](https://github.com/PapagoLabs/outtake/pull/304)
- Update module github.com/valyala/fasthttp to v1.75.0 by @renovate[bot] in [#301](https://github.com/PapagoLabs/outtake/pull/301)
- Lock file maintenance by @renovate[bot] in [#299](https://github.com/PapagoLabs/outtake/pull/299)
- Update module github.com/a-h/templ to v0.3.1070 by @renovate[bot] in [#298](https://github.com/PapagoLabs/outtake/pull/298)
- Update module github.com/molecule-man/go-brrr to v1.2.0 by @renovate[bot] in [#296](https://github.com/PapagoLabs/outtake/pull/296)
- Update github.com/google/pprof digest to ebaad5f by @renovate[bot] in [#295](https://github.com/PapagoLabs/outtake/pull/295)
- Update module golang.org/x/tools to v0.51.0 by @renovate[bot] in [#292](https://github.com/PapagoLabs/outtake/pull/292)
- Update module github.com/pierrec/lz4/v4 to v4.1.33 by @renovate[bot] in [#291](https://github.com/PapagoLabs/outtake/pull/291)
- Update module github.com/pierrec/lz4/v4 to v4.1.32 by @renovate[bot] in [#289](https://github.com/PapagoLabs/outtake/pull/289)
- Update github.com/google/pprof digest to 77d3b59 by @renovate[bot] in [#285](https://github.com/PapagoLabs/outtake/pull/285)
- Update github.com/google/pprof digest to 60bf690 by @renovate[bot] in [#281](https://github.com/PapagoLabs/outtake/pull/281)
- Update module github.com/tinylib/msgp to v1.6.5 by @renovate[bot] in [#270](https://github.com/PapagoLabs/outtake/pull/270)
- Update module github.com/aws/aws-sdk-go-v2/service/s3 to v1.114.0 by @renovate[bot] in [#271](https://github.com/PapagoLabs/outtake/pull/271)
- Update module github.com/minio/minlz to v1.2.1 by @renovate[bot] in [#266](https://github.com/PapagoLabs/outtake/pull/266)
- Update module github.com/klauspost/pgzip to v1.2.7 by @renovate[bot] in [#265](https://github.com/PapagoLabs/outtake/pull/265)
- Update module github.com/andybalholm/brotli to v1.2.6 by @renovate[bot] in [#259](https://github.com/PapagoLabs/outtake/pull/259)
- Add github sponsorship configuration by @nicholas-fedor in [#255](https://github.com/PapagoLabs/outtake/pull/255)
- Update module github.com/ncruces/go-strftime to v1.1.0 by @renovate[bot] in [#253](https://github.com/PapagoLabs/outtake/pull/253)
- Update module modernc.org/sqlite to v1.60.1 by @renovate[bot] in [#252](https://github.com/PapagoLabs/outtake/pull/252)
- Update module modernc.org/sqlite to v1.60.0 by @renovate[bot] in [#246](https://github.com/PapagoLabs/outtake/pull/246)
- Update module github.com/pierrec/lz4/v4 to v4.1.31 by @renovate[bot] in [#236](https://github.com/PapagoLabs/outtake/pull/236)
- Update github.com/google/pprof digest to aaccee0 by @renovate[bot] in [#228](https://github.com/PapagoLabs/outtake/pull/228)
- Update module github.com/onsi/gomega to v1.44.0 by @renovate[bot] in [#216](https://github.com/PapagoLabs/outtake/pull/216)
- Stop tracking generated templ output by @nicholas-fedor in [#214](https://github.com/PapagoLabs/outtake/pull/214)
- Update dependency golangci/golangci-lint to v2.14 by @renovate[bot] in [#212](https://github.com/PapagoLabs/outtake/pull/212)
- Update module github.com/klauspost/compress to v1.20.1 by @renovate[bot] in [#211](https://github.com/PapagoLabs/outtake/pull/211)
- Update module github.com/gofiber/schema to v1.8.8 by @renovate[bot] in [#208](https://github.com/PapagoLabs/outtake/pull/208)
- Update module github.com/andybalholm/brotli to v1.2.5 by @renovate[bot] in [#207](https://github.com/PapagoLabs/outtake/pull/207)
- Update aws-sdk-go-v2 monorepo by @renovate[bot] in [#203](https://github.com/PapagoLabs/outtake/pull/203)
- Update github/codeql-action action to v4.38.2 by @renovate[bot] in [#204](https://github.com/PapagoLabs/outtake/pull/204)
- Update module github.com/gofiber/utils/v2 to v2.6.0 by @renovate[bot] in [#201](https://github.com/PapagoLabs/outtake/pull/201)
- Update module modernc.org/libc to v1.77.1 by @renovate[bot] in [#197](https://github.com/PapagoLabs/outtake/pull/197)
- Update module github.com/gofiber/utils/v2 to v2.5.3 by @renovate[bot] in [#194](https://github.com/PapagoLabs/outtake/pull/194)
- Update module github.com/aws/aws-sdk-go-v2/service/s3 to v1.113.2 by @renovate[bot] in [#191](https://github.com/PapagoLabs/outtake/pull/191)
- Update golang:1.27.1-alpine docker digest to 8a5910f by @renovate[bot] in [#190](https://github.com/PapagoLabs/outtake/pull/190)
- Update module modernc.org/libc to v1.77.0 by @renovate[bot] in [#189](https://github.com/PapagoLabs/outtake/pull/189)
- Update module github.com/dustin/go-humanize to v1.1.0 by @renovate[bot] in [#187](https://github.com/PapagoLabs/outtake/pull/187)
- Update module github.com/ulikunitz/xz to v0.5.17 by @renovate[bot] in [#186](https://github.com/PapagoLabs/outtake/pull/186)
- Update orhun/git-cliff-action action to v4.9.1 by @renovate[bot] in [#184](https://github.com/PapagoLabs/outtake/pull/184)
- Update module github.com/molecule-man/go-brrr to v1.1.1 by @renovate[bot] in [#183](https://github.com/PapagoLabs/outtake/pull/183)
- Update module github.com/aws/smithy-go to v1.28.2 by @renovate[bot] in [#181](https://github.com/PapagoLabs/outtake/pull/181)
- Update github/codeql-action action to v4.38.1 by @renovate[bot] in [#180](https://github.com/PapagoLabs/outtake/pull/180)
- Update golang:1.27.1-alpine docker digest to 4cb7ac9 by @renovate[bot] in [#178](https://github.com/PapagoLabs/outtake/pull/178)
- Update alpine:3.24.2 docker digest to 294b683 by @renovate[bot] in [#177](https://github.com/PapagoLabs/outtake/pull/177)
- Update golang:1.27.1-alpine docker digest to e9bbdf2 by @renovate[bot] in [#175](https://github.com/PapagoLabs/outtake/pull/175)
- Update alpine:3.24.2 docker digest to 31b6477 by @renovate[bot] in [#174](https://github.com/PapagoLabs/outtake/pull/174)
- Update module github.com/onsi/ginkgo/v2 to v2.33.0 by @renovate[bot] in [#170](https://github.com/PapagoLabs/outtake/pull/170)
- Update alpine docker tag to v3.24.2 by @renovate[bot] in [#172](https://github.com/PapagoLabs/outtake/pull/172)
- Update module github.com/onsi/gomega to v1.43.1 by @renovate[bot] in [#169](https://github.com/PapagoLabs/outtake/pull/169)
- Update module modernc.org/libc to v1.76.0 by @renovate[bot] in [#168](https://github.com/PapagoLabs/outtake/pull/168)
- Update docker/setup-buildx-action action to v4.4.1 by @renovate[bot] in [#167](https://github.com/PapagoLabs/outtake/pull/167)
- Update module modernc.org/sqlite to v1.59.0 by @renovate[bot] in [#165](https://github.com/PapagoLabs/outtake/pull/165)
- Update docker/setup-qemu-action action to v4.4.0 by @renovate[bot] in [#163](https://github.com/PapagoLabs/outtake/pull/163)
- Update docker/setup-buildx-action action to v4.4.0 by @renovate[bot] in [#162](https://github.com/PapagoLabs/outtake/pull/162)

### Fixed

- Refuse a clip mark that is not a timecode by @nicholas-fedor in [#287](https://github.com/PapagoLabs/outtake/pull/287)
- Give the queue a lifetime and make its teardown safe by @nicholas-fedor in [#283](https://github.com/PapagoLabs/outtake/pull/283)
- Hand out job copies instead of the queue's own pointers by @nicholas-fedor in [#279](https://github.com/PapagoLabs/outtake/pull/279)
- Refuse a second job for an id that is already rendering by @nicholas-fedor in [#277](https://github.com/PapagoLabs/outtake/pull/277)
- Recover a panic in a job handler instead of taking the process down by @nicholas-fedor in [#275](https://github.com/PapagoLabs/outtake/pull/275)
- Restore ownership when a clip delete cannot be completed by @nicholas-fedor in [#274](https://github.com/PapagoLabs/outtake/pull/274)
- Stop a deleted job from being written back by @nicholas-fedor in [#269](https://github.com/PapagoLabs/outtake/pull/269)
- Bound a clip selection by the source and report it in place by @nicholas-fedor in [#263](https://github.com/PapagoLabs/outtake/pull/263)
- Derive clip length from the marks instead of the posted duration by @nicholas-fedor in [#250](https://github.com/PapagoLabs/outtake/pull/250)
- Preview the selection instead of a fixed 30 seconds by @nicholas-fedor in [#248](https://github.com/PapagoLabs/outtake/pull/248)
- Fix changelog automation workflows by @nicholas-fedor in [#234](https://github.com/PapagoLabs/outtake/pull/234)
- Persist clip width and fps on update by @nicholas-fedor in [#224](https://github.com/PapagoLabs/outtake/pull/224)
- Reject unsafe preview ids before building a path by @nicholas-fedor in [#222](https://github.com/PapagoLabs/outtake/pull/222)
- Poll only the live region of a clip card by @nicholas-fedor in [#220](https://github.com/PapagoLabs/outtake/pull/220)
- Return 200 from clip delete so htmx removes the card by @nicholas-fedor in [#219](https://github.com/PapagoLabs/outtake/pull/219)
- Carry export form state across preview redirects by @nicholas-fedor in [#215](https://github.com/PapagoLabs/outtake/pull/215)
- Preserve millisecond precision on preview marks by @nicholas-fedor in [#210](https://github.com/PapagoLabs/outtake/pull/210)

## [0.2.1] - 2026-09-15

### Chores

- Lock file maintenance by @renovate[bot] in [#153](https://github.com/PapagoLabs/outtake/pull/153)
- Update module github.com/pierrec/lz4/v4 to v4.1.30 by @renovate[bot] in [#160](https://github.com/PapagoLabs/outtake/pull/160)
- Update module github.com/gofiber/schema to v1.8.7 by @renovate[bot] in [#159](https://github.com/PapagoLabs/outtake/pull/159)
- Update module github.com/fxamacker/cbor/v2 to v2.9.4 by @renovate[bot] in [#157](https://github.com/PapagoLabs/outtake/pull/157)
- Update module github.com/aws/aws-sdk-go-v2/credentials to v1.20.5 by @renovate[bot] in [#156](https://github.com/PapagoLabs/outtake/pull/156)
- Update module github.com/gofiber/utils/v2 to v2.5.2 by @renovate[bot] in [#154](https://github.com/PapagoLabs/outtake/pull/154)
- Update module github.com/aws/aws-sdk-go-v2/service/s3 to v1.113.1 by @renovate[bot] in [#151](https://github.com/PapagoLabs/outtake/pull/151)
- Update module github.com/andybalholm/brotli to v1.2.4 by @renovate[bot] in [#149](https://github.com/PapagoLabs/outtake/pull/149)
- Update module github.com/molecule-man/go-brrr to v1.1.0 by @renovate[bot] in [#148](https://github.com/PapagoLabs/outtake/pull/148)
- Update aws-sdk-go-v2 monorepo by @renovate[bot] in [#146](https://github.com/PapagoLabs/outtake/pull/146)
- Update github/codeql-action action to v4.38.0 by @renovate[bot] in [#144](https://github.com/PapagoLabs/outtake/pull/144)

## [0.2.0] - 2026-09-09

### Added

- Add optional web-safe color for HDR clip exports by @nicholas-fedor in [#129](https://github.com/PapagoLabs/outtake/pull/129)

### Chores

- Update zizmorcore/zizmor-action action to v0.6.4 by @renovate[bot] in [#141](https://github.com/PapagoLabs/outtake/pull/141)
- Update module github.com/onsi/ginkgo/v2 to v2.32.2 by @renovate[bot] in [#140](https://github.com/PapagoLabs/outtake/pull/140)
- Update module golang.org/x/text to v0.42.0 by @renovate[bot] in [#139](https://github.com/PapagoLabs/outtake/pull/139)
- Update module github.com/aws/aws-sdk-go-v2/service/s3 to v1.112.0 by @renovate[bot] in [#137](https://github.com/PapagoLabs/outtake/pull/137)
- Update golang.org/x/exp digest to 85c1c22 by @renovate[bot] in [#136](https://github.com/PapagoLabs/outtake/pull/136)
- Update module golang.org/x/mod to v0.41.0 by @renovate[bot] in [#133](https://github.com/PapagoLabs/outtake/pull/133)
- Update module github.com/gofiber/schema to v1.8.6 by @renovate[bot] in [#132](https://github.com/PapagoLabs/outtake/pull/132)

### Fixed

- Stop GIF exports from decoding past the clip window by @nicholas-fedor in [#130](https://github.com/PapagoLabs/outtake/pull/130)

### Tests

- Replace real titles with type-labeled fixtures by @nicholas-fedor in [#127](https://github.com/PapagoLabs/outtake/pull/127)

## [0.1.1] - 2026-09-08

### Fixed

- Trust public origin behind tls proxy by @nicholas-fedor in [#125](https://github.com/PapagoLabs/outtake/pull/125)

## [0.1.0] - 2026-09-08

### Added

- Add media library sort, jump rail, and infinite scroll by @nicholas-fedor in [#114](https://github.com/PapagoLabs/outtake/pull/114)
- Add clip list filters and stable sort by @nicholas-fedor in [#112](https://github.com/PapagoLabs/outtake/pull/112)
- Add selectable ui color palettes by @nicholas-fedor in [#102](https://github.com/PapagoLabs/outtake/pull/102)
- Add Outtake chart with optional S3 and Postgres backends by @nicholas-fedor in [#68](https://github.com/PapagoLabs/outtake/pull/68)
- Add S3 and Postgres backends behind config by @nicholas-fedor in [#69](https://github.com/PapagoLabs/outtake/pull/69)
- Add README badge bar and centered header by @nicholas-fedor in [#65](https://github.com/PapagoLabs/outtake/pull/65)
- Add editor gopher brand icon set by @nicholas-fedor in [#62](https://github.com/PapagoLabs/outtake/pull/62)
- Add user-managed clip encode profiles by @nicholas-fedor in [#45](https://github.com/PapagoLabs/outtake/pull/45)
- Add GitHub Actions for templ, vet, lint, and test by @nicholas-fedor in [#3](https://github.com/PapagoLabs/outtake/pull/3)
- Add PapagoLabs style floor to AGENTS.md by @nicholas-fedor
- Add Plex clip manager by @nicholas-fedor

### Changed

- Install cosign during build workflow by @nicholas-fedor in [#123](https://github.com/PapagoLabs/outtake/pull/123)
- Disambiguate reusable concurrency groups by @nicholas-fedor in [#120](https://github.com/PapagoLabs/outtake/pull/120)
- Link live session titles to media by @nicholas-fedor in [#108](https://github.com/PapagoLabs/outtake/pull/108)
- Upgrade HTMX to v4 with CSP and CSRF by @nicholas-fedor in [#106](https://github.com/PapagoLabs/outtake/pull/106)
- Rewrite outtake agent instructions by @nicholas-fedor in [#100](https://github.com/PapagoLabs/outtake/pull/100)
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

- Update module github.com/jackc/pgx/v5 to v5.11.0 by @renovate[bot] in [#119](https://github.com/PapagoLabs/outtake/pull/119)
- Update module github.com/valyala/fasthttp to v1.74.0 by @renovate[bot] in [#117](https://github.com/PapagoLabs/outtake/pull/117)
- Update module github.com/gofiber/utils/v2 to v2.5.1 by @renovate[bot] in [#116](https://github.com/PapagoLabs/outtake/pull/116)
- Update github.com/google/pprof digest to 6331bc6 by @renovate[bot] in [#97](https://github.com/PapagoLabs/outtake/pull/97)
- Update module github.com/gofiber/utils/v2 to v2.5.0 by @renovate[bot] in [#96](https://github.com/PapagoLabs/outtake/pull/96)
- Update aws-sdk-go-v2 monorepo by @renovate[bot] in [#94](https://github.com/PapagoLabs/outtake/pull/94)
- Update github.com/google/pprof digest to d6c3cb2 by @renovate[bot] in [#93](https://github.com/PapagoLabs/outtake/pull/93)
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

- Align clip cards with the new export form by @nicholas-fedor in [#110](https://github.com/PapagoLabs/outtake/pull/110)
- Persist sidebar across navigation by @nicholas-fedor in [#104](https://github.com/PapagoLabs/outtake/pull/104)
- Overhaul library browse, clips, and shell by @nicholas-fedor in [#91](https://github.com/PapagoLabs/outtake/pull/91)
- Fix license badge by @nicholas-fedor in [#89](https://github.com/PapagoLabs/outtake/pull/89)
- Clear storage and database lint-go hits by @nicholas-fedor in [#75](https://github.com/PapagoLabs/outtake/pull/75)
- Clear gci noctx wsl and whitespace in handlers by @nicholas-fedor in [#17](https://github.com/PapagoLabs/outtake/pull/17)
- Clear assigned App lint hits by @nicholas-fedor in [#16](https://github.com/PapagoLabs/outtake/pull/16)
- Drop illegal exhaustruct_v5 exclude by @nicholas-fedor in [#13](https://github.com/PapagoLabs/outtake/pull/13)
- Use plex.EmptyServer in unbound handlers by @nicholas-fedor in [#6](https://github.com/PapagoLabs/outtake/pull/6)
- Fill empty structs for exhaustruct_v5 by @nicholas-fedor in [#5](https://github.com/PapagoLabs/outtake/pull/5)
- Redirect HTML form errors to the originating page by @nicholas-fedor in [#2](https://github.com/PapagoLabs/outtake/pull/2)

### New Contributors

- @github-actions[bot] made their first contribution in [#124](https://github.com/PapagoLabs/outtake/pull/124)
- @nicholas-fedor made their first contribution in [#123](https://github.com/PapagoLabs/outtake/pull/123)
- @renovate[bot] made their first contribution in [#119](https://github.com/PapagoLabs/outtake/pull/119)

## Compare Releases

- [unreleased](https://github.com/PapagoLabs/outtake/compare/v0.3.1...HEAD)
- [0.3.1](https://github.com/PapagoLabs/outtake/compare/v0.2.1...v0.3.1)
- [0.2.1](https://github.com/PapagoLabs/outtake/compare/v0.2.0...v0.2.1)
- [0.2.0](https://github.com/PapagoLabs/outtake/compare/v0.1.1...v0.2.0)
- [0.1.1](https://github.com/PapagoLabs/outtake/compare/v0.1.0...v0.1.1)

<!-- generated by git-cliff -->
