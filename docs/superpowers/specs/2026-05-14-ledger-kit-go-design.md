# ledger-kit (Go) — 설계 명세

- **모듈**: `github.com/hgwk/ledger-kit`
- **언어**: Go 1.22+, 표준 라이브러리만 (외부 의존성 0개 목표)
- **작성일**: 2026-05-14
- **상태**: 초안

## 1. 정체성

LLM 에이전트(주 writer)와 사람(주 reader)이 공유하는, **append-only 프로젝트 레저**. 한 대의 머신에 등록된 여러 프로젝트를 단일 페이지에서 본다.

- 타겟 레포에는 Node/Python 등 어떤 런타임 흔적도 남기지 않는다.
- 모든 쓰기와 검증은 단일 Go 바이너리(`ledger-kit`)를 통한다.
- 데이터는 레포 안 JSONL/JSON 파일에 저장되어 git이 자동으로 버전·동기화를 담당한다.

## 2. 배포

- 1차: GitHub Releases prebuilt binary (`darwin-arm64`, `darwin-amd64`, `linux-amd64`, `linux-arm64`) + `go install github.com/hgwk/ledger-kit@latest`
- 2차: Homebrew tap (후순위)
- 빌드: 표준 `go build`, `GOOS/GOARCH` cross-compile. CI는 태그 푸시 시 매트릭스 빌드 후 Release에 첨부.

## 3. 데이터 모델

### 3.0 Core 원칙

레저의 코어는 **3종 데이터 + 1종 운영 메타데이터**로 한정한다. 추가 파일은 만들지 않는다.

| 파일 | 역할 | 종류 |
|---|---|---|
| `goal.json` | 현재 프로젝트 목표 (덮어쓰기) | 데이터 |
| `tickets.jsonl` | 해야 할 일·상태 전이·정정 (append-only) | 데이터 |
| `worklog.jsonl` | 완료된 원자 단위 작업 (append-only) | 데이터 |
| `config.json` | 식별자·표시명·parent 규칙·branch convention·schema version | 운영 메타 |

**자주 제안되지만 별도 파일로 분리하지 않는 것들**:

| 후보 | 어디에 들어가는가 |
|---|---|
| decision log | `ticket.decision` / `worklog.notes` |
| blocked dependency | `ticket.blocked_by` |
| collaboration state | `ticket.lead` / `ticket.collab` / `ticket.claimed_by` / `ticket.claim_until` / `ticket.handoff_to` / `ticket.handoff` |
| work type classification | `ticket.category` |
| branch mapping | `ticket.branch` / `worklog.branch` + `config.branch_convention` |
| goal history | git history (필요시 worklog 한 행 자동 추가, §3.2 `log_goal_changes`) |
| comments / discussion | 도입하지 않음 (노이즈) |
| artifacts | `ticket.paths` / `worklog.paths` |
| acceptance / evidence | `ticket.acceptance` / `worklog.evidence` |
| 스키마 정의 | 별도 `schema.json` 두지 않음. 검증은 바이너리의 `verify`가 담당 |
| 프로젝트 레지스트리 | 레포 밖 `~/.ledger-kit/registry.json` |

이 원칙을 어기는 변경은 spec 개정을 통해서만 도입한다.

**운영 부산물(Operational artifacts)은 코어 데이터가 아니다.** 명령 실행 과정에서 생성되는 다음 파일들은 ledger 데이터 모델에 속하지 않으며 기본적으로 git-ignore 된다:

- `ledger/.lock` (쓰기 직렬화)
- `ledger/.backup/` (legacy import 백업)
- `ledger/import-errors.jsonl` (파싱 실패 행 보존)
- `ledger/legacy/` (`--archive-originals` 옵션 사용 시)

`init`/`import legacy --apply`는 `.gitignore`에 위 항목을 멱등하게 추가한다.

### 3.1 디렉터리 (타겟 레포)

```
<repo>/
  ledger/
    config.json         # project_id, slug, name, schema_version, parents, branch convention
    goal.json           # 현재 goal 스냅샷 (덮어쓰기, 이전 버전은 git history)
    tickets.jsonl       # append-only
    worklog.jsonl       # append-only
    .lock               # 쓰기 직전 임시 생성, 끝나면 삭제 (3.6 참조)
    instructions/
      AGENTS.ledger-kit.md
      CLAUDE.ledger-kit.md
  AGENTS.md             # LEDGER_KIT 블록 prepend (옵션)
  CLAUDE.md             # LEDGER_KIT 블록 prepend (옵션)
```

### 3.2 `ledger/config.json`

```json
{
  "schema_version": 1,
  "project_id": "9f8a7c6b5d4e3f2a1b0c9d8e7f6a5b4c",
  "slug": "myapp",
  "name": "My App",
  "parents": ["ROOT", "DOC", "FE", "BE", "OPS", "DEMO", "BUG", "LEGACY"],
  "branch_convention": "work/{ticket}",
  "log_goal_changes": false
}
```

- `project_id`: **128-bit random lowercase hex** (32자). `crypto/rand`로 생성. registry merge·머신 간 동일 프로젝트 인식의 유일한 근거. 절대 변경 안 함. (ULID는 외부 의존성 또는 직접 구현이 필요해 채택하지 않는다. 정렬성은 여기서 필요 없다.)
- `slug`: 표시·경로용 짧은 이름. 변경 가능.
- `name`: 사람용 표시명.
- **UI/로그 표시 규약**: `<slug>-<project_id 앞 6자>`. 예: `myapp-9f8a7c`.
- `schema_version`: 전역 스키마 버전. 각 row에는 넣지 않는다(3.5 참조).
- `log_goal_changes`: `true`면 `goal set`이 자동으로 worklog 한 행 append.

