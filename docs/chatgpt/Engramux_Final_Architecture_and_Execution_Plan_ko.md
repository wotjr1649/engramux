# Engramux 최종 아키텍처 및 실행 계획

> **문서 상태:** Design Freeze Candidate  
> **기준일:** 2026-08-25  
> **대상 호스트:** Claude Code, Codex  
> **주 구현 언어:** Go  
> **우선 플랫폼:** Windows 11 x64/arm64  
> **기준 원본:** `thedotmack/claude-mem` main, commit `e2d1df569a8f04075d40e92461128ece7cf04c82`  
> **프로젝트 성격:** Fork가 아닌 reference-driven greenfield reimplementation

---

## 1. 최종 결론

Engramux는 `claude-mem`의 TypeScript 소스를 파일 단위로 Go 문법으로 번역하는 프로젝트가 아니다.

Engramux의 정확한 정의는 다음과 같다.

> **Claude Code와 Codex에서 발생하는 개발 세션 이벤트를 수집하고, 사용자별 단 하나의 Go 서비스 워커가 모든 동시 세션을 다중화하여 저장·압축·검색·재주입하는 Windows-first 영속 메모리 엔진이다.**

최종 설계는 다음 원칙으로 고정한다.

| 항목 | 최종 결정 |
|---|---|
| 개발 방식 | 원본을 별도 read-only reference 디렉터리에 clone하고, 새 빈 workspace에서 독립 구현 |
| Git 관계 | `claude-mem`과 Git history·remote·branch 관계 없음 |
| 지원 호스트 | Claude Code, Codex만 지원 |
| 상주 프로세스 | 사용자당 `engramux.exe service` 정확히 1개 |
| 세션 처리 | 세션별 프로세스가 아니라 서비스 내부 논리 세션과 goroutine으로 관리 |
| Hook 연결 | 짧게 실행되는 relay가 Named Pipe로 단일 서비스에 이벤트 전달 |
| Hook relay | 서비스 워커가 아니며 상태·DB·LLM·HTTP listener를 소유하지 않음 |
| Hook IPC | Windows Named Pipe |
| MCP | 서비스가 직접 제공하는 loopback Streamable HTTP |
| 저장소 | SQLite WAL + FTS5 |
| 원본 이벤트 | append-only event journal이 source of truth |
| Vector | 선택 기능. FTS5가 항상 동작하는 기본 검색 경로 |
| AI 처리 | Hook critical path 밖의 bounded background workers |
| Provider | Host와 분리하여 명시적으로 선택 |
| 실패 정책 | 메모리 장애는 Claude Code와 Codex를 절대 차단하지 않는 fail-open |
| 외부 런타임 | Node, Bun, Python, uv, Chroma를 필수 의존성에서 제거 |
| 전역 지침 | `CLAUDE.md`, `AGENTS.md`, 전역 프롬프트 변경에 의존하지 않음 |
| 다른 호스트 | OpenCode, Cursor, Gemini, OpenClaw 등은 완전한 비범위 |

Engramux 1.0의 가장 중요한 정량적 불변식은 다음 다섯 가지다.

```text
1 persistent service process
0 per-session workers
0 mandatory child runtimes
0 fixed TCP ports for hook ingestion
0 blocking failures caused by memory availability
```

---

## 2. 범위

### 2.1 반드시 구현할 범위

Engramux 1.0은 다음 기능을 제공한다.

1. Claude Code lifecycle 이벤트 수집
2. Codex lifecycle 이벤트 수집
3. 여러 Claude/Codex 세션의 동시 처리
4. 프로젝트·repository·worktree 단위 메모리 분리
5. user prompt, tool use, session summary 저장
6. 세션 시작 시 관련 과거 context의 제한적 주입
7. MCP 기반 검색·timeline·상세 조회
8. SQLite FTS5 기반 검색
9. 선택적 embedding/vector reranking
10. `claude-mem.db`의 one-way import
11. Windows 로그인 시 자동 시작하는 사용자 단위 서비스 워커
12. 장애 복구, 로그, doctor, export, reindex
13. Claude Code와 Codex별 독립 Hook 입출력 계약
14. 데이터 저장 전 secret redaction과 path exclusion
15. 설치·업데이트·제거 시 호스트 설정의 원자적 관리

### 2.2 의도적으로 구현하지 않을 범위

다음은 1.0에서 제외한다.

- OpenCode, Cursor, Windsurf, Gemini, Antigravity, OpenClaw 지원
- Claude/Codex 이외 호스트의 추상화 선행 설계
- `CLAUDE.md`, `AGENTS.md`, 전역 system prompt 수정
- 별도 memory skill 또는 전역 instruction 주입
- 여러 PC 간 cloud sync
- multi-user server
- 원격 공개 MCP endpoint
- ChromaDB, chroma-mcp, Python sidecar
- 세션마다 별도 worker 또는 embedding process 실행
- Obsidian 연동
- Web Viewer를 1.0 release blocker로 지정
- WSL2를 Windows 기본 실행 경로로 사용
- 원본 `claude-mem` 저장소로 PR을 보내기 위한 호환 branch
- 원본 데이터베이스를 직접 수정하는 in-place migration

범위를 좁히는 이유는 단순하다. Engramux의 1차 품질 목표는 기능 수가 아니라 다음 세 가지다.

```text
Windows 안정성
Claude/Codex 계약 정확성
단일 서비스의 데이터 무손실 처리
```

---

## 3. 개발 모델: Fork가 아닌 독립 재구현

### 3.1 Workspace 구조

권장 workspace는 다음과 같다.

```text
D:\AI_DEV\
├─ references\
│  └─ claude-mem\
│     ├─ .git\
│     └─ ...
│
└─ projects\
   └─ engramux\
      ├─ .git\
      ├─ go.mod
      ├─ cmd\
      ├─ internal\
      ├─ plugins\
      ├─ tests\
      └─ docs\
```

역할은 명확히 분리한다.

```text
references\claude-mem
  - 읽기 및 동작 분석 전용
  - upstream commit 고정
  - Engramux 빌드 입력이 아님
  - Engramux module import 대상이 아님
  - 수정·배포 대상이 아님

projects\engramux
  - 완전히 새로운 Git repository
  - 독립 schema와 package
  - 독립 release와 issue tracker
  - Go 코드의 유일한 source of truth
```

### 3.2 기준 commit 고정

분석 기준은 floating `main`이 아니라 commit으로 고정한다.

```json
{
  "repository": "https://github.com/thedotmack/claude-mem",
  "commit": "e2d1df569a8f04075d40e92461128ece7cf04c82",
  "captured_at": "2026-08-25",
  "purpose": "behavioral reference only"
}
```

파일 위치:

```text
engramux\references\claude-mem.lock.json
```

원본이 업데이트되더라도 Engramux 구현 도중 기준이 조용히 바뀌지 않게 한다. 새 upstream을 분석하려면 별도 review와 lock 갱신 commit이 필요하다.

### 3.3 구현 방식

금지하는 접근:

```text
worker-service.ts → worker_service.go
SessionStore.ts   → session_store.go
ChromaSync.ts     → chroma_sync.go
```

권장 접근:

```text
원본 behavior 발견
  ↓
입력·출력 계약 추출
  ↓
golden fixture 작성
  ↓
문제와 불변식 정리
  ↓
Engramux 고유 설계
  ↓
Go 구현
  ↓
differential replay
```

즉, 원본의 **행동과 데이터 의미**를 참조하되 원본의 런타임·프로세스·dependency 구조는 계승하지 않는다.

### 3.4 Provenance와 라이선스

`claude-mem` 기준 저장소는 Apache License 2.0이다. Engramux에서 원본 코드를 직접 복사하거나 기계적으로 번역한 부분이 생기면 해당 파일과 배포물에 필요한 저작권·라이선스 고지를 유지해야 한다.

권장 문서:

```text
docs\upstream-reference.md
docs\provenance.md
THIRD_PARTY_NOTICES.md
```

`provenance.md`에는 다음을 기록한다.

- 분석한 upstream commit
- 참고한 파일과 behavior
- 직접 복사한 코드 유무
- 변환·번역한 코드 유무
- 독립 구현한 영역
- schema migration 출처
- 적용 라이선스와 NOTICE 처리

이 문서는 법률 자문을 대체하지 않지만, 향후 코드 출처와 라이선스 의무를 추적할 수 있게 한다.

---

## 4. 원본에서 유지·개선·폐기할 것

현재 기준 `claude-mem`은 lifecycle hook, 장기 실행 HTTP worker, SQLite/FTS5, 선택적 Chroma, MCP/search, Viewer로 구성된다. 유용한 제품 개념은 많지만 Windows 프로세스 수명과 host contract가 복잡하게 결합되어 있다.

### 4.1 유지할 개념

| 원본 개념 | Engramux 처리 |
|---|---|
| SessionStart context injection | 유지하되 token/byte budget 엄격히 제한 |
| UserPromptSubmit 저장 | 유지 |
| PostToolUse observation | 유지 |
| Stop/session summary | 유지하되 background job으로 처리 |
| SQLite 영속 저장 | 유지 |
| WAL | 유지 |
| FTS5 | 유지하고 기본 검색으로 승격 |
| progressive disclosure | 유지 |
| 프로젝트 필터 | 개선된 repository identity로 유지 |
| MCP 검색 | 유지하되 도구 수 축소 |
| session/observation/summary 구조 | 의미는 유지, schema는 재설계 |
| 비동기 압축 | 유지하되 durable queue로 강화 |
| Viewer 가능성 | core 안정화 후 선택 기능 |

### 4.2 개선할 개념

