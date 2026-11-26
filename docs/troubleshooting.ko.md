# dockviz-cli 버그 기록

개발 중 만난 버그의 증상, 원인, 해결을 기록합니다. 지원이 끝난 Networks 토폴로지
관련 항목은 정리했습니다(git 히스토리에 남아 있습니다).

---

## 1. 이미지 목록 순서가 불안정하고 여러 태그가 한 줄에 뭉쳐 나옴

**증상**
같은 이미지 ID에 태그가 여러 개면 `nginx:latest, nginx:alpine` 처럼 콤마로 합쳐서 한 행에 표시됨.
새로고침할 때마다 행 순서가 바뀜.

**원인**
`ImageList`가 반환하는 이미지 목록의 순서가 보장되지 않는다.
`Tags` 필드를 그대로 `strings.Join`으로 합치는 방식을 사용하고 있었음.

**해결**
태그 하나당 행 하나로 분리(one row per tag), 전체를 태그 이름 기준 알파벳 정렬로 변경.

```go
// 수정 전
type ImageInfo struct {
    Tags string // "nginx:latest, nginx:alpine"
}

// 수정 후
type ImageInfo struct {
    Tag     string   // "nginx:latest" (이 행의 태그)
    AllTags []string // ["nginx:latest", "nginx:alpine"] (삭제 경고용)
}
```

**관련 파일**: `internal/docker/images.go`, `internal/tui/view.go`

---

## 2. 이미지 삭제 시 태그 하나만 지우려도 이미지 전체가 삭제됨

**증상**
`nginx:alpine` 태그만 삭제하려고 `d` 키를 눌렀는데, `nginx:latest` 까지 같이 사라짐.

**원인**
`ImageRemove` 호출 시 `Force: true` 옵션을 사용하고 있었음.
`Force: true` 는 다른 태그가 있어도 강제로 이미지 전체를 삭제한다.

**해결**
`Force: false` 로 변경. 이렇게 하면 태그가 여러 개인 경우 해당 태그 하나만 제거되고 이미지는 남는다.
삭제 확인 팝업에도 멀티 태그 경고 문구를 추가해 의도를 명확히 전달.

```go
// 수정 전
client.ImageRemove(ctx, id, image.RemoveOptions{Force: true})

// 수정 후
client.ImageRemove(ctx, id, image.RemoveOptions{Force: false})
```

**관련 파일**: `internal/docker/images.go`

---

## 3. curl 설치 시 9바이트짜리 "Not found" 응답만 받음

**증상**
```bash
curl https://github.com/.../releases/latest/download/dockviz-linux-amd64 -o dockviz
# 결과: 9바이트 파일 ("Not found")
```

**원인**
GitHub Releases CDN이 실제 바이너리 URL로 **302 리다이렉트**를 보내는데,
`-L` 플래그 없이 curl을 실행하면 리다이렉트를 따라가지 않고 302 응답 본문만 저장한다.

**해결**
`-L` 플래그 추가 (리다이렉트 자동 추적), `-s` 플래그 추가 (진행 표시 억제).

```bash
# 수정 전
curl https://github.com/.../dockviz-linux-amd64 -o dockviz

# 수정 후
curl -sL https://github.com/.../dockviz-linux-amd64 -o dockviz
```

---

## 4. 새 버전 설치 후에도 dockviz --version이 구버전을 출력

**증상**
새 바이너리를 `/usr/local/bin/dockviz`에 설치했는데 `dockviz --version` 이 여전히 구버전 출력.

**원인**
이전에 설치된 바이너리가 다른 PATH 경로에 남아있고, 쉘이 그쪽을 먼저 찾았음.

```bash
which dockviz
# /usr/bin/dockviz  ← 구버전이 여기에 있었음
```

**해결**
```bash
rm $(which dockviz)       # 구버전 삭제
mv dockviz /usr/local/bin/dockviz  # 새 버전을 원하는 위치에 설치
```

이후엔 일관된 위치(`/usr/local/bin`)에만 설치되도록 curl 명령어를 표준화.

---

## 5. GitHub Actions가 수동으로 작성한 릴리스 노트를 덮어씀

**증상**
`gh release edit`으로 릴리스 노트를 작성해 뒀는데, 다음 태그 푸시 후 CI가 실행되면 릴리스 노트가 자동 생성된 내용으로 교체됨.

**원인**
`.github/workflows/release.yml`의 `softprops/action-gh-release` 액션에
`generate_release_notes: true` 가 설정되어 있었음.
이 옵션은 릴리스가 이미 존재해도 노트를 자동 생성한 내용으로 **덮어쓴다**.

**해결**
```yaml
# 수정 전
generate_release_notes: true

# 수정 후
generate_release_notes: false
```

