# Engramux — 작업 지침

Claude Code와 Codex의 세션 hook 이벤트를 자동 캡처해, 사용자당 하나의 Go 서비스가 모든
동시 세션을 다중화하여 SQLite에 저장하고 FTS5·MCP로 되돌려주는 Windows-first 메모리 엔진.
`thedotmack/claude-mem`의 참조 재구현이며 fork가 아니다. 참조 클론: `D:\AI_DEV\_refs\claude-mem`.

**이 파일이 지침의 유일한 원본이다.** Codex는 `AGENTS.md`를, Claude Code는 `CLAUDE.md`
(`@AGENTS.md` import)를 읽는다. Claude 전용 지침이 생기면 그때만 `CLAUDE.md`에 덧붙인다.
전역 계약(S1–S8 / W1–W11)은 이미 로드되므로 **여기엔 전역이 알 수 없는 프로젝트 고유 사실만** 적는다.

---

## 1. 절대 제약

| 항목 | 값 |
|---|---|
| module | `github.com/wotjr1649/engramux` |
| `go.mod` go directive | `1.25.7` (goose v3.27.3이 요구) · 로컬 툴체인 go1.27.0 |
| 빌드 타겟 | `GOOS=windows GOARCH=amd64` |
| **CGO** | `CGO_ENABLED=0` — 배포 아티팩트에 예외 없음. CGO를 요구하는 의존성은 채택 금지 |
| 바이너리 | `cmd/engramux`(CUI) · `cmd/engramux-service`(GUI, `-H=windowsgui`). 하나로는 불가능 — PE Subsystem은 바이너리당 하나다 |
| 금지 의존성 | Node·Bun·Python 런타임, 외부 vector DB, process supervisor 프레임워크, `capnspacehook/taskmaster`, CGO SQLite |
| 레이아웃 | `cmd/` + `internal/`. **`pkg/`를 만들지 않는다.** `golang-standards/project-layout`은 표준이 아니다(Russ Cox가 직접 부인, issue #117) |
| 마이그레이션 SQL | `internal/store/migrations/*.sql` — `embed.FS`는 상위 디렉터리를 못 본다 |

---

## 2. 이 저장소의 함정 — 전부 실측이나 1차 출처로 확인됨

여기 적힌 것은 **이미 한 번씩 물린 것들**이다. 다시 물리지 마라.

### 도구

| 함정 | 대응 |
|---|---|
| **bash heredoc이 `\\`를 `\`로 접는다.** `\\.\pipe\` 리터럴과 Go rune 리터럴(`'\n'`)이 깨져 `unexpected EOF`가 난다 | Go 소스와 백슬래시가 든 파일은 **반드시 Write 도구로** 쓴다. heredoc으로 쓰지 않는다 |
| `go test` 병렬 패키지가 단일 SQLite 파일과 파이프 이름에서 충돌한다 | **항상 `go test -p 1`** |
| Windows에서 `t.TempDir()` 정리가 열린 핸들 때문에 실패한다 | 테스트 종료 전 `*sql.DB`와 listener를 `Close()`. `-wal`/`-shm` 해제까지 확인 |
| `~/.claude/settings.json`, `~/.codex/hooks.json`, `~/.codex/config.toml` 등은 Edit 거부 대상이다 | 우회하지 않는다. 사용자에게 `! <command>` 실행을 요청한다 |
| `~/.claude-mem/claude-mem.db`는 개발 중에도 살아 있다 | **쓰기 모드로 열지 않는다.** 읽으려면 temp로 복사 후 연다 |

### SQLite (`modernc.org/sqlite`)

| 함정 | 대응 |
|---|---|
| **`_pragma` 값만 검증에서 제외된다.** `syncronous(3)` 같은 오타도 `err=nil`로 통과하고 SQLite가 조용히 무시한다 | 기동 시 모든 pragma를 **되읽어 기대값과 대조**한다. 불일치면 기동 실패 |
| `synchronous`는 **2가 FULL, 3은 EXTRA**다 | 3을 FULL이라고 적지 마라 |
| raw `BEGIN IMMEDIATE` SQL을 쓰면 `ROLLBACK` 하나만 놓쳐도 writer가 영구히 wedge된다 | `_txlock=immediate` + `db.BeginTx`/`tx.Commit`만 쓴다 |
| `_txlock`은 pragma가 아니라 **되읽기로 검증할 수 없는 유일한 DSN 항목**이다 | 트랜잭션 동작을 테스트로 고정한다 |
| `locking_mode=exclusive`는 **첫 접근 전에** 걸려야 `-shm`이 안 생긴다 | `_pragma=locking_mode(exclusive)`를 `journal_mode(wal)`과 함께 `_pragma`로 준다. `_journal_mode=WAL` 축약 키와 섞지 않는다 |
| exclusive + `SetMaxOpenConns(1)` = **2차 커넥션 불가.** CLI도 DB를 못 연다 | `status`/`doctor`/`search`는 전부 파이프 경유. "두 커넥션 경쟁" 테스트는 설계상 불가능하니 **"2차 커넥션이 거부되는지"를 단언**한다 |
| goose가 `;`로 문을 쪼개 **트리거 본문이 잘린다** | `CREATE TRIGGER`는 `-- +goose StatementBegin` / `-- +goose StatementEnd`로 감싼다 |
| v1.46.1 미만은 Commit 실패 시 커넥션 상태를 리셋하지 않는다 | `modernc.org/sqlite`를 v1.46.1 아래로 내리지 않는다. 현재 v1.57.0 |

### Windows

| 함정 | 대응 |
|---|---|
| **GUI(콘솔 없는) 부모가 기본 플래그로 CUI 자식을 띄우면 새 콘솔 창이 실제로 뜬다**(실측) | 서비스가 만드는 모든 자식 프로세스에 `SysProcAttr{HideWindow:true, CreationFlags:0x08000000}`(`CREATE_NO_WINDOW`) |
| `winio.ListenPipe`는 같은 이름이 이미 있으면 실패한다 | 테스트마다 고유 파이프 이름을 만들고 `t.Cleanup`으로 닫는다 |
| `winio.DialPipeContext`는 파이프가 **아직 없으면 즉시 실패**한다(`ERROR_PIPE_BUSY`만 재시도) | listener–dial 경합을 직접 재시도 루프로 흡수한다. 파이프 테스트 flakiness 1순위다 |
| `winio.DialPipe(path, nil)`은 조용히 2초 기본 타임아웃을 쓴다 | `DialPipeContext(ctx, path)`만 쓴다 |
| Windows `time.Now()` 해상도는 약 550µs다(30만 회 호출에 서로 다른 값 7개) | 시간으로 이벤트 순서를 정하지 않는다. 순서는 부분 순서 모델로 다룬다 |

### Go 툴체인

| 함정 | 대응 |
|---|---|
| **Go 1.27 `go test`가 `stdversion` vet을 기본 실행한다** — `go 1.25.7` 디렉티브 아래서 1.26/1.27 심볼을 쓰면 빌드는 되는데 **테스트가 실패**한다 | `synctest.Test`/`synctest.Wait`(1.25 GA)는 OK. `synctest.Sleep`·`encoding/json/v2`·`slog.NewMultiHandler`는 불가 |
| `testing/synctest`는 syscall과 실제 I/O에 통하지 않는다 | 파이프 I/O 테스트에 쓰지 않는다. 백오프·타임아웃·드레인 같은 시간 로직에만 |
| golangci-lint v2의 `std-error-handling` 프리셋이 **모든 `.Close()` 미검사를 제외**한다 | 그 프리셋을 켜지 않는다. `sqlclosecheck`·`rowserrcheck`·`noctx`·`errorlint`·`gosec`(G115)를 켠다 |
| `time` 채널은 Go 1.27부터 항상 unbuffered다 | 타이머 채널 버퍼링에 의존하는 코드를 쓰지 않는다 |
| **`-race`에 CGO-free 경로는 없다.** `CGO_ENABLED=1` **그리고** C 컴파일러 둘 다 필요하다. `race_windows.syso`는 설치돼 있지만 `runtime/race/race.go`의 빌드 태그가 `windows && amd64`를 포함하고 그 파일이 `import "C"`를 한다 — darwin만 `race_darwin_amd64.go`로 우회한다(golang/go#6508 OPEN, Windows 계획 없음). `-ldflags=-linkmode=internal`도 소용없다: 실패 지점이 링크가 아니라 `runtime/cgo` 컴파일이다 | 배포 빌드는 `CGO_ENABLED=0` 유지. 레이스 검증은 **테스트 전용 경로**로 분리한다(§3) |

---

## 3. 테스트 규율

**존재 단언 금지.** 이전 반복의 테스트는 고의 파괴 **20개 중 15개를 통과시켰다** — 트랜잭션
제어를 통째로 지워도, 4GiB를 실제로 할당해도, 보안 통제를 삭제해도 green이었다. 원인은 단언이
값이 아니라 존재(`!= ""`)를 본 것이다.

불변식을 지키는 테스트마다:

```
1. 테스트를 쓰고 red 확인          (TDD)
2. 구현해서 green                  (TDD)
3. 구현을 고의로 부순다 — 그 불변식만
4. 테스트가 red 가 되는지 확인      ← 안 되면 테스트가 가짜다
5. 되돌리고 green 재확인
```

3~5단계는 커밋에 남기지 않는다.

그 외:

- fixture 단언은 **정확한 값**으로 한다. 왕복 페이로드 바이트, 에러 종류(`errors.Is`), 서버 측 상태 변화.
- **`session_id == "selftest"` 캡처를 걸러라.** 합성 자가진단 캡처가 claude PreToolUse 버킷에 실재하고 타임스탬프가 가장 빨라 그냥 두면 `001.json`이 된다.
- `tool_response`는 **호스트마다 형태가 다르다**(실측): Claude `dict` 310/310이나 `stderr`/`interrupted` 키는 241/310에만 있고, Codex는 `str` 24 + `list` 15다. 객체를 가정하는 파서는 Codex 전부와 Claude 69건에서 조용히 틀린다.
- JSON 에러 문자열을 그대로 비교하지 않는다. Go 1.27이 `encoding/json` 구현을 바꿔 문구가 달라진다.

### 동시성 — 무엇이 무엇을 잡는지 착각하지 마라

이 프로젝트에서 가장 위험한 실패 모드는 **"synctest로 동시성을 검증했다"는 착각**이다.

| 도구 | CGO | 잡는 것 | **절대 못 잡는 것** |
|---|---|---|---|
| `go test -race` | **필요** | 실행된 경로의 진짜 데이터 레이스 | 실행 안 된 경로. 각 접근이 락으로 보호된 로직 경합 |
| `testing/synctest` | 불필요 | 타임아웃·백오프·티커 로직, bubble 전면 차단 시 deadlock 패닉 | **데이터 레이스를 전혀 보고하지 않는다** — bubble 안에서 두 goroutine이 `x++`를 동시에 해도 조용히 통과한다(실측). 실제 I/O·syscall은 durably blocking이 아니라 **파이프 I/O 테스트에 못 쓴다** |
| `goroutineleak` 프로파일 (1.27) | 불필요 | GC 사이클로 영구 차단된 goroutine, 스택까지 지목 | 데이터 레이스. 깨울 가능성이 남은 차단은 리크로 안 본다 |
| `go vet` / staticcheck | 불필요 | `copylock`, `lostcancel`, `atomic` 오용, SA2000~2003 | **모든 실제 데이터 레이스.** 구문 매칭이지 happens-before 분석이 아니다 |
| `-count=N -shuffle=on` | 불필요 | flake가 드러날 **확률**을 높인다 | 아무것도 보장하지 않는다. **통과는 증거가 아니다** |

`goroutineleak`은 지금 CGO 없이 되므로 테스트 하니스에 넣는다. `Profile.Count()`는 탐지
GC 사이클 **전에** 0을 반환하니 반드시 `WriteTo`로 탐지를 트리거하고 출력을 파싱한다:

```go
var buf bytes.Buffer
_ = pprof.Lookup("goroutineleak").WriteTo(&buf, 1)
first, _, _ := strings.Cut(buf.String(), "\n")
if !strings.HasSuffix(first, "total 0") { t.Errorf("goroutine leak:\n%s", buf.String()) }
```

`go.uber.org/goleak`은 넣지 않는다 — `goroutineleak`이 런타임 GC 기반이라 스택 문자열
휴리스틱보다 정확하고 의존성이 0이다.

---

## 4. 보안

- **`.capture/`를 절대 커밋하지 않는다.** 프롬프트 원문·파일 내용·도구 출력·사용자 경로가 그대로 들어 있다. fixture로 승격하려면 redaction과 **사용자명 경로 치환**을 통과한 사본만 옮긴다.
- **`origin`은 공개 저장소다**(`github.com/wotjr1649/engramux`). push는 전 세계에 게시하는 것이다. 커밋 전에 사용자명·머신명·이메일·실제 SID 유입을 확인한다.
- redaction은 **events insert 앞 단계**에서 한다. purge는 in-place 치환이다.
- bearer 토큰 정규식에 `\S+`를 쓰지 않는다 — 닫는 따옴표와 중괄호까지 먹어 **깨진 JSON을 DB에 쓴다**. `[^\s"',}\]]+`를 쓴다.
- `slog` redaction은 `slog.NewRecord` + `AddAttrs`로 레코드를 재구성한다. `Record.Attrs` 콜백의 `a.Value = …`는 **no-op**이라 시크릿이 로그에 그대로 남는다(실측).
- 같은 Windows user SID는 **신뢰 경계 안**으로 공식 선언했다. 그 전제 위에서 설계한다.

---

## 5. 문서

- **권위 문서는 `docs/superpowers/specs/2026-08-27-engramux-1.0-design.md` 하나다.** `docs/chatgpt/`의 원문서는 spec이 명시적으로 인용한 절에 한해서만 유효하다.
- **코드가 먼저다.** 동작하는 코드에서 spec을 정정한다. spec에 실행해보지 않은 코드를 적지 않는다 — rev.1과 rev.2가 그것 때문에 폐기됐다.
- **사실 주장에는 재현 명령을 함께 적는다.** 없으면 "미검증"으로 표시한다. 자체 검토(Self-Review)를 근거로 쓰지 않는다 — rev.2에서 거짓을 인증했다.
- spec rev.3은 아직 **정정 중**이다. §16이 본문을 고치지 않고 덮어쓴 구조라 본문과 모순난 곳이 남아 있다. 본문과 §16이 어긋나면 **§16이 이긴다.**