### 3.3 `ledger/goal.json`

```json
{
  "schema_version": 1,
  "track": "project",
  "version": "0.1.0",
  "updated": "2026-05-14T00:00:00Z",
  "source_of_truth": "README.md",
  "summary": "...",
  "success_criteria": ["..."]
}
```

- **덮어쓰기**가 기본. 이전 goal은 git log/blame으로 추적.
- 운영 프로젝트에서 변경 audit이 필요하면 `log_goal_changes: true`로 worklog 자동 행 보강.
- `goal set --log` 또는 `log_goal_changes: true`가 만드는 자동 worklog는 ticket 없는 행으로 기록한다. 기본 형태:
  ```json
  {
    "agent": "codex",
    "task": "goal set",
    "scope": "ledger",
    "result": "Updated project goal snapshot.",
    "paths": ["ledger/goal.json"],
    "commands": ["ledger-kit goal set --json @- --log"],
    "notes": ""
  }
  ```

### 3.4 `tickets.jsonl` 행

```json
{
  "n": 12,
  "ts": "2026-05-14T10:00:00Z",
  "parent_ticket": "ROOT",
  "ticket": "example-ticket",
  "agent": "codex",
  "role": "impl",
  "category": "feature",
  "status": "open",
  "task": "Implement example",
  "scope": "repo",
  "paths": ["src/example.ts"],
  "acceptance": ["go test ./...", "ledger-kit verify"],
  "blocked_by": [],
  "decision": "",
  "notes": "",
  "lead": "codex",
  "collab": ["claude"],
  "claimed_by": "codex",
  "claim_until": "2026-05-14T10:30:00Z",
  "handoff_to": "",
  "handoff": "",
  "branch": "work/example-ticket"
}
```

- `status` enum: `open`, `in_progress`, `blocked`, `audit_ready`, `changes_requested`, `done`, `cancelled`.
- `category`: optional work-type classifier. `parent_ticket`과 독립적인 축이다.
- `acceptance`: 완료 판단 기준. 검증 명령, 산출물, 스크린샷/trace 필요 조건 등을 짧은 문자열 배열로 둔다.
- `ticket`과 `task`는 non-empty string이어야 한다. 빈 문자열은 tree에서 `—` ghost ticket을 만들기 때문에 append 오류다.
- 동일 `ticket`에 여러 행이 생길 수 있음. **가장 큰 `n` 행이 현재 상태**.
- 정정(decision 번복, 필드 보정)은 같은 ticket에 새 행 append. 절대 기존 행 수정/삭제 안 함.

#### 3.4.0 Delivery lifecycle and audit gate

ledger-kit의 기본 delivery 흐름은 **Plan → Develop → Audit → Complete**다. 감리는 새 원장을 만들지 않고 `tickets.jsonl`의 상태 전이 row로 기록한다. `worklog.jsonl`은 여전히 완료 산출물 전용이다.

상태 전이:

```
open
→ in_progress
→ audit_ready
→ changes_requested
→ in_progress
→ audit_ready
→ done
```

규칙:

- `done` row는 구현 완료 주장이 아니라 audit 통과 결정이다.
- 구현자가 작업을 끝냈다고 판단하면 `status=audit_ready` row를 append한다. 이 row에는 `acceptance` 충족 증거를 `evidence` 또는 `notes`에 남긴다.
- 감리자는 `role=audit`인 `ticket event`를 append한다.
- 감리 통과 시 최신 row는 `status=done`, `audit_result=pass`가 된다.
- 감리 실패 시 최신 row는 `status=changes_requested`, `audit_result=changes_requested`가 된다. 이후 구현자가 다시 `in_progress` 또는 `audit_ready`로 전이한다.
- 단일 에이전트 작업도 최소 self-audit row를 남긴 뒤 `done`으로 닫는다.
- `worklog add`는 `done` audit row 이후에만 수행한다. 즉 worklog는 shipped/completed artifact 기록이지 "개발 완료 주장"이 아니다.

Audit optional fields:

| 필드 | 의미 |
|---|---|
| `audit_type` | `plan`, `code`, `qa`, `release`, `security` 중 하나. |
| `audit_result` | `pass` 또는 `changes_requested`. |
| `audit_notes` | acceptance, diff 범위, regression risk, 남은 보완 요청. |
| `evidence` | 감리자가 확인한 명령/로그/스크린샷/리포트 경로. |

예:

```json
{
  "ticket": "example-ticket",
  "role": "audit",
  "status": "changes_requested",
  "audit_type": "code",
  "audit_result": "changes_requested",
  "audit_notes": "dry-run path mutates files; --plan must be read-only",
  "evidence": ["go test ./...", "ledger-kit import legacy --plan"]
}
```

#### 3.4.1 Hierarchy vs dependency

- `parent_ticket`은 hierarchy/classification edge다. 티켓을 어느 작업 묶음에 넣을지 결정한다.
- `category`는 work-type classifier다. 작업 성격을 나타내며 `parent_ticket`을 대체하지 않는다.
- `blocked_by`는 dependency edge다. 이 티켓을 진행하기 전에 풀려야 하는 티켓 id 목록이다.
- 두 축은 독립적이다. blocker는 다른 parent에 속할 수 있다.
- `status=blocked`인 티켓은 `blocked_by`가 비어 있지 않아야 한다.
- `blocked_by`가 비어 있지 않은 티켓은 `status=open` 또는 `status=in_progress`일 수 있지만 ready queue에는 포함하지 않는다.