| 영역 | 기존 계열 문제 | Engramux 개선 |
|---|---|---|
| Worker lifecycle | PID·port·health·lock 신호가 분산될 수 있음 | Named Mutex + Named Pipe handshake를 단일 권위로 사용 |
| Windows listener | child handle inheritance로 ghost port 가능 | Hook ingress에서 TCP 제거, 기본 child process 0 |
| Hook wrapper | shell 탐색·로그인 shell·console flash | Go relay 직접 실행, shell 미사용 |
| 장애 정책 | worker 장애가 prompt/Read/Stop을 방해할 수 있음 | 모든 memory availability failure는 exit 0 |
| Codex output | Claude 전용 필드가 누출될 수 있음 | 완전히 독립된 Codex formatter |
| 프로젝트 식별 | basename 충돌 가능 | canonical Git identity + workspace/worktree key |
| Queue | memory 내부 비동기 처리의 복구 경계가 불명확할 수 있음 | SQLite durable jobs + retry + dead letter |
| 검색 | Vector sidecar가 검색 가용성에 영향 | FTS5 mandatory, vector optional |
| Provider | host와 processor가 암묵적으로 결합될 수 있음 | 명시적 provider policy와 credential boundary |
| Context | 과도하거나 오래된 context 주입 가능 | relevance·recency·byte budget·freshness 표시 |
| Migration | 원본 schema에 지속 종속 | one-way import 후 Engramux schema 독립 |
| Update | cache/marketplace 복수 경로가 worker identity를 분열시킬 수 있음 | 고정 설치 경로 하나와 원자적 교체 |

### 4.3 폐기할 구조

다음은 Go로 옮기지 않는다.

- Express worker
- Bun process manager
- Node/Bun runner
- hook마다 shell profile 탐색
- Python/uv/chroma-mcp process tree
- TCP port를 worker 생존의 근거로 사용하는 구조
- plugin cache directory에서 실행 binary를 동적으로 선택하는 구조
- Claude와 Codex가 같은 hook output serializer를 공유하는 구조
- transcript 파일 형식을 stable protocol로 취급하는 구조
- memory worker가 죽으면 hook failure counter를 올려 호스트를 차단하는 구조
- session별 provider process
- vector index 장애 시 전체 검색이 실패하는 구조

---

## 5. Engramux 핵심 불변식

구현과 review에서 다음 불변식을 깨는 변경은 architecture change로 취급한다.

### I-01. 단일 서비스

사용자 Windows logon session에는 장기 실행 `engramux.exe service`가 최대 하나만 존재한다.

### I-02. 세션은 프로세스가 아니다

Claude/Codex session과 subagent는 `SessionState`이며 OS process가 아니다.

### I-03. Relay는 worker가 아니다

Hook가 실행하는 `engramux.exe relay`는 stdin을 읽고 IPC 요청을 전송한 뒤 즉시 종료한다. DB, model, search index, HTTP listener를 열지 않는다.

### I-04. ACK는 durable commit 이후에만 반환한다

서비스는 event가 SQLite transaction에 commit된 뒤에만 `committed` ACK를 반환한다.

### I-05. At-least-once + idempotency

Relay retry나 host duplicate event가 발생해도 unique idempotency key로 중복 저장을 방지한다.

### I-06. 세션 내부 순서 보장

같은 logical session의 event는 순서대로 처리한다. 서로 다른 session은 병렬 처리할 수 있다.

### I-07. Hook critical path에 LLM이 없다

SessionStart context lookup을 제외한 AI compression, embedding, consolidation은 모두 background job이다.

### I-08. Memory는 fail-open이다

서비스·DB·provider·search 장애는 Claude Code 또는 Codex의 prompt, tool, Stop을 차단하지 않는다.

### I-09. Host output은 분리된다

Claude Code 전용 필드는 Codex 응답에 절대 나타나지 않는다. 반대도 동일하다.

### I-10. FTS 검색은 항상 독립 동작한다

Embedding provider나 vector index가 없어도 `memory_search`는 동작한다.

### I-11. 원본 DB는 읽기 전용이다

Migration 과정에서 `claude-mem.db`를 수정·vacuum·migrate하지 않는다.

### I-12. 전역 지침에 의존하지 않는다

Engramux는 `CLAUDE.md`, `AGENTS.md`, global prompt, host memory instruction 변경을 요구하지 않는다.

---

## 6. 최종 아키텍처

```text
┌─────────────────────────────────────────────────────────────────────┐
│                           AI HOSTS                                  │
│                                                                     │
│  Claude Code                            Codex CLI / App             │
│  ┌──────────────┐                      ┌──────────────┐             │
│  │ Native Hooks │                      │ Native Hooks │             │
│  │ MCP Client   │                      │ MCP Client   │             │
│  └──────┬───────┘                      └──────┬───────┘             │
└─────────┼─────────────────────────────────────┼─────────────────────┘
          │ command hook                         │ command hook
          ▼                                      ▼
┌──────────────────┐                    ┌──────────────────┐
│ Go Relay         │                    │ Go Relay         │
│ host=claude-code │                    │ host=codex       │
│ no state         │                    │ no state         │
└─────────┬────────┘                    └─────────┬────────┘
          └──────────────────┬────────────────────┘
                             │ Windows Named Pipe
                             │ length-prefixed JSON v1
                             ▼
┌─────────────────────────────────────────────────────────────────────┐
│                 ENGRAMUX USER SERVICE — ONE PROCESS                │
│                                                                     │
│  ┌──────────────┐   ┌────────────────┐   ┌──────────────────────┐  │
│  │ Pipe Ingress │ → │ Host Adapters  │ → │ Durable Event Journal│  │
│  └──────────────┘   │ Claude / Codex │   │ SQLite WAL           │  │
│                     └────────────────┘   └──────────┬───────────┘  │
│                                                     │              │
│  ┌───────────────────┐   ┌───────────────────┐      ▼              │
│  │ Session Registry  │ ← │ Session Router    │ ← Event Dispatcher │
│  └───────────────────┘   └───────────────────┘                     │
│             │                       │                               │
│             │               per-session ordered queue              │
│             │                       ▼                               │
│  ┌───────────────────┐   ┌────────────────────┐                    │
│  │ Context Engine    │   │ Durable Job Queue  │                    │
│  └───────────────────┘   └──────────┬─────────┘                    │
│                                     ▼                              │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │ Bounded Background Workers                                  │  │
│  │ extraction │ summarization │ embedding │ consolidation       │  │
│  └───────────────────────────┬──────────────────────────────────┘  │
│                              ▼                                      │
│  ┌────────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│  │ Memory Store       │  │ FTS5 Search     │  │ Optional Vector │ │
│  └────────────────────┘  └─────────────────┘  └─────────────────┘ │
│                                                                     │
│  ┌─────────────────────────┐    ┌────────────────────────────────┐ │
│  │ Streamable HTTP MCP     │    │ Diagnostics / Doctor / Export │ │
│  │ 127.0.0.1 only          │    │ Structured logs              │ │
│  └─────────────────────────┘    └────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────┘
                             ▲
                             │ Direct MCP connection
                ┌────────────┴────────────┐
                │ Claude Code / Codex MCP │
                └─────────────────────────┘
```

### 6.1 프로세스 모델

정상 상태:

```text
Persistent:
  engramux.exe service                1개

Per hook event:
  engramux.exe relay ...              매우 짧게 실행 후 종료

Default child processes:
  0개
```

여러 세션이 있어도 달라지지 않는다.

```text
Claude session A ─┐
Claude session B ─┤
Codex session C  ─┼── one Engramux service
Codex session D  ─┤
Subagents         ─┘
```

Relay 실행은 host hook의 현재 계약상 필요한 transport invocation이다. 이것은 session마다 유지되는 worker가 아니며, event마다 짧게 실행되는 무상태 client다.

---

## 7. 왜 Named Pipe + Relay를 선택하는가

### 7.1 가능한 대안

| 대안 | 장점 | 문제 |
|---|---|---|
| Hook마다 HTTP POST | 구현 단순, 일부 Claude hook에서 직접 지원 | Claude SessionStart는 직접 HTTP로 통일하기 어렵고 Codex는 command hook 중심 |
| Hook마다 STDIO MCP 실행 | 표준 protocol | 매 invocation MCP handshake가 과하고, session event capture와 search server 수명이 섞임 |
| Transcript watcher | Host hook 불필요 | transcript schema 변화, 지연, 누락, 파일 lock, 정확한 event semantic 상실 |
| Named Pipe relay | Windows 저지연, port 없음, ACL 가능, 두 host 공통 | 짧은 relay process는 필요 |
| 세션별 worker | 상태 격리 | process 폭증, lifecycle 복잡성, 사용자 요구와 반대 |

최종 선택은 Named Pipe relay다.

### 7.2 Relay 책임

Relay가 수행하는 일:

1. stdin을 정확히 한 번 읽는다.
2. host와 event type을 명시한다.
3. 입력 크기·JSON 구조를 검증한다.
4. 민감한 대형 binary payload를 제거하거나 축약한다.
5. versioned `EventEnvelope`를 만든다.
6. Named Pipe로 전송한다.
7. durable ACK 또는 context 응답을 제한 시간 내 수신한다.
8. host별 유효한 stdout을 한 번만 출력한다.
9. 항상 process handle과 pipe를 닫는다.

Relay가 하지 않는 일:

- SQLite open
- migration
- FTS query
- embedding
- LLM API 호출
- worker start loop
- health polling loop
- HTTP server start
- plugin path 탐색
- shell 실행
- child process 생성
- 긴 retry
- 전역 failure counter 증가

### 7.3 Wire protocol

초기 protocol은 Protobuf보다 versioned JSON이 적합하다. 디버깅과 fixture 검증이 쉽고 event 양이 로컬 IPC 범위이기 때문이다.

Frame:

