# Exact session resume

This specification adds one explicit read to the base design's MCP surface and the memory
architecture's five tools. It supersedes only their tool count. General FTS ranking, hook-time
injection, M7 activation, native memory and the five existing tool contracts do not change.

`get_session_resume` requires an absolute project worktree, a known host and an exact
`host_session_id`. It reads the latest captured `UserPromptSubmit` and `Stop` separately,
using only `prompt` and `last_assistant_message`, respectively. It does not infer decisions,
summarise tool output, include subagent replies or substitute a different session. No query
syntax, caller-selected event type or adjustable output limit is accepted.

Latest means descending ingestion timestamp, then stored rowid to break timestamp ties.
This is deterministic capture order, not proven conversation order: delayed spool replay can
arrive after a newer conversation turn. Both timestamps are returned so the reader can see
a prompt later than the captured reply, but this alone does not prove an unfinished turn.
Each missing event type returns null. A present event with a missing or non-string body
returns an empty body and its reference; an older answer is not substituted.

All scope predicates are parameterised and applied before selecting a row. Project matching
uses each event's project, since one host session can change worktrees. Host matching checks
both the session identity and the event. Scope is filtering correctness within one Windows
user, not an authorization boundary. Existing `get_event` retains its project-and-id check.

Whole payload masking precedes body extraction. Each decoded body is bounded to 2,400 UTF-8
bytes, with a truncation flag; the two bodies together cannot exceed 4,800 bytes. This is a
chosen response budget, not a measured relevance optimum. Payloads exceeding the existing
`get_event` payload budget are omitted whole and marked truncated. Stored event IDs that
exceed `get_event`'s accepted width or change under masking are omitted rather than returned
as broken references. Both records come from one SQL read snapshot; this does not establish
that they belong to one conversation turn. JSON escaping and metadata are additional to
the decoded-body budget, with at most six serialized bytes per retained body byte.

The read shares the service's existing read concurrency and deadline, and checks expiration
after masking as well as during SQL. It is MCP-only: no new CLI command, named-pipe request,
host configuration, migration or automatic injection route is introduced.

Acceptance requires exact project/host/session filtering, deterministic ties, latest-event
selection despite metadata distractors, missing-body behavior, whole-payload masking, UTF-8
and byte bounds, cancellation, and the actual MCP handler's argument/result/error behavior.
An opt-in real-session comparison must report whether the final captured answer is reachable
and useful for resumption. That evidence is a narrow workflow result, not a general recall,
M7 precision, owner assessment or autonomous task-success claim.