#### 3.4.2 Category values

`category`는 기존 데이터 호환을 위해 optional로 시작한다. 새 row writer는 가능한 한 채운다. reader는 누락 시 `parent_ticket` 또는 ticket prefix로 display-only 추론을 할 수 있지만, 추론값을 기존 row에 직접 쓰지 않는다.

권장 값:

- `feature`
- `bug`
- `docs`
- `ops`
- `design`
- `test`
- `infra`
- `research`
- `demo`
- `release`
- `cleanup`

`category`는 `role`과 다르다. 예를 들어 `category=feature`, `role=design`은 feature ticket의 contract/design row를 뜻한다.

#### 3.4.3 Collaboration fields

Codex/Claude처럼 여러 에이전트가 같은 repo에서 릴레이할 수 있도록 협업 상태는 ticket row 안에 둔다. 별도 collaboration 파일은 만들지 않는다.

| 필드 | 의미 |
|---|---|
| `lead` | ticket 최종 책임 agent. 기본적으로 한 명. |
| `collab` | 협업 agent 목록. 예: `["claude"]`. |
| `claimed_by` | 현재 이 ticket을 작업 중이라고 claim한 agent. |
| `claim_until` | claim 만료 시각(UTC ISO8601). 만료 후 다른 agent가 이어받을 수 있다. |
| `handoff_to` | 이어받을 대상 agent. 비어 있으면 특정 대상 없음. |
| `handoff` | 이어받을 요약. 파일 경로, 남은 acceptance, 막힌 결정, 검증 명령을 포함한다. |

협업 규칙:

- 코드 편집 전 에이전트는 `ticket event`로 `claimed_by`와 `claim_until`을 append해야 한다.
- 아직 만료되지 않은 다른 agent의 active claim이 있고 `paths`가 겹치면 편집하지 않는 것이 원칙이다.
- 이어받기 요청은 `handoff_to`, `handoff`, `paths`, 남은 검증 명령을 포함한 `ticket event`로 남긴다.
- claim/handoff도 append-only다. 이전 claim을 수정하지 않고 새 event로 갱신한다.

### 3.5 `worklog.jsonl` 행

`ticket`은 **optional**이다(예: `goal set --log`로 자동 생성된 행은 ticket 없이 기록). 그 외 필드는 필수.

```json
{
  "n": 7,
  "ts": "2026-05-14T10:10:00Z",
  "ticket": "example-ticket",
  "agent": "codex",
  "task": "example-ticket — implementation",
  "scope": "repo",
  "result": "Implemented example.",
  "paths": ["src/example.ts"],
  "commands": ["go test ./..."],
  "evidence": ["docs/demo/output.txt"],
  "notes": "",
  "branch": "work/example-ticket",
  "commit": ""
}
```

- 완료된 원자 단위 작업만 기록. 미완료 작업을 worklog에 쓰지 않는다.
- `evidence`: 완료를 뒷받침하는 산출물 경로 또는 짧은 증거 설명. trace, screenshot, report, captured output 등을 기록한다.
- `task`는 non-empty string이어야 한다. `ticket`이 있는 worklog라면 `ticket`도 non-empty string이어야 한다. `ticket=""`은 optional ticket이 아니라 append 오류다.
- `ticket` 없는 행은 `--strict` 모드에서 warning. 기본 검증에서는 통과.

### 3.6 동시성: `ledger/.lock` / `~/.ledger-kit/registry.lock`

단일 사용자라도 Codex / Claude / pre-commit 훅 / 사람 터미널이 동시에 append할 수 있다. `n` 무결성을 지키기 위해 **모든 쓰기 경로**는 다음 락 프로토콜을 따른다. 레지스트리(`~/.ledger-kit/registry.json`)도 동일 프로토콜로 `~/.ledger-kit/registry.lock`을 사용한다 — `init`/`list --prune`/`registry repair` 동시 실행 충돌 방지.

```
lock_path = <repo>/ledger/.lock
try:
  open(lock_path, O_CREATE|O_EXCL|O_WRONLY, 0644)        # 획득
catch EEXIST:
  if mtime(lock_path) < now - 30s:                        # stale 판정
    remove(lock_path)
    retry
  else:
    sleep 50ms, retry  (총 5초까지)
    if still fail: exit non-zero with "ledger busy"
write PID, hostname, ts to lock file
do the append (read last n, write new line with n+1)
remove(lock_path)
```

- OS advisory lock(`flock` 등)은 macOS/Linux 외 호환성 이슈가 있어 채택하지 않는다.
- `O_EXCL` + stale timeout으로 의존성 0개·이식성 확보.
- `ledger-kit doctor`(후순위)에서 stale lock 진단·정리 옵션 제공.

## 4. CLI

### 4.1 셋업·관리 (사람)

```
ledger-kit init [--target PATH] [--name NAME] [--slug SLUG]
                [--instructions agents,claude | --no-instructions]
ledger-kit view [--port 3030]
ledger-kit list [--prune]
ledger-kit unregister (--project-id ID | --path PATH)
ledger-kit registry repair
ledger-kit hooks install
ledger-kit hooks uninstall
ledger-kit instructions install [--instructions agents,claude]
ledger-kit instructions uninstall
ledger-kit verify [--target PATH] [--strict]
ledger-kit doctor [--target PATH] [--json]
ledger-kit insights [--target PATH] [--json]
ledger-kit next --ticket ID [--target PATH] [--format text|json]
ledger-kit suggest worklog --ticket ID [--target PATH]
ledger-kit suggest commit --ticket ID [--target PATH]
```