```text
[4-byte little-endian payload length][UTF-8 JSON payload]
```

Hard limits:

```text
max frame             4 MiB
default field cap     512 KiB
pipe connect deadline 25 ms
capture total budget  250 ms
context total budget  500 ms
```

초과 payload는 무조건 실패시키기보다 다음 metadata를 남기고 안전하게 축약한다.

```json
{
  "truncated": true,
  "original_bytes": 3829104,
  "stored_bytes": 524288,
  "sha256": "..."
}
```

### 7.4 EventEnvelope

```go
type EventEnvelope struct {
    Version         uint16          `json:"version"`
    EventID         string          `json:"event_id"`
    IdempotencyKey  string          `json:"idempotency_key"`
    Host            Host            `json:"host"`
    EventType       EventType       `json:"event_type"`
    SessionID       string          `json:"session_id"`
    ParentSessionID string          `json:"parent_session_id,omitempty"`
    ProjectRoot     string          `json:"project_root,omitempty"`
    ProjectKey      string          `json:"project_key,omitempty"`
    HostTimestamp   time.Time       `json:"host_timestamp,omitempty"`
    ReceivedAt      time.Time       `json:"received_at"`
    Sequence        uint64          `json:"sequence,omitempty"`
    Payload         json.RawMessage `json:"payload"`
    PayloadHash     string          `json:"payload_hash"`
    PrivacyClass    PrivacyClass    `json:"privacy_class"`
    RelayVersion    string          `json:"relay_version"`
}
```

ACK:

```json
{
  "version": 1,
  "event_id": "evt_...",
  "status": "committed",
  "boot_id": "boot_...",
  "context": null
}
```

`SessionStart`에서만 context가 포함될 수 있다.

---

## 8. 단일 Windows 서비스 워커

### 8.1 기본 설치 방식

Engramux 1.0의 기본 실행 방식은 **사용자 단위 background service worker**다.

권장:

```text
Windows Task Scheduler
  trigger: user logon
  executable: %LOCALAPPDATA%\Programs\Engramux\engramux.exe
  args: service run
  hidden: true
  restart on failure: enabled
```

이 방식의 목적:

- 관리자 권한 없이 설치
- 현재 사용자의 `%LOCALAPPDATA%`, credential, config 사용
- LocalSystem과 사용자 profile의 혼동 제거
- 로그인 전 실행이 불필요
- 사용자별 DB와 pipe를 자연스럽게 분리

Windows SCM 서비스는 enterprise 선택 모드로 나중에 추가할 수 있으나 1.0 기본값으로 두지 않는다. LocalSystem 서비스가 사용자 token과 memory DB를 대신 소유하게 만들면 보안·경로·credential 문제가 복잡해진다.

### 8.2 Singleton 권위

PID 파일은 singleton의 권위가 아니다.

권위 순서:

1. Windows Named Mutex 획득
2. Named Pipe handshake 응답
3. `boot_id` 일치
4. owner record는 진단 자료로만 사용

예시 이름:

```text
Mutex:
  Local\Engramux.Service.v1.<UserSidHash>

Pipe:
  \\.\pipe\engramux.v1.<UserSidHash>
```

두 번째 서비스 실행이 mutex를 얻지 못하면 기존 pipe에 `status` handshake를 하고 정상 서비스가 확인되면 즉시 종료한다.

### 8.3 Owner record

```json
{
  "pid": 12340,
  "boot_id": "boot_019...",
  "version": "0.1.0",
  "binary_path": "C:\\Users\\...\\Engramux\\engramux.exe",
  "pipe_name": "\\\\.\\pipe\\engramux.v1....",
  "mcp_endpoint": "http://127.0.0.1:47183/mcp",
  "started_at": "2026-08-25T00:00:00+09:00"
}
```

파일 위치:

```text
%LOCALAPPDATA%\Engramux\runtime\owner.json
```

이 파일이 stale하더라도 mutex와 pipe가 진실이다.

### 8.4 Process ownership

기본 provider 구현은 HTTP client이므로 service의 child process 수는 0이다.

나중에 local model이나 helper process를 도입한다면:

- 모든 child를 Windows Job Object에 할당
- `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`
- explicit handle inheritance
- bounded memory/CPU
- verified child identity
- shutdown latch 이후 respawn 금지

를 적용한다.

---

## 9. Claude Code 통합

### 9.1 Claude plugin의 책임

Claude Code plugin은 wiring만 담당한다.

```text
plugins\claude-code\
├─ .claude-plugin\
│  └─ plugin.json
├─ hooks\
│  └─ hooks.json
└─ .mcp.json
```

의도적으로 포함하지 않는 것:

```text
skills\
agents\
commands\
CLAUDE.md
global prompt fragments
Node scripts
Bun scripts
```

검색과 조회는 MCP tool description만으로 동작하게 한다.

### 9.2 기본 lifecycle

| Event | 기본 동작 | 동기성 | 실패 |
|---|---|---:|---|
| SessionStart | event 저장 + compact context 요청 | 짧은 동기 | 빈 context로 exit 0 |
| UserPromptSubmit | prompt durable 저장 | 짧은 동기 | optional spool 후 exit 0 |
| PostToolUse | tool event durable 저장 | 짧은 동기 또는 host async | exit 0 |
| Stop | summary job enqueue | 짧은 동기 | exit 0 |
| SessionEnd | session completion enqueue | 짧은 동기 | exit 0 |
| PreCompact | compact checkpoint 저장 | 짧은 동기 | exit 0 |
| PostCompact | compact 이후 context marker | 짧은 동기 | exit 0 |

`PreToolUse:Read` 기반 file-context 주입은 기본 비활성화한다.

이유:

- memory 장애가 `Read`를 막아서는 안 된다.
- 모든 Read에 hook를 실행하면 overhead가 커진다.
- subagent에서 불필요한 context가 삽입될 수 있다.
- MCP search와 SessionStart context로 대부분의 목적을 달성할 수 있다.

필요한 사용자를 위해 opt-in 기능으로만 제공한다.

### 9.3 Claude output formatter

Claude formatter는 Claude hook schema만 담당한다.

```go
type ClaudeAdapter interface {
    ParseHookInput(event EventType, raw []byte) (NormalizedEvent, error)
    FormatSuccess(event EventType, result HookResult) ([]byte, error)
    FormatFailOpen(event EventType, reason string) []byte
}
```

Capture event의 성공 응답은 가능한 한 stdout을 비운다. Context가 필요한 이벤트만 정확히 하나의 JSON document를 출력한다.

금지:

- 두 개 JSON object 연속 출력
- diagnostic을 stdout에 출력
- `exit 2`로 memory 장애 통지
- stderr spam
- shell fallback echo와 relay output의 중복

---

## 10. Codex 통합

### 10.1 Codex plugin의 책임

```text
plugins\codex\
├─ .codex-plugin\
│  └─ plugin.json
├─ hooks\
│  └─ hooks.json
└─ .mcp.json 또는 설치용 config fragment
```

Codex adapter도 wiring만 가진다.

### 10.2 Codex 계약

현재 Codex hook 실행 경로는 command handler를 기준으로 설계해야 한다. 따라서 Windows에서는 설치 시 절대 경로와 `commandWindows`를 생성한다.

예시 개념:

```json
{
  "type": "command",
  "commandWindows": "\"C:\\Users\\...\\Engramux\\engramux.exe\" relay --host codex --event post-tool-use"
}
```

실제 JSON은 지원 Codex 버전의 checked-in fixture에 대해 검증해서 생성한다. 손으로 배포 artifact를 편집하지 않는다.

### 10.3 Codex formatter 분리

```go
type CodexAdapter interface {
    ParseHookInput(event EventType, raw []byte) (NormalizedEvent, error)
    FormatSuccess(event EventType, result HookResult) ([]byte, error)
    FormatFailOpen(event EventType, reason string) []byte
}
```

Codex path에서 금지:

- `suppressOutput`
- Claude의 `hookSpecificOutput`
- Claude 전용 event field
- POSIX shell-only command
- transcript JSONL 구조에 대한 강한 의존
- unsupported output key

`ClaudeAdapter`와 `CodexAdapter`는 normalized domain event까지만 공유한다. 최종 host JSON serializer는 공유하지 않는다.

### 10.4 Codex IDE 범위

Codex IDE extension은 MCP 연결을 통한 검색·조회는 지원 범위에 포함한다. Plugin lifecycle 자동 capture는 해당 surface가 공식 hook/plugin contract를 실제로 실행하는지 contract test로 확인된 경우만 지원으로 표시한다.

즉 문서와 installer는 다음을 구분해서 표시한다.

| Surface | Capture | Context injection | MCP search |
|---|---:|---:|---:|
| Codex CLI | 지원 | 지원 | 지원 |
| Codex App/Desktop | contract test 통과 시 지원 | contract test 통과 시 지원 | 지원 |
| Codex IDE extension | host hook 지원 확인 범위 | 제한적 | 지원 |

지원하지 않는 surface를 설치 성공으로 과장하지 않는다.

---

## 11. Host contract test

Claude Code와 Codex는 같은 의미의 lifecycle event를 갖더라도 입력·출력 schema가 동일하지 않다.

따라서 repository에 실제 fixture를 저장한다.

```text
tests\fixtures\hosts\
├─ claude-code\
│  ├─ session-start.json
│  ├─ user-prompt-submit.json
│  ├─ post-tool-use.json
│  ├─ stop.json
│  └─ expected\
└─ codex\
   ├─ session-start.json
   ├─ user-prompt-submit.json
   ├─ post-tool-use.json
   ├─ stop.json
   └─ expected\
```

CI가 검증할 항목:

