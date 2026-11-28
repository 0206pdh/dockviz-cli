# 잔존 리소스(Leftovers) 탐지 계획

S3의 미완료 multipart upload처럼, 평소에는 보이지 않으면서 디스크를 계속 차지하는
Docker 리소스를 찾아 보여주고 안전하게 정리하는 기능의 설계 계획입니다. 구현 전
단계의 문서이며, 항목 ID는 [TODOS.ko.md](../TODOS.ko.md)의 T-023, T-024 ~ T-029와 같습니다.

---

## 1. 대상 리소스와 사실 확인

공식 동작을 기준으로 확인했습니다. "실측 필요"로 표시한 항목은 Phase 0에서 실제
daemon으로 확인한 뒤 확정합니다.

| 리소스 | 생기는 이유 | 확인 결과 | dockviz 현재 상태 |
|---|---|---|---|
| 멈춘 컨테이너 | `docker run`에 `--rm`이 없음 | 맞음. `Exited` 상태로 쓰기 레이어와 로그 파일이 남음 | Containers 카테고리 prune으로 처리 |
| dangling 이미지 | 같은 태그로 다시 build/pull | classic image store에서는 맞음(이전 이미지가 `<none>:<none>`으로 남음). **containerd image store(Docker Desktop 기본)에서는 같은 태그 재빌드가 dangling 이미지를 남기지 않음(2026-11-27 실측)** | Images prune으로 처리 |
| 익명 볼륨 | Dockerfile `VOLUME`, `-v /path` | 맞음. `docker rm`에 `-v`가 없으면 남음. 단, `docker run --rm`은 익명 볼륨도 함께 지움 | Volumes prune으로 처리 |
| 빌드 캐시 | BuildKit 캐시 누적 | 맞음. 단, `docker buildx create`로 만든 별도 builder의 캐시는 그 builder의 `buildx_buildkit_*_state` 볼륨에 따로 쌓여 기본 build cache 집계와 `docker builder prune`에서 빠짐 | 기본 builder만 처리 |
| 네트워크 | compose나 수동으로 만든 bridge | 맞음. 다만 디스크는 거의 차지하지 않음. 실제 문제는 기본 주소 풀 고갈(`could not find an available, non-overlapping IPv4 address pool`) | 없음 |
| 로그 파일 | json-file 드라이버에 rotation 없음 | 대체로 맞음. 정확히는 컨테이너가 **삭제될 때까지** 남음(멈춰도 유지). `daemon.json`의 `log-opts`나 `local` 드라이버를 쓰면 rotation됨. **Docker Desktop(Mac, 4.100.1)의 기본 드라이버는 `local`이고, 이때 `LogPath`가 빈 값이며 로그는 `<container>/local-logs/`에 있음(2026-11-27 실측)** | Container Logs 카테고리로 처리. `LogPath` 기준이라 `local` 드라이버는 측정 못 함 |
| compose orphan 컨테이너 | compose 파일에서 service를 빼고 `up` | 맞음. `--remove-orphans`로 지움. **실행 중일 수도 있음** | 없음 |

### 목록에 없던 항목

| 리소스 | 설명 |
|---|---|
| 삭제된 Compose 프로젝트의 named volume | `compose down`은 기본적으로 볼륨을 남김. Engine 23(API 1.42)부터 `docker volume prune`은 기본적으로 익명 볼륨만 지우므로 named volume은 가장 오래 남음. **S3 고아 객체와 가장 비슷한 경우** |
| `compose run` one-off 컨테이너 | `--rm` 없이 실행하면 `com.docker.compose.oneoff=True` 라벨이 붙은 컨테이너가 남음 |
| Docker가 추적하지 않는 overlay2 디렉터리 | daemon 비정상 종료나 삭제 실패로 남음. API에 나타나지 않음 |

### 계획에 영향을 주는 제약