### 4.2 에이전트 쓰기 경로

```
ledger-kit ticket add   --json @-      # 새 ticket 첫 행 (status=open|in_progress)
ledger-kit ticket event --json @-      # 상태 전이·정정·노트 추가용 새 행
ledger-kit worklog add  --json @-
ledger-kit goal set     --json @- [--log]
```

- `add`/`event` 명명으로 **append-only 의미를 단어 자체에 박는다**. `update`라는 어휘는 LLM이 "파일 수정"으로 오인할 수 있어 의도적으로 회피.
- 입력은 3가지 형태: `--json '<inline>'`, `--json @-`(stdin), `--json @<path>`(파일). 플래그 기반 빌더는 만들지 않는다 — LLM이 JSON 한 덩어리를 쓰는 것이 가장 안정적이다.
- 출력은 부여된 `n`, `ts` 등을 포함한 정규화된 행 전체를 stdout JSON으로 반환.

#### 4.2.1 `ticket add` 의미

- 새 ticket 생성. 동일 `ticket` id가 이미 존재하면 fail.
- 입력 JSON에 ticket required 필드가 모두 있어야 함(§3.4).

#### 4.2.2 `ticket event` 의미 (carry-forward)

- 기존 ticket의 **최신 row를 base로 로드**한 뒤, 입력 JSON을 **오버레이**해서 정규화된 새 행을 append.
- 따라서 에이전트는 변경할 필드만 보내면 된다. 예: `{"ticket":"x","status":"done","notes":"shipped"}`.
- `ticket` 필드는 필수. 그 외 required 필드는 base row에서 상속.
- 기존 ticket이 없으면 fail (새 ticket은 `ticket add`만 가능).
- 오버레이는 shallow replace다. 입력 JSON에 들어온 필드는 base 필드를 통째로 대체한다. `paths`, `blocked_by` 같은 배열도 merge하지 않고 replace한다.

#### 4.2.3 `worklog add` 의미

- 항상 새 행 append. carry-forward 없음.
- `ticket`은 optional (§3.5).

### 4.2.4 Contextual guidance on writes

`ticket add`, `ticket event`, `worklog add`는 기본 stdout에 정규화된 row JSON을 출력한다. LLM이 다음 행동을 놓치지 않도록, 상태 전이별 guidance는 stderr에 짧게 출력한다. `--json`/automation 소비자는 stdout JSON만 파싱하면 된다.

Guidance는 정적 reminder가 아니라 **최신 ticket state machine**에서 계산한다.

예:

- `ticket add status=open`
  - acceptance를 채웠는지 확인
  - broad change면 plan row를 먼저 `audit_ready`로 올릴 것
  - archive/reference 검토 결과를 `notes`에 남길 것
- `ticket event status=in_progress`
  - `claimed_by`, `claim_until`, `paths`를 확인
  - 같은 paths에 active claim이 있으면 멈출 것
- `ticket event status=audit_ready`
  - 아직 worklog를 쓰지 말 것
  - auditor가 `changes_requested` 또는 `done` audit row를 append해야 함
  - evidence/commands/paths를 남길 것
- `ticket event status=changes_requested`
  - worklog를 쓰지 말 것
  - 다음 구현 row는 `in_progress`로 재개하고 `audit_notes`를 반영할 것
- `ticket event status=done`
  - `role=audit`, `audit_result=pass`, `evidence`가 있는지 확인
  - 이제 worklog append 가능
  - `suggest worklog` / `suggest commit`을 안내
- `worklog add`
  - 연결된 ticket의 최신 상태가 audit-pass `done`인지 확인
  - 아니면 warning guidance를 stderr에 출력

### 4.3 조회

```
ledger-kit tickets [--status open] [--parent ROOT] [--ticket ID]
ledger-kit worklog [--ticket ID] [--since TS]
ledger-kit tree [--write]              # docs/delivery/TICKETS.md 생성/출력
ledger-kit goal show
ledger-kit next --ticket ID [--format text|json]
ledger-kit suggest worklog --ticket ID
ledger-kit suggest commit --ticket ID
```

#### 4.3.1 `next`

`next`는 최신 ticket row를 읽고 지금 해야 할 다음 행동을 출력한다. LLM/사람이 ticket 상태를 확인한 직후 실행하는 명령이다.

출력 예:

```text
Ticket example-ticket is audit_ready.

Next:
- Do not append worklog yet.
- Append an audit row:
  status=done, role=audit, audit_result=pass
  or status=changes_requested, role=audit, audit_result=changes_requested
- Include evidence: tests, verify command, diff review, screenshot/report if relevant.
```

`--format json`은 자동화용:

```json
{
  "ticket": "example-ticket",
  "status": "audit_ready",
  "actions": ["do_not_append_worklog", "append_audit_row"],
  "warnings": [],
  "suggested_commands": ["ledger-kit ticket event --json @-"]
}
```

#### 4.3.2 `suggest`

`suggest`는 최신 row와 연결 worklog/evidence를 보고 LLM이 바로 사용할 수 있는 skeleton을 출력한다. 파일을 쓰지 않는다.

```
ledger-kit suggest worklog --ticket ID
ledger-kit suggest commit --ticket ID
```