1. 모든 fixture를 parse할 수 있다.
2. unknown field를 안전하게 무시한다.
3. 필수 field 누락 시 panic하지 않는다.
4. 출력에 host가 허용하지 않는 field가 없다.
5. stdout은 최대 한 개의 JSON document다.
6. capture event는 memory 장애 시에도 blocking code를 반환하지 않는다.
7. Windows command path에 shell quoting defect가 없다.
8. host version 변경으로 fixture가 갱신될 때 diff가 review된다.

---

## 12. Event journal과 데이터 모델

### 12.1 Source of truth

Engramux의 source of truth는 append-only `events`다.

```text
events
  ├─ prompts
  ├─ observations
  ├─ session_summaries
  ├─ memory_items
  ├─ embeddings
  └─ search indexes
```

아래 데이터는 재생성 가능한 derived state다.

- observations
- summaries
- memory items
- embeddings
- FTS index
- context digest cache

이 구조의 장점:

- parser/model bug 수정 후 replay 가능
- provider 변경 후 재처리 가능
- migration rollback이 쉬움
- ACK된 원본 이벤트의 유실 여부를 검증 가능
- dead-letter event를 추적 가능

### 12.2 권장 핵심 schema

```sql
CREATE TABLE projects (
    id                TEXT PRIMARY KEY,
    repository_key    TEXT NOT NULL,
    workspace_key     TEXT NOT NULL,
    canonical_root    TEXT NOT NULL,
    display_name      TEXT NOT NULL,
    git_remote_hash   TEXT,
    created_at_ms     INTEGER NOT NULL,
    last_seen_at_ms   INTEGER NOT NULL,
    UNIQUE(repository_key, workspace_key)
);

CREATE TABLE sessions (
    id                   TEXT PRIMARY KEY,
    host                 TEXT NOT NULL,
    host_session_id      TEXT NOT NULL,
    parent_session_id    TEXT,
    project_id           TEXT NOT NULL,
    status               TEXT NOT NULL,
    started_at_ms        INTEGER NOT NULL,
    last_activity_at_ms  INTEGER NOT NULL,
    completed_at_ms      INTEGER,
    last_sequence        INTEGER NOT NULL DEFAULT 0,
    metadata_json        TEXT NOT NULL DEFAULT '{}',
    UNIQUE(host, host_session_id),
    FOREIGN KEY(project_id) REFERENCES projects(id)
);

CREATE TABLE events (
    id                  TEXT PRIMARY KEY,
    host                TEXT NOT NULL,
    event_type          TEXT NOT NULL,
    session_id          TEXT NOT NULL,
    project_id          TEXT NOT NULL,
    sequence_no         INTEGER,
    idempotency_key     TEXT NOT NULL,
    payload_json        TEXT NOT NULL,
    payload_sha256      TEXT NOT NULL,
    privacy_class       TEXT NOT NULL,
    host_timestamp_ms   INTEGER,
    received_at_ms      INTEGER NOT NULL,
    schema_version      INTEGER NOT NULL,
    relay_version       TEXT NOT NULL,
    UNIQUE(host, idempotency_key),
    FOREIGN KEY(session_id) REFERENCES sessions(id)
);

CREATE INDEX idx_events_session_order
ON events(session_id, sequence_no, received_at_ms);

CREATE INDEX idx_events_project_time
ON events(project_id, received_at_ms DESC);

CREATE TABLE jobs (
    id                TEXT PRIMARY KEY,
    kind              TEXT NOT NULL,
    event_id          TEXT,
    session_id        TEXT,
    state             TEXT NOT NULL,
    priority          INTEGER NOT NULL DEFAULT 100,
    attempt_count     INTEGER NOT NULL DEFAULT 0,
    available_at_ms   INTEGER NOT NULL,
    lease_owner       TEXT,
    lease_until_ms    INTEGER,
    payload_json      TEXT NOT NULL,
    last_error        TEXT,
    created_at_ms     INTEGER NOT NULL,
    updated_at_ms     INTEGER NOT NULL,
    UNIQUE(kind, event_id)
);

CREATE TABLE dead_letters (
    id               TEXT PRIMARY KEY,
    job_id           TEXT NOT NULL,
    kind             TEXT NOT NULL,
    payload_json     TEXT NOT NULL,
    error_class      TEXT NOT NULL,
    error_message    TEXT NOT NULL,
    attempts         INTEGER NOT NULL,
    failed_at_ms     INTEGER NOT NULL
);

CREATE TABLE prompts (
    id               TEXT PRIMARY KEY,
    event_id         TEXT NOT NULL UNIQUE,
    session_id       TEXT NOT NULL,
    project_id       TEXT NOT NULL,
    prompt_number    INTEGER,
    prompt_text      TEXT NOT NULL,
    created_at_ms    INTEGER NOT NULL
);

CREATE TABLE observations (
    id               TEXT PRIMARY KEY,
    source_event_id  TEXT NOT NULL,
    session_id       TEXT NOT NULL,
    project_id       TEXT NOT NULL,
    prompt_number    INTEGER,
    type             TEXT,
    title            TEXT,
    narrative        TEXT,
    facts_json       TEXT NOT NULL DEFAULT '[]',
    concepts_json    TEXT NOT NULL DEFAULT '[]',
    files_read_json  TEXT NOT NULL DEFAULT '[]',
    files_changed_json TEXT NOT NULL DEFAULT '[]',
    confidence       REAL,
    processor_id     TEXT,
    created_at_ms    INTEGER NOT NULL,
    UNIQUE(source_event_id, processor_id)
);

CREATE TABLE session_summaries (
    id               TEXT PRIMARY KEY,
    session_id       TEXT NOT NULL,
    project_id       TEXT NOT NULL,
    through_sequence INTEGER NOT NULL,
    request_text     TEXT,
    investigated     TEXT,
    learned          TEXT,
    completed        TEXT,
    next_steps       TEXT,
    notes            TEXT,
    processor_id     TEXT,
    created_at_ms    INTEGER NOT NULL,
    UNIQUE(session_id, through_sequence, processor_id)
);

CREATE TABLE memory_items (
    id               TEXT PRIMARY KEY,
    project_id       TEXT NOT NULL,
    session_id       TEXT,
    source_type      TEXT NOT NULL,
    source_id        TEXT NOT NULL,
    memory_type      TEXT NOT NULL,
    title            TEXT,
    body             TEXT NOT NULL,
    facts_json       TEXT NOT NULL DEFAULT '[]',
    concepts_json    TEXT NOT NULL DEFAULT '[]',
    file_refs_json   TEXT NOT NULL DEFAULT '[]',
    importance       REAL NOT NULL DEFAULT 0.5,
    confidence       REAL NOT NULL DEFAULT 0.5,
    valid_from_ms    INTEGER,
    valid_to_ms      INTEGER,
    superseded_by    TEXT,
    created_at_ms    INTEGER NOT NULL,
    updated_at_ms    INTEGER NOT NULL
);

CREATE TABLE embeddings (
    memory_id         TEXT NOT NULL,
    provider          TEXT NOT NULL,
    model             TEXT NOT NULL,
    dimensions        INTEGER NOT NULL,
    vector_blob       BLOB NOT NULL,
    content_sha256    TEXT NOT NULL,
    created_at_ms     INTEGER NOT NULL,
    PRIMARY KEY(memory_id, provider, model)
);
```

실제 migration은 작은 번호 단위로 분리하고 historical fixture DB에서 순차 검증한다.

### 12.3 FTS5

```sql
CREATE VIRTUAL TABLE memory_items_fts USING fts5(
    title,
    body,
    facts,
    concepts,
    file_refs,
    content='memory_items',
    content_rowid='rowid',
    tokenize='unicode61'
);
```

FTS trigger와 rebuild command를 제공한다.

```text
engramux reindex --fts
engramux verify --fts
```

---

## 13. 프로젝트와 Worktree 식별

프로젝트 basename만 key로 쓰지 않는다.

### 13.1 두 단계 identity

```text
RepositoryIdentity
  - canonical Git common dir
  - normalized remote identity
  - repository fingerprint

WorkspaceIdentity
  - current worktree root
  - branch/worktree marker
  - absolute canonical path hash
```

예:

```go
type ProjectIdentity struct {
    RepositoryKey string
    WorkspaceKey  string
    CanonicalRoot string
    DisplayName   string
    RemoteHash    string
}
```

### 13.2 Windows normalization

- drive letter 대소문자 정규화
- `\\?\` prefix 정규화
- symlink/junction 최종 경로 확인
- path separator 통일
- Git worktree의 common dir와 worktree root 분리
- remote URL에는 credential을 저장하지 않고 canonicalized hash만 저장
- repository가 아닌 디렉터리는 canonical path hash로 fallback

검색 기본 정책:

1. 현재 workspace exact match boost
2. 같은 repository 다른 worktree 포함
3. 다른 repository는 명시적 global search에서만 포함

---

## 14. Session router와 동시성

### 14.1 세션 key

```go
type SessionKey struct {
    Host      Host
    SessionID string
}
```

Claude와 Codex가 같은 session ID 문자열을 사용해도 충돌하지 않는다.

### 14.2 Ordering

```text
global ingress
    ↓
idempotency check + event commit
    ↓
session shard selection
    ↓
per-session ordered processor
    ↓
global bounded worker pools
```

같은 session:

```text
prompt #1
tool #2
tool #3
stop #4
```

순서를 유지한다.

다른 session:

```text
Claude A #2
Codex B #18
Claude C #4
```

는 병렬 처리한다.

### 14.3 Bounded concurrency

초기 기본값 예시:

```text
event dispatch workers       min(CPU, 8)
summary workers              2
embedding workers            2
consolidation workers        1
provider global concurrency  4
per-session provider         1
```

모든 값은 config로 조정 가능하지만 무제한 goroutine spawn은 금지한다.

### 14.4 Backpressure

- ingress ACK는 raw event commit까지만 보장
- derived processing queue가 포화되어도 capture는 계속 가능
- jobs table에서 priority 사용
- 오래된 embedding보다 SessionStart에 필요한 recent summary를 우선
- disk quota를 넘으면 raw event를 임의 삭제하지 않고 capture degradation mode 진입
- 사용자에게 host hook error 대신 `engramux status`와 log에 진단 표시

---

## 15. Memory 처리 pipeline

```text
Raw Event
  ↓
