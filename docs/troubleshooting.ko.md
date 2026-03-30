# dockviz-cli 트러블슈팅 기록

개발 과정에서 마주친 버그와 해결 과정을 기록합니다.

---

## 1. 네트워크 토폴로지에 컨테이너가 하나도 안 뜸

**증상**
Networks 탭의 토폴로지 그래프에 컨테이너가 표시되지 않고 `(no containers)` 만 출력됨.
실제로 컨테이너들을 동일 네트워크에 연결했음에도 불구하고.

**원인**
Docker API의 `GET /networks` (= `NetworkList`) 응답에는 `Containers` 필드가 **항상 비어있다**.
Docker 공식 SDK 문서에는 명시되어 있지 않지만, 실제 API 동작이 그렇다.

**해결**
`NetworkList`로 네트워크 목록만 가져온 다음, 각 네트워크마다 `NetworkInspect`를 별도로 호출해야 컨테이너 정보가 채워진다.

```go
// 수정 전 — Containers가 항상 비어있음
networks, _ := cli.NetworkList(ctx, network.ListOptions{})

// 수정 후 — 네트워크마다 Inspect 호출
detail, _ := cli.NetworkInspect(ctx, n.ID, network.InspectOptions{Verbose: false})
// detail.Containers 에 컨테이너 목록이 있음
```

**관련 파일**: `internal/docker/networks.go`

---

## 2. 같은 네트워크 안 컨테이너 순서가 매번 바뀜

**증상**
`app-network : ● api-server ─── ● nginx-proxy` 였다가 새로고침하면
`app-network : ● nginx-proxy ─── ● api-server` 로 순서가 뒤바뀜.

**원인**
`NetworkInspect`가 반환하는 `Containers` 필드의 타입이 `map[string]NetworkContainer`다.
Go의 map은 순회 순서가 **비결정적(non-deterministic)** 이다. 매번 다른 순서로 나온다.

**해결**
map에서 슬라이스로 변환한 뒤 이름 기준으로 `sort.Slice` 정렬.

```go
sort.Slice(endpoints, func(i, j int) bool {
    return endpoints[i].Name < endpoints[j].Name
})
```

**관련 파일**: `internal/docker/networks.go`

---

## 3. 시스템 네트워크(bridge / host / none) 위치가 매번 바뀜

**증상**
Networks 탭에서 `bridge`, `host`, `none` 이 목록 어디에 위치할지 매번 달랐다.
사용자 정의 네트워크들 사이에 섞여 들어옴.

**원인**
`NetworkList` 응답 자체의 순서가 보장되지 않는다. Docker가 내부적으로 map 기반으로 관리하기 때문.

**해결**
`sort.SliceStable`로 정렬 기준을 직접 지정: 사용자 정의 네트워크는 알파벳 순으로 앞에, 시스템 네트워크(bridge → host → none)는 항상 뒤에 고정.

```go
sysOrder := map[string]int{"bridge": 0, "host": 1, "none": 2}
sort.SliceStable(result, func(i, j int) bool {
    ri, iSys := sysOrder[result[i].Name]
    rj, jSys := sysOrder[result[j].Name]
    if iSys != jSys {
        return !iSys // 시스템 네트워크는 뒤로
    }
    if iSys {
        return ri < rj // 시스템 네트워크끼리는 고정 순서
    }
    return result[i].Name < result[j].Name // 사용자 네트워크는 알파벳 순
})
```

**관련 파일**: `internal/docker/networks.go`

---

## 4. 이미지 목록 순서가 불안정하고 여러 태그가 한 줄에 뭉쳐 나옴

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

## 5. 이미지 삭제 시 태그 하나만 지우려도 이미지 전체가 삭제됨

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

## 6. Windows에서 파일 수정 도구가 매칭 실패

**증상**
Windows 환경에서 코드 편집 시도 시 "old_string not found" 오류 발생.
파일 내용을 눈으로 보면 분명히 있는 문자열인데도 매칭이 안 됨.

**원인**
Windows에서 `git clone` 시 `core.autocrlf=true` 설정이 기본값이면 줄바꿈 문자를 자동으로 CRLF(`\r\n`)로 변환한다.
파일 편집 도구는 바이트 단위로 정확히 매칭하는데, 검색 문자열은 LF(`\n`)만 포함하고 실제 파일은 CRLF(`\r\n`)이라서 불일치 발생.

**해결**
해당 파일을 Go 스크립트로 직접 읽어 `\r\n`을 `\n`으로 치환 후 저장. 이후에는 정상 동작.

```go
data, _ := os.ReadFile(path)
data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
os.WriteFile(path, data, 0644)
```

근본적인 해결은 `.gitattributes`에 `* text=auto eol=lf` 를 추가하거나 `git config core.autocrlf false` 로 설정하는 것.

---

## 7. curl 설치 시 9바이트짜리 "Not found" 응답만 받음

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

## 8. 새 버전 설치 후에도 dockviz --version이 구버전을 출력

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

## 9. GitHub Actions가 수동으로 작성한 릴리스 노트를 덮어씀

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

## 10. docker run 명령어 입력 후 > 프롬프트로 멈춤

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

## 11. 통계 히스토리 차트(`g` 키) 확인용 테스트 컨테이너

**배경**

`g` 키를 누르면 선택한 컨테이너의 CPU/MEM 히스토리를 전체 화면 차트로 볼 수 있다.
임계선(80% 빨강, 50% 노랑), 막대 높낮이 변화, 색 전환이 제대로 동작하는지 확인하려면
**변동성 있는** 부하를 만드는 컨테이너가 필요하다. 항상 100%이거나 항상 낮은 값이면 변화가 보이지 않는다.

**흔히 쓰는 스트레스 컨테이너의 문제**

```bash
# 나쁜 예 — 계속 100% → 막대 높이 변화 없음
docker run -d --name cpu-stress progrium/stress --cpu 2

# 나쁜 예 — 너무 낮고 일정 → 임계선 도달 안 함
docker run -d --name idle-app alpine sleep infinity
```

**변동성 있는 테스트 컨테이너**

```bash
# CPU 펄스 — 4초 부하 / 6초 휴식 반복, 50% 임계선을 오르내림
docker run -d --name cpu-pulse alpine sh -c "
while true; do
  yes > /dev/null &
  PID=\$!
  sleep 4
  kill \$PID
  sleep 6
done"

# CPU 웨이브 — 강약을 번갈아 가며 부하 생성
docker run -d --name cpu-wave alpine sh -c "
while true; do
  dd if=/dev/urandom of=/dev/null bs=1M count=200 2>/dev/null
  sleep 8
  dd if=/dev/urandom of=/dev/null bs=1M count=50 2>/dev/null
  sleep 5
  sleep 3
done"

# CPU 사인파형 — 단계적으로 올라갔다 내려오는 패턴, 막대가 자라고 줄어드는 것을 보기 좋음
docker run -d --name cpu-sin alpine sh -c "
i=0
while true; do
  i=\$((i+1))
  mod=\$((i % 10))
  if [ \$mod -lt 5 ]; then
    yes > /dev/null &
    PID=\$!
    sleep \$((mod + 1))
    kill \$PID 2>/dev/null
  else
    sleep 2
  fi
done"
```

두세 개를 동시에 실행한 뒤 차트 뷰에서 `↑`/`↓` 로 컨테이너를 바꿔가며 부하 패턴을 비교할 수 있다.

**정리**

```bash
docker rm -f cpu-pulse cpu-wave cpu-sin
```