- `suggest worklog`: audit-pass `done` row 이후 사용할 worklog JSON skeleton 출력.
- `suggest commit`: Conventional Commit 한 줄, PR summary, verification block skeleton 출력.
- ticket이 `done` audit pass가 아니면 skeleton 대신 다음 상태 전이 guidance를 출력하고 non-zero가 아닌 warning으로 종료한다.

### 4.4 운영 진단

```
ledger-kit doctor [--target PATH] [--json]
ledger-kit insights [--target PATH] [--json]
```

`doctor`는 ledger 품질 진단 리포트다. `verify`가 커밋 차단용 fail/warn 검증이라면, `doctor`는 운영자가 다음 보정 작업을 판단할 수 있게 문제 후보를 모아 보여준다. 기본 종료코드는 0이며, JSON 출력은 자동화용이다.

`doctor`가 감지하는 항목:

- ghost row: `ticket=""` 또는 `task=""`처럼 tree에 `—` 노드를 만드는 row
- missing or unknown `category` on latest active ticket
- orphan worklog
- closed/done ticket without worklog
- commandless worklog
- stale claim
- `status=blocked`인데 `blocked_by=[]`
- obsolete blocker: 최신 상태가 `done`/`cancelled`인 ticket을 계속 `blocked_by`에 들고 있는 active ticket
- timestamp drift / correction-heavy ticket / supersede-heavy ticket
- append-only 보정이 필요한 후보. `doctor`는 직접 수정하지 않고 어떤 `ticket event`를 append해야 하는지 설명한다.

`insights`는 현재 프로젝트 운영 상태를 요약한다.

- Ready Queue: `status=open`이고 `blocked_by=[]`인 ticket
- Critical Path / Top Blockers: active ticket의 `blocked_by` 역참조 순위
- Stale In Progress: 오래된 `in_progress`, worklog/claim이 없는 진행 ticket
- Worklog Coverage: closed ticket without worklog, orphan worklog, command/evidence coverage
- Active Claims: 아직 만료되지 않은 `claimed_by`/`claim_until`
- Handoff Queue: `handoff_to`가 있는 ticket

`view`는 `insights` 계산을 사용해 같은 내용을 UI 카드로 보여준다.

## 5. 자동 필드 부여

에이전트 부담을 줄이기 위해 바이너리가 다음을 자동 채운다 (입력 JSON에 있으면 입력값 우선).

| 필드 | 출처 |
|---|---|
| `n` | `tickets.jsonl`/`worklog.jsonl`의 마지막 행 `+1` |
| `ts` | UTC ISO8601 (밀리초 절단) |
| `branch` | `git rev-parse --abbrev-ref HEAD` |
| `commit` | 빈 문자열 (worklog 한정, 향후 pre-commit 통합 시 채움) |
| `agent` | 5.1의 우선순위 |

### 5.1 `agent` 해석 우선순위

1. 입력 JSON 안의 `agent` 필드
2. `LEDGER_AGENT` 환경변수
3. 감지: `CODEX_*` 존재 → `codex`, `CLAUDECODE`/`CLAUDE_CODE_*` → `claude`, `CURSOR_*` → `cursor`
4. 그래도 없으면 `$USER` — 단 이 경우 stderr에 경고 출력
5. 위 어떤 것도 결정 불가하면 **fail**. 절대 `unknown`을 조용히 넣지 않는다.

Legacy import는 유일한 예외다. 과거 행은 보존이 우선이므로 `agent`를 결정할 수 없으면 `legacy`로 정규화하고 warning을 출력한다(§11.4).

## 6. 검증 (`verify`)

### 6.1 Fail (커밋 차단 수준)

- JSON parse 실패
- 필수 필드 누락 또는 non-empty semantic string이 빈 문자열인 경우
  - ticket row non-empty: `parent_ticket`, `ticket`, `agent`, `role`, `status`, `task`, `scope`
  - worklog row non-empty: `agent`, `task`, `scope`, `result`; `ticket` 필드가 있으면 non-empty여야 함
  - `branch`/`commit`은 필드 존재는 요구하지만 git 상태에 따라 빈 문자열일 수 있다.
- `n` 위반: **strictly consecutive from 1, no gaps, no duplicates**. (역행·중복·빈 번호 모두 fail)
- `ts` 위반: UTC ISO8601 형식이 아니거나, 같은 JSONL 파일 안에서 비감소(non-decreasing) 순서를 깨는 경우
- `status` enum 위반
- `config.json` 스키마 위반
- `goal.json` 스키마 위반

### 6.2 Warn (기본 출력하되 종료코드 0)

- orphan worklog (`ticket`이 어떤 ticket row와도 매칭 안 됨)
- closed/done ticket인데 worklog 없음
- `parent_ticket`이 `config.parents`에 없음
- `category`가 권장 값 목록에 없음
- `branch` 누락 또는 `branch_convention`과 불일치. 단, convention 불일치 warning은 target이 git worktree 안이고 branch가 비어 있지 않을 때만 출력한다.
- `commands`와 `evidence`가 모두 비어 있는 worklog
- `acceptance`가 있는 ticket이 `done`인데 연결된 worklog의 `commands`/`evidence`가 비어 있음
- 최신 ticket이 `audit_ready`인데 `evidence`와 `notes`가 모두 비어 있음
- 최신 ticket이 `done`인데 `role`이 `audit`이 아니거나 `audit_result=pass`가 없음
- 최신 ticket이 `changes_requested`인데 `audit_notes`가 비어 있음
- `status=blocked`인데 `blocked_by`가 비어 있음
- active ticket이 최신 상태가 `done`/`cancelled`인 ticket을 `blocked_by`에 계속 들고 있음
- `status=blocked`인데 남은 unresolved blocker가 없음
- `status=in_progress`인데 `claimed_by`가 없음
- `claim_until`이 현재 시각보다 과거라 stale claim으로 보임
- 서로 다른 agent의 active claim이 같은 `paths`를 포함함
- `handoff_to`가 있는데 `handoff`가 비어 있음