Normalization
  ↓
Security Redaction
  ↓
Deterministic Extraction
  ↓
Durable Job
  ↓
Optional LLM Processing
  ↓
Observation / Summary
  ↓
Memory Consolidation
  ↓
FTS5
  ↓
Optional Embedding
```

### 15.1 Deterministic extraction

LLM 없이도 다음은 추출할 수 있다.

- tool 이름
- 읽은 파일
- 변경한 파일
- command exit status
- diff 통계
- prompt 순번
- event timestamp
- project/worktree
- error code와 제한된 error message
- payload hash

Provider가 없어도 raw capture + FTS search가 동작해야 한다.

### 15.2 Observation 생성

Observation은 tool event 하나를 그대로 복사하는 것이 아니라 다음 질문에 답해야 한다.

- 무엇을 수행했는가
- 무엇이 변경되었는가
- 왜 중요한가
- 어떤 파일·component가 관련되는가
- decision, bugfix, feature, refactor, discovery 중 무엇인가
- 이후 세션이 알아야 하는 사실인가

Noise filter:

- 동일한 read 반복
- 단순 directory listing
- empty output
- heartbeat
- status polling
- 민감한 credential 확인
- 대형 generated output

은 memory 후보에서 낮게 평가한다.

### 15.3 Consolidation

중복 memory를 계속 추가하지 않는다.

```text
새 memory
  ↓
same project + concept + file candidate 검색
  ↓
duplicate / update / contradict / independent 판정
  ↓
append, merge, supersede 중 하나
```

기존 memory를 물리적으로 덮어쓰기보다:

```text
superseded_by
valid_from
valid_to
```

를 이용해 변경 이력을 보존한다.

---

## 16. Provider 설계

### 16.1 Host와 Provider를 분리한다

다음은 서로 다른 개념이다.

```text
Host:
  Claude Code
  Codex

Processor Provider:
  Anthropic API
  OpenAI API
  Local/None
```

Codex에서 event가 왔다고 Claude provider를 자동 호출하지 않는다. Claude Code event라고 Codex/OpenAI provider를 자동 호출하지도 않는다.

### 16.2 Provider interface

```go
type Generator interface {
    ID() string
    GenerateObservation(
        context.Context,
        ObservationRequest,
    ) (ObservationResult, error)

    GenerateSessionSummary(
        context.Context,
        SummaryRequest,
    ) (SummaryResult, error)

    Consolidate(
        context.Context,
        ConsolidationRequest,
    ) (ConsolidationResult, error)
}

type Embedder interface {
    ID() string
    Dimensions() int
    Embed(
        context.Context,
        []string,
    ) ([][]float32, error)
}
```

### 16.3 지원 mode

```toml
[processor]
mode = "deterministic" # deterministic | anthropic | openai | off
fallback = "deterministic"
max_concurrency = 2

[embedding]
enabled = false
provider = "openai"
```

권장 기본값:

```text
capture + deterministic extraction + FTS5 = 항상 사용 가능
LLM observation/summarization              = 명시적으로 활성화
embedding                                  = 선택
```

### 16.4 Credential 경계

Engramux는 다음 파일을 몰래 읽지 않는다.

```text
~/.claude/.credentials.json
~/.codex/auth.json
```

지원 경로:

- Engramux 전용 protected config/credential store
- 환경 변수
- 향후 Windows Credential Manager

호스트 구독 token을 비공식적으로 재사용하는 설계는 제외한다.

### 16.5 Provider 장애

- exponential backoff + jitter
- rate limit 분류
- auth error는 무한 retry하지 않음
- transient error만 제한 retry
- 실패 job은 dead letter
- Hook ACK와 무관
- FTS/raw memory는 계속 제공

---

## 17. Search와 MCP

### 17.1 MCP transport

Engramux service가 하나의 Streamable HTTP MCP endpoint를 제공한다.

```text
http://127.0.0.1:<persisted-port>/mcp
```

보안:

- `127.0.0.1`만 bind
- bearer token 필수
- token file은 current-user ACL
- CORS 불필요 또는 strict localhost
- remote bind 금지
- `/health`는 최소 정보만 반환
- MCP port는 설치 시 선택해 config에 원자적으로 기록

Hook event는 MCP port를 사용하지 않는다. 따라서 MCP port 장애가 capture를 막지 않는다.

### 17.2 MCP tool 최소화

1.0 tool은 네 개로 제한한다.

#### `memory_search`

조건 기반 검색:

```json
{
  "query": "barcode protocol sequence mismatch",
  "project_scope": "current_repository",
  "types": ["decision", "bugfix"],
  "files": ["DeviceCodec.cs"],
  "since": "2026-01-01",
  "limit": 20
}
```

#### `memory_timeline`

시간 흐름과 전후 context 조회.

#### `memory_get`

선택한 memory/observation/summary의 상세 조회.

#### `memory_context`

현재 project/task에 맞춘 제한된 context bundle 반환.

파괴적 `forget/delete`는 1.0 MCP에서 제외하고 CLI에서 명시적으로 제공한다.

```text
engramux forget --id ...
engramux purge --project ... --confirm
```

### 17.3 Progressive disclosure

```text
search
  ↓ small result IDs + title + snippet
timeline
  ↓ surrounding events
get
  ↓ selected full details
```

첫 호출에 수십 KB를 반환하지 않는다.

### 17.4 Ranking

기본 ranking:

```text
BM25 FTS score
+ exact project boost
+ same repository boost
+ file overlap boost
+ recency decay
+ importance
+ optional vector similarity
```

FTS와 vector 결과는 Reciprocal Rank Fusion으로 결합한다.

```text
RRF(d) = Σ 1 / (k + rank_i(d))
```

Vector가 비활성화되거나 실패하면 BM25 경로만 사용한다.

### 17.5 Vector 단계

#### 1.0

- embedding BLOB 저장
- FTS/project/time으로 후보를 충분히 축소
- 후보 집합에 Go cosine similarity 적용
- vector extension을 release blocker로 두지 않음

#### 1.1 이후

- Windows stress와 migration 검증을 통과한 SQLite vector virtual table 또는 검증된 extension 도입 검토
- 동일한 `VectorIndex` interface 뒤에 구현
- FTS fallback 유지

---

## 18. Context injection

### 18.1 목적

SessionStart context는 과거 대화를 재현하는 것이 아니라, 현재 작업을 시작하는 데 필요한 최소한의 연속성을 제공한다.

기본 구성:

```text
- 최근 중요한 결정
- 현재 미완료 작업
- 최근 변경 파일
- 반복해서 발생한 문제
- 현재 repository에만 관련된 사실
```

### 18.2 Budget

예시 기본값:

```toml
[context]
max_bytes = 8192
max_items = 12
max_sessions = 5
include_other_worktrees = true
include_subagents = false
```

Provider token 계산이 불가능한 상황에서도 byte cap은 반드시 지킨다.

### 18.3 Freshness와 충돌

각 항목에는 내부적으로 다음을 유지한다.

- source timestamp
- confidence
- superseded 여부
- 현재 workspace와의 거리
- source host
- source session

오래된 memory를 최신 사실처럼 표현하지 않는다.

### 18.4 Timeout

SessionStart relay는 context를 최대 500ms까지만 기다린다.

실패하면:

```text
정상 exit 0
context 없음
session event는 가능하면 spool 또는 이후 재전송
```

Claude/Codex session 시작 자체를 지연시키지 않는다.

---

## 19. Local spool과 데이터 무손실

서비스가 일시적으로 내려가 있을 수 있다.

### 19.1 Fail-open과 capture 보존의 균형

Relay는 다음 순서로 동작한다.

```text
pipe connect
  ├─ 성공 → service commit ACK → exit 0
  └─ 실패
       ├─ bounded local spool append 성공 → exit 0
       └─ spool 실패 → diagnostic log → exit 0
```

Spool 위치:

```text
%LOCALAPPDATA%\Engramux\spool\
```

Spool 제약:

- current-user ACL
- append-only framed records
- file당 size cap
- 전체 quota
- age retention
- checksum
- idempotency key
- service recovery 시 원자적 import
- import 완료 후 rename → delete

SessionStart context는 spool로 대체할 수 없으므로 빈 context로 진행한다.

### 19.2 Crash consistency

중요 crash point를 테스트한다.

1. pipe read 전
2. event validation 후
3. transaction commit 직전
4. commit 직후 ACK 전
5. ACK 직후 relay 종료 전
6. job lease 획득 후
7. derived row 저장 후
8. embedding 저장 중

원칙:

- ACK 전 crash: relay가 retry할 수 있음
- commit 후 ACK 전 crash: idempotency로 duplicate 방지
- derived job crash: lease 만료 후 재처리
- FTS/vector crash: rebuild 가능

---

## 20. `claude-mem` 데이터 migration

### 20.1 Migration 원칙

```text
Source DB: read-only
Destination: new Engramux DB
Direction: one way
Rollback: destination 삭제 후 재실행 가능
```

명령:

```text
engramux migrate claude-mem \
  --source "%USERPROFILE%\.claude-mem\claude-mem.db" \
  --dry-run

engramux migrate claude-mem \
  --source "%USERPROFILE%\.claude-mem\claude-mem.db" \
  --backup-manifest
