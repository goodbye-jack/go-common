# Workflow Migration Design

## 1. Background

This document defines how the existing Flowable Java adapter capabilities will be migrated into `go-common`.

Current source of truth:

- Java adapter: `/usr/local/work/workspace/dev_personal_project/zhuoue-flowable-6.8.1-adapter`
- Demo backend: `/usr/local/work/workspace/dev_personal_project/flowable-biz-demo`
- Demo frontend: `/usr/local/work/workspace/dev_personal_project/flowable-biz-frontend`
- Target host project: `/usr/local/work/workspace/dev_personal_project/go-common`

The migration target is not a direct Java-to-Go file translation. The target is a reusable workflow capability layer inside `go-common`.

## 2. Goals

Primary goals:

- Move workflow integration capability into `go-common`
- Preserve the current stable external API contract as much as possible
- Reuse existing `go-common` infrastructure where it already fits
- Remove Java/Spring/DB-coupled implementation patterns
- Make the workflow capability embeddable by multiple business systems

Non-goals for phase 1:

- Full support for every Flowable advanced construct
- One-shot replacement of all Java adapter runtime behavior
- Frontend UX redesign beyond capability parity

## 3. Current Capability Baseline

Stable capability already verified in the Java adapter:

- Current user and LDAP directory APIs
- Todo and done task APIs
- Manager and department lookup
- Task context and form reference parsing
- Progress view and progress timeline APIs
- BPMN XML retrieval for frontend diagram rendering
- Process start, task completion, process detail and related workflow operations

Current business expectation:

- Todo and done list remain available
- Progress query remains available by process instance and by business ID
- Timeline shows:
  `流程发起 + 真实已办节点 + 处理中节点 + 正向后续节点 + 结束`
- Some nodes may have forms, some nodes may not
- LDAP remains the source for user, manager and department resolution

## 4. Observations From go-common

### 4.1 Existing reusable foundations

The following packages are suitable as migration hosts:

- `http/`: Gin server, route registration, middleware chain
- `ldap/`: OpenLDAP connectivity and base CRUD
- `approval/`: business approval interception model
- `rbac/`: authorization integration
- `config/`: YAML-driven configuration loading
- `utils/`: JWT and common constants

### 4.2 Current limitations

These limitations must be addressed in the workflow design:

- `http` context currently only carries a thin `UserID` and tenant header model
- `utils/jwt.go` only stores a single string payload
- `ldap.ValidateUser` currently uses `phone + password`, not `uid + password`
- `ldap` is closer to organization master-data CRUD than workflow directory projection
- `config` has strong package-init side effects, including auto DB initialization
- `example/` currently prevents `go build ./...` from being fully green because there is no active `main`

## 5. Target Architecture

Add a new top-level package:

- `workflow/`

Recommended sub-packages:

- `workflow/context`
- `workflow/types`
- `workflow/engine/flowable`
- `workflow/directory`
- `workflow/formref`
- `workflow/progress`
- `workflow/bpmn`
- `workflow/api`
- `workflow/approvalbridge`

### 5.1 Package responsibilities

`workflow/context`

- Resolve current workflow user context from Gin request
- Normalize user ID, user name, tenant ID, system code, groups and roles

`workflow/types`

- Shared DTO definitions
- Request and response models exposed to business systems

`workflow/engine/flowable`

- Flowable REST integration only
- No business rules
- No frontend-specific shaping

`workflow/directory`

- Workflow-facing directory service
- Reuse `ldap/` as data source
- Expose user, manager, department and position resolution

`workflow/formref`

- Parse task `formKey`
- Resolve form model metadata
- Produce field and outcome reference for business UI

`workflow/progress`

- Todo and done enrichment
- Progress view
- Progress timeline
- Future-path inference

`workflow/bpmn`

- BPMN XML parsing
- Node and sequence flow graph
- Conditional branch selection
- Forward path deduction

`workflow/api`

- HTTP route registration for workflow APIs
- Reuse `go-common/http`

`workflow/approvalbridge`

- Bridge `approval.ApprovalHandler` with Flowable-backed approval orchestration

## 6. Core Interfaces

### 6.1 User context

```go
type UserContext struct {
    UserID     string
    UserName   string
    TenantID   string
    SystemCode string
    Groups     []string
    Roles      []string
}

type ContextResolver interface {
    Resolve(c *gin.Context) (*UserContext, error)
}
```

### 6.2 Directory service

```go
type DirectoryService interface {
    GetCurrentUser(ctx context.Context, userID string) (*DirectoryUserProfile, error)
    GetUser(ctx context.Context, userID string) (*DirectoryUserProfile, error)
    GetManager(ctx context.Context, userID string) (*DirectoryUserSummary, error)
    GetDepartment(ctx context.Context, userID string) (*DirectoryDepartment, error)
}
```

### 6.3 Engine client

```go
type EngineClient interface {
    StartProcess(ctx context.Context, req *StartProcessRequest) (*StartProcessResponse, error)
    ListTodo(ctx context.Context, user *UserContext, query *TaskQuery) (*TaskPage, error)
    ListDone(ctx context.Context, user *UserContext, query *TaskQuery) (*TaskPage, error)
    GetTaskContext(ctx context.Context, taskID string, user *UserContext) (*TaskContextResponse, error)
    CompleteTask(ctx context.Context, taskID string, req *CompleteTaskRequest, user *UserContext) error
    GetProgressView(ctx context.Context, processInstanceID string, user *UserContext) (*ProcessProgressViewResponse, error)
    GetProgressTimeline(ctx context.Context, processInstanceID string, user *UserContext) (*ProcessProgressTimelineResponse, error)
    GetDefinitionXML(ctx context.Context, processInstanceID string, user *UserContext) ([]byte, error)
}
```