릴리스 노트는 CI 완료 후 `gh release edit v0.x.x --notes "..."` 로 수동 작성.

**관련 파일**: `.github/workflows/release.yml`

---

## 6. docker run 명령어 입력 후 > 프롬프트로 멈춤

**증상**
```bash
docker run -d --name api-server \
  --network app-network \
  -p 3000:3000 node:alpine sh -c "..."
>
```
명령어가 끝나지 않고 `>` 프롬프트가 계속 나타남.

**원인**
백슬래시(`\`)로 줄 이어쓰기를 했는데 마지막 줄에서 쉘이 명령어가 아직 끝나지 않은 것으로 판단.
따옴표가 열린 채로 줄을 나눴거나, 백슬래시 뒤에 공백이 있었던 경우.

**해결**
`Ctrl+C`로 현재 입력을 취소하고, 명령어를 한 줄로 이어 붙여서 다시 실행.

```bash
docker run -d --name api-server --network app-network -p 3000:3000 node:alpine sh -c "while true; do sleep 1; done"
```

---

## 7. Disk Usage 패널의 Local Volumes 프룬이 표시된 것보다 적게 회수됨

**증상**
Disk Usage 패널이 Local Volumes에서 예를 들어 96MB를 reclaimable로 표시. `d`로 프룬해도 0MB 회수로 나오고, 다음 fetch에서도 96MB가 그대로 남아있음.

**원인**
`DiskUsage()`는 `RefCount == 0`인 모든 볼륨(이름 있든 없든)을 reclaimable로 계산했다. 반면 `PruneVolumes()`는 필터 없이 `VolumesPrune`을 호출했는데, Docker API 1.42 이상에서는 필터 없는 볼륨 프룬을 **익명(anonymous) 볼륨에만** 조용히 제한한다 (moby의 `volume/service/convert.go` — `withPrune`이 `all=true`를 명시하지 않으면 `label=AnonymousLabel` 필터를 자동으로 추가함). 컨테이너에 지금 당장 붙어있지 않은 이름 있는 볼륨(가장 흔한 케이스)은 `d`를 아무리 눌러도 전혀 삭제되지 않았다.

**해결**
`all=true`를 명시적으로 전달해서 패널에 표시된 것과 실제 프룬 대상을 일치시킴.

```go
// 수정 전
report, err := c.cli.VolumesPrune(c.ctx, filters.NewArgs())

// 수정 후
report, err := c.cli.VolumesPrune(c.ctx, filters.NewArgs(filters.Arg("all", "true")))
```

**관련 파일**: `internal/docker/diskusage.go`

---

## 8. Docker Desktop / WSL2 환경에서 Container Logs 용량이 항상 0으로 나옴

**증상**
Windows나 macOS의 Docker Desktop 환경에서 Disk Usage 패널의 Container Logs 행이 항상 0을 보여줌. `docker logs <container>`로는 분명히 수 메가바이트 로그가 나오는데도 그렇다.

**원인**
Container Logs 카테고리는 `ContainerInspect(id).LogPath`를 읽어서 그 경로를 로컬 파일시스템에서 직접 stat한다 — 컨테이너 로그의 디스크 용량을 알려주는 Docker API 자체가 없어서 이 방법 말고는 측정할 수단이 없다. Docker Desktop은 `dockerd`가 별도 VM 안에서 돈다(WSL2 백엔드면 숨겨진 `docker-desktop` distro, Hyper-V/macOS면 별도 VM). 그래서 `LogPath`가 dockviz 자신의 파일시스템에는 존재하지 않는 경로를 가리키게 되고, `os.Stat`은 그냥 "파일 없음"을 반환한다 — 이건 "이 컨테이너가 아직 로그가 없는 것"과 구분이 안 된다.

이건 일반적인 의미의 "버그"가 아니다 — 애초에 읽을 로컬 파일이 없는 상황이다. dockviz와 `dockerd`가 파일시스템을 공유할 때만 동작한다: `dockerd`가 호스트에 직접 설치된 순수 리눅스(rootful/rootless 무관), 또는 dockviz가 (Docker Desktop 연동이 아니라) `dockerd`가 직접 돌고 있는 것과 같은 WSL2 distro 안에서 실행되는 경우.

**해결**
별도 수정 없음. 지원 환경 제약으로 문서화 ([reference.ko.md](reference.ko.md#container-logs-제약)의 환경 표와 [validation.ko.md](validation.ko.md#7-수동-체크리스트-container-logs)의 체크리스트 참고). 원격까지 지원하려면 로컬 경로를 stat하는 대신 `ContainerLogs`로 스트리밍하며 바이트를 세는 방식으로 바꿔야 함 — 후속 작업으로 `TODOS.ko.md` T-004에 남겨둠.

**관련 파일**: `internal/docker/diskusage.go`