### 6.3 `--strict`

warn도 fail로 승격. CI나 엄격한 프로젝트에서 사용.

## 7. 멀티 프로젝트 뷰

### 7.1 레지스트리: `~/.ledger-kit/registry.json`

```json
{
  "version": 1,
  "projects": [
    {
      "project_id": "9f8a7c6b5d4e3f2a1b0c9d8e7f6a5b4c",
      "slug": "myapp",
      "name": "My App",
      "paths": ["/myapp"],
      "registered_at": "2026-05-14T00:00:00Z",
      "last_seen": "2026-05-14T10:00:00Z"
    }
  ]
}
```

- `init`이 자동 등록. 동일 `project_id`가 새 경로에서 발견되면 `paths`에 append.
- `list --prune`: 존재하지 않는 경로·읽을 수 없는 `config.json`을 가진 항목 정리.
- `unregister --path PATH`: 특정 경로만 제거 (다른 머신 체크아웃은 보존).
- `unregister --project-id ID`: 항목 전체 제거.
- `registry repair`: 손상된 registry를 백업 후 재구성.

### 7.2 뷰어: `ldgr view`

- 단일 정적 바이너리에 `embed.FS`로 HTML/CSS/JS 동봉.
- localhost:3030 (포트는 플래그로 변경 가능).
- 라우트:
  - `GET /` — 셸 HTML
  - `GET /api/projects` — registry + 각 프로젝트 요약 (goal summary, open 티켓 수, 최근 worklog ts)
  - `GET /api/projects/{project_id}/goal`
  - `GET /api/projects/{project_id}/tickets`
  - `GET /api/projects/{project_id}/worklog`
- UI:
  - 좌측 사이드바: 프로젝트 목록 (slug + 요약) + 선택 프로젝트 내 page navigation.
  - Dashboard page: control-tower overview. Goal summary, Progress, Parent ticket completion, active claims, recent audit/worklog activity.
  - Kanban page: ticket lifecycle columns. 기본 컬럼은 `Plan`, `Implement`, `Verify`, `Complete`이며 `tickets.jsonl` status/role에서 계산한다.
  - Tree page: `parent_ticket → tickets → worklogs` drill-down.
  - Worklog page: completed delivery timeline.
  - Insights page: ready queue, blockers, stale work, invalidated rows, orphan/coverage warnings.
  - 갱신: 클라이언트 폴링 5초 간격
- 의존성: 프론트엔드는 vanilla JS, 빌드 없음

#### 7.2.1 Control-tower dashboard metrics

Viewer는 단순 파일 브라우저가 아니라 운영 현황을 빠르게 읽는 control-tower여야 한다. Dashboard page의 필수 계산:

- `Progress`: latest ticket 기준 전체 완료율. `done` / (`open` + `in_progress` + `blocked` + `audit_ready` + `changes_requested` + `done`)로 계산하고 `cancelled`는 분모에서 제외한다.
- `Parent ticket completion`: parent별 완료율, open/active/done/cancelled count, blocked count.
- `Audit pipeline`: `audit_ready`, `changes_requested`, weak `done`(audit pass 없는 done) count.
- `Delivery health`: closed without worklog, orphan worklog, invalidated rows, command/evidence coverage.
- `Recent activity`: 최신 ticket events + worklog rows 혼합 timeline.

Kanban mapping:

| Column | Included latest ticket rows |
|---|---|
| `Plan` | `open` rows, plus `role=plan` rows that are not done/cancelled |
| `Implement` | `in_progress`, `blocked`, `changes_requested` |
| `Verify` | `audit_ready`, `role=audit` rows that are not done/cancelled |
| `Complete` | `done`, `cancelled` |

Kanban cards show ticket id, parent, category, task, blocked_by, owner/claim, age, branch, and evidence/audit badges. Clicking a card opens the ticket detail drawer with latest row, history, linked worklogs, paths, and `ldgr next` guidance.

## 8. Hook 통합 (idempotent + 보존)

### 8.1 `hooks install`

- `.git/hooks/pre-commit` 처리 방식:
  - 파일 없음 → shebang + marker 블록을 가진 새 스크립트 생성, `chmod +x`
  - 파일 있음 → **shebang 다음 라인에 marker 블록 insert**. 기존 내용은 marker 뒤에 보존. (마지막 라인에 넣으면 기존 스크립트의 `exit 0`가 우리를 우회할 수 있어 의도적으로 상단 삽입.)
  - shebang 없는 기존 파일 → 새 shebang(`#!/usr/bin/env bash`)을 prepend한 뒤 marker 삽입.
  - marker 블록 이미 존재 → no-op (idempotent)
- marker:
  ```
  # >>> LEDGER_KIT_HOOK_START >>>
  ledger-kit verify || exit 1
  # <<< LEDGER_KIT_HOOK_END <<<
  ```
- 안전 백업: `.git/hooks/pre-commit.ledger-kit.bak` (uninstall에서 사용).

### 8.2 `hooks uninstall`

- marker 블록만 제거. 파일이 우리가 만든 것뿐이면 통째로 삭제. 기존 사용자 hook은 보존.

