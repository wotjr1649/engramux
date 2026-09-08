# Proposal: host-mediated history selection

Status: proposed for owner decision, not an active replacement of the memory architecture.
This document supersedes nothing. It does not establish success of the active goal.

## Decision requested

Authorize a change in where automatic history selection takes place: the existing Claude Code
or Codex model decides whether its current task needs history and calls Engramux's existing
MCP tools. The service supplies scoped, masked records and performs no model inference.
Hook-time automatic injection remains disabled and its failed candidate experiments remain
failed. This is a workflow change requiring an explicit owner decision, not a way to mark
the original hook-selection improvement complete under a different name.

## Why this is proposed

The strict-prefix diagnostic found no returned search candidates for 50 agent-labelled wanted
prompts. Broadening and latest-reply candidates have separately failed relevance checks.
These observations do not prove that every deterministic approach must fail. They do support
trying a different location for query formulation and judging missing context instead of
continuing to tune the same rejected rules. The host already processes the task context;
the hook Request exposes only prompt, project and excluded event ID.

## Concrete workflow and boundary

The host formulates a task-specific search when history is needed, reads selected event or
memory records, and checks their claims against current task evidence before using them.
Existing tools are search, get_event, get_memory, list_sessions and get_session_resume;
their names were verified in internal/mcpserver/tools.go. The last tool returns the latest
session material, not a complete chronology of decisions. It cannot alone establish that an
older task remains open or that a later reply supersedes an earlier decision.

The service's raw record, project scope, redaction and read limits remain in force. Captured
requests and approvals remain data. No own LLM calls, new model runtime, API key or sidecar
is proposed. This decision alone authorizes no host configuration edit, automatic activation,
installation, remote write, publication or new destination for private data.

## Work after a decision

First freeze the host retrieval policy, task set, available context and artifacts, acceptance
checks, finite call count and elapsed-time budget. Compare actual task artifacts with history
access off/on, including already-visible context, irrelevant newer replies and superseded
decisions. Measure errors, completion, calls and elapsed time; do not substitute citation count
or retrieval success for task success. Public fixtures can establish mechanics only. Real-work
evidence must use an authorized local route and keep agent estimates separate from owner labels.

Independent review identified variability in the host's tool choice, query and interpretation.
The policy must constrain available tools, project targets and retries without assuming the
host model is deterministic. Cases with no tool call, unsuccessful calls or budget exhaustion
stay in the denominator. Stop additional retrieval at the declared limit; do not pretend
already-delivered records can be removed from the model's context. Relevance and incremental
utility require separate predeclared judgments. Current project scope must not silently broaden
because a capture or generated query suggests another destination.

Only after that evidence would an integration proposal be justified. The hook's existing time
budget is not evidence about host-turn latency. Official M7, owner M8 P5, original labels and
their thresholds remain unchanged. Defender remains the final separate discussion; publication
conditions remain unresolved. If the owner retains hook-time selection as the required route,
this alternative is not implemented as its substitute.