### 6.4 HTTP module registration

```go
type Module interface {
    Register(server *http.HTTPServer)
}
```

## 7. API Compatibility Strategy

The following API contracts should be preserved first:

- `GET /api/me`
- `GET /api/me/tasks/todo`
- `GET /api/me/tasks/done`
- `GET /api/me/manager`
- `GET /api/me/department`
- `GET /api/org/users/{userId}`
- `GET /api/org/users/{userId}/manager`
- `GET /api/org/users/{userId}/department`
- `GET /api/process-instances/{id}/progress-view`
- `GET /api/process-instances/{id}/progress-timeline`
- `GET /api/biz/{bizId}/progress-view`
- `GET /api/biz/{bizId}/progress-timeline`
- `GET /api/process/instance/{id}/definition-xml`

Compatibility rule:

- Business systems should not need to change request and response contracts during phase 1
- Internal implementation may change from Java service to Go package

## 8. Directory and LDAP Design

### 8.1 What can be reused

Directly reusable:

- OpenLDAP connection creation
- User lookup by DN and UID
- Department and position lookup
- Base organization CRUD

### 8.2 What must be added

New workflow-facing directory behavior is required:

- Validate login by `uid + password`
- Query user profile by `uid`
- Resolve manager by workflow user
- Resolve department by workflow user
- Resolve position by workflow user
- Expose normalized profile DTOs for workflow APIs

### 8.3 Mapping notes for current LDAP structure

Based on the current LDAP conventions already used in the adapter:

- user login key: `uid`
- name fields: `cn`, `givenName`, `sn`
- department link: `departmentNumber`
- position link: `title`
- manager relation currently needs a dedicated mapping strategy

If manager is not stored directly in LDAP for all users, the workflow directory layer must support:

- LDAP-first resolution
- fallback mapping or business-side extension

## 9. Progress and BPMN Rules

### 9.1 Timeline definition

Phase-1 timeline rule is fixed as:

- `流程发起`
- real completed task records
- currently active task nodes
- forward reachable positive-path nodes
- `结束`

### 9.2 Re-entry and loop rule

When a node is re-entered because of rework or repeated approval:

- the same node may appear multiple times in timeline
- each record is treated as a separate occurrence
- the frontend should display repeated entries in chronological order

### 9.3 Gateway rule

Phase-1 condition handling:

- support simple expressions such as `==` and `!=`
- support positive-path deduction for common exclusive and parallel gateways

Phase-2 condition handling:

- complex EL
- subprocess
- multi-instance
- callActivity

### 9.4 Diagram rendering rule

Phase 1:

- frontend renders BPMN XML directly
- highlight completed, current and future nodes

Phase 1.1:

- add traversed-edge highlighting
- add full path highlighting

## 10. Data Source Strategy

### 10.1 Rule

Do not carry over Java adapter DB-coupled access patterns into Go unless there is no safe REST alternative.

### 10.2 Preferred source order

1. Flowable official REST API
2. BPMN XML resource from Flowable repository API
3. Direct DB access only as an explicit fallback and only behind a dedicated abstraction

### 10.3 Reason

- lower coupling to Flowable internal tables
- easier upgrade path
- cleaner reusable SDK design

## 11. Integration With go-common/http

Recommended integration model:

- `workflow/api.Register(server *http.HTTPServer)`
- use `server.RouteAPI(...)` to expose workflow routes
- add workflow-specific middleware only where necessary

Additional context support is required in `go-common/http`:

- structured workflow user context in Gin
- helper methods beyond current `GetUser` and `GetTenant`

Recommended additions:

- `SetUserContext(c, *workflowcontext.UserContext)`
- `GetUserContext(c) (*workflowcontext.UserContext, bool)`

## 12. Version and Release Strategy

### 12.1 Branching

Use a dedicated development branch for this migration:

- `feature/workflow-v2`

### 12.2 Tag policy

Use staged release tags. Recommended sequence:

- `v1.3.0-dev.1`
- `v1.3.0-dev.2`
- `v1.3.0-rc.1`
- `v1.3.0`

If the migration later proves to be API-breaking at the module level, promote to:

- `v2.0.0-dev.1`
- `v2.0.0-rc.1`
- `v2.0.0`

### 12.3 Current recommendation

Start with the `v1.3.0-dev.x` path first.

Reason:

- current migration can still preserve most external API contracts
- no need yet to change module import path
- lower adoption cost for business systems

## 13. Execution Plan

The migration will proceed in major phases. Each major phase must be confirmed by the user before execution.

### Phase 1: Design and version policy

- finalize this document
- freeze migration boundary
- freeze release naming strategy

### Phase 2: Module skeleton

- create `workflow/` package tree
- create base DTOs
- create context and engine interfaces

### Phase 3: Core capability migration

- migrate current user and organization APIs
- migrate todo and done task APIs
- migrate progress view and progress timeline APIs
- migrate BPMN XML retrieval

### Phase 4: Form and approval bridge

- migrate form reference parsing
- connect `approval` middleware with Flowable-backed approval orchestration

### Phase 5: Joint verification and cutover

- compare Java and Go outputs
- verify business integration
- define Java adapter retirement window

## 14. Immediate Next Step

The next major phase is:

- Phase 2: module skeleton creation

This phase should not start until explicit user confirmation is given.