## 9. AGENTS.md / CLAUDE.md 통합 (idempotent + 보존)

RTK의 `@RTK.md` 패턴처럼 긴 mutable prose를 각 instruction 파일에 직접 주입하지 않는다. 기본은 repo 안 ledger-owned instruction 파일을 만들고, `AGENTS.md`/`CLAUDE.md`에는 작은 marker 포인터만 둔다. 실제 강제는 prose가 아니라 `ledger-kit verify`, `doctor`, hooks가 담당한다.

생성 파일:

```
ledger/instructions/AGENTS.ledger-kit.md
ledger/instructions/CLAUDE.ledger-kit.md
```

`instructions install`은 marker 블록을 파일 **상단**에 prepend한다.

- Claude는 `@file` include가 동작하므로 기본 `reference` 모드에서 include를 쓴다.
  ```
  <!-- LEDGER_KIT_START -->
  @ledger/instructions/CLAUDE.ledger-kit.md
  <!-- LEDGER_KIT_END -->
  ```
- Codex/other agents는 include 지원이 환경마다 다를 수 있으므로 기본은 minimal pointer다.
  ```
  <!-- LEDGER_KIT_START -->
  Read and follow: ledger/instructions/AGENTS.ledger-kit.md
  <!-- LEDGER_KIT_END -->
  ```
- `--mode=inline`은 긴 prose를 marker 안에 직접 주입하는 호환 모드다.
- `--mode=reference`는 위 포인터 방식이며 기본값이다.
- `--mode=symlink`는 후순위/opt-in이다. 파일 시스템 정책이 프로젝트마다 달라 기본값으로 쓰지 않는다.
- `ledger/instructions/*.ledger-kit.md` 파일은 CLI 호출 방법, append-only 규칙, claim/handoff 규칙, 검증 명령만 담는다. 프로젝트별 운영 지침은 기존 AGENTS/CLAUDE 본문에 남긴다.
- 이미 존재 → no-op. 블록 내용은 버전 차이가 있어도 사용자 콘텐츠 보호 차원에서 자동 갱신하지 않는다(추후 `instructions update` 별도 검토).
- `--no-instructions` 또는 `init` 후 `instructions uninstall`로 제거 가능.

## 10. 패키지 구조

```
ledger-kit/
  go.mod
  main.go
  cmd/
    init.go view.go verify.go
    ticket.go worklog.go goal.go doctor.go insights.go
    tree.go list.go hooks.go instructions.go registry.go
  internal/
    ledger/    # 타입, JSONL read/append, 모노토닉 n, 락
    config/    # ledger/config.json
    registry/  # ~/.ledger-kit/registry.json
    gitutil/   # branch, commit, remote 베스트에포트
    agent/     # 에이전트 해석 우선순위
    verify/    # fail/warn 분리, --strict
    doctor/    # 운영 품질 진단, 보정 row 제안
    insights/  # ready queue, blocker, stale, coverage 계산
    guidance/  # ticket state machine 기반 next/suggest 메시지
    viewer/
      server.go
      assets/  # embed: index.html app.js style.css
  templates/   # init 시드: goal.json, config.json, AGENTS_BLOCK.md
               # instruction bodies: AGENTS.ledger-kit.md, CLAUDE.ledger-kit.md
  .github/workflows/release.yml
  README.md  LICENSE
```

## 11. Legacy 데이터 import

기존 프로젝트(루트의 `agent-tickets.jsonl`, `agent-worklog.jsonl`, `goal.json` 또는 구버전 `ledger/` 구조)를 신규 표준으로 옮기는 안전 경로. **항상 2단계**.

### 11.1 명령

```
ledger-kit import legacy --target PATH --plan
ledger-kit import legacy --target PATH --apply [--archive-originals]
```

- `--plan`: 어떤 파일도 변경하지 않고 리포트만 출력.
- `--apply`: 실제 반영. 멱등(idempotent rebuild).
- `--archive-originals`: 원본을 `ledger/legacy/`로 이동. **기본은 원본 보존**.

### 11.2 Source 자동 탐지

- `<target>/agent-tickets.jsonl`
- `<target>/agent-worklog.jsonl`
- `<target>/goal.json`
- `<target>/ledger/*.jsonl` (구버전 위치)

### 11.3 Plan 리포트 형식 (stdout)

```
Legacy import plan

Sources:
  agent-tickets.jsonl        245 rows
  agent-worklog.jsonl        223 rows
  goal.json                  found

Target:
  ledger/config.json         create
  ledger/tickets.jsonl       create/update
  ledger/worklog.jsonl       create/update
  ledger/goal.json           create/update

Changes:
  ticket rows imported       245
  worklog rows imported      223
  parent_ticket inferred     97
  branch inferred            12
  orphan worklogs            4 warning
  parse errors               0

Original files:
  preserve in place
```

### 11.4 정책

1. **원본 무삭제**: `--archive-originals` 명시할 때만 `ledger/legacy/`로 이동.
2. **의미 보존**: `task`, `decision`, `notes`, `status`, `blocked_by` 등 의미 필드는 절대 변경하지 않는다.
3. **envelope만 보강**:
   - `parent_ticket` 누락 → **다음 순서로 결정**:
     1. ticket id 접두사가 `config.parents`의 한 항목과 매칭(예: `BUG-123` → `BUG`)
     2. 매칭 실패 → `LEGACY`
     3. **`ROOT`는 명시적으로 root workstream인 경우에만**. import 추론으로는 ROOT를 부여하지 않는다.
   - `branch` 누락 → 비워두거나 `branch_convention` 기반 best-effort
   - `n` 누락 → 행 순서대로 부여 (consecutive from 1)
   - `ts` 누락 → import 시각으로 부여하되 warning
   - `agent` 누락 → §5.1 우선순위 적용, 결정 못 하면 `legacy`로 정규화하고 warning (legacy import 전용 예외)