```

### 20.2 기본 mapping

| claude-mem | Engramux |
|---|---|
| `sdk_sessions` | `sessions` |
| `user_prompts` | `prompts` + synthetic import events |
| `observations` | `observations` + `memory_items` |
| `session_summaries` | `session_summaries` + `memory_items` |
| FTS virtual tables | import하지 않고 rebuild |
| Chroma vectors | 기본적으로 import하지 않고 선택적 재생성 |
| legacy tables | 별도 compatibility reader로 best effort |

### 20.3 Import provenance

모든 imported row에 기록:

```text
source_system = claude-mem
source_database_hash
source_table
source_primary_key
source_schema_version
import_batch_id
```

중복 import를 방지한다.

### 20.4 Cutover

권장 cutover:

1. `claude-mem` 자동 capture 비활성화
2. source DB hash와 backup 생성
3. Engramux dry-run
4. import
5. row count·sample·FTS verify
6. Claude/Codex Engramux adapter 설치
7. 1회 shadow validation
8. Engramux만 활성화
9. source DB 보존

두 memory plugin을 동시에 장기 실행하지 않는다. 동일한 event가 중복 capture될 수 있기 때문이다.

---

## 21. Security와 Privacy

### 21.1 Data directory ACL

```text
%LOCALAPPDATA%\Engramux\
```

은 현재 사용자와 SYSTEM만 접근하도록 ACL을 설정한다.

Named Pipe도 default ACL을 사용하지 않고 명시적 DACL을 적용한다.

### 21.2 저장 전 redaction

기본 redaction 대상:

- `Authorization` header
- bearer token
- API key
- password
- private key
- `.env`
- `auth.json`
- credential files
- connection string password
- cookie
- PEM blocks
- SSH keys
- database secrets
- base64 binary payload

Redaction stage:

```text
host payload
  ↓ size limiter
  ↓ path policy
  ↓ structured secret detector
  ↓ text secret detector
  ↓ persistence
```

### 21.3 Path policy

```toml
[privacy]
exclude_globs = [
  "**/.env",
  "**/.env.*",
  "**/*.pem",
  "**/*.key",
  "**/auth.json",
  "**/.credentials.json",
  "**/.git/**"
]

private_tags = true
store_raw_tool_output = false
```

### 21.4 Transcript 취급

Host가 제공하는 transcript path는 신뢰할 수 없는 입력으로 취급한다.

- path canonicalization
- allowed root 확인
- file existence race 대응
- max read size
- missing file은 정상 skip
- transcript schema에 core logic을 결합하지 않음
- full transcript 영구 저장을 기본 비활성화

### 21.5 MCP security

- loopback bind
- bearer token
- read-only tools에 annotation
- destructive operation은 MCP에서 제외
- request body cap
- rate limit
- detailed stack trace 비노출
- token rotation command 제공

```text
engramux security rotate-mcp-token
```

---

## 22. 장애 정책과 복구

### 22.1 장애별 동작

| 장애 | Host 동작 | Engramux 동작 |
|---|---|---|
| service 미실행 | 계속 진행 | spool 시도, 로그 |
| pipe timeout | 계속 진행 | event 재전송 가능 |
| DB busy | 계속 진행 | bounded retry 후 spool/dead letter |
| disk full | 계속 진행 | capture degraded, 명확한 diagnostic |
| provider auth 실패 | 계속 진행 | job 중단, deterministic fallback |
| rate limit | 계속 진행 | backoff |
| FTS 손상 | 계속 진행 | raw/DB 유지, rebuild 필요 표시 |
| vector 손상 | FTS로 검색 | vector disable/rebuild |
| transcript missing | 계속 진행 | summary skip |
| malformed hook payload | 계속 진행 | redacted diagnostic |
| Codex schema 변화 | 해당 event fail-open | contract test/adapter update |
| service crash | host 계속 진행 | scheduler restart + spool import |

### 22.2 절대 하지 않을 동작

- worker unavailable 때문에 `exit 2`
- PreToolUse 실패로 Read deny
- Stop hook retry loop
- prompt submit 차단
- health check 무한 대기
- stale PID를 맹신
- port가 bind되었다는 이유만으로 정상 worker로 판정
- user prompt에 반복적인 error JSON 주입
- memory 복구를 위해 사용자 repository를 수정

---

## 23. Logging, Metrics, Doctor

### 23.1 Structured logs

형식:

```json
{
  "ts": "2026-08-25T00:00:00.123+09:00",
  "level": "INFO",
  "component": "pipe_ingress",
  "event": "event_committed",
  "boot_id": "boot_...",
  "event_id": "evt_...",
  "host": "codex",
  "session_hash": "...",
  "elapsed_ms": 8
}
```

민감한 prompt/tool body를 기본 log에 기록하지 않는다.

### 23.2 Log rotation

- size 기반 rotation
- 보존 기간
- 날짜 경계에서 새 파일
- active service가 쓰는 파일과 doctor가 읽는 파일이 동일
- crash log 별도
- log write 실패가 hook을 차단하지 않음

### 23.3 Metrics

초기 internal metrics:

- ingress count/latency
- duplicate event count
- spool count
- jobs queued/running/dead
- provider latency/error/rate limit
- FTS query latency
- context size
- DB WAL size
- service restarts
- active logical sessions
- dropped/truncated payload count

### 23.4 Doctor

```text
engramux doctor
```

출력 항목:

1. binary/version/hash
2. service singleton
3. mutex 상태
4. pipe handshake/boot ID
5. task scheduler 등록
6. data directory ACL
7. pipe ACL
8. SQLite integrity/checkpoint
9. migration version
10. job backlog/dead letters
11. FTS integrity
12. MCP bind/token
13. Claude hook contract
14. Codex hook contract
15. relay warm latency
16. context latency
17. provider credential 상태
18. spool backlog
19. disk quota
20. 최근 crash/restart

`doctor --json`도 제공한다.

---

## 24. CLI

```text
engramux service install
engramux service uninstall
engramux service start
engramux service stop
engramux service restart
engramux service status
engramux service run

engramux relay --host claude-code --event session-start
engramux relay --host codex --event post-tool-use

engramux plugin install claude-code
engramux plugin install codex
engramux plugin uninstall claude-code
engramux plugin uninstall codex
engramux plugin verify claude-code
engramux plugin verify codex

engramux migrate claude-mem --source <path>
engramux migrate status
engramux reindex --fts
engramux reindex --vectors
engramux replay --from-event <id>

engramux search <query>
engramux timeline --session <id>
engramux get <id>

engramux export --output <file>
engramux import --input <file>
engramux forget --id <id>
engramux purge --project <key> --confirm

engramux doctor
engramux status
engramux logs
engramux repair
engramux version
```

`relay`는 내부 host adapter가 사용하는 command이지만 직접 regression test도 가능하게 한다.

---

## 25. Repository 구조

```text
engramux\
├─ cmd\
│  └─ engramux\
│     └─ main.go
│
├─ internal\
│  ├─ app\
│  │  ├─ command.go
│  │  └─ lifecycle.go
│  │
│  ├─ service\
│  │  ├─ service.go
│  │  ├─ singleton_windows.go
│  │  ├─ task_scheduler_windows.go
│  │  └─ owner_record.go
│  │
│  ├─ host\
│  │  ├─ adapter.go
│  │  ├─ claude\
│  │  │  ├─ parser.go
│  │  │  ├─ formatter.go
│  │  │  └─ contract.go
│  │  └─ codex\
│  │     ├─ parser.go
│  │     ├─ formatter.go
│  │     └─ contract.go
│  │
│  ├─ transport\
│  │  ├─ framing\
│  │  ├─ namedpipe\
│  │  │  ├─ client_windows.go
│  │  │  ├─ server_windows.go
│  │  │  └─ security_windows.go
│  │  └─ mcp\
│  │     ├─ server.go
│  │     ├─ auth.go
│  │     └─ tools.go
│  │
│  ├─ event\
│  │  ├─ envelope.go
│  │  ├─ normalize.go
│  │  ├─ idempotency.go
│  │  └─ dispatcher.go
│  │
│  ├─ session\
│  │  ├─ registry.go
│  │  ├─ router.go
│  │  ├─ sequence.go
│  │  └─ project_identity.go
│  │
│  ├─ storage\
│  │  └─ sqlite\
│  │     ├─ database.go
│  │     ├─ migrations\
│  │     ├─ event_store.go
│  │     ├─ job_store.go
│  │     ├─ memory_store.go
│  │     └─ integrity.go
│  │
│  ├─ jobs\
│  │  ├─ scheduler.go
│  │  ├─ lease.go
│  │  ├─ retry.go
│  │  └─ dead_letter.go
│  │
│  ├─ memory\
│  │  ├─ extractor.go
│  │  ├─ observation.go
│  │  ├─ summary.go
│  │  ├─ consolidation.go
│  │  └─ context.go
│  │
│  ├─ search\
│  │  ├─ fts.go
│  │  ├─ ranking.go
│  │  ├─ vector.go
│  │  └─ timeline.go
│  │
│  ├─ provider\
│  │  ├─ generator.go
│  │  ├─ embedder.go
│  │  ├─ deterministic\
│  │  ├─ anthropic\
│  │  └─ openai\
│  │
│  ├─ privacy\
│  │  ├─ redactor.go
│  │  ├─ path_policy.go
│  │  └─ payload_limiter.go
│  │
│  ├─ migration\
│  │  └─ claudemem\
│  │
│  ├─ spool\
│  ├─ diagnostics\
│  ├─ config\
│  └─ version\
│
├─ plugins\
│  ├─ claude-code\
│  └─ codex\
│
├─ tests\
│  ├─ contracts\
│  ├─ fixtures\
│  ├─ differential\
│  ├─ migration\
│  ├─ concurrency\
│  ├─ chaos\
│  ├─ windows\
│  ├─ performance\
│  └─ soak\
│
├─ docs\
│  ├─ adr\
│  ├─ upstream-reference.md
│  ├─ provenance.md
│  └─ protocol.md
│
├─ references\
│  └─ claude-mem.lock.json
│
├─ go.mod
├─ LICENSE
└─ THIRD_PARTY_NOTICES.md
```

---

## 26. Go dependency 원칙

### 26.1 최소 dependency

권장 핵심:

- Go standard library
- `modernc.org/sqlite` 계열의 CGO-free SQLite driver
- Microsoft `go-winio` 계열 Named Pipe 지원
- 공식 또는 검증된 MCP Go SDK
- 최소한의 UUID/ULID package
- structured logging package 또는 `log/slog`

### 26.2 금지 또는 엄격 검토

- CGO를 필수로 만드는 dependency
- Python runtime
- Node/Bun runtime
- shell command에 의존하는 Windows 기능
- global npm install
- 외부 vector database
- process supervisor framework
- hidden credential scraping
- build 시 reference repo를 읽는 code generation
- package init에서 goroutine을 시작하는 dependency

### 26.3 Build 목표

```text
GOOS=windows
GOARCH=amd64
CGO_ENABLED=0

