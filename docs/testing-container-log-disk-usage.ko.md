# 테스트: Container Logs 디스크 사용량 카테고리

**상태:** 2026-07-22, `internal/docker/diskusage.go`와 vendored `docker/docker` v28.0.4 소스 정적 분석을 바탕으로 작성. 실제 Docker 데몬에 대해서는 아직 실행하지 못함 — 이 세션은 Windows 호스트였고 로컬 Docker Desktop엔 아무 컨테이너도 없어 네이티브 리눅스 `dockerd`가 없었음. `diskusage.go`를 건드리는 다음 릴리스 전에 실제 리눅스 머신에서 이 체크리스트를 돌려야 함.

## 왜 `go test`만으로는 부족한가

`go test ./...`는 `logFileSizes`(순수 함수, 실제 임시 파일로 테이블 테스트됨)는 커버하지만, 실제로 Docker와 통신하는 `Client`의 메서드 — `DiskUsage()`, `PruneLogs()` — 는 이 패키지의 다른 실제-데몬 메서드들과 마찬가지로 커버리지 0%다 (`TODOS.md` T-005 참고). 이건 유닛 테스트를 더 쓴다고 메워지는 갭이 아니다 — 이 기능이 실제로 동작하는지를 결정하는 두 가지는 (1) dockviz 프로세스와 `dockerd`가 파일시스템을 공유하는지, (2) `/var/lib/docker/containers`의 파일 권한인데, 둘 다 코드의 속성이 아니라 **환경**의 속성이라 Go 유닛 테스트로 표현할 수가 없다. 검증하려면 각 환경 형태별로 실제 바이너리를 실제 데몬에 대고 돌려서 재봐야 한다.

## 지원 환경 매트릭스

| 환경 | `dockerd`와 파일시스템 공유? | 예상 결과 |
|---|---|---|
| 네이티브 리눅스, rootful `dockerd`, dockviz를 `docker` 그룹 비-root 유저로 실행 | O | `Unavailable = "permission denied on N container(s) — try sudo"` |
| 네이티브 리눅스, rootful `dockerd`, dockviz를 `sudo`로 실행 | O | 정상 수치, 프룬 동작 |
| 네이티브 리눅스, rootless `dockerd` | O (로그가 유저 소유) | `sudo` 없이도 정상 수치, 프룬 동작 |
| WSL2 distro 안에 `dockerd`를 직접 설치 (Docker Desktop 연동 아님) | O | 위 네이티브 리눅스 행들과 동일 |
| Docker Desktop, Windows (WSL2 백엔드) | X — `dockerd`가 별도의 숨겨진 `docker-desktop` distro 안에서 돎 | Logs 행이 0, 경고 없음 (`docs/troubleshooting.ko.md#13` 참고) |
| Docker Desktop, macOS (VM 백엔드) | X | 위와 동일 |
| 원격 `DOCKER_HOST=tcp://...` | X | 위와 동일 |

이 매트릭스 자체가 제일 먼저 확인해야 할 것이다 — 아래 정량 측정을 하기 전에, 각 행의 "예상 결과"가 실제로 그렇게 나오는지부터 확인. 이 프로젝트가 README에서("만들게 된 이유") 명시한 주 타겟이 네이티브 리눅스 서버 운영자이므로, 위 세 행이 가장 중요하다.

## 정량 측정 절차

네이티브 리눅스 Docker 호스트(rootful, 기본 설치 — 실제 dockviz 서버 사용자 대부분이 마주칠 행)에서 진행.

**1. 결정론적 로그 크기를 만드는 컨테이너 실행**

```bash
docker run -d --name logtest alpine sh -c '
  i=0
  while [ $i -lt 5000 ]; do
    echo "line $i $(head -c 100 </dev/zero | tr "\0" x)"
    i=$((i+1))
  done
  sleep 3600
'
```

**2. 파일시스템에서 직접 실측치 확보**

```bash
LOGPATH=$(docker inspect --format='{{.LogPath}}' logtest)
stat -c%s "$LOGPATH"          # 활성 로그 파일의 실측 바이트
```

**3. 패널과 비교**

`dockviz`(또는 테스트 대상 행에 맞게 `sudo dockviz`) 실행 후 Disk Usage 탭으로 이동, Container Logs 행의 SIZE 컬럼 확인.

*통과 조건:* `실측_바이트 / 1024 / 1024`가 화면 표시값과 `FormatSize` 반올림 단위 이내로 일치. 사실상 정확히 일치해야 함 — `SizeMB`/`ReclaimMB`는 추정치가 아니라 `fi.Size()`에서 바로 계산됨.