- **볼륨의 "마지막 사용 시점"은 API에 없습니다.** `CreatedAt`과 라벨만 있습니다.
  "오래 안 쓴 볼륨"은 판단할 수 없고, 소유자(프로젝트)의 존재 여부로만 판단합니다.
- **`compose down` 후 볼륨을 일부러 남기는 경우가 정상 사용입니다.** DB 데이터를
  유지하려고 down/up을 반복하는 흔한 패턴이므로, 프로젝트 컨테이너가 없다는 이유만으로
  "삭제 대상"이라고 단정하면 안 됩니다.
- **볼륨 라벨에는 프로젝트 경로가 없습니다.** `com.docker.compose.project.working_dir`은
  컨테이너에만 붙습니다. 컨테이너까지 지워진 프로젝트는 디렉터리 존재 여부를 확인할 수
  없습니다.
- **기존 Volumes prune은 `all=true`입니다.** 연결되지 않은 named volume을 모두
  지우므로, 일부러 남긴 Compose 데이터 볼륨도 함께 지워집니다. 잔존 리소스 탐지가
  생기면 이 prune의 경고 문구도 함께 구체화해야 합니다.
- **개별 삭제 API는 회수 용량을 돌려주지 않습니다.** `VolumeRemove`,
  `ContainerRemove`는 prune과 달리 `SpaceReclaimed`가 없으므로, 삭제 전 측정값을
  "측정값 기준"으로 표시해야 합니다. "표시한 숫자 = 실제 회수량" 불변식과의 관계를
  문서에 명시합니다.

### 측정 경로: 로컬 daemon과 VM daemon

로그 파일, overlay2처럼 daemon의 데이터 디렉터리를 직접 봐야 하는 항목은 두 경로로
측정합니다.

| 경로 | 조건 | 방법 |
|---|---|---|
| 직접 | daemon과 같은 파일시스템 (네이티브 Linux) | 지금처럼 파일을 직접 `stat` |
| helper 컨테이너 | daemon이 VM 안이거나 원격 (Docker Desktop, 원격 host) | `docker info`의 `DockerRootDir`를 읽기 전용으로 마운트한 짧은 컨테이너를 띄워 크기만 읽고 삭제 |

2026-11-27 Docker Desktop(Mac)에서 `busybox` 컨테이너에 `/var/lib/docker`를
`:ro`로 마운트해 `containers/`, `volumes/` 크기와 컨테이너별 로그 디렉터리 크기를
읽을 수 있음을 확인했습니다.

helper 경로의 제약:

- 이미지가 필요합니다. 없으면 pull이 일어나므로 자동 갱신이 아니라 **요청 시에만**
  실행하고, 오프라인이면 "측정 불가"로 표시합니다.
- helper 컨테이너 자체가 event와 목록에 잡히므로 고정 라벨을 붙이고 dockviz의
  Problems와 잔존 리소스 판정에서 제외합니다.
- 데이터 디렉터리를 읽기 전용으로만 마운트하고, 삭제는 helper로 하지 않습니다.
  삭제는 계속 Docker API로만 합니다.
- Docker Desktop의 Enhanced Container Isolation 같은 설정이 이 마운트를 막을 수
  있으므로 실패하면 "측정 불가"로 표시합니다.

---

## 2. 판정 규칙 초안

각 항목에 **확실도**를 붙입니다. `certain`은 정리해도 되는 근거가 명확한 것,
`likely`는 사용자 판단이 필요한 것, `report`는 보여주기만 하는 것입니다.