GOOS=windows
GOARCH=arm64
CGO_ENABLED=0
```

단일 signed executable을 기본 배포물로 한다.

---

## 27. 설치와 업데이트

### 27.1 고정 경로

```text
%LOCALAPPDATA%\Programs\Engramux\engramux.exe
```

Hook config는 plugin cache를 검색하지 않고 이 고정 경로를 참조한다.

### 27.2 설치 순서

1. binary 서명 검증
2. fixed path에 stage
3. data/config directory 생성
4. ACL 설정
5. MCP token 생성
6. SQLite 초기화
7. Scheduled Task 등록
8. service 시작
9. pipe handshake
10. Claude plugin 설정
11. Codex plugin 설정
12. MCP 연결 설정
13. contract smoke test
14. doctor 결과 출력

### 27.3 업데이트

Windows에서는 실행 중 binary를 직접 덮어쓰지 않는다.

```text
download/stage new binary
  ↓
verify signature/hash
  ↓
request service drain
  ↓
stop scheduled worker
  ↓
atomic replace
  ↓
start
  ↓
pipe boot_id/version verify
  ↓
plugin contract verify
```

실패하면 이전 binary로 rollback한다.

Hook 경로는 고정되어 있으므로 version directory mtime나 cache scan이 없다.

### 27.4 제거

기본 uninstall은 memory DB를 보존한다.

```text
engramux service uninstall
engramux plugin uninstall claude-code
engramux plugin uninstall codex
```

데이터 삭제는 별도 explicit option:

```text
engramux uninstall --purge-data --confirm
```

---

## 28. 구현 단계와 Gate

기간 추정보다 **완료 조건**으로 진행한다. 이전 단계 gate가 green이 아니면 다음 단계로 넘어가지 않는다.

### Phase 0 — Scope와 ADR Freeze

작업:

- 이 문서 승인
- upstream commit pin
- 지원 surface 명시
- non-goal 고정
- LICENSE/provenance 정책
- ADR-001~013 작성

Exit gate:

- Claude Code/Codex 이외 host 코드가 없음
- fork/upstream remote 없음
- 서비스 1개 원칙이 ADR로 고정
- “relay는 worker가 아니다”가 명시됨

### Phase 1 — Upstream Behavior Extraction

작업:

- claude-mem lifecycle 추출
- DB schema와 migration fixture 수집
- MCP search behavior 수집
- context injection fixture 수집
- known Windows/Codex defect catalog 작성
- input/output golden fixture 생성

Exit gate:

- 구현 없이 contract test가 red 상태로 존재
- 모든 원본 behavior에 “유지/개선/폐기” 판정
- reference commit 외의 floating source 사용 없음

### Phase 2 — Go Service Skeleton

작업:

- `engramux.exe`
- config/data path
- Named Mutex
- Named Pipe server
- owner record
- graceful shutdown
- Scheduled Task install/start/stop
- structured logging
- `status`, `doctor` 초기 버전

Exit gate:

- 동시 30회 start에서 persistent service 정확히 1개
- service kill 후 scheduler recovery
- visible console window 0
- stale owner record가 duplicate worker를 만들지 않음

### Phase 3 — Relay와 Host Adapter

작업:

- length-prefixed framing
- Claude parser/formatter
- Codex parser/formatter
- fail-open policy
- spool
- host fixture test
- plugin config generator

Exit gate:

- warm relay P95 목표 충족
- Claude/Codex fixture 전부 green
- Codex output에 `suppressOutput` 없음
- 서비스 중지 상태에서도 prompt/tool/Stop이 차단되지 않음
- stdout 중복 JSON 0

### Phase 4 — Durable Event Journal

작업:

- SQLite/WAL
- migration framework
- projects/sessions/events/jobs
- transaction ACK
- idempotency
- per-session ordering
- crash injection

Exit gate:

- commit 후 ACK 전 crash에서 duplicate 없음
- 100 concurrent logical sessions
- 같은 session ordering 위반 0
- ACK된 event 유실 0
- DB integrity green

### Phase 5 — Claude-mem Compatibility와 Import

작업:

- source DB reader
- sessions/prompts/observations/summaries mapping
- dry-run
- import provenance
- FTS rebuild
- row count verification

Exit gate:

- source DB hash 불변
- historical fixture migration green
- 반복 import 중복 0
- imported search sample 일치
- source DB write syscall 0 검증

### Phase 6 — Memory Processing

작업:

- deterministic extractor
- noise filter
- observation job
- summary job
- consolidation
- provider interfaces
- Anthropic/OpenAI adapter
- credential boundary
- dead letter

Exit gate:

- provider off에서도 capture/search 가능
- provider timeout이 hook latency에 영향 없음
- retry storm 없음
- 같은 source event 중복 observation 없음
- secret fixture가 provider request와 DB에 남지 않음

### Phase 7 — FTS5, Context, MCP

작업:

- FTS schema/trigger
- ranking
- context budget
- MCP Streamable HTTP
- token auth
- 네 개 tool
- Claude/Codex MCP config

Exit gate:

- embedding 없이 모든 MCP search 동작
- 100k memory 기준 search target 검증
- context byte cap 위반 0
- remote interface bind 0
- invalid token 거부
- read-only annotation green

### Phase 8 — Plugin Packaging

작업:

- Claude plugin
- Codex plugin
- fixed binary path
- install/uninstall/verify
- upgrade rollback
- clean machine package test

Exit gate:

- Node/Bun/Python 없는 clean Windows에서 설치
- plugin cache/version path 탐색 없음
- Claude smoke green
- Codex smoke green
- uninstall 후 host config 원복

### Phase 9 — Windows Hardening

작업:

- logon/logoff
- hibernate/resume
- crash/restart
- disk full/read-only
- antivirus delay
- path with spaces/Korean username
- locked DB
- update during active sessions
- Task Scheduler corruption
- pipe ACL validation

Exit gate:

- ghost hook TCP port 문제 구조상 없음
- orphan provider child 0
- console flash 0
- 72-hour soak에서 process count 고정
- restart 후 spool 자동 회수
- host 차단 0

### Phase 10 — Performance와 Release Freeze

작업:

- benchmark
- memory profile
- DB growth
- WAL checkpoint
- retention/export
- release signing
- SBOM
- final documentation

Exit gate:

- 아래 SLO/acceptance criterion 전부 통과
- blocker/open critical defect 0
- supported matrix 실측 완료
- installer rollback 검증
- `doctor` 전체 green

---

## 29. Test 전략

### 29.1 Unit

- host parser
- host formatter
- framing
- idempotency
- project identity
- redaction
- ranking
- context budgeting
- retry classification
- config atomic write

### 29.2 Contract

- Claude Code actual payload fixture
- Codex actual payload fixture
- built plugin config validation
- unsupported field detection
- stdout/exit semantics
- `commandWindows` quoting

### 29.3 Differential

원본과 완전히 같은 내부 구현을 요구하지 않는다. 사용자 관점 behavior를 비교한다.

- prompt 저장
- observation 의미
- summary field
- search result relevance
- timeline order
- context injection 범위
- migration row mapping

### 29.4 Windows lifecycle

| 시나리오 | 기대 |
|---|---|
| 서비스 강제 종료 | scheduler restart, spool import |
| 30개 hook 동시 시작 | service 1개 |
| sleep/hibernate 후 resume | pipe handshake 복구 |
| 사용자 logoff | 정상 drain 또는 bounded shutdown |
| update 중 active session | event 보존, rollback 가능 |
| path에 공백/한글 | hook 실행 성공 |
| antivirus로 binary open 지연 | bounded timeout, host fail-open |
| DB file lock | bounded retry, host 정상 |
| disk full | 명확한 degraded 상태, host 정상 |
| owner.json 삭제 | 서비스 계속 정상 |
| owner.json stale PID | duplicate 서비스 없음 |
| MCP port 충돌 | repair가 새 port/config 원자 적용 |
| malformed stdin | panic 없음, host 정상 |

### 29.5 Chaos

- transaction fault injection
- random service kill
- provider 429/401/500
- MCP client disconnect
- pipe partial frame
- spool corruption
- FTS trigger failure
- vector rebuild failure
- clock jump
- duplicate/out-of-order event

### 29.6 Migration

- 빈 DB
- current claude-mem DB
- 오래된 legacy DB
- 매우 큰 DB
- 깨진 optional table
- partially migrated fixture
- duplicate import
- non-ASCII content

### 29.7 Soak

- 다수 Claude/Codex session
- 반복 tool events
- provider on/off
- service restart
- hibernate cycle
- FTS query load
- DB growth와 WAL checkpoint
- handle/goroutine/RSS 누수

---

## 30. 목표 SLO와 Release Acceptance

아래 수치는 구현 완료 후 실측해야 하는 **목표**다.

| 항목 | 목표 |
|---|---:|
| Persistent service process | 정확히 1 |
| Mandatory child runtime | 0 |
| Warm capture relay P95 | ≤ 100 ms |
| Capture hard budget | ≤ 250 ms |
| SessionStart context P95 | ≤ 500 ms |
| SQLite durable ingest P95 | ≤ 25 ms |
| FTS search P95, 100k items | ≤ 150 ms |
| Service cold start | ≤ 1 s |
| Idle RSS | ≤ 100 MB |
| Concurrent logical sessions | ≥ 100 |
| ACK된 event 유실 | 0 |
| Duplicate derived row | 0 |
| Host-blocking memory failure | 0 |
| Windows console flash | 0 |
| Default child process | 0 |
| Pipe/MCP remote exposure | 0 |
| Secret fixture persistence | 0 |
| 72h soak process growth | 0 |

Release blocker:

- event loss
- host block
- duplicate service
- unsupported host output
- source DB mutation
- credential leakage
- non-loopback MCP bind
- migration corruption
- Windows console window 반복
- dependency로 Node/Bun/Python 요구

---

## 31. 주요 위험과 대응

### R-01. Host schema 변경

대응:

- host별 fixture
- tolerant parser
- strict formatter
- plugin version compatibility table
- CI에서 built artifact 검증

### R-02. Relay process overhead

대응:

- shell 없이 Go binary 직접 실행
- config/path discovery 제거
- binary warm startup benchmark
- capture payload 최소화
- 필요 시 Claude의 지원 event만 direct HTTP optimization 검토

단, v1에서는 ingress 경로를 둘로 나눠 복잡성을 높이지 않는다.

### R-03. Provider 비용과 credential

대응:

- provider 명시 설정
- deterministic default/fallback
- host auth file 미접근
- usage metrics
- concurrency cap
- model hard-code 금지

### R-04. SQLite 단일 writer

대응:

- ingress transaction 최소화
- WAL
- single write coordinator 또는 짧은 transaction
- durable jobs
- busy timeout
- checkpoint 정책
- benchmark 기반 batch

### R-05. Vector dependency 재도입

대응:

- FTS mandatory
- vector interface 분리
- 1.0에서 bounded candidate cosine
- extension은 후속 검증
- vector 실패가 release 전체를 깨지 않음

### R-06. Service autostart 실패

대응:

- Task Scheduler verification
- relay fail-open + spool
- `doctor`와 `repair`
- installer가 start/handshake까지 확인
- 향후 optional SCM mode

### R-07. 원본과 기능 차이

대응:

- 기능 단위 compatibility matrix
- differential fixture
- “동일 내부 구조”가 아니라 “동일 사용자 가치” 기준
- 폐기한 기능을 문서화

### R-08. 범위 팽창

대응:

- Claude Code/Codex 외 host PR 거부
- Viewer/cloud sync/Obsidian은 별도 roadmap
- phase gate 전에 신규 feature 금지
- ADR 변경 없이 architecture 확장 금지

---

## 32. ADR 목록

```text
ADR-001  Engramux는 fork가 아닌 reference-driven greenfield reimplementation이다.
ADR-002  지원 host는 Claude Code와 Codex로 제한한다.
ADR-003  사용자당 단 하나의 persistent service worker를 사용한다.
ADR-004  Hook relay는 무상태 transport이며 worker가 아니다.
ADR-005  Hook ingress는 Windows Named Pipe를 사용한다.
ADR-006  MCP는 loopback Streamable HTTP를 사용한다.
ADR-007  Append-only event journal을 source of truth로 사용한다.
ADR-008  Claude와 Codex adapter/formatter를 분리한다.
ADR-009  모든 memory availability failure는 fail-open이다.
ADR-010  FTS5는 필수, vector는 선택 기능이다.
ADR-011  Host와 AI processor provider를 분리한다.
ADR-012  claude-mem migration은 one-way이며 source는 read-only다.
ADR-013  사용자 단위 Scheduled Task service를 기본으로 한다.
ADR-014  전역 지침, skill, CLAUDE.md, AGENTS.md에 의존하지 않는다.
ADR-015  고정 설치 경로 하나만 host config에서 참조한다.
ADR-016  Web Viewer는 1.0 release blocker가 아니다.
ADR-017  PreToolUse Read context injection은 기본 비활성화한다.
ADR-018  MCP destructive tools는 1.0에서 제외한다.
```

각 ADR에는 Context, Decision, Alternatives, Consequences, Validation을 작성한다.

---

## 33. 바로 시작할 작업 순서

1. `D:\AI_DEV\references\claude-mem`에 기준 commit clone/checkout
2. reference directory가 Engramux build에 포함되지 않도록 경계 설정
3. `D:\AI_DEV\projects\engramux` 빈 Git repository 생성
4. `claude-mem.lock.json` 작성
5. 이 문서와 ADR-001~018 commit
6. Apache-2.0 provenance 정책과 `THIRD_PARTY_NOTICES.md` 초안 작성
7. Claude Code 실제 hook payload fixture 수집
8. Codex 실제 hook payload fixture 수집
9. `claude-mem` DB historical fixture를 비식별화해 테스트 자산화
10. upstream issue defect catalog 작성
11. `cmd/engramux`와 config path skeleton 구현
12. Windows Named Mutex 구현
13. current-user DACL Named Pipe echo server 구현
14. relay framing/timeout benchmark 작성
15. service Scheduled Task installer 구현
16. Claude/Codex parser를 별도 package로 구현
17. fail-open golden output test 작성
18. SQLite event journal과 transaction ACK 구현
19. crash/idempotency test 작성
20. session router와 ordering test 구현
21. local spool 구현
22. deterministic extractor 구현
23. claude-mem one-way importer 구현
24. FTS5와 네 개 MCP tool 구현
25. provider abstraction과 optional adapters 구현
26. packaging/upgrade/rollback 구현
27. Windows chaos/soak test
28. 실측 SLO 충족 후 1.0 freeze

가장 먼저 구현할 vertical slice는 다음이어야 한다.

```text
Claude/Codex fixture
  → relay
  → Named Pipe
  → one service
  → SQLite events commit
  → ACK
  → service restart
  → idempotent replay
