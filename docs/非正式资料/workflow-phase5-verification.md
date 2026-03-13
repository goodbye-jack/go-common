# Workflow Phase 5 Verification And Cutover

## 1. Scope

This document records the Phase 5 result for migrating workflow capability from the Java adapter into `go-common`.

Source adapter:

- `/usr/local/work/workspace/dev_personal_project/zhuoue-flowable-6.8.1-adapter`

Target module:

- `/usr/local/work/workspace/dev_personal_project/go-common/workflow`

Verification date:

- March 10, 2026

## 2. What Is Already Migrated

The following capability is now available inside `go-common/workflow`:

- current user context resolution
- LDAP-backed user, manager and department lookup
- my todo list
- my done list
- process start
- task complete
- task context
- task form reference parsing
- process progress view
- process progress timeline
- progress lookup by process instance ID
- progress lookup by business ID
- BPMN definition XML retrieval

## 3. API Compatibility Matrix

### 3.1 Java adapter endpoints already covered by go-common

| Java adapter endpoint | go-common endpoint | Status |
| --- | --- | --- |
| `GET /api/me` | `GET /api/me` | migrated |
| `GET /api/me/tasks/todo` | `GET /api/me/tasks/todo` | migrated |
| `GET /api/me/tasks/done` | `GET /api/me/tasks/done` | migrated |
| `GET /api/me/manager` | `GET /api/me/manager` | migrated |
| `GET /api/me/department` | `GET /api/me/department` | migrated |
| `GET /api/org/users/{userId}` | `GET /api/org/users/:userId` | migrated |
| `GET /api/org/users/{userId}/manager` | `GET /api/org/users/:userId/manager` | migrated |
| `GET /api/org/users/{userId}/department` | `GET /api/org/users/:userId/department` | migrated |
| `POST /api/process/start` | `POST /api/process/start` | migrated |
| `GET /api/process-instances/{id}/progress-view` | `GET /api/process-instances/:id/progress-view` | migrated |
| `GET /api/process-instances/{id}/progress-timeline` | `GET /api/process-instances/:id/progress-timeline` | migrated |
| `GET /api/biz/{bizId}/progress-view` | `GET /api/biz/:bizId/progress-view` | migrated |
| `GET /api/biz/{bizId}/progress-timeline` | `GET /api/biz/:bizId/progress-timeline` | migrated |
| `GET /api/tasks/{id}/context` | `GET /api/tasks/:id/context` | migrated |
| `POST /api/tasks/{id}/complete` | `POST /api/tasks/:id/complete` | migrated |
| `GET /api/process/instance/{id}/definition-xml` | `GET /api/process/instance/:id/definition-xml` | migrated |

### 3.2 Java adapter endpoints not migrated into go-common yet

These remain Java-only today:

- `GET /api/tasks/my`
- `GET /api/tasks/claimable`
- `GET /api/tasks/history`
- `GET /api/tasks/done`
- `GET /api/tasks/{id}`
- `POST /api/tasks/{id}/claim`
- `POST /api/tasks/{id}/unclaim`
- `POST /api/tasks/{id}/delegate`
- `POST /api/tasks/{id}/resolve`
- `POST /api/tasks/{id}/comment`
- `GET /api/process/instance/{id}`
- `GET /api/process/instance/byBizId/{bizId}`
- `POST /api/process/instance/{id}/terminate`
- `GET /api/process/history/byBizId/{bizId}`
- `GET /api/process/instance/{id}/diagram`
- `GET /api/definitions`
- `POST /api/sso/login`
- `POST /api/sso/users/sync`
- `POST /api/sso/groups/sync`
- `ALL /api/flowable/**`

Conclusion:

- `go-common/workflow` is ready to replace the Java adapter for the core business approval chain that the current business frontend already depends on.
- `go-common/workflow` is not yet a full one-to-one replacement for every adapter endpoint.

## 4. Output Comparison Conclusion

### 4.1 Comparable output areas

The following output model is intentionally aligned with the Java adapter:

- current user profile shape
- directory manager and department response shape
- todo and done list shape
- task context shape
- task form reference shape
- progress summary shape
- progress timeline semantics

### 4.2 Known acceptable differences

- `go-common` task list now includes `progress` summary directly on each task item for easier frontend rendering.
- `go-common` API layer currently wraps responses in `{"data": ..., "message": "success"}` using existing `go-common/http` style.
- some Java adapter proxy-style endpoints are intentionally not carried over.

### 4.3 Remaining technical limits

- BPMN forward-path deduction still mainly supports common approval flows and simple conditions such as `==` and `!=`
- subprocess, call activity, multi-instance and complex EL are not fully covered
- progress-view currently returns BPMN XML only; path-edge highlighting still belongs to frontend rendering logic

## 5. Business Integration Result

For business integration, the minimum usable chain is now complete:

1. business system resolves current user from existing login token
2. `workflow/context` builds workflow user context from Gin request
3. business system starts a process by calling `POST /api/process/start`
4. business system loads my todo from `GET /api/me/tasks/todo`
5. business system loads task detail from `GET /api/tasks/:id/context`
6. business system reads `formRef` as node field reference only
7. business system writes its own page payload and completes task with `POST /api/tasks/:id/complete`
8. business system shows progress using:
   - `GET /api/process-instances/:id/progress-view`
   - `GET /api/process-instances/:id/progress-timeline`
   - or the `bizId` variants

This matches the business direction already confirmed during previous phases:

- Flowable form is a node reference model
- business frontend does not depend on Flowable official form renderer
- some nodes may have forms and some may not

## 6. Recommended Cutover Plan

### 6.1 Suggested timeline

Use a staged cutover instead of one-shot replacement.

Recommended timeline:

- March 10, 2026 to March 14, 2026
  - local integration verification
  - compare Java adapter and go-common output for the migrated endpoints
- March 15, 2026 to March 21, 2026
  - business test environment dual-run
  - keep Java adapter as fallback
- March 22, 2026 to March 28, 2026
  - tag `v1.3.0-rc.1`
  - pilot one business system on go-common workflow routes
- March 29, 2026 to April 5, 2026
  - cut core migrated endpoints to go-common by default
  - keep Java adapter alive only for non-migrated endpoints
- after April 5, 2026
  - if no regression appears for 7 consecutive days, start Java adapter retirement for migrated endpoints

### 6.2 Retirement rule

Do not retire the full Java adapter in one step.

Retire in two layers:

- first retire migrated endpoints
- keep Java adapter only for legacy-only endpoints until they are migrated or explicitly abandoned

## 7. Cutover Checklist

- register `workflow/api` into the host business service
- provide Flowable REST config
- provide LDAP config
- confirm JWT login middleware populates `UserID`
- confirm request headers carry tenant and optional system code
- verify one start-process request
- verify one todo query
- verify one task context query with `formRef`
- verify one task complete request
- verify one done query
- verify one progress-view query
- verify one progress-timeline query
- verify one query by `bizId`

## 8. Release Recommendation

For the current state, the recommended release path is:

- `v1.3.0-dev.3`
- `v1.3.0-rc.1`
- `v1.3.0`

Reason:

- the migrated core chain is now usable
- there are still legacy endpoints outside the migrated scope
- a release candidate stage is still necessary before final replacement