4. **parse/semantic error 보존**: 파싱 실패·애매한 행·`ticket=""`/`task=""` 같은 ghost row는 버리지 않고 `ledger/import-errors.jsonl`에 원문 그대로 기록한다. target `tickets.jsonl`/`worklog.jsonl`에는 ghost row를 쓰지 않는다.
5. **멱등 재빌드**: append가 아니라 target ledger를 새로 계산해서 기존 target과 비교.
   - 같으면 "no changes" 출력.
   - 다르면 `ledger/.backup/YYYYMMDD-HHMMSS/`에 기존 파일 백업 후 교체.
6. **두 번 실행해도 행이 중복되지 않는다**.

### 11.5 백업

```
ledger/.backup/20260514-153000/
  tickets.jsonl
  worklog.jsonl
  goal.json
  config.json
```

- `--apply` 시 변경 발생한 파일만 백업.
- `.gitignore`에 `ledger/.backup/`, `ledger/.lock`, `ledger/import-errors.jsonl` 자동 추가(이미 있으면 no-op).

### 11.6 안전 가드

- target에 이미 신규 표준 ledger가 있고 legacy source가 더 작으면(행 수 감소 등) `--plan`은 경고를 띄우고 `--apply`는 `--force` 없이는 거부.
- `--apply` 중 락(`ledger/.lock`)을 보유한 채로 진행.

## 12. MVP 범위 및 구현 순서

1차 릴리스에 포함되는 기능과 **구현 권장 순서**:

1. `init`, config/goal 시드, 레지스트리 등록 (+ `registry.lock`)
2. 파일 락 프로토콜 (`ledger/.lock`) + JSONL append 라이브러리
3. `ticket add` / `ticket event`(carry-forward) / `worklog add` / `goal set` (+ `--log`)
4. `verify` (fail/warn 분리, `--strict`)
5. `doctor` / `insights` (운영 품질 리포트, ready/blocker/stale/coverage 계산)
6. `import legacy --plan` / `--apply` (멱등 재빌드, 백업, parse-error 보존)
7. `view` (다중 프로젝트 대시보드, embed.FS; `insights` 계산 재사용)
8. `hooks install` / `uninstall`, `instructions install` / `uninstall`, `next`, `suggest` (idempotent reference mode + state-aware LLM guidance)
9. GitHub Actions release workflow (matrix cross-compile)

각 단계는 직전 단계가 동작해야 검증 가능하므로 순서를 따른다. 특히 1·3·5는 핵심 — 락과 carry-forward, project_id 정책이 여기서 굳어진다.

## 13. Out of Scope (의도적 제외)

- MCP 서버 — `serve-mcp` 서브커맨드로 후속.
- `view` 데몬화 — 셸에서 직접 실행.
- TUI 뷰어.
- 머신 간 동기화 — git이 한다.
- 인증·멀티 사용자.
- Web socket / 실시간 푸시 — 폴링으로 충분.
- 플래그 기반 빌더 CLI(`--task ... --paths ...`) — JSON 단일 경로 유지.
- 자동 repair 적용 — 1차는 `doctor`가 보정 row 후보를 제안만 한다. 실제 적용 자동화는 후속.
- command taxonomy/alias 관리 — `doctor`가 commandless worklog를 잡는 수준까지만 1차에 포함.

## 14. 결정 요약

| 항목 | 결정 |
|---|---|
| 언어 | Go 1.22+, 표준 lib only |
| 데이터 위치 | repo 내, git 추적 |
| 데이터 포맷 | JSONL append-only + JSON 스냅샷 |
| 프로젝트 ID | `project_id`(128-bit hex, crypto/rand) + `slug` + `name` 분리, 표시는 `<slug>-<hex 앞6자>` |
| 동시성 | `ledger/.lock` + `~/.ledger-kit/registry.lock` (O_EXCL + 30s stale) |
| `n` 정책 | strictly consecutive from 1 (gap/중복/역행 모두 fail) |
| `ticket event` | 최신 row carry-forward + 입력 오버레이 |
| 입력 형태 | `--json '<inline>'`, `--json @-`, `--json @<path>` |
| Hook 삽입 위치 | shebang 다음(상단), 마지막 라인 아님 |
| Legacy parent 추론 | prefix 매칭 → `LEGACY`. `ROOT`는 명시적일 때만 |
| 에이전트 식별 | 명시 → env → 감지 → fail (`unknown` 금지) |
| 쓰기 어휘 | `add` / `event` (append-only 의미 강제) |
| 검증 모드 | fail / warn 분리, `--strict` 승격 |
| 운영 진단 | `doctor`(품질 문제 후보) + `insights`(ready/blocker/stale/coverage/claim/handoff) |
| 완료 증거 | `ticket.acceptance` + `worklog.commands`/`worklog.evidence` |
| Hook | marker 기반 idempotent, 기존 보존 |
| 지시문 파일 | ledger-owned instruction file + small marker pointer; Claude는 `@file`, Codex는 minimal pointer |
| 뷰어 배포 | `embed.FS` 단일 바이너리, 폴링 UI |
| Legacy import | 2단계 `--plan`/`--apply`, 멱등 재빌드, 원본 보존 기본, parse-error 보존 |