| 종류 | 판정 조건 | 확실도 | 삭제 허용 |
|---|---|---|---|
| compose orphan 컨테이너 | 컨테이너의 `com.docker.compose.project`가 로드된 Compose 프로젝트와 같고, `com.docker.compose.service`가 현재 services에 없음 (`com.docker.compose.oneoff=True`는 제외) | certain (Compose 파일이 있을 때만 판정) | 멈춘 것만. 실측에서 orphan은 **계속 실행 중**이었으므로 대부분 안내 대상 |
| compose one-off 컨테이너 | `com.docker.compose.oneoff=True`이고 멈춤 | certain | 예 |
| 현재 프로젝트에서 빠진 볼륨 | 볼륨의 project 라벨이 로드된 프로젝트와 같고, `com.docker.compose.volume`이 현재 volumes에 없음. orphan 컨테이너가 붙잡고 있으면(refCount > 0) 그 orphan과 묶어서 표시 | certain | 연결된 컨테이너가 없을 때만 (데이터 경고 유지) |
| 주인 없는 Compose 볼륨 | project 라벨이 있지만 그 프로젝트의 컨테이너가 하나도 없음 | likely | 예 (프로젝트 이름, 생성일, 크기를 보여주고 개별 확인) |
| 익명 볼륨 | `com.docker.volume.anonymous` 라벨이 있고 연결된 컨테이너 없음 (실측: 라벨이 붙음, 이름은 64자 hex) | certain | 예 |
| 오래 멈춘 컨테이너 | `Exited`/`Dead`이고 inspect의 `State.FinishedAt`으로부터 N일 경과 (목록 API에는 종료 시각이 없어 멈춘 컨테이너만 inspect) | likely | 예 |
| buildx builder 캐시 | `buildx_buildkit_*_state` 볼륨 크기. buildkit 컨테이너가 사용 중이라 일반 볼륨 판정에서는 "사용 중"으로 보이므로 따로 분류 | report | 아니오 (`docker buildx prune --builder <name>` 안내) |
| 미사용 네트워크 | 사용자 생성 네트워크 중 연결된 컨테이너 없음 + 전체 개수/주소 풀 사용률 | report | 아니오 (1차) |
| 추적되지 않는 overlay2 | 로컬 Linux에서 `/var/lib/docker/overlay2` 실제 용량과 API 집계의 차이 | report | 아니오 (영구히 삭제 기능 없음) |

---

## 3. Phase 계획

진행 순서의 원칙은 세 가지입니다.

1. **연결이 먼저:** Mac의 Docker Desktop처럼 socket이 기본 위치에 없는
   환경에서 dockviz가 설정 없이 붙어야 실측도 테스트도 할 수 있습니다.
2. **API만으로 되는 것이 먼저:** 잔존 리소스 판정의 핵심(orphan 컨테이너, 볼륨)은
   Docker API만으로 계산되므로 로컬/VM/원격 어디서나 같게 동작합니다. 데이터
   디렉터리를 직접 봐야 하는 항목(로그 파일, overlay2)은 뒤로 미룹니다.
3. **읽기 전용이 먼저:** 판정과 표시를 충분히 써 본 뒤에 삭제를 붙입니다.

| Phase | ID | 내용 | 환경 의존 |
|---|---|---|---|
| 0 | T-023 | Docker context 지원 | 없음 |
| 1 | T-024 | 실측 시나리오 (개발자 검증용, Go) | 환경별로 실행해 비교 |
| 2 | T-025 | 탐지 로직 (읽기 전용, 순수 함수) | 없음 (API만 사용) |
| 3 | T-026 | 화면 표시 | 없음 |
| 4 | T-027 | 개별 삭제 + 실제 daemon 통합 테스트(T-007) | 없음 |
| 5 | T-028 | VM/원격 측정 경로(helper)와 보고 전용 항목 | 로컬/VM 분기 |
| 6 | T-029 | 정리 정책 조정 (선택) | 없음 |

### Phase 0: Docker context 지원 (T-023)

`docker` CLI와 같은 순서로 연결할 endpoint를 정합니다.

1. `--context` 플래그
2. `--host` 플래그 (`--context`와 함께 쓰면 오류)
3. `DOCKER_HOST`
4. `DOCKER_CONTEXT`
5. `~/.docker/config.json`(또는 `DOCKER_CONFIG`)의 `currentContext`
6. OS 기본값