```

이 slice가 완성되기 전에는 LLM, embedding, Viewer를 구현하지 않는다.

---

## 34. 최종 권고

Engramux는 “claude-mem을 Go로 다시 작성한 버전”이라는 설명보다 다음 설명이 정확하다.

> **Engramux is a Windows-first, Go-native persistent development memory service for Claude Code and Codex, using one per-user worker to multiplex every session.**

프로젝트 성공 여부는 기능 개수로 판단하지 않는다.

최종 성공 기준:

1. Claude Code와 Codex가 같은 service를 안정적으로 공유한다.
2. session 수가 증가해도 persistent process 수는 1이다.
3. memory 기능이 죽어도 host는 계속 정상 동작한다.
4. ACK된 event는 crash 후에도 남아 있다.
5. Windows에서 ghost port, shell flash, orphan runtime이 없다.
6. Node/Bun/Python/Chroma 없이 설치된다.
7. Claude와 Codex output contract가 서로 오염되지 않는다.
8. 기존 `claude-mem` memory를 원본 변경 없이 가져올 수 있다.
9. FTS search는 provider/vector와 무관하게 항상 동작한다.
10. 모든 architecture decision이 테스트 가능한 gate로 연결된다.

따라서 최종 진행 방향은 다음으로 고정하는 것이 가장 적합하다.

```text
Reference analysis
  → Contract extraction
  → One-service Go foundation
  → Durable event journal
  → Claude/Codex adapters
  → Migration compatibility
  → Memory processing
  → FTS/MCP
  → Windows hardening
  → Performance/soak
  → Freeze
```

---

## 35. 기준 자료

### Upstream repository

- `thedotmack/claude-mem`
- 기준 commit: `e2d1df569a8f04075d40e92461128ece7cf04c82`
- Architecture overview:
  - `docs/public/architecture/overview.mdx`
- Worker:
  - `docs/public/architecture/worker-service.mdx`
- Hooks:
  - `docs/public/architecture/hooks.mdx`
- Database:
  - `docs/public/architecture/database.mdx`
- License:
  - `LICENSE`

### 주요 upstream issue traceability

- #3692 — Windows inherited listening socket / dead worker port
- #3603 — worker port and liveness authority
- #3602 — child process ownership
- #3605 — thin fail-open hook wrapper
- #3611 — host integration contracts
- #2975, #2844, #2765 — Codex `suppressOutput`
- #3568 — PreToolUse Read blocked by worker failure
- #2966 — Stop hook retry/block loop
- #3303 — synchronous hook timeout
- #3128, #2963 — Chroma child and ghost listener
- #3280 — concatenated Claude hook JSON
- #3324 — subagent file context pollution

### Official host references

- Claude Code Hooks:
  - `https://code.claude.com/docs/en/hooks`
- Claude Code Plugins:
  - `https://code.claude.com/docs/en/plugins`
- Codex Hooks:
  - `https://developers.openai.com/codex/hooks`
- Codex MCP:
  - `https://developers.openai.com/codex/mcp`
- Codex Plugins:
  - `https://developers.openai.com/codex/plugins`

### Go/Windows references

- `modernc.org/sqlite`
- Microsoft `go-winio`
- Microsoft Named Pipe security and access rights
- Microsoft Windows Job Objects
- `golang.org/x/sys/windows/svc`

---

**End of document**