**4. 프룬 후 재확인**

Container Logs 행을 선택하고 `d`로 프룬, 확인. `stat -c%s "$LOGPATH"` 재실행 — `0`이어야 함.

**5. 컨테이너가 안 죽었는지 확인**

```bash
docker logs -f logtest &      # 프룬 전후로 계속 tail
# 프룬 후 새 라인 기록
docker exec logtest sh -c 'echo post-truncate-line'
```

*통과 조건:* 컨테이너가 여전히 `running`(`docker ps`)이고, 기존에 띄워둔 `docker logs -f` 세션에 `post-truncate-line`이 그대로 찍힘 — dockerd가 들고 있던 fd가 truncate를 견뎌냈다는 뜻 (볼륨에 적용된 동일한 fd 원리는 `docs/troubleshooting.ko.md#12`, truncate-in-place를 쓰는 이유는 `diskusage.go`의 `PruneLogs` 주석 참고).

**6. 로그 회전 파일 처리**

```bash
docker run -d --name logtest-rotate --log-opt max-size=1k --log-opt max-file=3 \
  alpine sh -c 'i=0; while [ $i -lt 5000 ]; do echo "line $i padding padding padding"; i=$((i+1)); done; sleep 3600'
ls "$(docker inspect --format='{{.LogPath}}' logtest-rotate)"*
```

*통과 조건:* `.log.1`/`.log.2` 파일이 존재하고, dockviz가 계산한 이 컨테이너의 SizeMB에 이게 포함되어 있으며(`du -cb "$LOGPATH"*`와 교차 검증), 프룬 후엔 회전 파일들이 완전히 사라지고(`ls`에 더 이상 안 잡힘) 활성 `.log` 파일만 0바이트로 남아있음 — 회전 파일은 `os.Remove`, 활성 파일은 `os.Truncate`만 적용되기 때문.

**7. 권한 거부 경로**

1~3번을 `/var/lib/docker`가 기본값대로 root 소유인 리눅스 머신에서, `docker` 그룹(비-root, sudo 없음) 유저로 반복.

*통과 조건:* Total/SizeMB는 `0`이지만, 행 아래에 `⚠ permission denied on N container(s) — try sudo`가 표시됨 — 텅 빈 것과 구분 안 되는 조용한 `0`이 아니어야 함. 같은 과정을 `sudo dockviz`로 재실행해서 이번엔 수치가 정상적으로 채워지는지 확인.

**8. 미지원 환경 스모크 테스트**

Docker Desktop이나 원격 `DOCKER_HOST`에서, Logs 행이 그냥 경고 없이 `0`으로 나오고 패널이 죽지 않는지 확인 — 이 경로에서는 `permDenied`가 의도적으로 `false`다 (`logFileSizes` 주석 참고), 사용자에게 알려줄 실질적인 내용이 없기 때문. 이 단계는 수치보다는 **패닉이 안 나는지**를 확인하는 게 핵심.

## 통과/실패 기준

| 체크 항목 | 통과 조건 |
|---|---|
| SizeMB 정확도 | `\|dockviz SizeMB − 실측 MB\|`가 반올림 단위 이내 |
| ReclaimMB 불변식 | 프룬 전 `ReclaimMB == SizeMB` (발견된 모든 바이트는 설계상 100% 회수 가능) |
| 프룬 정확성 | 프룬 후 실측 파일 크기가 `0` |
| 컨테이너 생존 | `running` 상태 유지, truncate 이후 기록된 라인도 `docker logs -f`에 계속 찍힘 |
| 회전 파일 | SizeMB에 포함되고, 프룬 후 완전히 삭제됨(빈 파일로 남지 않음) |
| 권한 거부 UX | daemon 소유 로그 경로를 못 읽을 때 조용한 `0`이 아니라 `Unavailable` 메시지 표시 |
| 미지원 환경 안전성 | 패닉·에러 다이얼로그 없이 깔끔한 `0`으로 보임 |

## 남은 한계

이 절차 전체가 수동이다. 이 저장소의 GitHub Actions 러너에는 지금 Docker 데몬이 없어서(`TODOS.md` T-005) 이대로는 CI에서 하나도 못 돌린다. 이게 해결되기 전까지는, 이 문서를 `go test`의 대체가 아니라 `internal/docker/diskusage.go`를 건드리는 릴리스 전 체크리스트로 취급할 것.