`docker` CLI의 `resolveContextName`과 같은 순서입니다. context에서 읽은 endpoint는
`DOCKER_HOST`, `DOCKER_CERT_PATH` 같은 환경변수에 덮이지 않습니다.

context endpoint는 `~/.docker/contexts/meta/<sha256(이름)>/meta.json`의
`Endpoints.docker.Host`에서 읽습니다(2026-11-27 Mac Docker Desktop에서 구조 확인).
`docker/cli` 의존성은 추가하지 않고 직접 파싱합니다. TLS 인증서가 있는 context는
`contexts/tls/<sha256>/docker/`의 파일을 사용합니다.

**완료 기준:** Mac Docker Desktop에서 `dockviz`가 플래그나 환경변수 없이
연결됨. 우선순위 규칙은 임시 디렉터리를 쓰는 단위 테스트로 고정

### Phase 1: 실측 시나리오 (T-024)

**목적은 개발자 검증입니다.** 판정 규칙이 실제 API 필드로 계산되는지 코드 작성 전에
확인합니다. 사용자 기능이 아닙니다.

- `scenarios/leftovers/`에 Go 프로그램으로 작성 (OS 상관없이 같은 스크립트, T-008의
  첫 사례)
- 모든 리소스에 `dockviz.test=leftovers` 라벨을 붙이고, 마지막에 라벨 기준으로 정리.
  `--keep`이면 정리하지 않고 남겨서 dockviz 화면으로 확인 가능
- 만들 리소스: orphan service 컨테이너, `compose run` one-off, `down` 후 남은 named
  volume, `VOLUME` 이미지의 익명 볼륨, 같은 태그 재빌드로 생긴 dangling 이미지, 별도
  buildx builder, 사용자 네트워크 여러 개, 멈춘 컨테이너
- 각 리소스의 `List`/`Inspect`/`DiskUsage` 응답에서 라벨, `UsageData`, `CreatedAt`,
  `State.FinishedAt`, 로그 드라이버를 JSON으로 출력
- 실행 환경과 이미 알려진 차이:

| 환경 | 이미지 저장소 | 기본 로그 드라이버 | 상태 |
|---|---|---|---|
| Docker Desktop (Mac, 4.100.1) | containerd image store | `local` (`LogPath` 빈 값) | **2026-11-27 실행 완료** ([validation §10](validation.ko.md#10-잔존-리소스-실측-leftovers)) |
| 네이티브 Linux (CI ubuntu) | 실측 필요 | `json-file` 예상 | Phase 4에서 CI로 |
| Docker Desktop (Windows WSL2) | 기존 검증 환경 | 실측 필요 | 선택 |

- 결과는 [validation.ko.md](validation.ko.md)에 환경별로 기록하고 §1의 "실측 필요"를 확정

**완료 기준:** §2의 판정 조건이 모든 대상 환경에서 API 필드로 계산 가능하다는 근거가 있음.
환경마다 다른 항목은 판정 규칙에 분기로 반영

### Phase 2: 탐지 로직, 읽기 전용 (T-025)

- `internal/docker`에 `Leftover` 타입 추가: 종류, ID, 이름, 크기, 프로젝트, 판정
  이유, 확실도, 삭제 가능 여부
- 판정은 **순수 함수**: 입력은 컨테이너 목록, 볼륨 목록, `compose.Context`(없을 수
  있음), 현재 시각. Phase 1에서 저장한 실제 API 응답을 테스트 fixture로 사용
- `DockerClient`에 `ListLeftovers()` 추가, `DemoClient`에도 각 종류의 예시 데이터
- Docker API만 사용하므로 로컬/VM/원격 구분 없음. 삭제 기능 없음

**완료 기준:** §2의 certain/likely 항목이 단위 테스트로 고정됨. Compose 파일이 없을
때 Compose 의존 판정이 "판정 불가"로 빠지는 것까지 테스트

### Phase 3: 화면 표시 (T-026)

- Disk Usage에 "Leftovers" 행 추가. Enter로 목록 overlay를 열어 종류, 이름, 크기,
  판정 이유, 확실도를 표시
- Problems에는 합계만 info로 표시 (예: "3 leftover volumes, 4.2GB")
- 기존 Volumes prune 확인창에 "이 중 N개는 Compose 프로젝트 볼륨" 경고 추가
- [reference.ko.md](reference.ko.md)에 판정 규칙과 확실도 의미 문서화

**완료 기준:** `--demo`에서 모든 종류가 보이고, view 테스트로 화면이 고정됨.
`scenarios/leftovers --keep` 후 실제 daemon에서 같은 목록이 보임

### Phase 4: 개별 삭제 + 통합 테스트 (T-027, T-007)

- 목록에서 한 항목씩 `d`로 삭제, 확인창 필수. 일괄 삭제는 certain 항목에만 허용할지
  이 단계에서 결정
- 삭제 직전에 대상을 다시 조회해 상태가 바뀌었으면 중단
- `force` 사용 금지: 볼륨은 연결되어 있으면 실패하도록 두고, 컨테이너는 멈춘 것만 삭제
- 회수 용량은 "삭제 전 측정값" 기준임을 화면과 문서에 표시
- Phase 1 시나리오의 리소스 생성 코드를 재사용해 CI ubuntu에서 탐지와 삭제를 검증
  (`DOCKVIZ_INTEGRATION=1`)

**완료 기준:** CI에서 각 종류를 만들고 dockviz로 지운 뒤 라벨 기준으로 남은 것이 없음

### Phase 5: VM/원격 측정 경로와 보고 전용 항목 (T-028)

데이터 디렉터리를 직접 봐야 하는 항목을 §1 "측정 경로"의 helper 방식으로
로컬/VM/원격 모두에서 동작하게 합니다.

- 측정 경로 선택: 로컬에서 파일에 접근 가능하면 직접, 아니면 helper 컨테이너.
  helper는 요청 시에만 실행
- Container Logs 개선: `local` 드라이버(`local-logs/`) 지원, Docker Desktop에서도
  크기 측정. 기존 T-004(로그 스트리밍으로 크기 측정)를 대체
- 로그 정리(`PruneLogs`)는 helper로 하지 않음. VM에서는 지금처럼 "daemon 호스트에서
  정리" 안내 유지
- 보고 전용: buildx builder별 캐시 크기와 정리 명령, 미사용 네트워크와 주소 풀 고갈
  위험, overlay2/containerd 실제 용량과 API 집계 차이. 삭제 기능은 넣지 않음

**완료 기준:** Mac Docker Desktop과 네이티브 Linux에서 같은 항목이 측정되거나, 안 되는
환경에서는 이유와 함께 "측정 불가"로 표시됨

### Phase 6 (선택): 정리 정책 조정 (T-029)

- 오래 멈춘 컨테이너 기준 일수 등을 T-018 설정 파일로 조정
- 실제로 써 본 뒤 likely 판정의 오탐 사례를 반영

---

## 4. 결정이 필요한 것

| 항목 | 선택지 | 기본 제안 |
|---|---|---|
| 오래 멈춘 컨테이너 기준 | 7일 / 14일 / 30일 | 14일 |
| 주인 없는 Compose 볼륨 | likely로 보여주기 / 아예 제외 | likely로 보여주되 일괄 삭제에서 제외 |
| 일괄 삭제 | 허용 안 함 / certain만 허용 | Phase 4에서 개별 삭제를 먼저 써 보고 결정 |
| 기존 Volumes prune(`all=true`) | 유지 / 익명 볼륨만으로 축소 | 유지하고 경고만 구체화 (기존 검증 결과와 동작을 바꾸지 않음) |
| helper 이미지 | `busybox` 고정 태그 / 사용자 지정 | `busybox` 고정 태그, `--helper-image`로 변경 가능 |
